// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sebastienrousseau/askiso/internal/codes"
	"go.uber.org/goleak"
	"pgregory.net/rapid"
)

func TestWorkspaceRefreshIsOneSnapshotForConcurrentReaders(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	source, workspace, external := workspaceFixture(t)
	options := Options{Source: source, Workspace: workspace, ExternalCodes: external}
	if _, err := Import(options); err != nil {
		t.Fatal(err)
	}

	const readers = 16
	const refreshes = 20
	stop := make(chan struct{})
	errs := make(chan error, readers)
	var wg sync.WaitGroup
	for worker := range readers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if worker%2 == 0 {
					report, err := Verify(source, workspace)
					if err != nil || report.Failed != 0 || report.Cases != 2 {
						errs <- fmt.Errorf("Verify returned report=%+v err=%v", report, err)
						return
					}
					continue
				}
				runtime, err := LoadRuntime(workspace)
				if err != nil || runtime.Pack == nil || runtime.ExternalCodes == nil {
					errs <- fmt.Errorf("LoadRuntime returned runtime=%+v err=%v", runtime, err)
					return
				}
			}
		}(worker)
	}

	for range refreshes {
		if _, err := Import(options); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestPublishedGenerationIsCompleteAndPinned(t *testing.T) {
	source, workspace, external := workspaceFixture(t)
	manifest, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external, GenerateSamples: true})
	if err != nil {
		t.Fatal(err)
	}
	var pointer workspacePointer
	if err := readJSON(filepath.Join(workspace, CurrentFile), &pointer); err != nil {
		t.Fatal(err)
	}
	if pointer.Format != pointerFormat || pointer.Generation != manifest.Fingerprint ||
		pointer.ManifestFingerprint != manifest.Fingerprint {
		t.Fatalf("pointer = %+v, manifest = %s", pointer, manifest.Fingerprint)
	}
	active, err := activeWorkspaceRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workspace, GenerationsDir, manifest.Fingerprint)
	if active != want {
		t.Fatalf("active generation = %s, want %s", active, want)
	}
	for _, relative := range []string{ManifestFile, SuiteFile, manifest.Pack, filepath.ToSlash(filepath.Join(".askiso", "external-codes.tsv"))} {
		generationData, err := os.ReadFile(filepath.Join(active, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		mirrorData, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(generationData, mirrorData) {
			t.Fatalf("compatibility mirror differs for %s", relative)
		}
	}
	loaded, err := LoadRuntime(workspace)
	if err != nil || loaded.Manifest.dataRoot != active {
		t.Fatalf("runtime did not pin generation: runtime=%+v err=%v", loaded, err)
	}
}

func TestFailedActivationRetainsPreviousGeneration(t *testing.T) {
	source, workspace, external := workspaceFixture(t)
	first, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external})
	if err != nil {
		t.Fatal(err)
	}
	pointerPath := filepath.Join(workspace, CurrentFile)
	before, err := os.ReadFile(pointerPath)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("activation interrupted")
	original := writeWorkspacePointer
	writeWorkspacePointer = func(string, *workspacePointer) error { return sentinel }
	t.Cleanup(func() { writeWorkspacePointer = original })
	if _, err := Import(Options{
		Source: source, Workspace: workspace, ExternalCodes: external,
		EntitlementAcknowledged: true,
	}); !errors.Is(err, sentinel) {
		t.Fatalf("interrupted import error = %v", err)
	}
	writeWorkspacePointer = original
	after, err := os.ReadFile(pointerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("failed activation changed pointer:\nbefore=%s\nafter=%s", before, after)
	}
	loaded, err := LoadRuntime(workspace)
	if err != nil || loaded.Manifest.Fingerprint != first.Fingerprint {
		t.Fatalf("previous generation was not retained: runtime=%+v err=%v", loaded, err)
	}
	entries, err := os.ReadDir(filepath.Join(workspace, GenerationsDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagingPrefix) {
			t.Fatalf("failed import leaked staging directory %s", entry.Name())
		}
	}
}

func TestActiveGenerationRejectsPointerAndArtifactCorruption(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, string, workspacePointer)
	}{
		{name: "malformed pointer", edit: func(t *testing.T, workspace string, _ workspacePointer) {
			writeWorkspaceFile(t, filepath.Join(workspace, CurrentFile), `{`)
		}},
		{name: "traversal generation", edit: func(t *testing.T, workspace string, pointer workspacePointer) {
			pointer.Generation = "../manifest.json"
			if err := writeJSON(filepath.Join(workspace, CurrentFile), &pointer); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing generation", edit: func(t *testing.T, workspace string, pointer workspacePointer) {
			pointer.Generation = strings.Repeat("0", 24)
			pointer.ManifestFingerprint = pointer.Generation
			if err := writeJSON(filepath.Join(workspace, CurrentFile), &pointer); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt active manifest", edit: func(t *testing.T, workspace string, pointer workspacePointer) {
			writeWorkspaceFile(t, filepath.Join(workspace, GenerationsDir, pointer.Generation, ManifestFile), `{`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, workspace, _ := workspaceFixture(t)
			if _, err := Import(Options{Source: source, Workspace: workspace}); err != nil {
				t.Fatal(err)
			}
			var pointer workspacePointer
			if err := readJSON(filepath.Join(workspace, CurrentFile), &pointer); err != nil {
				t.Fatal(err)
			}
			test.edit(t, workspace, pointer)
			if _, err := LoadRuntime(workspace); err == nil {
				t.Fatal("corrupt generation state was accepted")
			}
		})
	}
}

func TestWorkspacePublicationAcrossProcesses(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	source, workspace, external := workspaceFixture(t)
	barrier := filepath.Join(t.TempDir(), "start")
	const processes = 12
	commands := make([]*exec.Cmd, 0, processes)
	var outputs = make([]bytes.Buffer, processes)
	for index := range processes {
		command := exec.Command(os.Args[0], "-test.run=^TestWorkspacePublicationProcessHelper$", "-test.count=1")
		command.Env = append(os.Environ(),
			"ASKISO_WORKSPACE_PROCESS_HELPER=1",
			"ASKISO_WORKSPACE_SOURCE="+source,
			"ASKISO_WORKSPACE_PATH="+workspace,
			"ASKISO_WORKSPACE_EXTERNAL="+external,
			"ASKISO_WORKSPACE_BARRIER="+barrier,
			fmt.Sprintf("ASKISO_WORKSPACE_ACK=%t", index%2 == 0),
		)
		command.Stdout, command.Stderr = &outputs[index], &outputs[index]
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
	}
	writeWorkspaceFile(t, barrier, "go")
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("process %d failed: %v\n%s", index, err, outputs[index].String())
		}
	}
	manifest, err := LoadManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntime(workspace)
	if err != nil || loaded.Manifest.Fingerprint != manifest.Fingerprint {
		t.Fatalf("runtime = %+v, manifest=%+v, err=%v", loaded, manifest, err)
	}
	verification, err := Verify(source, workspace)
	if err != nil || verification.Failed != 0 || verification.Cases != 2 {
		t.Fatalf("verification = %+v, err=%v", verification, err)
	}
	var mirror Manifest
	if err := readJSON(filepath.Join(workspace, ManifestFile), &mirror); err != nil {
		t.Fatal(err)
	}
	if mirror.Fingerprint != manifest.Fingerprint {
		t.Fatalf("serialized publication lock allowed mirror=%s pointer=%s", mirror.Fingerprint, manifest.Fingerprint)
	}
}

func TestWorkspacePublicationProcessHelper(t *testing.T) {
	if os.Getenv("ASKISO_WORKSPACE_PROCESS_HELPER") != "1" {
		return
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(os.Getenv("ASKISO_WORKSPACE_BARRIER")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for publication barrier")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, err := Import(Options{
		Source:                  os.Getenv("ASKISO_WORKSPACE_SOURCE"),
		Workspace:               os.Getenv("ASKISO_WORKSPACE_PATH"),
		ExternalCodes:           os.Getenv("ASKISO_WORKSPACE_EXTERNAL"),
		EntitlementAcknowledged: os.Getenv("ASKISO_WORKSPACE_ACK") == "true",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublicationLockRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	source, workspace, _ := workspaceFixture(t)
	if _, err := Import(Options{Source: source, Workspace: workspace}); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workspace, publicationLockFile)
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "lock"), lockPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(Options{Source: source, Workspace: workspace}); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("publication lock symlink error = %v", err)
	}
}

func TestGenerationInventoryAndRollback(t *testing.T) {
	source, workspace, external := workspaceFixture(t)
	first, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Import(Options{
		Source: source, Workspace: workspace, ExternalCodes: external,
		EntitlementAcknowledged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("distinct workspace states produced one generation")
	}
	generations, err := ListGenerations(workspace)
	if err != nil || len(generations) != 2 {
		t.Fatalf("generations = %+v, err=%v", generations, err)
	}
	active := 0
	for _, generation := range generations {
		if !generation.Valid || generation.Release != "SR2025" {
			t.Fatalf("invalid inventory item: %+v", generation)
		}
		if generation.Active {
			active++
			if generation.Fingerprint != second.Fingerprint {
				t.Fatalf("active generation = %s, want %s", generation.Fingerprint, second.Fingerprint)
			}
		}
	}
	if active != 1 {
		t.Fatalf("active generation count = %d", active)
	}
	rolledBack, err := ActivateGeneration(workspace, first.Fingerprint)
	if err != nil || rolledBack.Fingerprint != first.Fingerprint {
		t.Fatalf("rollback = %+v, err=%v", rolledBack, err)
	}
	loaded, err := LoadRuntime(workspace)
	if err != nil || loaded.Manifest.Fingerprint != first.Fingerprint || loaded.Manifest.EntitlementAcknowledged {
		t.Fatalf("rolled-back runtime = %+v, err=%v", loaded, err)
	}
	var mirror Manifest
	if err := readJSON(filepath.Join(workspace, ManifestFile), &mirror); err != nil {
		t.Fatal(err)
	}
	if mirror.Fingerprint != first.Fingerprint {
		t.Fatalf("rollback mirror = %s, want %s", mirror.Fingerprint, first.Fingerprint)
	}
	for _, fingerprint := range []string{"", "../manifest.json", strings.Repeat("f", 24)} {
		if _, err := ActivateGeneration(workspace, fingerprint); err == nil {
			t.Fatalf("invalid generation %q was activated", fingerprint)
		}
	}
}

func TestPruneGenerationsRequiresConfirmationAndRetainsActive(t *testing.T) {
	source, workspace, external := workspaceFixture(t)
	if _, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external}); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external, EntitlementAcknowledged: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneGenerations(workspace, 1, false); err == nil {
		t.Fatal("expected confirmation error")
	}
	removed, err := PruneGenerations(workspace, 1, true)
	if err != nil || removed != 1 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	items, err := ListGenerations(workspace)
	if err != nil || len(items) != 1 || !items[0].Active || !items[0].Valid {
		t.Fatalf("inventory=%+v err=%v", items, err)
	}
}

func TestGenerationInventoryIgnoresCrashStagingDirectory(t *testing.T) {
	source, workspace, external := workspaceFixture(t)
	if _, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external}); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(workspace, GenerationsDir, stagingPrefix+"crash")
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, ManifestFile), []byte(`{"partial":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := ListGenerations(workspace)
	if err != nil || len(items) != 1 || !items[0].Valid {
		t.Fatalf("inventory=%+v err=%v", items, err)
	}
}

func TestGenerationStateMachineMatchesOracle(t *testing.T) {
	source, workspace, external := workspaceFixture(t)
	first, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Import(Options{
		Source: source, Workspace: workspace, ExternalCodes: external,
		EntitlementAcknowledged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fingerprints := []string{first.Fingerprint, second.Fingerprint}

	rapid.Check(t, func(t *rapid.T) {
		if _, err := ActivateGeneration(workspace, first.Fingerprint); err != nil {
			t.Fatal(err)
		}
		oracle := first.Fingerprint
		actions := rapid.SliceOfN(rapid.IntRange(0, 3), 0, 8).Draw(t, "actions")
		for step, action := range actions {
			switch action {
			case 0, 1:
				if _, err := ActivateGeneration(workspace, fingerprints[action]); err != nil {
					t.Fatalf("step %d activate %d: %v", step, action, err)
				}
				oracle = fingerprints[action]
			case 2:
				if _, err := ActivateGeneration(workspace, strings.Repeat("f", 24)); err == nil {
					t.Fatalf("step %d invalid generation was activated", step)
				}
			case 3:
				generations, err := ListGenerations(workspace)
				if err != nil || len(generations) != 2 {
					t.Fatalf("step %d inventory=%+v err=%v", step, generations, err)
				}
			}
			manifest, err := LoadManifest(workspace)
			if err != nil || manifest.Fingerprint != oracle {
				t.Fatalf("step %d active=%+v err=%v, oracle=%s", step, manifest, err, oracle)
			}
			var mirror Manifest
			if err := readJSON(filepath.Join(workspace, ManifestFile), &mirror); err != nil || mirror.Fingerprint != oracle {
				t.Fatalf("step %d mirror=%+v err=%v, oracle=%s", step, mirror, err, oracle)
			}
		}
	})
}

func TestGenerationInventoryMarksCorruptSnapshots(t *testing.T) {
	source, workspace, _ := workspaceFixture(t)
	manifest, err := Import(Options{Source: source, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, filepath.Join(workspace, GenerationsDir, manifest.Fingerprint, SuiteFile), `{`)
	generations, err := ListGenerations(workspace)
	if err != nil || len(generations) != 1 || generations[0].Valid || generations[0].Error == "" {
		t.Fatalf("corrupt inventory = %+v, err=%v", generations, err)
	}
	if _, err := ActivateGeneration(workspace, manifest.Fingerprint); err == nil {
		t.Fatal("corrupt generation was activated")
	}
}

func TestGenerationSizeRejectsSymlinksAndCountsFiles(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, filepath.Join(root, "manifest.json"), "12345")
	size, err := generationSize(root)
	if err != nil || size != 5 {
		t.Fatalf("generation size = %d, err=%v", size, err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	if err := os.Symlink(filepath.Join(root, "manifest.json"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := generationSize(root); err == nil || !strings.Contains(err.Error(), "unsafe symlink") {
		t.Fatalf("symlinked generation was accepted: %v", err)
	}
	if _, err := generationSize(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing generation size was accepted")
	}
}

func TestGenerationInventoryRejectsUnexpectedSymlink(t *testing.T) {
	source, workspace, _ := workspaceFixture(t)
	manifest, err := Import(Options{Source: source, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	root := filepath.Join(workspace, GenerationsDir, manifest.Fingerprint)
	if err := os.Symlink(filepath.Join(root, ManifestFile), filepath.Join(root, "unexpected")); err != nil {
		t.Fatal(err)
	}
	generations, err := ListGenerations(workspace)
	if err != nil || len(generations) != 1 || generations[0].Valid ||
		!strings.Contains(generations[0].Error, "unsafe symlink") {
		t.Fatalf("unexpected symlink inventory = %+v, err=%v", generations, err)
	}
}

func TestGenerationSecurityFailureBranches(t *testing.T) {
	t.Run("begin rejects unsafe generations path", func(t *testing.T) {
		workspace := t.TempDir()
		writeWorkspaceFile(t, filepath.Join(workspace, GenerationsDir), "not a directory")
		if _, err := beginGeneration(workspace); err == nil {
			t.Fatal("file-valued generations path was accepted")
		}
	})

	t.Run("publish requires complete identity", func(t *testing.T) {
		if err := publishGeneration(t.TempDir(), "stage", nil, &Suite{}); err == nil {
			t.Fatal("nil publication manifest was accepted")
		}
		if err := publishGeneration(t.TempDir(), "stage", &Manifest{Fingerprint: "invalid"}, &Suite{}); err == nil {
			t.Fatal("invalid publication fingerprint was accepted")
		}
	})

	t.Run("publish rejects incomplete and colliding generations", func(t *testing.T) {
		workspace := t.TempDir()
		stage, err := beginGeneration(workspace)
		if err != nil {
			t.Fatal(err)
		}
		fingerprint := strings.Repeat("1", 24)
		if err := publishGeneration(workspace, stage, &Manifest{Fingerprint: fingerprint}, &Suite{}); err == nil ||
			!strings.Contains(err.Error(), "validating workspace generation") {
			t.Fatalf("incomplete publication error = %v", err)
		}

		stage, err = beginGeneration(workspace)
		if err != nil {
			t.Fatal(err)
		}
		collision := strings.Repeat("2", 24)
		if err := os.Mkdir(filepath.Join(workspace, GenerationsDir, collision), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := publishGeneration(workspace, stage, &Manifest{Fingerprint: collision}, &Suite{}); err == nil ||
			!strings.Contains(err.Error(), "publishing workspace generation") {
			t.Fatalf("invalid collision error = %v", err)
		}
	})

	t.Run("inventory handles absent and unsafe stores", func(t *testing.T) {
		workspace := t.TempDir()
		if generations, err := ListGenerations(workspace); err != nil || generations != nil {
			t.Fatalf("empty inventory = %+v, err=%v", generations, err)
		}
		writeWorkspaceFile(t, filepath.Join(workspace, GenerationsDir), "unsafe")
		if _, err := ListGenerations(workspace); err == nil {
			t.Fatal("unsafe generation store was inventoried")
		}
	})

	t.Run("inventory and activation reject missing workspaces", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		if _, err := ListGenerations(missing); err == nil {
			t.Fatal("missing workspace was inventoried")
		}
		if _, err := ActivateGeneration(missing, strings.Repeat("a", 24)); err == nil {
			t.Fatal("generation was activated in a missing workspace")
		}
		workspace := t.TempDir()
		if _, err := ActivateGeneration(workspace, strings.Repeat("a", 24)); err == nil ||
			!strings.Contains(err.Error(), "generations path") {
			t.Fatalf("missing generations error = %v", err)
		}
	})

	t.Run("inventory ignores staging and unrelated entries", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.Mkdir(filepath.Join(workspace, GenerationsDir), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{stagingPrefix + "abandoned", "README"} {
			writeWorkspaceFile(t, filepath.Join(workspace, GenerationsDir, name), "ignored")
		}
		if generations, err := ListGenerations(workspace); err != nil || len(generations) != 0 {
			t.Fatalf("ignored inventory = %+v, err=%v", generations, err)
		}
	})

	t.Run("inventory marks a file named as a generation invalid", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.Mkdir(filepath.Join(workspace, GenerationsDir), 0o700); err != nil {
			t.Fatal(err)
		}
		writeWorkspaceFile(t, filepath.Join(workspace, GenerationsDir, strings.Repeat("b", 24)), "not a directory")
		generations, err := ListGenerations(workspace)
		if err != nil || len(generations) != 1 || generations[0].Valid || generations[0].Error == "" {
			t.Fatalf("file generation inventory = %+v, err=%v", generations, err)
		}
	})

	t.Run("active pointer cannot be a symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink semantics differ on Windows")
		}
		workspace := t.TempDir()
		target := filepath.Join(t.TempDir(), "pointer")
		writeWorkspaceFile(t, target, `{}`)
		if err := os.Symlink(target, filepath.Join(workspace, CurrentFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := activeWorkspaceRoot(workspace); err == nil || !strings.Contains(err.Error(), "non-symlink") {
			t.Fatalf("pointer symlink error = %v", err)
		}
	})

	t.Run("generation parent cannot be a symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink semantics differ on Windows")
		}
		workspace := t.TempDir()
		fingerprint := strings.Repeat("a", 24)
		if err := writeJSON(filepath.Join(workspace, CurrentFile), &workspacePointer{
			Format: pointerFormat, Generation: fingerprint, ManifestFingerprint: fingerprint,
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), filepath.Join(workspace, GenerationsDir)); err != nil {
			t.Fatal(err)
		}
		if _, err := activeWorkspaceRoot(workspace); err == nil || !strings.Contains(err.Error(), "generations path") {
			t.Fatalf("generation-parent symlink error = %v", err)
		}
	})

	t.Run("active generation cannot be a symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink semantics differ on Windows")
		}
		workspace := t.TempDir()
		fingerprint := strings.Repeat("c", 24)
		if err := writeJSON(filepath.Join(workspace, CurrentFile), &workspacePointer{
			Format: pointerFormat, Generation: fingerprint, ManifestFingerprint: fingerprint,
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(workspace, GenerationsDir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), filepath.Join(workspace, GenerationsDir, fingerprint)); err != nil {
			t.Fatal(err)
		}
		if _, err := activeWorkspaceRoot(workspace); err == nil || !strings.Contains(err.Error(), "generation is missing or unsafe") {
			t.Fatalf("generation symlink error = %v", err)
		}
	})

	t.Run("active pointer must match the generation manifest", func(t *testing.T) {
		source, workspace, external := workspaceFixture(t)
		first, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external})
		if err != nil {
			t.Fatal(err)
		}
		second, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external, EntitlementAcknowledged: true})
		if err != nil {
			t.Fatal(err)
		}
		firstRoot := filepath.Join(workspace, GenerationsDir, first.Fingerprint)
		if err := os.RemoveAll(firstRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(workspace, GenerationsDir, second.Fingerprint), firstRoot); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(workspace, CurrentFile), &workspacePointer{
			Format: pointerFormat, Generation: first.Fingerprint, ManifestFingerprint: first.Fingerprint,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := activeWorkspaceRoot(workspace); err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("pointer/manifest mismatch error = %v", err)
		}
		generations, err := ListGenerations(workspace)
		if err != nil || len(generations) != 1 || generations[0].Valid ||
			!strings.Contains(generations[0].Error, "does not match") {
			t.Fatalf("mismatched generation inventory = %+v, err=%v", generations, err)
		}
	})

	t.Run("multi-step operations reject generation drift", func(t *testing.T) {
		manifest := &Manifest{Fingerprint: strings.Repeat("a", 24)}
		if err := ensureWorkspaceSnapshot(strings.Repeat("b", 24), manifest); err == nil ||
			!strings.Contains(err.Error(), "changed during operation") {
			t.Fatalf("generation drift error = %v", err)
		}
		if err := ensureWorkspaceSnapshot(manifest.Fingerprint, manifest); err != nil {
			t.Fatalf("stable generation rejected: %v", err)
		}
		if err := ensureWorkspaceSnapshot("", nil); err != nil {
			t.Fatalf("test-double snapshot rejected: %v", err)
		}
	})

	t.Run("publication callback error is preserved", func(t *testing.T) {
		workspace := t.TempDir()
		sentinel := errors.New("publish failed")
		if err := withPublicationLock(workspace, func() error { return sentinel }); !errors.Is(err, sentinel) {
			t.Fatalf("callback error = %v", err)
		}
		if err := os.Remove(filepath.Join(workspace, publicationLockFile)); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(workspace, publicationLockFile), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := withPublicationLock(workspace, func() error { return nil }); err == nil || !strings.Contains(err.Error(), "regular") {
			t.Fatalf("directory lock error = %v", err)
		}
	})

	t.Run("mirror rejects missing and unsafe artifacts", func(t *testing.T) {
		generation, workspace := t.TempDir(), t.TempDir()
		manifest := &Manifest{Pack: "missing.cbpr-pack.json"}
		if err := mirrorGeneration(workspace, generation, manifest, &Suite{}); err == nil || !strings.Contains(err.Error(), "missing or unsafe") {
			t.Fatalf("missing mirror artifact error = %v", err)
		}
		manifest = &Manifest{ExternalCodes: &ExternalPublication{}}
		for _, relative := range []string{ManifestFile, SuiteFile, filepath.ToSlash(filepath.Join(".askiso", "external-codes.tsv"))} {
			writeWorkspaceFile(t, filepath.Join(generation, filepath.FromSlash(relative)), "artifact")
		}
		writeWorkspaceFile(t, filepath.Join(workspace, ".askiso"), "unsafe")
		if err := mirrorGeneration(workspace, generation, manifest, &Suite{}); err == nil || !strings.Contains(err.Error(), "directory is unsafe") {
			t.Fatalf("unsafe mirror directory error = %v", err)
		}
	})

	t.Run("mirror deduplicates paths and reports atomic replacement errors", func(t *testing.T) {
		generation, workspace := t.TempDir(), t.TempDir()
		for _, relative := range []string{ManifestFile, SuiteFile, filepath.ToSlash(filepath.Join(GeneratedDir, "same.xml"))} {
			writeWorkspaceFile(t, filepath.Join(generation, filepath.FromSlash(relative)), "artifact")
		}
		suite := &Suite{Cases: []SuiteCase{
			{Origin: "generated", Sample: filepath.ToSlash(filepath.Join(GeneratedDir, "same.xml"))},
			{Origin: "generated", Sample: filepath.ToSlash(filepath.Join(GeneratedDir, "same.xml"))},
		}}
		if err := mirrorGeneration(workspace, generation, &Manifest{}, suite); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(workspace, ManifestFile)); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(workspace, ManifestFile), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := mirrorGeneration(workspace, generation, &Manifest{}, &Suite{}); err == nil {
			t.Fatal("atomic replacement of a directory unexpectedly succeeded")
		}
	})

	t.Run("mirror propagates artifact read errors", func(t *testing.T) {
		generation, workspace := t.TempDir(), t.TempDir()
		path := filepath.Join(generation, ManifestFile)
		writeWorkspaceFile(t, path, "manifest")
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
		if err := mirrorGeneration(workspace, generation, &Manifest{}, &Suite{}); err == nil {
			t.Fatal("unreadable generation artifact was mirrored")
		}
	})

	t.Run("generation validation rejects each incomplete layer", func(t *testing.T) {
		source, workspace, _ := workspaceFixture(t)
		manifest, err := Import(Options{Source: source, Workspace: workspace})
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(workspace, GenerationsDir, manifest.Fingerprint)
		mismatch := *manifest
		mismatch.Fingerprint = strings.Repeat("d", 24)
		if _, err := validatePublishedGeneration(root, &mismatch); err == nil || !strings.Contains(err.Error(), "requested snapshot") {
			t.Fatalf("expected-manifest mismatch = %v", err)
		}
		if err := os.Remove(filepath.Join(root, SuiteFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := validatePublishedGeneration(root, manifest); err == nil {
			t.Fatal("generation without suite was accepted")
		}
	})

	t.Run("generation validation rejects runtime and suite inconsistencies", func(t *testing.T) {
		source, workspace, _ := workspaceFixture(t)
		manifest, err := Import(Options{Source: source, Workspace: workspace})
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(workspace, GenerationsDir, manifest.Fingerprint)
		writeWorkspaceFile(t, filepath.Join(root, manifest.Pack), `{`)
		if _, err := validatePublishedGeneration(root, manifest); err == nil || !strings.Contains(err.Error(), "loading workspace pack") {
			t.Fatalf("runtime validation error = %v", err)
		}

		source, workspace, _ = workspaceFixture(t)
		manifest, err = Import(Options{Source: source, Workspace: workspace})
		if err != nil {
			t.Fatal(err)
		}
		root = filepath.Join(workspace, GenerationsDir, manifest.Fingerprint)
		var suite Suite
		if err := readJSON(filepath.Join(root, SuiteFile), &suite); err != nil {
			t.Fatal(err)
		}
		suite.Release = "SR2024"
		if err := writeJSON(filepath.Join(root, SuiteFile), &suite); err != nil {
			t.Fatal(err)
		}
		if _, err := validatePublishedGeneration(root, manifest); err == nil || !strings.Contains(err.Error(), "suite does not match") {
			t.Fatalf("suite validation error = %v", err)
		}
	})

	t.Run("generation validation rejects unsafe generated paths", func(t *testing.T) {
		source, workspace, external := workspaceFixture(t)
		manifest, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external, GenerateSamples: true})
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(workspace, GenerationsDir, manifest.Fingerprint)
		var suite Suite
		if err := readJSON(filepath.Join(root, SuiteFile), &suite); err != nil {
			t.Fatal(err)
		}
		for index := range suite.Cases {
			if suite.Cases[index].Origin == "generated" {
				suite.Cases[index].Sample = "../outside.xml"
				break
			}
		}
		suite.Fingerprint = suiteFingerprint(&suite)
		manifest.SuiteFingerprint = suite.Fingerprint
		manifest.Fingerprint = manifestFingerprint(manifest)
		if err := writeJSON(filepath.Join(root, SuiteFile), &suite); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(root, ManifestFile), manifest); err != nil {
			t.Fatal(err)
		}
		if _, err := validatePublishedGeneration(root, manifest); err == nil || !strings.Contains(err.Error(), "unsafe generated sample") {
			t.Fatalf("unsafe generated path error = %v", err)
		}
	})

	t.Run("activation rejects missing and mismatched generation manifests", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.Mkdir(filepath.Join(workspace, GenerationsDir), 0o700); err != nil {
			t.Fatal(err)
		}
		fingerprint := strings.Repeat("e", 24)
		if err := os.Mkdir(filepath.Join(workspace, GenerationsDir, fingerprint), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := ActivateGeneration(workspace, fingerprint); err == nil {
			t.Fatal("generation without manifest was activated")
		}

		source, populated, _ := workspaceFixture(t)
		first, err := Import(Options{Source: source, Workspace: populated})
		if err != nil {
			t.Fatal(err)
		}
		second, err := Import(Options{Source: source, Workspace: populated, EntitlementAcknowledged: true})
		if err != nil {
			t.Fatal(err)
		}
		firstRoot := filepath.Join(populated, GenerationsDir, first.Fingerprint)
		if err := os.RemoveAll(firstRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(populated, GenerationsDir, second.Fingerprint), firstRoot); err != nil {
			t.Fatal(err)
		}
		if _, err := ActivateGeneration(populated, first.Fingerprint); err == nil || !strings.Contains(err.Error(), "does not match its directory") {
			t.Fatalf("activation fingerprint mismatch = %v", err)
		}
	})

	t.Run("activation propagates mirror failure", func(t *testing.T) {
		source, workspace, external := workspaceFixture(t)
		manifest, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external})
		if err != nil {
			t.Fatal(err)
		}
		store := filepath.Dir(codes.ExternalCodesPath(workspace))
		if err := os.RemoveAll(store); err != nil {
			t.Fatal(err)
		}
		writeWorkspaceFile(t, store, "unsafe")
		if _, err := ActivateGeneration(workspace, manifest.Fingerprint); err == nil || !strings.Contains(err.Error(), "directory is unsafe") {
			t.Fatalf("activation mirror error = %v", err)
		}
	})

	t.Run("directory creation failure is returned", func(t *testing.T) {
		root := t.TempDir()
		parent := filepath.Join(root, "file")
		writeWorkspaceFile(t, parent, "not a directory")
		if err := ensurePrivateDirectory(filepath.Join(parent, "child")); err == nil {
			t.Fatal("directory creation below a file succeeded")
		}
		if err := syncGenerationDirectory(filepath.Join(root, "missing")); err == nil {
			t.Fatal("syncing a missing generation directory succeeded")
		}
	})

	t.Run("publication lock open failure is returned", func(t *testing.T) {
		if err := withPublicationLock(filepath.Join(t.TempDir(), "missing"), func() error { return nil }); err == nil ||
			!strings.Contains(err.Error(), "opening workspace publication lock") {
			t.Fatalf("lock open error = %v", err)
		}
	})

	t.Run("publication lock lstat failure is returned", func(t *testing.T) {
		root := t.TempDir()
		workspace := filepath.Join(root, "workspace-file")
		writeWorkspaceFile(t, workspace, "not a directory")
		if err := withPublicationLock(workspace, func() error { return nil }); err == nil ||
			!strings.Contains(err.Error(), "checking workspace publication lock") {
			t.Fatalf("lock lstat error = %v", err)
		}
	})

	t.Run("generated hash is validated before activation", func(t *testing.T) {
		source, workspace, external := workspaceFixture(t)
		manifest, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external, GenerateSamples: true})
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(workspace, GenerationsDir, manifest.Fingerprint)
		var suite Suite
		if err := readJSON(filepath.Join(root, SuiteFile), &suite); err != nil {
			t.Fatal(err)
		}
		for _, testCase := range suite.Cases {
			if testCase.Origin == "generated" {
				writeWorkspaceFile(t, filepath.Join(root, filepath.FromSlash(testCase.Sample)), "tampered")
				break
			}
		}
		if _, err := validatePublishedGeneration(root, manifest); err == nil || !strings.Contains(err.Error(), "fingerprint changed") {
			t.Fatalf("generated hash error = %v", err)
		}
	})
}

func TestPublicationLockDependencyFailures(t *testing.T) {
	sentinel := errors.New("lock dependency failed")
	t.Run("initial lstat", func(t *testing.T) {
		original := publicationLstat
		publicationLstat = func(string) (os.FileInfo, error) { return nil, sentinel }
		t.Cleanup(func() { publicationLstat = original })
		if err := withPublicationLock(t.TempDir(), func() error { return nil }); !errors.Is(err, sentinel) {
			t.Fatalf("initial lstat error = %v", err)
		}
	})
	t.Run("open", func(t *testing.T) {
		original := publicationOpen
		publicationOpen = func(string, int, os.FileMode) (*os.File, error) { return nil, sentinel }
		t.Cleanup(func() { publicationOpen = original })
		if err := withPublicationLock(t.TempDir(), func() error { return nil }); !errors.Is(err, sentinel) {
			t.Fatalf("open error = %v", err)
		}
	})
	t.Run("chmod", func(t *testing.T) {
		original := publicationChmod
		publicationChmod = func(*os.File) error { return sentinel }
		t.Cleanup(func() { publicationChmod = original })
		if err := withPublicationLock(t.TempDir(), func() error { return nil }); !errors.Is(err, sentinel) {
			t.Fatalf("chmod error = %v", err)
		}
	})
	t.Run("post-open lstat", func(t *testing.T) {
		original := publicationLstat
		calls := 0
		publicationLstat = func(path string) (os.FileInfo, error) {
			calls++
			if calls == 2 {
				return nil, sentinel
			}
			return original(path)
		}
		t.Cleanup(func() { publicationLstat = original })
		if err := withPublicationLock(t.TempDir(), func() error { return nil }); err == nil ||
			!strings.Contains(err.Error(), "changed during open") {
			t.Fatalf("post-open lstat error = %v", err)
		}
	})
	t.Run("file stat", func(t *testing.T) {
		original := publicationStat
		publicationStat = func(*os.File) (os.FileInfo, error) { return nil, sentinel }
		t.Cleanup(func() { publicationStat = original })
		if err := withPublicationLock(t.TempDir(), func() error { return nil }); err == nil ||
			!strings.Contains(err.Error(), "changed during open") {
			t.Fatalf("file stat error = %v", err)
		}
	})
	t.Run("lock", func(t *testing.T) {
		original := publicationLock
		publicationLock = func(*os.File) error { return sentinel }
		t.Cleanup(func() { publicationLock = original })
		if err := withPublicationLock(t.TempDir(), func() error { return nil }); !errors.Is(err, sentinel) {
			t.Fatalf("lock error = %v", err)
		}
	})
}

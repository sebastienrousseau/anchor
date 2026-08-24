// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package registrygen

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleManifest = `[
  {"id":"1036","title":"Payments Clearing and Settlement V11","category":"payments-clearing-and-settlement",
   "version":"v11","files_count":3,"status":"success",
   "files":["pacs.008.001.10.xsd","pacs.009.001.10.xsd","ISO20022_MDRPart1.pdf"]},
  {"id":"691","title":"Account Switching V03","category":"account-switching",
   "version":"v03","files_count":1,"status":"success","files":["acmt.027.001.03.xsd"]},
  {"id":"1162","title":"Corporate Actions V15BAHHas variants","category":"corporate-actions",
   "version":"v15","files_count":1,"status":"success","files":["seev.031.001.14.xsd"]},
  {"id":"999","title":"Broken Set V01","category":"broken","version":"v01",
   "files_count":0,"status":"failed","files":[]}
]`

func TestRenderProducesBothSections(t *testing.T) {
	blob, stats, err := Render([]byte(sampleManifest))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if stats.Sets != 3 {
		t.Errorf("sets = %d, want 3 (the failed one is skipped)", stats.Sets)
	}
	if stats.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", stats.Skipped)
	}
	if stats.Messages != 4 {
		t.Errorf("messages = %d, want 4", stats.Messages)
	}

	if !strings.HasPrefix(blob, "#SETS\n") {
		t.Error("blob should open with the sets section")
	}
	if !strings.Contains(blob, "#MESSAGES\n") {
		t.Error("blob should contain the messages section")
	}
	if !strings.Contains(blob, "pacs.008.001.10\t1036") {
		t.Errorf("message-to-set mapping missing:\n%s", blob)
	}
}

// RA titles carry scraper artefacts glued to the version token.
func TestDisplayNamesAreCleaned(t *testing.T) {
	blob, _, err := Render([]byte(sampleManifest))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(blob, "1036\tPayments Clearing and Settlement\t") {
		t.Errorf("trailing version token not stripped:\n%s", blob)
	}
	if !strings.Contains(blob, "1162\tCorporate Actions\t") {
		t.Errorf("'V15BAHHas variants' not stripped:\n%s", blob)
	}
	if strings.Contains(blob, "Has variants") || strings.Contains(blob, "BAH\t") {
		t.Errorf("artefacts survived:\n%s", blob)
	}
}

func TestSetsAreSortedNumerically(t *testing.T) {
	blob, _, err := Render([]byte(sampleManifest))
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, line := range strings.Split(blob, "\n") {
		if line == "#MESSAGES" {
			break
		}
		if line == "" || line == "#SETS" {
			continue
		}
		ids = append(ids, atoi(strings.SplitN(line, "\t", 2)[0]))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("set ids not sorted: %v", ids)
			break
		}
	}
}

func TestRenderRejectsBadInput(t *testing.T) {
	for name, in := range map[string]string{
		"not json":   `{{{`,
		"empty list": `[]`,
		"wrong type": `{"id":"1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Render([]byte(in)); err == nil {
				t.Errorf("Render should reject %s", name)
			}
		})
	}
}

func TestBuildWritesCompressedBlob(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifest, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "registry.tsv.gz")

	stats, err := Build(manifest, out)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if stats.Bytes == 0 {
		t.Error("Build reported no bytes written")
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("output is not gzip: %v", err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "pacs.008.001.10") {
		t.Error("decompressed blob is missing its content")
	}
}

func TestBuildReportsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Build(filepath.Join(dir, "nope.json"), filepath.Join(dir, "out.gz")); err == nil {
		t.Error("a missing manifest should be an error")
	}

	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifest, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(manifest, filepath.Join(dir, "no-such-dir", "out.gz")); err == nil {
		t.Error("an unwritable destination should be an error")
	}
}

func TestNumericIDAndAtoi(t *testing.T) {
	if got := numericID("1036\tName\tslug\tv11\t3"); got != 1036 {
		t.Errorf("numericID = %d, want 1036", got)
	}
	if got := atoi("not-a-number"); got != 0 {
		t.Errorf("atoi on junk = %d, want 0", got)
	}
}

func TestRenderKeepsTitleWhenStrippingLeavesNothing(t *testing.T) {
	// A title that is only a version token has nothing left after stripping, so
	// the original is kept rather than producing a nameless set.
	blob, _, err := Render([]byte(`[{"id":"1","title":"V01","category":"c","version":"v01",
	  "files_count":1,"status":"success","files":["pacs.008.001.10.xsd"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(blob, "1\tV01\t") {
		t.Errorf("the original title should be kept:\n%s", blob)
	}
}

func TestRenderGroupsMultipleSetsPerMessage(t *testing.T) {
	blob, _, err := Render([]byte(`[
	  {"id":"20","title":"A V01","category":"a","version":"v01","files_count":1,
	   "status":"success","files":["pacs.008.001.10.xsd"]},
	  {"id":"3","title":"B V01","category":"b","version":"v01","files_count":1,
	   "status":"success","files":["pacs.008.001.10.xsd"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	// Set ids are sorted numerically, not lexically.
	if !strings.Contains(blob, "pacs.008.001.10\t3,20") {
		t.Errorf("set ids should be numerically sorted:\n%s", blob)
	}
}

func TestWriteBlobReportsWriteFailure(t *testing.T) {
	if err := writeBlob(failingWriter{}, "content"); err == nil {
		t.Error("a failing writer should be reported")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

var errWrite = errors.New("write failed")

func TestRunCommand(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifest, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "registry.tsv.gz")

	var stdout, stderr strings.Builder
	code := Run([]string{"-manifest", manifest, "-out", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3 sets") {
		t.Errorf("summary should report the set count: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "skipped 1") {
		t.Errorf("the skipped set should be reported: %s", stdout.String())
	}

	// A missing manifest fails with a diagnostic.
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"-manifest", filepath.Join(dir, "nope.json"), "-out", out}, &stdout, &stderr); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gen-registry:") {
		t.Errorf("stderr should carry the diagnostic: %s", stderr.String())
	}

	// Bad flags.
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"-nonsense"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

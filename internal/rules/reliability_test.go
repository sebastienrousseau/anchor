// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCBPRPackConcurrentExportsDoNotRaceOrMutate(t *testing.T) {
	pack := reliabilityPack("primary", "Z", "A")
	before, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}

	const writers = 128
	dir := t.TempDir()
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			path := filepath.Join(dir, fmt.Sprintf("pack-%03d.json", index))
			if err := WriteCBPRPack(path, pack); err != nil {
				errs <- err
				return
			}
			loaded, err := LoadCBPRPack(path)
			if err != nil {
				errs <- err
				return
			}
			if loaded.Fingerprint == "" || len(loaded.Constraints) != 1 {
				errs <- fmt.Errorf("invalid round trip: %+v", loaded)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	after, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("export mutated source pack\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestLegacyV1PackRemainsLoadableAndStable(t *testing.T) {
	path := filepath.Join("testdata", "legacy-v1.cbpr-pack.json")
	pack, err := LoadCBPRPack(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Constraints) != 1 || pack.Constraints[0].ID != "CBPR-PACK-LEGACY000001" {
		t.Fatalf("legacy identity changed: %+v", pack.Constraints)
	}
	written := filepath.Join(t.TempDir(), "round-trip.cbpr-pack.json")
	if err := WriteCBPRPack(written, pack); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := LoadCBPRPack(written)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(pack)
	gotJSON, _ := json.Marshal(roundTrip)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("legacy round trip changed semantics\nwant: %s\ngot:  %s", wantJSON, gotJSON)
	}
}

func FuzzCBPRPackMergeAlgebra(f *testing.F) {
	f.Add("alpha", "Z", "A", uint8(1))
	f.Add("beta", "SHAR", "CRED", uint8(7))
	f.Add("", "same", "same", uint8(255))

	f.Fuzz(func(t *testing.T, source, firstValue, secondValue string, selector uint8) {
		source = boundedPackText(source)
		firstValue = boundedPackText(firstValue)
		secondValue = boundedPackText(secondValue)
		left := reliabilityPack(source+"-left", firstValue, secondValue)
		right := reliabilityPack(source+"-right", secondValue, firstValue)
		if selector%2 == 0 {
			right.Warnings = append(right.Warnings, left.Warnings[0])
		}

		leftBefore, _ := json.Marshal(left)
		rightBefore, _ := json.Marshal(right)
		leftRight, err := MergeCBPRPacks(left, right)
		if err != nil {
			t.Fatal(err)
		}
		rightLeft, err := MergeCBPRPacks(right, left)
		if err != nil {
			t.Fatal(err)
		}
		if leftRight.Fingerprint != rightLeft.Fingerprint {
			t.Fatalf("merge is not commutative: %s != %s", leftRight.Fingerprint, rightLeft.Fingerprint)
		}
		leftAfter, _ := json.Marshal(left)
		rightAfter, _ := json.Marshal(right)
		if string(leftBefore) != string(leftAfter) || string(rightBefore) != string(rightAfter) {
			t.Fatal("merge mutated an input pack")
		}

		leftRight.Sources[0].UsageIdentifiers[0] = "modified"
		leftRight.Constraints[0].Path[0] = "modified"
		leftRight.Constraints[0].Values[0] = "modified"
		finalLeft, _ := json.Marshal(left)
		finalRight, _ := json.Marshal(right)
		if string(leftBefore) != string(finalLeft) || string(rightBefore) != string(finalRight) {
			t.Fatal("merged output aliases input storage")
		}
	})
}

func reliabilityPack(source, firstValue, secondValue string) *CBPRPack {
	return &CBPRPack{
		Format: cbprPackFormat,
		Sources: []CBPRPackSource{{
			Name:             source + ".pdf",
			SHA256:           strings.Repeat("a", 64),
			MessageID:        "pacs.008.001.08",
			UsageIdentifiers: []string{"swift.cbprplus.stp.03", "swift.cbprplus.03"},
			Constraints:      1,
		}},
		Constraints: []CBPRPackConstraint{{
			Source:           source + ".pdf",
			MessageID:        "pacs.008.001.08",
			UsageIdentifiers: []string{"swift.cbprplus.stp.03", "swift.cbprplus.03"},
			Path:             []string{"Document", "FIToFICstmrCdtTrf", "GrpHdr"},
			Min:              0,
			Max:              1,
			Values:           []string{secondValue, firstValue},
			WhenPath:         []string{"Document", "FIToFICstmrCdtTrf"},
			WhenValues:       []string{firstValue, secondValue},
		}},
		Warnings: []string{"locally compiled"},
	}
}

func boundedPackText(value string) string {
	if len(value) > 128 {
		value = value[:128]
	}
	if value == "" {
		return "value"
	}
	return value
}

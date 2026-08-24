// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// The correctness bar for a validator written from scratch is agreement with
// the reference implementation. This runs AskIso and xmllint over every sample
// message in an installed catalogue and reports where they disagree.
//
// It needs both a catalogue and xmllint, so it skips on a clean CI runner. Run
// it locally with:
//
//	go test ./internal/validator/ -run Differential -v
//
// ASKISO_DIFF_LIMIT caps how many pairs are checked (default 400); set it to 0
// for the whole catalogue.
func TestDifferentialAgainstXmllint(t *testing.T) {
	if testing.Short() {
		t.Skip("differential test is slow")
	}
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skip("xmllint not installed")
	}
	root, err := catalog.Resolve("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}

	pairs := collectPairs(t, root)
	if len(pairs) == 0 {
		t.Skip("catalogue has no sample messages to compare")
	}

	limit := envInt("ASKISO_DIFF_LIMIT", 400)
	step := 1
	if limit > 0 && len(pairs) > limit {
		step = (len(pairs) + limit - 1) / limit
	}

	var (
		compared      int
		agree         int
		bothValid     int
		bothInvalid   int
		falseReject   []string // AskIso rejects, xmllint accepts -- the serious kind
		falseAccept   []string
		parseFailures []string
	)

	for i := 0; i < len(pairs); i += step {
		p := pairs[i]

		schema, err := xsd.ParseFile(p.xsd)
		if err != nil {
			parseFailures = append(parseFailures, filepath.Base(p.xsd)+": "+err.Error())
			continue
		}
		instance, err := os.ReadFile(p.xml)
		if err != nil {
			continue
		}

		compared++
		ours := validator.Validate(instance, schema)
		theirs := xmllintValid(p.xsd, p.xml)

		switch {
		case ours.Valid == theirs:
			agree++
			if theirs {
				bothValid++
			} else {
				bothInvalid++
			}
		case !ours.Valid && theirs:
			detail := filepath.Base(p.xml)
			if len(ours.Errors) > 0 {
				detail += " -- " + ours.Errors[0].String()
			}
			falseReject = append(falseReject, detail)
		default:
			falseAccept = append(falseAccept, filepath.Base(p.xml))
		}
	}

	pct := 0.0
	if compared > 0 {
		pct = 100 * float64(agree) / float64(compared)
	}
	t.Logf("compared %d of %d pairs: %d agree (%.1f%%), %d false rejects, %d false accepts",
		compared, len(pairs), agree, pct, len(falseReject), len(falseAccept))
	t.Logf("  agreed valid: %d   agreed invalid: %d", bothValid, bothInvalid)

	// Agreement is only meaningful if both engines accepted a real number of
	// documents; agreeing that everything is broken proves nothing.
	if bothValid == 0 && compared > 50 {
		t.Error("no document was accepted by either engine; the comparison is vacuous")
	}

	for _, f := range head(parseFailures, 5) {
		t.Errorf("schema failed to parse: %s", f)
	}

	// A false reject means AskIso calls a valid message invalid. That is the
	// failure mode that breaks a user's pipeline, so it is an error.
	for _, f := range head(falseReject, 10) {
		t.Errorf("false reject: %s", f)
	}
	if len(falseReject) > 10 {
		t.Errorf("... and %d more false rejects", len(falseReject)-10)
	}

	// A false accept is a missed defect: worth reporting, not worth failing on,
	// because the catalogue's own samples are largely invalid to begin with.
	if len(falseAccept) > 0 {
		t.Logf("false accepts (%d), first few: %v", len(falseAccept), head(falseAccept, 5))
	}
}

type pair struct{ xsd, xml string }

func collectPairs(t *testing.T, root string) []pair {
	t.Helper()
	var pairs []pair

	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".xsd") {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(p), ".xsd")
		versionDir := filepath.Dir(filepath.Dir(p))
		sample := filepath.Join(versionDir, "Sample Messages", base+".xml")
		if _, err := os.Stat(sample); err == nil {
			pairs = append(pairs, pair{xsd: p, xml: sample})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking catalogue: %v", err)
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].xml < pairs[j].xml })
	return pairs
}

func xmllintValid(schemaPath, instancePath string) bool {
	cmd := exec.Command("xmllint", "--noout", "--nonet", "--schema", schemaPath, instancePath)
	return cmd.Run() == nil
}

func head(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

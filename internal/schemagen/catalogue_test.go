// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package schemagen_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/schemagen"
	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// The claim is that any message can be generated from its schema. The only way
// to know is to try every one the user has installed and validate the result.
//
//	ASKISO_GEN_LIMIT=0 go test ./internal/schemagen/ -run Installed -v
func TestEveryInstalledMessageGenerates(t *testing.T) {
	if testing.Short() {
		t.Skip("this walks the whole catalogue")
	}
	root, err := catalog.Resolve("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}
	idx, err := catalog.Load(root)
	if err != nil {
		t.Skipf("the catalogue would not load: %v", err)
	}

	limit := 400
	if v := os.Getenv("ASKISO_GEN_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	// One schema per path: several message identifiers can share one file.
	seen := map[string]bool{}
	var paths []string
	for _, msg := range idx.Messages {
		if msg.XSDPath == "" || seen[msg.XSDPath] {
			continue
		}
		seen[msg.XSDPath] = true
		paths = append(paths, msg.XSDPath)
	}
	sort.Strings(paths)
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
	}
	if len(paths) == 0 {
		t.Skip("the catalogue holds no schemas")
	}

	var generated, invalid int
	failures := map[string]int{}

	for _, path := range paths {
		schema, err := xsd.ParseFile(path)
		if err != nil {
			t.Errorf("%s: parsing: %v", filepath.Base(path), err)
			continue
		}

		res, err := schemagen.Generate(schema, schemagen.DefaultOptions())
		if err != nil {
			t.Errorf("%s: generating: %v", filepath.Base(path), err)
			continue
		}
		generated++

		verdict := validator.Validate([]byte(res.XML), schema)
		if verdict.Valid {
			continue
		}
		invalid++
		for _, e := range verdict.Errors {
			failures[e.Rule]++
		}
		if invalid <= 5 {
			t.Errorf("%s: the generated message does not validate:\n  %s\n%s",
				filepath.Base(path), firstErrors(verdict, 4), res.XML)
		}
	}

	if invalid > 0 {
		var kinds []string
		for rule, n := range failures {
			kinds = append(kinds, fmt.Sprintf("%s=%d", rule, n))
		}
		sort.Strings(kinds)
		t.Errorf("%d of %d generated messages are invalid; failures by rule: %s",
			invalid, generated, strings.Join(kinds, " "))
	}
	t.Logf("generated and validated %d schema(s); %d invalid", generated, invalid)
}

func firstErrors(res *validator.Result, n int) string {
	var out []string
	for i, e := range res.Errors {
		if i == n {
			out = append(out, fmt.Sprintf("... and %d more", len(res.Errors)-n))
			break
		}
		out = append(out, e.String())
	}
	return strings.Join(out, "\n  ")
}

// TestGeneratedMessagesLintClean checks the second half of the claim: a
// generated message is not merely schema-valid but semantically sound. An IBAN
// that fails its checksum matches the pattern and would still be wrong to hand
// anyone as an example.
func TestGeneratedMessagesLintClean(t *testing.T) {
	if testing.Short() {
		t.Skip("this walks the whole catalogue")
	}
	root, err := catalog.Resolve("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}
	idx, err := catalog.Load(root)
	if err != nil {
		t.Skipf("the catalogue would not load: %v", err)
	}

	limit := 400
	if v := os.Getenv("ASKISO_GEN_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	seen := map[string]bool{}
	var paths []string
	for _, msg := range idx.Messages {
		if msg.XSDPath == "" || seen[msg.XSDPath] {
			continue
		}
		seen[msg.XSDPath] = true
		paths = append(paths, msg.XSDPath)
	}
	sort.Strings(paths)
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
	}

	var checked, dirty int
	byRule := map[string]int{}

	for _, path := range paths {
		schema, err := xsd.ParseFile(path)
		if err != nil {
			continue
		}
		res, err := schemagen.Generate(schema, schemagen.DefaultOptions())
		if err != nil {
			continue
		}

		lint, err := iso20022.Lint([]byte(res.XML), "")
		if err != nil {
			continue
		}
		checked++
		if lint.Errors == 0 {
			continue
		}
		dirty++
		for _, issue := range lint.Issues {
			if issue.Severity == iso20022.SeverityError {
				byRule[issue.Rule]++
			}
		}
		if dirty <= 3 {
			t.Errorf("%s: the generated message does not lint clean: %+v",
				filepath.Base(path), lint.Issues)
		}
	}

	if checked == 0 {
		t.Skip("nothing to check")
	}
	if dirty > 0 {
		var kinds []string
		for rule, n := range byRule {
			kinds = append(kinds, fmt.Sprintf("%s=%d", rule, n))
		}
		sort.Strings(kinds)
		t.Errorf("%d of %d generated messages have lint errors: %s",
			dirty, checked, strings.Join(kinds, " "))
	}
	t.Logf("linted %d generated message(s); %d with errors", checked, dirty)
}

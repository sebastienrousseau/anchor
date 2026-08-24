// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package xsd_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// Every schema in an installed catalogue must parse, and every type the root
// element references must resolve inside the same document. AskIso's validator
// is only as good as this, so it is the acceptance test for the parser.
//
// It needs a catalogue, so it skips on a clean CI runner.
func TestParsesEveryInstalledSchema(t *testing.T) {
	root, err := catalog.Resolve("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}

	var total, parsed int
	failures := map[string]int{}
	var examples []string
	roots := map[string]int{}
	unresolved := map[string]int{}

	err = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".xsd") {
			return nil
		}
		total++

		s, perr := xsd.ParseFile(p)
		if perr != nil {
			msg := perr.Error()
			if i := strings.LastIndex(msg, ".xsd: "); i > 0 {
				msg = msg[i+6:]
			}
			failures[msg]++
			if len(examples) < 5 {
				examples = append(examples, filepath.Base(p)+": "+msg)
			}
			return nil
		}
		parsed++

		el, ok := s.RootElement()
		if !ok {
			roots["(none)"]++
			return nil
		}
		roots[el.Name]++

		if !xsd.IsBuiltin(el.Type) {
			_, isComplex := s.ResolveComplex(el.Type)
			_, isSimple := s.ResolveSimple(el.Type)
			if !isComplex && !isSimple {
				unresolved[el.Type]++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking catalogue: %v", err)
	}
	if total == 0 {
		t.Skip("catalogue holds no schemas")
	}

	t.Logf("schemas=%d parsed=%d roots=%v", total, parsed, roots)

	if parsed != total {
		type kv struct {
			reason string
			n      int
		}
		var list []kv
		for k, v := range failures {
			list = append(list, kv{k, v})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
		for i, e := range list {
			if i == 8 {
				break
			}
			t.Errorf("%d schema(s) failed: %s", e.n, e.reason)
		}
		for _, ex := range examples {
			t.Logf("  example: %s", ex)
		}
	}

	if len(unresolved) > 0 {
		t.Errorf("root element types that do not resolve in their own schema: %v", unresolved)
	}
}

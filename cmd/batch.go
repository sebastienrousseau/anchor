// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/sebastienrousseau/askiso/internal/rules"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
	"github.com/spf13/cobra"
)

var (
	batchProfile string
	batchFormat  string
	batchSchema  bool
	batchWorkers int
	batchQuiet   bool
)

// FileReport is the outcome for one message.
type FileReport struct {
	File       string                 `json:"file"`
	MessageID  string                 `json:"message_id,omitempty"`
	Lint       *iso20022.LintResult   `json:"lint,omitempty"`
	Schema     *iso20022.SchemaResult `json:"schema,omitempty"`
	Profile    *rules.Result          `json:"profile,omitempty"`
	Err        string                 `json:"error,omitempty"`
	ErrorCount int                    `json:"error_count"`
}

// BatchReport aggregates a run.
type BatchReport struct {
	Files    int          `json:"files"`
	Passed   int          `json:"passed"`
	Failed   int          `json:"failed"`
	Errors   int          `json:"error_count"`
	Profile  string       `json:"profile,omitempty"`
	Reports  []FileReport `json:"reports"`
	Skipped  int          `json:"skipped"`
	Duration string       `json:"-"`
}

var batchCmd = &cobra.Command{
	Use:   "batch <file-or-directory>...",
	Short: "Validate and lint many messages at once",
	Long: `Batch runs the business-rule linter, and optionally schema validation and a
scheme rule profile, across every message in a directory or glob.

This is the shape real operational work takes: a folder of messages, one report,
one exit code. With --format sarif the output uploads straight to GitHub code
scanning, so a pull request that introduces a non-compliant address is annotated
rather than silently merged.`,
	Example: `  askiso batch ./messages
  askiso batch ./messages --profile cbpr-2026
  askiso batch ./messages --profile cbpr-2026 --format sarif > askiso.sarif
  askiso batch ./out --schema --profile cbpr-2026`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := collectMessages(args)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("no .xml files found in %s", strings.Join(args, ", "))
		}

		if batchProfile != "" {
			if _, err := rules.Get(batchProfile); err != nil {
				return err
			}
		}

		var cat *iso20022.Catalogue
		if batchSchema {
			cat, err = iso20022.OpenCatalogue(catalogPath)
			if err != nil {
				return fmt.Errorf("--schema needs an installed catalogue:\n\n%w", err)
			}
		}

		report := runBatch(files, cat)

		switch batchFormat {
		case "sarif":
			results := make([]*rules.Result, 0, len(report.Reports))
			for i := range report.Reports {
				if report.Reports[i].Profile != nil {
					results = append(results, report.Reports[i].Profile)
				}
			}
			if err := rules.WriteSARIF(os.Stdout, results...); err != nil {
				return err
			}
		case "json":
			out, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
		default:
			printBatchText(report)
		}

		if report.Failed > 0 {
			return errSilent
		}
		return nil
	},
}

// collectMessages expands the arguments into a sorted list of XML files.
func collectMessages(args []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string

	add := func(p string) {
		if !strings.EqualFold(filepath.Ext(p), ".xml") || seen[p] {
			return
		}
		seen[p] = true
		files = append(files, p)
	}

	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			// Not a path: treat it as a glob.
			matches, gerr := filepath.Glob(arg)
			if gerr != nil || len(matches) == 0 {
				return nil, fmt.Errorf("no such file or directory: %s", arg)
			}
			for _, m := range matches {
				add(m)
			}
			continue
		}

		if !info.IsDir() {
			add(arg)
			continue
		}
		err = filepath.Walk(arg, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			add(p)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}

// runBatch checks every file, in parallel. Reports come back in input order so
// output is deterministic regardless of scheduling.
func runBatch(files []string, cat *iso20022.Catalogue) *BatchReport {
	workers := batchWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(files) {
		workers = len(files)
	}

	reports := make([]FileReport, len(files))
	jobs := make(chan int)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				reports[i] = checkOne(files[i], cat)
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	out := &BatchReport{Files: len(files), Profile: batchProfile, Reports: reports}
	for _, r := range reports {
		if r.ErrorCount > 0 || r.Err != "" {
			out.Failed++
			out.Errors += r.ErrorCount
		} else {
			out.Passed++
		}
	}
	return out
}

func checkOne(path string, cat *iso20022.Catalogue) FileReport {
	rep := FileReport{File: path}

	data, err := os.ReadFile(path)
	if err != nil {
		rep.Err = err.Error()
		rep.ErrorCount = 1
		return rep
	}
	if id, err := iso20022.MessageIDFromInstance(data); err == nil {
		rep.MessageID = id
	}

	lintRes, err := iso20022.Lint(data, path)
	if err != nil {
		rep.Err = err.Error()
		rep.ErrorCount = 1
		return rep
	}
	rep.Lint = lintRes
	rep.ErrorCount += lintRes.Errors

	if cat != nil {
		schemaRes, err := cat.Validate(data)
		switch {
		case err != nil:
			// A message whose schema is not installed is reported, not counted
			// as a failure of the message itself.
			rep.Err = err.Error()
		default:
			rep.Schema = schemaRes
			rep.ErrorCount += len(schemaRes.Errors)
		}
	}

	if batchProfile != "" {
		profRes, err := iso20022.CheckProfile(data, batchProfile, path)
		if err != nil {
			rep.Err = err.Error()
			rep.ErrorCount++
		} else {
			rep.Profile = profRes
			rep.ErrorCount += profRes.Errors
		}
	}

	return rep
}

func printBatchText(report *BatchReport) {
	fmt.Printf("\n%s %d file(s)\n\n", headStyle.Render(" BATCH "), report.Files)

	for _, r := range report.Reports {
		if r.ErrorCount == 0 && r.Err == "" {
			if !batchQuiet {
				fmt.Printf("  %s %s\n", tickMark, filepath.Base(r.File))
			}
			continue
		}

		fmt.Printf("  %s %s", crossMark, filepath.Base(r.File))
		if r.MessageID != "" {
			fmt.Printf("  %s", subtleStyle.Render(r.MessageID))
		}
		fmt.Println()

		if r.Err != "" {
			fmt.Printf("      %s\n", firstLineOf(r.Err))
		}
		if r.Lint != nil {
			for _, i := range r.Lint.Issues {
				if i.Severity == iso20022.SeverityError {
					fmt.Printf("      [%s] %s\n", i.Rule, i.Message)
				}
			}
		}
		if r.Schema != nil {
			for _, e := range r.Schema.Errors {
				fmt.Printf("      %d:%d [%s] %s\n", e.Line, e.Column, e.Rule, e.Message)
			}
		}
		if r.Profile != nil {
			for _, f := range r.Profile.Findings {
				if f.Severity == rules.SeverityError {
					fmt.Printf("      [%s] %s\n         at %s\n", f.RuleID, f.Message, f.Path)
				}
			}
		}
		fmt.Println()
	}

	fmt.Printf("  %d passed, %d failed, %d error(s) total\n\n",
		report.Passed, report.Failed, report.Errors)
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func init() {
	batchCmd.Flags().StringVar(&batchProfile, "profile", "",
		"Scheme rule profile to apply ("+strings.Join(rules.Names(), ", ")+")")
	batchCmd.Flags().StringVar(&batchFormat, "format", "text", "Output format: text, json, or sarif")
	batchCmd.Flags().BoolVar(&batchSchema, "schema", false,
		"Also validate each message against its schema (needs an installed catalogue)")
	batchCmd.Flags().IntVar(&batchWorkers, "workers", 0, "Parallel workers (default: one per CPU)")
	batchCmd.Flags().BoolVarP(&batchQuiet, "quiet", "s", false, "Only list files that failed")
	RootCmd.AddCommand(batchCmd)
}

// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Message represents an individual ISO 20022 message definition.
type Message struct {
	ID            string // e.g. "pacs.008.001.10"
	BaseCode      string // e.g. "pacs.008"
	Domain        string // e.g. "Payments Clearing and Settlement"
	Category      string
	Version       string // e.g. "Version 10.0"
	XSDPath       string
	XMLSamplePath string
	MDRPaths      []string
	MUGPaths      []string
}

// VersionInfo represents a single version directory within a category.
type VersionInfo struct {
	Name        string // e.g. "Version 1.0"
	Path        string
	SchemaCount int
	ReportCount int
	Messages    []Message
}

// Category represents a message set category (e.g. "Account Switching").
type Category struct {
	Name         string
	Path         string
	READMEPath   string
	Versions     []VersionInfo
	TotalSchemas int
	TotalReports int
	Messages     []Message
}

// Index holds the complete scanned catalog.
type Index struct {
	RootDir    string
	Categories []Category
	Messages   []Message
	MessageMap map[string]Message
}

// Load scans a catalogue root directory and builds the message index.
//
// The catalogue is not redistributed with AskIso; rootDir is normally produced
// by Resolve. A directory that yields no messages is an error, never an empty
// index -- silently returning zero results is how a broken install goes
// unnoticed.
func Load(rootDir string) (*Index, error) {
	idx, err := scanFilesystemParallel(rootDir)
	if err != nil {
		return nil, fmt.Errorf("scanning catalogue at %s: %w", rootDir, err)
	}
	if len(idx.Messages) == 0 {
		return nil, &NotFoundError{Searched: []string{rootDir}}
	}
	return idx, nil
}

// LoadResolved locates a catalogue and loads it in one step.
func LoadResolved(override string) (*Index, error) {
	root, err := Resolve(override)
	if err != nil {
		return nil, err
	}
	return Load(root)
}

// skipDir reports directories that are never message-set categories.
func skipDir(name string) bool {
	switch name {
	case "cmd", "internal", "pkg", "examples", "scripts", "build", "dist", "testdata":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func scanFilesystemParallel(rootDir string) (*Index, error) {
	idx := &Index{
		RootDir:    rootDir,
		Categories: make([]Category, 0, 60),
		Messages:   make([]Message, 0, 5000),
		MessageMap: make(map[string]Message),
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}

	type catJob struct {
		name string
		path string
	}

	var jobs []catJob
	for _, entry := range entries {
		if !entry.IsDir() || skipDir(entry.Name()) {
			continue
		}
		jobs = append(jobs, catJob{
			name: entry.Name(),
			path: filepath.Join(rootDir, entry.Name()),
		})
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > len(jobs) {
		numWorkers = len(jobs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobsChan := make(chan catJob, len(jobs))
	resultsChan := make(chan Category, len(jobs))

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobsChan {
				cat := processCategory(j.name, j.path)
				if len(cat.Versions) > 0 {
					resultsChan <- cat
				}
			}
		}()
	}

	for _, j := range jobs {
		jobsChan <- j
	}
	close(jobsChan)

	wg.Wait()
	close(resultsChan)

	for cat := range resultsChan {
		idx.Categories = append(idx.Categories, cat)
		for _, msg := range cat.Messages {
			idx.Messages = append(idx.Messages, msg)
			idx.MessageMap[msg.ID] = msg
		}
	}

	sort.Slice(idx.Categories, func(i, j int) bool {
		return idx.Categories[i].Name < idx.Categories[j].Name
	})

	sort.Slice(idx.Messages, func(i, j int) bool {
		return idx.Messages[i].ID < idx.Messages[j].ID
	})

	return idx, nil
}

func processCategory(name, catPath string) Category {
	readmePath := filepath.Join(catPath, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		readmePath = ""
	}

	cat := Category{
		Name:       name,
		Path:       catPath,
		READMEPath: readmePath,
		Versions:   make([]VersionInfo, 0, 10),
		Messages:   make([]Message, 0, 100),
	}

	verEntries, err := os.ReadDir(catPath)
	if err != nil {
		return cat
	}

	for _, vEntry := range verEntries {
		if !vEntry.IsDir() || !strings.HasPrefix(vEntry.Name(), "Version") {
			continue
		}

		verPath := filepath.Join(catPath, vEntry.Name())
		ver := VersionInfo{
			Name:     vEntry.Name(),
			Path:     verPath,
			Messages: make([]Message, 0, 50),
		}

		xsdDir := filepath.Join(verPath, "Schemas")
		sampleDir := filepath.Join(verPath, "Sample Messages")
		mdrDir := filepath.Join(verPath, "Message Definition Reports")
		mugDir := filepath.Join(verPath, "Message Usage Guidelines")

		mdrFiles := listFiles(mdrDir)
		mugFiles := listFiles(mugDir)
		ver.ReportCount = len(mdrFiles) + len(mugFiles)

		if xsdFiles, err := os.ReadDir(xsdDir); err == nil {
			for _, xsdF := range xsdFiles {
				if !xsdF.IsDir() && strings.HasSuffix(xsdF.Name(), ".xsd") {
					msgID := strings.TrimSuffix(xsdF.Name(), ".xsd")
					baseCode := extractBaseCode(msgID)

					xmlSample := filepath.Join(sampleDir, msgID+".xml")
					if _, err := os.Stat(xmlSample); os.IsNotExist(err) {
						xmlSample = ""
					}

					msg := Message{
						ID:            msgID,
						BaseCode:      baseCode,
						Domain:        name,
						Category:      name,
						Version:       vEntry.Name(),
						XSDPath:       filepath.Join(xsdDir, xsdF.Name()),
						XMLSamplePath: xmlSample,
						MDRPaths:      mdrFiles,
						MUGPaths:      mugFiles,
					}

					ver.Messages = append(ver.Messages, msg)
					cat.Messages = append(cat.Messages, msg)
				}
			}
		}

		ver.SchemaCount = len(ver.Messages)
		cat.TotalSchemas += ver.SchemaCount
		cat.TotalReports += ver.ReportCount
		cat.Versions = append(cat.Versions, ver)
	}

	sort.Slice(cat.Versions, func(i, j int) bool {
		return cat.Versions[i].Name < cat.Versions[j].Name
	})

	return cat
}

// Search performs a ranked/scored search over all messages.
func (idx *Index) Search(query string) []Message {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return idx.Messages
	}

	type scoredMsg struct {
		msg   Message
		score int
	}

	var matches []scoredMsg
	for _, m := range idx.Messages {
		idLower := strings.ToLower(m.ID)
		baseLower := strings.ToLower(m.BaseCode)
		catLower := strings.ToLower(m.Category)

		score := 0
		if idLower == q {
			score += 100 // Exact ID match
		} else if baseLower == q {
			score += 80 // Exact base code match (e.g. pacs.008)
		} else if strings.HasPrefix(idLower, q) {
			score += 60 // Prefix match
		} else if strings.Contains(idLower, q) {
			score += 40
		} else if strings.Contains(baseLower, q) {
			score += 30
		} else if strings.Contains(catLower, q) {
			score += 20
		}

		if score > 0 {
			matches = append(matches, scoredMsg{msg: m, score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].msg.ID < matches[j].msg.ID
	})

	res := make([]Message, len(matches))
	for i, sm := range matches {
		res[i] = sm.msg
	}
	return res
}

func listFiles(dir string) []string {
	var res []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res
	}
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			res = append(res, filepath.Join(dir, e.Name()))
		}
	}
	return res
}

func extractBaseCode(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return id
}

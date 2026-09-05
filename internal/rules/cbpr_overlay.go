// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const CBPROverlayFormat = "askiso-cbpr-rule-overlay/v1"

// CBPROverlay is an operator-authored, machine-readable translation of rules
// that cannot be represented by XSD. AskISO never derives these conditions
// from proprietary narrative text automatically.
type CBPROverlay struct {
	Format      string               `json:"format"`
	Release     string               `json:"release"`
	Description string               `json:"description,omitempty"`
	Constraints []CBPRPackConstraint `json:"constraints"`
}

// CompileCBPROverlay validates and compiles a local overlay into the existing
// content-minimised pack format.
func CompileCBPROverlay(path string) (*CBPRPack, error) {
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("reading CBPR+ rule overlay: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxCompiledPackSize {
		return nil, fmt.Errorf("CBPR+ rule overlay must be a regular file no larger than %d bytes", maxCompiledPackSize)
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("reading CBPR+ rule overlay: %w", err)
	}
	var overlay CBPROverlay
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&overlay); err != nil {
		return nil, fmt.Errorf("decoding CBPR+ rule overlay: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("decoding CBPR+ rule overlay: trailing JSON content")
	}
	if overlay.Format != CBPROverlayFormat || strings.ToUpper(strings.TrimSpace(overlay.Release)) != "SR2025" {
		return nil, errors.New("overlay must use askiso-cbpr-rule-overlay/v1 for SR2025")
	}
	if len(overlay.Constraints) == 0 {
		return nil, errors.New("overlay has no constraints")
	}
	digest := sha256.Sum256(data)
	source := CBPRPackSource{Name: filepath.Base(clean), SHA256: hex.EncodeToString(digest[:])}
	pack := &CBPRPack{Format: cbprPackFormat}
	for index := range overlay.Constraints {
		constraint := overlay.Constraints[index]
		if constraint.Source == "" {
			constraint.Source = source.Name
		}
		if err := validatePackConstraint(constraint); err != nil {
			return nil, fmt.Errorf("constraint %d: %w", index+1, err)
		}
		pack.Constraints = append(pack.Constraints, constraint)
	}
	source.Constraints = len(pack.Constraints)
	pack.Sources = []CBPRPackSource{source}
	pack.Warnings = []string{"conditional/narrative rules were explicitly authored by the operator; AskISO did not infer them from prose"}
	pack.normalise()
	pack.Fingerprint = packFingerprint(pack)
	return pack, nil
}

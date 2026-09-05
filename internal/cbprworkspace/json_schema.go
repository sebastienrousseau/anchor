// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/rules"
)

type guidelineJSONMetadata struct {
	MessageID        string
	UsageIdentifiers []string
}

type myStandardsJSON struct {
	Schema      string                     `json:"$schema"`
	Type        string                     `json:"type"`
	Comment     json.RawMessage            `json:"$comment"`
	Definitions map[string]json.RawMessage `json:"definitions"`
}

type myStandardsComment struct {
	Group          string `json:"group"`
	Collection     string `json:"collection"`
	UsageGuideline string `json:"usageGuideline"`
	BaseMessage    string `json:"baseMessage"`
}

func inspectMyStandardsJSON(path string) (guidelineJSONMetadata, bool, error) {
	data, err := readBounded(path)
	if err != nil {
		return guidelineJSONMetadata{}, false, err
	}
	var document myStandardsJSON
	if err := json.Unmarshal(data, &document); err != nil {
		return guidelineJSONMetadata{}, false, fmt.Errorf("decoding JSON: %w", err)
	}
	if len(document.Comment) == 0 || string(document.Comment) == "null" {
		return guidelineJSONMetadata{}, false, nil
	}
	var comment myStandardsComment
	if err := json.Unmarshal(document.Comment, &comment); err != nil {
		// Draft JSON Schemas may legally use a string $comment. That is not the
		// object-shaped metadata emitted by MyStandards and should be ignored.
		return guidelineJSONMetadata{}, false, nil
	}
	group := strings.ToLower(strings.NewReplacer("-", " ", "+", " ").Replace(comment.Group))
	if !strings.Contains(group, "cross border payments") ||
		strings.TrimSpace(comment.UsageGuideline) == "" {
		return guidelineJSONMetadata{}, false, nil
	}
	if strings.TrimSpace(document.Schema) == "" || !strings.EqualFold(strings.TrimSpace(document.Type), "object") || len(document.Definitions) == 0 {
		return guidelineJSONMetadata{}, true, errorsForGuidelineJSON("missing $schema, object type, or definitions")
	}
	if !strings.Contains(strings.ToLower(comment.Collection), "sr2025") {
		return guidelineJSONMetadata{}, true, errorsForGuidelineJSON("collection metadata that is not SR2025")
	}
	messageID := strings.ToLower(strings.TrimSpace(comment.BaseMessage))
	if messageFromNamespace("urn:iso:std:iso:20022:tech:xsd:"+messageID) != messageID {
		return guidelineJSONMetadata{}, true, errorsForGuidelineJSON("invalid baseMessage metadata")
	}
	if !strings.Contains(strings.ToLower(comment.UsageGuideline), messageID) {
		return guidelineJSONMetadata{}, true, errorsForGuidelineJSON("a usageGuideline that does not match baseMessage")
	}
	usageIdentifiers := rules.CBPRUsageIdentifiers(messageID, comment.UsageGuideline)
	if len(usageIdentifiers) == 0 {
		return guidelineJSONMetadata{}, true, errorsForGuidelineJSON("baseMessage is not in the supported SR2025 collection")
	}
	return guidelineJSONMetadata{MessageID: messageID, UsageIdentifiers: usageIdentifiers}, true, nil
}

func errorsForGuidelineJSON(detail string) error {
	return fmt.Errorf("recognised CBPR+ export has %s", detail)
}

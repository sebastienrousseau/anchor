// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyFINplusEnvelopeBindings(t *testing.T) {
	source, workspace, external := workspaceFixture(t)
	validEnvelope := `<Envelope><AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02"><Fr/><To/><BizMsgIdr>MSG-1</BizMsgIdr><MsgDefIdr>pacs.008.001.08</MsgDefIdr><BizSvc>swift.cbprplus.03</BizSvc><CreDt>2026-09-05T08:00:00Z</CreDt></AppHdr><Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>SALA</Purpose></Document></Envelope>`
	invalidEnvelope := strings.Replace(validEnvelope, "pacs.008.001.08</MsgDefIdr>", "pacs.009.001.08</MsgDefIdr>", 1)
	writeWorkspaceFile(t, filepath.Join(source, "samples", "envelope.xml"), validEnvelope)
	writeWorkspaceFile(t, filepath.Join(source, "samples", "envelope-bah.invalid.xml"), invalidEnvelope)
	if _, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: external}); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(source, workspace)
	if err != nil || report.Failed != 0 || report.Passed != 4 {
		t.Fatalf("envelope verification = %+v, %v", report, err)
	}
	foundEnvelopeError := false
	for _, result := range report.Results {
		if strings.Contains(result.ID, "envelope-bah") && result.EnvelopeErrors > 0 {
			foundEnvelopeError = true
		}
	}
	if !foundEnvelopeError {
		t.Fatalf("BAH mismatch was not attributed: %+v", report.Results)
	}
}

func TestEnvelopeValidationFallbacks(t *testing.T) {
	bare := []byte(`<Document xmlns="urn:test"/>`)
	if payload, errors := validationPayload(bare, "test", ""); string(payload) != string(bare) || errors != 0 {
		t.Fatalf("bare payload = %s, %d", payload, errors)
	}
	malformed := []byte(`<Envelope><Document>`)
	if payload, errors := validationPayload(malformed, "test", ""); string(payload) != string(malformed) || errors != 0 {
		t.Fatalf("malformed fallback = %s, %d", payload, errors)
	}
	noDocument := []byte(`<Envelope/>`)
	if payload, errors := validationPayload(noDocument, "test", ""); string(payload) != string(noDocument) || errors != 0 {
		t.Fatalf("no-document fallback = %s, %d", payload, errors)
	}
	info := envelopeInfo{wrapped: true, appHeader: true, headerNamespace: businessApplicationHeaderNS,
		from: true, to: true, businessMessageID: "id", messageDefinitionID: "pacs.008.001.08",
		businessService: "swift.cbprplus.03", creationDate: "not-a-date"}
	if got := envelopeErrorCount(info, "pacs.008.001.08", "swift.cbprplus.03"); got != 1 {
		t.Fatalf("invalid header date errors = %d", got)
	}
	if got := envelopeErrorCount(envelopeInfo{}, "", ""); got != 0 {
		t.Fatalf("bare metadata errors = %d", got)
	}
	if got := envelopeErrorCount(envelopeInfo{wrapped: true}, "pacs.008.001.08", "swift.cbprplus.03"); got != 8 {
		t.Fatalf("empty wrapped metadata errors = %d", got)
	}
	outsideHeader := []byte(`<Envelope><Fr/><To/><AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02"><BizMsgIdr>id</BizMsgIdr><MsgDefIdr>pacs.008.001.08</MsgDefIdr><BizSvc>swift.cbprplus.03</BizSvc><CreDt>2026-09-05T08:00:00Z</CreDt></AppHdr><Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"/></Envelope>`)
	if _, errors := validationPayload(outsideHeader, "pacs.008.001.08", "swift.cbprplus.03"); errors != 2 {
		t.Fatalf("elements outside AppHdr satisfied header requirements: %d", errors)
	}
}

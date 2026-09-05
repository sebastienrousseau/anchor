// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

const (
	TransportEnvelope       = "envelope"
	TransportRequestPayload = "request-payload"
	TransportSwiftDataPDU   = "swift-datapdu"
)

// SampleExportOptions selects the local wrapper used around AppHdr+Document.
// DataPDU metadata is a transport template and still requires validation by
// the operator's entitled interface/network schema and Swift test service.
type SampleExportOptions struct {
	Profile    string `json:"profile,omitempty"`
	SenderDN   string `json:"sender_dn,omitempty"`
	ReceiverDN string `json:"receiver_dn,omitempty"`
	Service    string `json:"service,omitempty"`
}

func normaliseTransportOptions(options SampleExportOptions) (SampleExportOptions, error) {
	options.Profile = strings.ToLower(strings.TrimSpace(options.Profile))
	if options.Profile == "" {
		options.Profile = TransportEnvelope
	}
	switch options.Profile {
	case TransportEnvelope, TransportRequestPayload:
	case TransportSwiftDataPDU:
		if strings.TrimSpace(options.SenderDN) == "" || strings.TrimSpace(options.ReceiverDN) == "" {
			return options, errors.New("swift-datapdu requires sender and receiver DNs")
		}
		if strings.TrimSpace(options.Service) == "" {
			options.Service = "swift.finplus"
		}
	default:
		return options, fmt.Errorf("unsupported transport profile %q", options.Profile)
	}
	return options, nil
}

func transportEnvelope(header, payload []byte, messageID, reference string, options SampleExportOptions) ([]byte, error) {
	options, err := normaliseTransportOptions(options)
	if err != nil {
		return nil, err
	}
	headerText := strings.TrimSpace(string(stripXMLDeclaration(header)))
	payloadText := strings.TrimSpace(string(stripXMLDeclaration(payload)))
	switch options.Profile {
	case TransportEnvelope:
		return []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Envelope>\n" + headerText + "\n" + payloadText + "\n</Envelope>\n"), nil
	case TransportRequestPayload:
		return []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<RequestPayload>\n" + headerText + "\n" + payloadText + "\n</RequestPayload>\n"), nil
	default:
		return []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
			"<Saa:DataPDU xmlns:Saa=\"urn:swift:saa:xsd:saa.2.0\">\n" +
			"  <Saa:Revision>2.0.14</Saa:Revision>\n  <Saa:Header><Saa:Message>\n" +
			"    <Saa:SenderReference>" + xmlText(reference) + "</Saa:SenderReference>\n" +
			"    <Saa:MessageIdentifier>" + xmlText(messageID) + "</Saa:MessageIdentifier>\n" +
			"    <Saa:Format>MX</Saa:Format>\n" +
			"    <Saa:Sender><Saa:DN>" + xmlText(options.SenderDN) + "</Saa:DN></Saa:Sender>\n" +
			"    <Saa:Receiver><Saa:DN>" + xmlText(options.ReceiverDN) + "</Saa:DN></Saa:Receiver>\n" +
			"    <Saa:NetworkInfo><Saa:Service>" + xmlText(options.Service) + "</Saa:Service></Saa:NetworkInfo>\n" +
			"  </Saa:Message></Saa:Header>\n  <Saa:Body>\n" + headerText + "\n" + payloadText + "\n  </Saa:Body>\n</Saa:DataPDU>\n"), nil
	}
}

func xmlText(value string) string {
	var output bytes.Buffer
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}

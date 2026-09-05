// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"bytes"
	"encoding/xml"
	"strings"
	"time"
)

const businessApplicationHeaderNS = "urn:iso:std:iso:20022:tech:xsd:head.001.001.02"

type envelopeInfo struct {
	wrapped, appHeader, from, to bool
	headerNamespace              string
	businessMessageID            string
	messageDefinitionID          string
	businessService              string
	creationDate                 string
}

// validationPayload extracts a Document from a FINplus envelope and checks the
// header-to-payload bindings that a payload XSD cannot express. A bare Document
// remains valid input for developer fixtures.
func validationPayload(data []byte, messageID, businessService string) ([]byte, int) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var rootSeen bool
	var info envelopeInfo
	var capture string
	headerDepth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			return data, 0 // the schema validator reports well-formedness
		}
		switch value := token.(type) {
		case xml.StartElement:
			if !rootSeen {
				rootSeen = true
				if value.Name.Local == "Document" {
					return data, 0
				}
				info.wrapped = true
			}
			if value.Name.Local == "Document" {
				payload, ok := encodeElement(decoder, value)
				if !ok {
					return data, 0
				}
				return payload, envelopeErrorCount(info, messageID, businessService)
			}
			if value.Name.Local == "AppHdr" && headerDepth == 0 {
				info.appHeader = true
				info.headerNamespace = value.Name.Space
				headerDepth = 1
				continue
			}
			if headerDepth == 0 {
				continue
			}
			headerDepth++
			switch value.Name.Local {
			case "Fr":
				info.from = true
			case "To":
				info.to = true
			case "BizMsgIdr", "MsgDefIdr", "BizSvc", "CreDt":
				capture = value.Name.Local
			}
		case xml.CharData:
			if headerDepth == 0 {
				continue
			}
			text := strings.TrimSpace(string(value))
			if text == "" {
				continue
			}
			switch capture {
			case "BizMsgIdr":
				info.businessMessageID += text
			case "MsgDefIdr":
				info.messageDefinitionID += text
			case "BizSvc":
				info.businessService += text
			case "CreDt":
				info.creationDate += text
			}
		case xml.EndElement:
			if value.Name.Local == capture {
				capture = ""
			}
			if headerDepth > 0 {
				headerDepth--
			}
		}
	}
}

func encodeElement(decoder *xml.Decoder, start xml.StartElement) ([]byte, bool) {
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	if encoder.EncodeToken(start) != nil {
		return nil, false
	}
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		if _, ok := token.(xml.StartElement); ok {
			depth++
		}
		if _, ok := token.(xml.EndElement); ok {
			depth--
		}
		if encoder.EncodeToken(token) != nil {
			return nil, false
		}
	}
	if encoder.Flush() != nil {
		return nil, false
	}
	return output.Bytes(), true
}

func envelopeErrorCount(info envelopeInfo, messageID, businessService string) int {
	if !info.wrapped {
		return 0
	}
	errors := 0
	for _, present := range []bool{
		info.appHeader, info.headerNamespace == businessApplicationHeaderNS,
		info.from, info.to, info.businessMessageID != "", info.creationDate != "",
		info.messageDefinitionID == messageID,
	} {
		if !present {
			errors++
		}
	}
	if businessService != "" && info.businessService != businessService {
		errors++
	}
	if info.creationDate != "" {
		if _, err := time.Parse(time.RFC3339Nano, info.creationDate); err != nil {
			errors++
		}
	}
	return errors
}

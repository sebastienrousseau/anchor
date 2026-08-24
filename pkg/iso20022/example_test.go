// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package iso20022_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// These run under `go test`, so the documentation cannot drift from the API.

func ExampleGenerate() {
	xml, err := iso20022.Generate(iso20022.GeneratorOptions{
		MsgType: "pacs.008",
		Preset:  "sepa",
		Amount:  "12500.50",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(bytes.Contains([]byte(xml), []byte("pacs.008.001.10")))
	// Output: true
}

func ExampleLint() {
	xml, _ := iso20022.Generate(iso20022.DefaultGeneratorOptions("pacs.008"))

	res, err := iso20022.Lint([]byte(xml), "transfer.xml")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("errors=%d warnings=%d\n", res.Errors, res.Warnings)
	// Output: errors=0 warnings=0
}

func ExampleValidateAgainst() {
	schema := `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:demo" xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified" targetNamespace="urn:demo">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence><xs:element name="Ccy" type="Cur"/></xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Cur">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`

	res, err := iso20022.ValidateAgainst(
		[]byte(`<Document xmlns="urn:demo"><Ccy>EURO</Ccy></Document>`), []byte(schema))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(res.Valid, res.Errors[0].Rule)
	// Output: false pattern
}

func ExampleXMLToJSON() {
	json, err := iso20022.XMLToJSON([]byte(`<Document><MsgId>ABC</MsgId></Document>`))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(bytes.Contains(json, []byte(`"MsgId"`)))
	// Output: true
}

func ExampleLookupCode() {
	for _, c := range iso20022.LookupCode("AC04") {
		fmt.Printf("%s = %s\n", c.Code, c.Name)
	}
	// Output: AC04 = Account Closed
}

func ExampleTranslateSWIFT() {
	m, ok := iso20022.TranslateSWIFT("MT103")
	if !ok {
		fmt.Println("no mapping")
		return
	}
	fmt.Printf("%s -> %s\n", m.MTCode, m.MXCode)
	// Output: MT103 -> pacs.008.001.10
}

// ExampleCatalogue_Lookup shows the light-mode contract: a real identifier
// always resolves, and reports where to download the schema when it is absent.
func ExampleCatalogue_Lookup() {
	var cat *iso20022.Catalogue // nil is valid: light mode

	info, err := cat.Lookup("pacs.008.001.10")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(info.ID, info.Installed)
	fmt.Println(info.Sets[0].DownloadURL())
	// Output:
	// pacs.008.001.10 false
	// https://www.iso20022.org/message-set/1036/download
}

func ExampleValidateIBAN() {
	ok, reason := iso20022.ValidateIBAN("DE89370400440532013000")
	fmt.Println(ok, reason == "")

	ok, _ = iso20022.ValidateIBAN("DE00370400440532013000")
	fmt.Println(ok)
	// Output:
	// true true
	// false
}

// ExampleGenerateLifecycle builds a linked four-stage payment chain, then posts
// the interbank leg to an in-process mock clearing rail.
func ExampleGenerateLifecycle() {
	chain, err := iso20022.GenerateLifecycle(iso20022.DefaultGeneratorOptions("pacs.008"))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(len(chain.Steps) >= 4)
	// Output: true
}

// ExampleLint_httpService shows the SDK inside a request handler, which is how
// a payment service would use it.
func ExampleLint_httpService() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		res, err := iso20022.Lint(body, "inbound.xml")
		if err != nil || res.Errors > 0 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	xml, _ := iso20022.Generate(iso20022.DefaultGeneratorOptions("pacs.008"))
	resp, err := http.Post(srv.URL, "application/xml", bytes.NewReader([]byte(xml)))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	fmt.Println(resp.StatusCode)
	// Output: 202
}

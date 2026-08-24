// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/swift"
	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// A conversion that produces a schema-invalid document is worthless however good
// its report reads, so every supported MT type is converted and validated
// against the real schema. This needs a catalogue, so it skips without one.
//
//	make conformance
func TestConvertedMessagesValidate(t *testing.T) {
	root, err := catalog.Resolve("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}
	idx, err := catalog.Load(root)
	if err != nil {
		t.Skipf("catalogue would not load: %v", err)
	}

	cases := []struct{ name, raw string }{
		{"mt101", mt101Fixture},
		{"mt104", mt104Fixture},
		{"mt104-minimal", mt104Minimal},
		{"mt107", mt107Fixture},
		{"mt204", mt204Fixture},
		{"mt204-minimal", mt204Minimal},
		{"mt101-minimal", mt101Minimal},
		{"mt103", mt103Fixture},
		{"mt103-minimal", mt103Minimal},
		{"mt202", mt202Fixture},
		{"mt940", mt940Fixture},
		{"mt940-entries", mt940EntriesFixture},
		{"mt192", mt192Fixture},
		{"mt195", mt195Fixture},
		{"mt196", mt196Fixture},
		{"mt295-minimal", mt295Minimal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := swift.Parse([]byte(tc.raw))
			if err != nil {
				t.Fatalf("parsing: %v", err)
			}
			conv, err := swift.Convert(m)
			if err != nil {
				t.Fatalf("converting: %v", err)
			}

			schemaPath := ""
			for _, msg := range idx.Messages {
				if msg.ID == conv.TargetType && msg.XSDPath != "" {
					schemaPath = msg.XSDPath
					break
				}
			}
			if schemaPath == "" {
				t.Skipf("%s is not installed in this catalogue", conv.TargetType)
			}

			schema, err := xsd.ParseFile(schemaPath)
			if err != nil {
				t.Fatalf("parsing %s: %v", schemaPath, err)
			}
			res := validator.Validate([]byte(conv.XML), schema)
			if !res.Valid {
				for _, e := range res.Errors {
					t.Errorf("%d:%d [%s] %s", e.Line, e.Column, e.Rule, e.Message)
				}
				t.Fatalf("converted %s does not validate against %s\n%s",
					tc.name, conv.TargetType, conv.XML)
			}

			// Cross-check against libxml2 when it is available: agreement with the
			// reference implementation is what makes the verdict trustworthy.
			if _, err := exec.LookPath("xmllint"); err != nil {
				return
			}
			out := filepath.Join(t.TempDir(), tc.name+".xml")
			if err := os.WriteFile(out, []byte(conv.XML), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("xmllint", "--noout", "--schema", schemaPath, out)
			if combined, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("xmllint rejected the converted message: %v\n%s", err, combined)
			}
		})
	}
}

const mt103Fixture = `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:REF20260824001
:23B:CRED
:23E:PHON/CALL TREASURY DESK
:32A:260824EUR24950,00
:33B:GBP21000,00
:36:1,1880
:50K:/GB29NWBK60161331926819
ACME TRADING LIMITED
14 GRESHAM STREET
LONDON EC2V 7NN
:52A:BANKGB2LXXX
:57A:BANKDEFFXXX
:59:/DE89370400440532013000
MUELLER GMBH
HAUPTSTRASSE 12
60311 FRANKFURT AM MAIN
:70:INVOICE 2026-0815 CONSULTING SERVICES
:71A:SHA
:71F:EUR25,00
:71G:EUR25,00
-}{5:{CHK:123456789ABC}}`

// The minimal case exercises every placeholder and fallback at once.
const mt103Minimal = `{1:F01}{2:I103}{4:
:20:REF1
:32A:260824EUR1,00
-}`

const mt202Fixture = `{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{4:
:20:COVER20260824
:21:REF20260824001
:32A:260824EUR25000,00
:52A:BANKGB2LXXX
:53A:CHASGB2LXXX
:57A:BANKDEFFXXX
:58A:DEUTDEFFXXX
-}`

const mt940Fixture = `{1:F01BANKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:
:20:STMT20260824
:25:GB29NWBK60161331926819
:28C:00123/001
:60F:C260823EUR100000,00
:61:2608240824C25000,00NTRFREF20260824001//BANKREF
:86:INVOICE 2026-0815
:62F:C260824EUR125000,00
-}`

const mt101Fixture = `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ20260824001
:28D:1/1
:30:260826
:50H:/GB29NWBK60161331926819
ACME TRADING LIMITED
14 GRESHAM STREET
LONDON EC2V 7NN
:52A:BANKGB2LXXX
:21:PAY-001
:23E:URGP
:32B:EUR12500,00
:57A:BANKDEFFXXX
:59:/DE89370400440532013000
MUELLER GMBH
HAUPTSTRASSE 12
:70:INVOICE 2026-0815
:71A:SHA
:21:PAY-002
:21F:FX-4471
:32B:USD8000,00
:36:1,0840
:57A:CHASUS33XXX
:59:/US64SVBKUS6S3300958879
NORTHWIND INC
:70:INVOICE 2026-0816
:71A:OUR
:25A:/GB29NWBK60161331926819
-}`

// Every mandatory element has to be present even when the source supplies
// nothing to fill it, so the minimal case is the one most likely to break.
const mt101Minimal = `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
-}`

const mt104Fixture = `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-20260824-001
:23E:AUTH
:30:260901
:50C:ACMEGB2LXXX
:50K:/GB29NWBK60161331926819
ACME UTILITIES LIMITED
14 GRESHAM STREET
LONDON
:52A:BANKGB2LXXX
:71A:SHA
:21:COLL-001
:21C:MANDATE-4471
:32B:EUR120,50
:57A:BANKDEFFXXX
:59:/DE89370400440532013000
MUELLER GMBH
:70:INVOICE 2026-0901
:21:COLL-002
:21C:MANDATE-4472
:32B:EUR75,00
:71A:OUR
:25A:/GB29NWBK60161331926819
:57A:CHASUS33XXX
:59:/US64SVBKUS6S3300958879
NORTHWIND INC
-}`

// Every mandatory element still has to be present when the source supplies
// nothing to fill it: Cdtr, CdtrAcct, CdtrAgt, Dbtr, DbtrAcct and DbtrAgt are
// all required.
const mt104Minimal = `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:21:COLL-1
:32B:EUR1,00
-}`

const mt107Fixture = `{1:F01ACMEGB2LAXXX0000000000}{2:I107BANKGB2LXXXXN}{4:
:20:GDD-20260824
:30:260901
:50K:/GB29NWBK60161331926819
ACME UTILITIES LIMITED
:52A:BANKGB2LXXX
:21:COLL-001
:21C:MANDATE-9001
:32B:GBP42,00
:57A:MIDLGB22XXX
:59:/GB94BARC10201530093459
SMITH AND SONS
-}`

const mt204Fixture = `{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:
:20:FMDD-20260824
:19:250000,00
:30:260826
:58A:BANKGB2LXXX
:20:TXN-001
:21:REL-001
:32B:EUR150000,00
:53A:DEUTDEFFXXX
:72:/ACC/MARGIN CALL
:20:TXN-002
:21:REL-002
:32B:EUR100000,00
:53A:BNPAFRPPXXX
-}`

const mt204Minimal = `{1:F01}{2:I204}{4:
:20:FMDD-1
:20:TXN-1
:32B:EUR1,00
-}`

const mt192Fixture = `{1:F01BANKGB2LAXXX0000000000}{2:I192BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:CANC20260824001
:21:REF20260824001
:11S:103
260824
:32A:260824EUR25000,00
:79:/AC03/BENEFICIARY ACCOUNT CLOSED
PLEASE CANCEL AND RETURN FUNDS
-}`

const mt195Fixture = `{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:
:20:QRY20260824001
:21:REF20260824001
:11S:103
260824
:75:/1/WAS THIS PAYMENT CREDITED
/2/PLEASE CONFIRM VALUE DATE
-}`

const mt196Fixture = `{1:F01BANKDEFFAXXX0000000000}{2:I196BANKGB2LXXXXN}{4:
:20:ANS20260824001
:21:QRY20260824001
:76:/1/CREDITED 24 AUGUST 2026
/2/VALUE DATE CONFIRMED
-}`

// Another category, and nothing optional: the mandatory elements still have to
// be there.
const mt295Minimal = `{1:F01}{2:I295}{4:
:20:QRY-1
-}`

// A statement with entries in it, which is what a real MT940 carries.
const mt940EntriesFixture = `{1:F01NWBKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:
:20:STMT20260824
:25:GB29NWBK60161331926819
:28C:00123/001
:60F:C260823EUR100000,00
:61:2608240823C25000,00NTRFE2E-0001//SVCR-9001
:86:INCOMING TRANSFER
INVOICE 2026-0815
:61:260824RD150,00NMSCNONREF
:86:REVERSED CHARGE
:62F:C260824EUR124850,00
-}`

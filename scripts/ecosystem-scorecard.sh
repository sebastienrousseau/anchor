#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT

set -euo pipefail

SCORE_FLOOR="0.90"
PASS_COUNT=0
TOTAL_PROBES=8

echo ""
echo "=========================================================================="
echo "                   Anchor ISO 20022 Ecosystem Scorecard                   "
echo "=========================================================================="
echo "Host: $(uname -s) $(uname -m) | Go: $(go version | awk '{print $3}')"
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "--------------------------------------------------------------------------"
printf " %-4s | %-38s | %-12s | %-8s\n" "#" "Quality Probe" "Command" "Status"
echo "--------------------------------------------------------------------------"

run_probe() {
    local num="$1"
    local name="$2"
    local cmd="$3"
    local exec_cmd="$4"

    if eval "$exec_cmd" > /dev/null 2>&1; then
        printf " %-4s | %-38s | %-12s | \033[32mPASS\033[0m\n" "$num" "$name" "$cmd"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        printf " %-4s | %-38s | %-12s | \033[31mFAIL\033[0m\n" "$num" "$name" "$cmd"
    fi
}

run_probe "01" "Binary Compilation & Stripping" "make build" "make build"
run_probe "02" "Code Formatting Cleanliness" "gofmt -l" "[ -z \"\$(gofmt -l cmd/ internal/ pkg/ 2>/dev/null)\" ]"
run_probe "03" "Static Analysis & Vet" "go vet" "go vet ./..."
run_probe "04" "Unit Test Suite & Concurrency" "go test" "go test -v ./..."
if ./anchor doctor >/dev/null 2>&1; then
    run_probe "05" "Catalogue integrity" "anchor info" "./anchor info pacs.008.001.10"
else
    printf " %-4s | %-38s | %-12s | \033[33mSKIP\033[0m\n" "05" "Catalogue integrity" "anchor info"
    printf "      | %-38s |\n" "no catalogue installed - see README"
    TOTAL_PROBES=$((TOTAL_PROBES - 1))
fi
run_probe "06" "Semantic Linter (IBAN & UUIDv4)" "anchor lint" "./anchor generate pacs.008 --preset sepa -o /tmp/probe_pacs.xml && ./anchor lint /tmp/probe_pacs.xml"
run_probe "07" "End-to-End Flow Simulator" "anchor flow" "./anchor flow --preset sepa"
run_probe "08" "XML <-> JSON Bi-directional" "anchor convert" "./anchor convert /tmp/probe_pacs.xml --to-json"

rm -f /tmp/probe_pacs.xml

echo "--------------------------------------------------------------------------"
SCORE=$(awk "BEGIN {printf \"%.2f\", $PASS_COUNT / $TOTAL_PROBES}")
printf " Summary: %d/%d probes passed. Overall Quality Score: \033[1;36m%s\033[0m / 1.00\n" "$PASS_COUNT" "$TOTAL_PROBES" "$SCORE"
echo "=========================================================================="
echo ""

if awk "BEGIN {exit !($SCORE >= $SCORE_FLOOR)}"; then
    echo "✅ Ecosystem Scorecard PASS (Score $SCORE >= Floor $SCORE_FLOOR)"
    exit 0
else
    echo "❌ Ecosystem Scorecard FAIL (Score $SCORE < Floor $SCORE_FLOOR)"
    exit 1
fi

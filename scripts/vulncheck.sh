#!/usr/bin/env bash
# vulncheck.sh — reachability-aware Go vulnerability scan.
#
# Runs govulncheck and fails if any *reachable* vulnerability is found that is
# not on the documented allowlist. The allowlist is parsed from osv-scanner.toml
# (the [[IgnoredVulns]] ids), so suppressions and their rationale live in one
# place shared by govulncheck (this script), osv-scanner, and CI.
#
# Used by `make vulncheck` and .github/workflows/build.yml. Run from repo root.
set -u

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

# Invoke the govulncheck binary directly: `go run pkg@latest` does NOT
# propagate the child's exit code (it exits 1 for any failure), which would
# make findings (exit 3) indistinguishable from tool errors.
GOVULNCHECK="$(command -v govulncheck || true)"
if [ -z "$GOVULNCHECK" ]; then
	echo "govulncheck not found in PATH, installing..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	GOVULNCHECK="$(go env GOPATH)/bin/govulncheck"
fi

"$GOVULNCHECK" ./... | tee "$tmpdir/report"
status=${PIPESTATUS[0]}

# govulncheck exit codes: 0 = clean, 3 = vulnerabilities found. Anything else
# means the tool itself failed (network, module load, ...): fail loudly instead
# of passing on an empty report.
if [ "$status" -ne 0 ] && [ "$status" -ne 3 ]; then
	echo "error: govulncheck failed to run (exit $status)" >&2
	exit 1
fi

grep -oE 'GO-[0-9]{4}-[0-9]+' osv-scanner.toml 2>/dev/null | sort -u >"$tmpdir/allow"
grep -oE 'GO-[0-9]{4}-[0-9]+' "$tmpdir/report" | sort -u >"$tmpdir/found"

unexpected="$(comm -23 "$tmpdir/found" "$tmpdir/allow")"
if [ -n "$unexpected" ]; then
	echo "" >&2
	echo "error: reachable vulnerabilities not in the allowlist:" >&2
	echo "$unexpected" >&2
	echo "Fix them, or — if genuinely non-exploitable — add each ID to osv-scanner.toml with a reason." >&2
	exit 1
fi

echo "OK: no reachable vulnerabilities outside the documented allowlist ($(tr '\n' ' ' <"$tmpdir/allow"))."

#!/bin/bash
# Lint gate for commits.
#   run   - run `make lint`; on success record the staged tree and save the staged
#           diff for the commit review, on failure clear the record.
#   check - exit 0 when the record matches what is staged right now.
# The record lives in bin/lint-gate/ (git-ignored).

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
GATE_DIR="bin/lint-gate"

case "${1:-}" in
run)
	rm -rf "$GATE_DIR"
	make lint
	mkdir -p "$GATE_DIR"
	git diff --cached >"$GATE_DIR/staged.diff"
	git write-tree >"$GATE_DIR/tree"
	echo "✅ Lint passed for the staged changes."
	;;
check)
	[[ -f "$GATE_DIR/tree" ]] && [[ "$(cat "$GATE_DIR/tree")" == "$(git write-tree)" ]]
	;;
*)
	echo "usage: $0 run|check" >&2
	exit 2
	;;
esac

#!/bin/bash
# Lint gate for commits.
#   run   - lint every language found in the tracked files (whole project, not just the
#           diff); on success record the staged tree and save the staged diff for the
#           commit review. Any failure, missing linter or unlintable language blocks.
#   check - exit 0 when the record matches what is staged right now.
# Project settings live in scripts/lint-gate.conf; the record in .lint-gate/ (git-ignored).

set -uo pipefail # no -e: run every linter, then report all problems together

cd "$(git rev-parse --show-toplevel)" || exit 1
GATE_DIR=".lint-gate"
CONF="scripts/lint-gate.conf"
LINT_CMD=""
COVERED=""
NOT_CODE=""
# shellcheck source=/dev/null
[[ -f "$CONF" ]] && source "$CONF"

FAILED=()
MISSING=()
UNSUPPORTED=()

covered() { [[ " $COVERED " == *" $1 "* ]]; }

need() { # need <command> <install hint>
	command -v "$1" >/dev/null && return 0
	MISSING+=("$1 (install: $2)")
	return 1
}

step() { # step <label> <command...>
	local label=$1
	shift
	echo "▶ $label"
	"$@" </dev/null || FAILED+=("$label")
}

# module_dirs <manifest> <source pathspec>: the dirs holding the manifest, plus the repo root
# when a source file sits outside all of them, so such files fail at the root instead of
# being skipped.
# shortcut: nested modules inside a go.work or Cargo workspace are linted more than once; fine until it gets slow.
module_dirs() {
	local mods
	mods=$(git ls-files -- ":(glob)**/$1" | sed -e 's|/*[^/]*$||' -e 's|^$|.|')
	git ls-files -- "$2" | MODS="$mods" awk '
		BEGIN { n = split(ENVIRON["MODS"], m, "\n"); for (i = 1; i <= n; i++) print m[i] }
		{ for (i = 1; i <= n; i++) if (m[i] == "." || index($0, m[i] "/") == 1) next; orphan = 1 }
		END { if (orphan) print "." }' | sort -u
}

lint_go() {
	need golangci-lint "brew install golangci-lint" || return 0
	local dir
	while IFS= read -r dir; do
		# shellcheck disable=SC2016 # $1 expands in the inner bash
		step "golangci-lint in $dir" bash -c 'cd "$1" && golangci-lint run ./...' _ "$dir"
	done < <(module_dirs go.mod '*.go')
}

lint_rust() {
	if ! cargo clippy --version >/dev/null 2>&1; then
		MISSING+=("cargo clippy (install: rustup component add clippy; Rust itself: https://rustup.rs)")
		return 0
	fi
	local dir
	while IFS= read -r dir; do
		# shellcheck disable=SC2016 # $1 expands in the inner bash
		step "cargo clippy in $dir" bash -c 'cd "$1" && cargo clippy --all-targets -- -D warnings' _ "$dir"
	done < <(module_dirs Cargo.toml '*.rs')
}

lint_python() {
	need ruff "brew install ruff" || return 0
	step "ruff" ruff check .
}

lint_js() {
	need oxlint "npm install -g oxlint" || return 0
	step "oxlint" oxlint --react-plugin --deny-warnings .
}

lint_shell() {
	need shellcheck "brew install shellcheck" || return 0
	step "shellcheck" bash -c 'git ls-files -z -- "*.sh" "*.bash" | xargs -0 shellcheck'
}

# Map each tracked file extension to a language. Anything that is neither a known
# language nor on the non-code list is reported as unsupported, so new languages fail closed.
detect_languages() {
	local ext lang
	LANGS=" "
	while IFS= read -r ext; do
		[[ " $NOT_CODE " == *" $ext "* ]] && continue
		case "$ext" in
		go) lang=go ;;
		py | pyi) lang=python ;;
		rs) lang=rust ;;
		js | jsx | mjs | cjs | ts | tsx | mts | cts) lang=js ;;
		sh | bash) lang=shell ;;
		md | markdown | mdx | txt | rst | adoc | json | jsonc | json5 | yaml | yml | toml | ini | cfg | conf | \
			env | lock | sum | mod | work | csv | tsv | xml | plist | svg | png | jpg | jpeg | gif | webp | ico | \
			bmp | pdf | woff | woff2 | ttf | otf | eot | mp3 | mp4 | wav | webm | mov | zip | gz | tgz | tar | \
			html | htm | css | scss | sass | less | tmpl | tpl | gotmpl | j2 | diff | patch | log | example | \
			sample | template | gitkeep | pem | crt | pub | ldif | service | dockerfile)
			continue ;;
		*)
			covered "$ext" || UNSUPPORTED+=(".$ext")
			continue
			;;
		esac
		[[ "$LANGS" == *" $lang "* ]] || LANGS+="$lang "
	done < <(git ls-files | sed -n -e 's|.*/||' -e '/^\./d' -e 's/.*\.//p' | tr '[:upper:]' '[:lower:]' | sort -u)
}

report() { # report <heading> <items...>
	echo "❌ $1"
	shift
	printf '   - %s\n' "$@"
}

case "${1:-}" in
run)
	rm -rf "$GATE_DIR"
	[[ -n "$LINT_CMD" ]] && step "project lint: $LINT_CMD" bash -c "$LINT_CMD"
	detect_languages
	# Built-in languages are always linted the same way; COVERED only applies to other file types.
	for lang in $LANGS; do
		"lint_$lang"
	done

	ok=1
	if ((${#MISSING[@]})); then
		ok=0
		report "Linters not installed (ask the user before installing; then rerun):" "${MISSING[@]}"
	fi
	if ((${#UNSUPPORTED[@]})); then
		ok=0
		report "No built-in linter for these file types:" "${UNSUPPORTED[@]}"
		echo "   Fix: add a command that lints them to LINT_CMD in $CONF and list the"
		echo "   extensions (without the dot) in COVERED; or, only if they really are not code,"
		echo "   list them in NOT_CODE there."
	fi
	if ((${#FAILED[@]})); then
		ok=0
		report "Lint failed:" "${FAILED[@]}"
	fi
	((ok)) || exit 1

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

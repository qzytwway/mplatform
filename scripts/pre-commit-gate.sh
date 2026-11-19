#!/bin/bash
# pre-commit-gate: the body of the git pre-commit hook.
#   (no argument) - format the staged files and restage them, then lint the staged files.
#                   Blocks the commit on lint errors, missing tools, partially staged files
#                   and code file types it has no linter for. Skip once: git commit --no-verify
#   install       - point .git/hooks/pre-commit at this script (for a fresh clone).
# Project settings: scripts/pre-commit-gate.conf

set -uo pipefail # no -e: run every tool, then report all problems together
exec 1>&2        # git shows hook output from stderr
# GUI git clients often start hooks without the login shell's PATH
PATH="$PATH:/opt/homebrew/bin:/usr/local/bin:$HOME/.cargo/bin:$HOME/go/bin:$HOME/.local/bin"

cd "$(git rev-parse --show-toplevel)" || exit 1
CONF="scripts/pre-commit-gate.conf"
MARK="# pre-commit-gate"
# shellcheck disable=SC2016 # expands when the hook runs
CALL='"$(git rev-parse --show-toplevel)/scripts/pre-commit-gate.sh" || exit 1'

if [[ "${1:-}" == install ]]; then
	hook=$(git rev-parse --git-path hooks/pre-commit) # follows core.hooksPath (husky etc.)
	if [[ -f "$hook" ]] && ! grep -qF "$MARK" "$hook"; then
		echo "已经有别的 pre-commit 钩子，没有改动：$hook"
		exit 3
	fi
	mkdir -p "$(dirname "$hook")"
	printf '#!/bin/sh\n%s\n%s\n' "$MARK" "$CALL" >"$hook"
	chmod +x "$hook"
	echo "✅ 已安装：$hook"
	exit 0
fi

LINT_CMD=""
COVERED=""
NOT_CODE=""
# shellcheck source=/dev/null
[[ -f "$CONF" ]] && source "$CONF"

LOG=$(mktemp)
trap 'rm -f "$LOG"' EXIT
FAILED=()
MISSING=()
PARTIAL=()
UNSUPPORTED=()
GO=()
PY=()
RS=()
JS=()
SH=()
OTHER=()

has() { # has <word> <list...>
	local w=$1
	shift
	[[ " $* " == *" $w "* ]]
}

missing() { # missing <text>: record once
	has "$1" "${MISSING[*]-}" || MISSING+=("$1")
}

need() { # need <command> <install hint>
	command -v "$1" >/dev/null && return 0
	missing "$1（安装：$2）"
	return 1
}

step() { # step <label> <command...>: run it, keep its output for the report
	local label=$1
	shift
	echo "▶ $label" >>"$LOG"
	"$@" </dev/null >>"$LOG" 2>&1 || FAILED+=("$label")
}

nearest() { # nearest <file> <path>: closest ancestor dir of the file that contains <path>
	local d=$1
	while [[ "$d" == */* ]]; do
		d=${d%/*}
		[[ -e "$d/$2" ]] && echo "$d" && return
	done
	[[ -e "$2" ]] && echo .
}

lang_of() { # lang_of <extension>: go py rs js sh other skip unsupported
	has "$1" "$NOT_CODE" && echo skip && return
	case "$1" in
	go) echo go ;;
	py | pyi) echo py ;;
	rs) echo rs ;;
	js | jsx | mjs | cjs | ts | tsx | mts | cts) echo js ;;
	sh | bash) echo sh ;;
	md | markdown | mdx | txt | rst | adoc | json | jsonc | json5 | yaml | yml | toml | ini | cfg | conf | \
		env | lock | sum | mod | work | csv | tsv | xml | plist | svg | png | jpg | jpeg | gif | webp | ico | \
		bmp | pdf | woff | woff2 | ttf | otf | eot | mp3 | mp4 | wav | webm | mov | zip | gz | tgz | tar | \
		html | htm | css | scss | sass | less | tmpl | tpl | gotmpl | j2 | diff | patch | log | example | \
		sample | template | gitkeep | pem | crt | pub | ldif | service | dockerfile)
		echo skip
		;;
	*) if has "$1" "$COVERED"; then echo other; else echo unsupported; fi ;;
	esac
}

# Sort the staged files by language. A code file that also has unstaged edits blocks:
# restaging it after formatting would pull those edits into the commit.
classify() {
	local f base lang
	while IFS= read -r -d '' f; do
		base=${f##*/}
		# shortcut: files without an extension (Makefile, extensionless scripts) and dotfiles are not checked; add a rule if they matter.
		[[ "$base" == *.* && "$base" != .* ]] || continue
		lang=$(lang_of "$(tr '[:upper:]' '[:lower:]' <<<"${base##*.}")")
		case "$lang" in
		skip) continue ;;
		unsupported)
			UNSUPPORTED+=("$f")
			continue
			;;
		esac
		if ! git diff --quiet -- "$f"; then
			PARTIAL+=("$f")
			continue
		fi
		case "$lang" in
		go) GO+=("$f") ;;
		py) PY+=("$f") ;;
		rs) RS+=("$f") ;;
		js) JS+=("$f") ;;
		sh) SH+=("$f") ;;
		other) OTHER+=("$f") ;;
		esac
	done < <(git diff --cached --name-only -z --diff-filter=ACMR)
}

rust_edition() { # rust_edition <file>: edition from the nearest Cargo.toml, 2021 if unknown
	local dir ed
	dir=$(nearest "$1" Cargo.toml)
	ed=$(sed -n 's/^edition *= *"\([0-9]*\)".*/\1/p' "${dir:-.}/Cargo.toml" 2>/dev/null | head -1)
	echo "${ed:-2021}"
}

format_file() { # format_file <file> <lang>: rewrite the file in place
	local dir
	case "$2" in
	go) need gofmt "随 Go 一起安装：brew install go" && step "gofmt $1" gofmt -w "$1" ;;
	py) need ruff "brew install ruff" && step "ruff format $1" ruff format -q --force-exclude "$1" ;;
	# shortcut: rustfmt also formats out-of-line child modules of the file; those edits stay unstaged.
	rs) need rustfmt "rustup component add rustfmt" && step "rustfmt $1" rustfmt --edition "$(rust_edition "$1")" "$1" ;;
	sh) need shfmt "brew install shfmt" && step "shfmt $1" shfmt -w "$1" ;;
	js)
		dir=$(nearest "$1" node_modules/.bin/prettier)
		if [[ -z "$dir" ]]; then
			dir=$(nearest "$1" package.json)
			missing "prettier（在项目里安装：cd ${dir:-.} && npm install -D prettier）"
			return
		fi
		# run from $dir so the project's prettier config and .prettierignore apply
		# shellcheck disable=SC2016 # $1/$2 expand in the inner bash
		step "prettier $1" bash -c 'cd "$1" && node_modules/.bin/prettier --write --log-level warn "$2"' _ "$dir" "${1#"$dir"/}"
		;;
	esac
}

format_all() { # format_all <lang> <files...>: format, then restage what changed
	local lang=$1 f before
	shift
	for f in "$@"; do
		before=$(git hash-object -- "$f")
		format_file "$f" "$lang"
		[[ "$(git hash-object -- "$f")" == "$before" ]] || git add -- "$f"
	done
}

lint_go() { # golangci-lint works per package: lint the packages that hold the staged files
	need golangci-lint "brew install golangci-lint" || return 0
	local f mod rel pairs
	pairs=$(for f in "$@"; do
		mod=$(nearest "$f" go.mod)
		mod=${mod:-.}
		rel=${f#"$mod"/}
		[[ "$rel" == */* ]] && rel="./${rel%/*}" || rel=.
		printf '%s\t%s\n' "$mod" "$rel"
	done | sort -u)
	local pkgs
	while IFS= read -r mod; do
		pkgs=()
		while IFS= read -r rel; do pkgs+=("$rel"); done < <(awk -F'\t' -v m="$mod" '$1 == m { print $2 }' <<<"$pairs")
		# shellcheck disable=SC2016 # $1/$@ expand in the inner bash
		step "golangci-lint in $mod" bash -c 'cd "$1" && shift && golangci-lint run "$@"' _ "$mod" "${pkgs[@]}"
	done < <(cut -f1 <<<"$pairs" | sort -u)
}

lint_rust() { # clippy works per crate: lint the crates that hold the staged files
	if ! cargo clippy --version >/dev/null 2>&1; then
		missing "cargo clippy（安装：rustup component add clippy；没有 Rust 先装：https://rustup.rs）"
		return 0
	fi
	local f dir
	while IFS= read -r dir; do
		# shellcheck disable=SC2016 # $1 expands in the inner bash
		step "cargo clippy in $dir" bash -c 'cd "$1" && cargo clippy --all-targets -- -D warnings' _ "$dir"
	done < <(for f in "$@"; do
		dir=$(nearest "$f" Cargo.toml)
		echo "${dir:-.}"
	done | sort -u)
}

lint_all() {
	((${#PY[@]})) && need ruff "brew install ruff" && step "ruff check" ruff check --force-exclude "${PY[@]}"
	((${#JS[@]})) && need oxlint "npm install -g oxlint" && step "oxlint" oxlint --react-plugin --deny-warnings "${JS[@]}"
	((${#SH[@]})) && need shellcheck "brew install shellcheck" && step "shellcheck" shellcheck "${SH[@]}"
	((${#GO[@]})) && lint_go "${GO[@]}"
	((${#RS[@]})) && lint_rust "${RS[@]}"
	((${#OTHER[@]})) && [[ -n "$LINT_CMD" ]] && step "LINT_CMD: $LINT_CMD" bash -c "$LINT_CMD"' "$@"' _ "${OTHER[@]}"
	return 0
}

report() { # report <heading> <items...>
	echo "❌ $1"
	shift
	printf '   - %s\n' "$@"
}

classify
((${#GO[@]})) && format_all go "${GO[@]}"
((${#PY[@]})) && format_all py "${PY[@]}"
((${#RS[@]})) && format_all rs "${RS[@]}"
((${#JS[@]})) && format_all js "${JS[@]}"
((${#SH[@]})) && format_all sh "${SH[@]}"
lint_all

ok=1
if ((${#MISSING[@]})); then
	ok=0
	report "缺少工具（装好后重新提交）：" "${MISSING[@]}"
fi
if ((${#PARTIAL[@]})); then
	ok=0
	report "这些文件只暂存了一部分改动（git add 整个文件，或先 git stash 掉其余改动）：" "${PARTIAL[@]}"
fi
if ((${#UNSUPPORTED[@]})); then
	ok=0
	report "这些代码文件没有检查工具：" "${UNSUPPORTED[@]}"
	echo "   解决：在 $CONF 的 LINT_CMD 里写一条检查它们的命令，并把扩展名（不带点）加进 COVERED；"
	echo "   只有确实不是代码时，才把扩展名加进 NOT_CODE。"
fi
if ((${#FAILED[@]})); then
	ok=0
	report "检查没通过：" "${FAILED[@]}"
fi
if ((ok)); then
	echo "✅ pre-commit-gate：格式化和 lint 都通过了。"
	exit 0
fi
echo "提交被拦下了。改好后重新 git add 再提交；紧急情况可用 git commit --no-verify 跳过检查。"
if [[ -s "$LOG" ]]; then
	echo
	echo "---- 工具原始输出 ----"
	cat "$LOG"
fi
exit 1

#!/bin/bash
# sizereport.sh — report added/changed lines of code for the
# unified-frame-spans feature, against the pre-feature baseline.
#
# Splits files into "implementation" vs "test" and reports gross
# lines added plus a "net" figure that excludes blank lines and
# pure-comment lines (Go //-comments and /* ... */ blocks).
#
# Usage: ./CLAUDE.scripts/sizereport.sh [BASE]
#   BASE defaults to 230d818^ (commit just before the first
#   cleanroom-branch feature commit).

set -e
set -o pipefail

cd "$(dirname "$0")/.."

BASE="${1:-230d818^}"

# Files we want to exclude entirely from the feature report:
#   - docs/* — design docs, plans, scouts, working log
#   - CLAUDE.md / CODING-PROCESS.md — project meta
#   - regression.sh / presub.sh — dev tooling
#   - gozen/* — unrelated prior cleanup that happened on this branch
#   - cmd/E, cmd/logtowin — unrelated deletes
#   - file/file_hash.go, file/buffer_writer*.go — pre-feature commits
#   - README.md / guide / go.mod / go.sum / .gitignore / .github/* — chore
exclude_re='^(docs/|CLAUDE\.md|CODING-PROCESS\.md|regression\.sh|presub\.sh|gozen/|cmd/E/|cmd/logtowin/|file/file_hash\.go|file/buffer_writer|README\.md|guide$|go\.mod|go\.sum|\.gitignore|\.github/)'

# Count "net" lines from a diff hunk: + lines (not "+++"), with
# trailing/leading whitespace stripped, that are not blank and
# not pure Go comments. Handles // line comments and tracks /* */
# block-comment state. Lines whose non-comment portion is empty
# (e.g., "}" preceded only by /* x */) count as code.
net_count() {
	awk '
	BEGIN { in_block = 0 }
	# Only consider added lines from the diff (skip the +++ header).
	/^\+\+\+/ { next }
	/^\+/ {
		line = substr($0, 2)
		# Strip leading/trailing whitespace.
		gsub(/^[ \t]+|[ \t]+$/, "", line)
		if (line == "") next
		# Block-comment continuation.
		if (in_block) {
			if (match(line, /\*\//)) {
				rest = substr(line, RSTART + 2)
				gsub(/^[ \t]+|[ \t]+$/, "", rest)
				in_block = 0
				if (rest == "") next
				# Anything after */ on the same line counts.
				code++
				next
			}
			next
		}
		# Pure //-comment line.
		if (substr(line, 1, 2) == "//") next
		# Block-comment that starts and ends on this line.
		if (substr(line, 1, 2) == "/*") {
			if (match(line, /\*\//)) {
				rest = substr(line, RSTART + 2)
				gsub(/^[ \t]+|[ \t]+$/, "", rest)
				if (rest == "") next
				code++
				next
			}
			in_block = 1
			next
		}
		code++
	}
	END { print code+0 }
	'
}

gross_count() {
	# Count + lines, excluding the +++ file header.
	awk '/^\+\+\+/ { next } /^\+/ { n++ } END { print n+0 }'
}

# Build space-separated lists of feature files (changed in BASE..HEAD).
impl_files=""
test_files=""
while IFS= read -r f; do
	[ -z "$f" ] && continue
	case "$f" in
		*_test.go) test_files="$test_files $f" ;;
		*)         impl_files="$impl_files $f" ;;
	esac
done < <(git diff --name-only "$BASE"..HEAD | grep -vE "$exclude_re" || true)

sum_files() {
	local list=$1
	local counter=$2
	local total=0
	local f
	for f in $list; do
		c=$(git diff "$BASE"..HEAD -- "$f" | $counter)
		total=$((total + c))
	done
	echo "$total"
}

impl_gross=$(sum_files "$impl_files" gross_count)
impl_net=$(sum_files "$impl_files" net_count)
test_gross=$(sum_files "$test_files" gross_count)
test_net=$(sum_files "$test_files" net_count)

printf 'unified-frame-spans size report — baseline %s\n\n' "$BASE"
printf '%-20s %10s %10s\n' "category" "gross" "net"
printf '%-20s %10s %10s\n' "--------" "-----" "---"
printf '%-20s %10d %10d\n' "implementation" "$impl_gross" "$impl_net"
printf '%-20s %10d %10d\n' "tests"          "$test_gross" "$test_net"
printf '%-20s %10d %10d\n' "TOTAL"          "$((impl_gross + test_gross))" "$((impl_net + test_net))"

printf '\nimplementation files:\n'
for f in $impl_files; do printf '  %s\n' "$f"; done
printf '\ntest files:\n'
for f in $test_files; do printf '  %s\n' "$f"; done

#!/bin/bash
# diff_baselines.sh — build a single HTML page that shows each
# failing test's baseline (left) next to its trial (right) for
# side-by-side visual review.
#
# Usage: ./CLAUDE.scripts/diff_baselines.sh > /tmp/diff.html
#        open /tmp/diff.html
#
# Run after `go test ./frame/` has produced _trial.html files
# for the failing tests.

set -e
cd "$(dirname "$0")/.."
ROOT=$(pwd)

cat <<HEADER
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>frame baseline diff</title>
<style>
body { font-family: sans-serif; margin: 1em; }
h2 { background: #f0f0f0; padding: 0.3em; }
.row { display: flex; gap: 1em; margin-bottom: 2em; align-items: flex-start; }
.col { flex: 1; min-width: 0; border: 1px solid #ccc; padding: 0.5em; }
.col h3 { margin: 0 0 0.5em 0; font-weight: normal; }
.col.old h3 { color: #888; }
.col.new h3 { color: #060; }
iframe { width: 100%; height: 420px; border: 1px dashed #ddd; }
</style>
</head>
<body>
<h1>frame baseline diff (B5 word-wrap)</h1>
<p>Left: existing baseline (upstream's partial-fit-split layout).
Right: trial output from the B5 word-boundary-wrap impl.
Review each pair; once approved, the trial will become the new baseline.</p>
HEADER

for trial in $(find frame/testdata -name '*_trial.html' | sort); do
	base="${trial%_trial.html}.html"
	name=$(echo "$trial" | sed -E 's|frame/testdata/||; s|_trial\.html$||')
	if [ ! -f "$base" ]; then
		printf '<h2>%s</h2><p>no baseline file at %s</p>\n' "$name" "$base"
		continue
	fi
	# Absolute file:// URLs so iframes resolve regardless of
	# the diff page's location.
	absbase="file://$ROOT/$base"
	abstrial="file://$ROOT/$trial"
	printf '<h2>%s</h2>\n<div class="row"><div class="col old"><h3>baseline (upstream partial-fit)</h3><iframe src="%s"></iframe></div><div class="col new"><h3>trial (B5 word-wrap)</h3><iframe src="%s"></iframe></div></div>\n' \
		"$name" "$absbase" "$abstrial"
done

echo "</body></html>"

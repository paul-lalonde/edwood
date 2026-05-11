# Working Log — `cleanroom` branch

A handoff-to-future-self log per project CLAUDE.md. Read at the start
of every session on this branch; update at the end of every session
in which the branch changes.

## Purpose

Clean-room re-implementation of the unified-frame-spans feature on
top of `rjkroege/edwood`'s `master`. Spec lives at
`docs/designs/features/unified-frame-spans.md` (§12 lays out the
nine-phase implementation plan; the plan doc at
`docs/plans/PLAN_unified-frame-spans.md` is the working checklist).

The "earlier work" the spec refers to is the styled-text /
markdown-rendering branches in this same repo (Markdown-take-two,
unify-frame-interface, rich-text-markdown, spans-file-external-styling,
…) and in the sibling working copy at `/Users/paul/dev/edwood`. None
of it is reused wholesale; specific tests may be borrowed per the
§13.1 reuse criteria, and only after confirming each test exercises
observable behavior rather than the incumbent's internal layout.

## Base

- Branch: `cleanroom`
- Branched from: `refs/remotes/upstream/master`
- HEAD at Phase 0 setup: `e6ccb75` ("Delete the rest of gozen")
- Upstream remote: `https://github.com/rjkroege/edwood.git` (fetched
  2026-05-11; HEAD coincides with the branch base).

There is *also* a stale local branch named `upstream/master` at
`61193d0`. Do not use it — the canonical upstream pointer is
`refs/remotes/upstream/master`.

## Backup refs

None preserved on this branch yet. The prior styled-text work is
preserved as the dozens of branches already in this repo (see
`git branch -a`). They are read-only references for this rewrite.

## Commit graph

```
e6ccb75 (cleanroom, upstream/master)  ← Phase 0 base
```

No commits have been added on the `cleanroom` branch yet beyond the
upstream base. Untracked at session 1 end: `CLAUDE.md`,
`CODING-PROCESS.md`, `docs/`, `regression.sh`. These are
project-level setup, not yet committed (pending user approval to
commit).

## Test status

`./regression.sh` is green at HEAD. It mirrors the GitHub Actions
workflow (`.github/workflows/edwood.yml`) and runs:

1. `gofmt -d -s .` (no diff)
2. `go vet .` (root package, matches CI)
3. `staticcheck -checks inherit,-U1000,-SA4003 ./...` via `go run`
4. `misspell -error .` via `go run`
5. `go test -race ./...`

Every Phase >= 1 commit must keep `./regression.sh` green.

## Quirks & loose ends

- `go vet ./...` (note the `./...`, not the `.` CI uses) reports
  `file/buffer_adapter.go:53:2: unreachable code` — a pre-existing
  upstream wart in `IndexRune`. CI does not gate on subpackages and
  upstream tolerates it. Leave it alone unless touching `file/`.
- `staticcheck` and `misspell` are run via `go run @version` (no
  `go install`). First invocation downloads modules; subsequent
  runs use the module cache.
- The `cleanroom` working tree has untracked top-level docs
  (`CLAUDE.md`, `CODING-PROCESS.md`) that mirror files at
  `/Users/paul/` and `/Users/paul/dev/edwood-cleanroom/`. They
  exist locally to satisfy session bootstrap but have not been
  committed yet.

## Session summaries

### 2026-05-11 — Phase 0 setup

- Confirmed `cleanroom` sits exactly on `refs/remotes/upstream/master`
  (`e6ccb75`). Fetched upstream; nothing to pull.
- Verified `gofmt`, `go vet .`, `go test -race ./...` all green at
  baseline. All test packages pass: root, complete, drawutil,
  dumpfile, file, frame, ninep, regexp, runes, sam.
- Wrote `regression.sh` mirroring CI exactly; uses `go run` for the
  two lint tools to avoid `$GOPATH/bin` installs.
- Wrote `docs/working-log.md` (this file) and
  `docs/plans/PLAN_unified-frame-spans.md`.
- Did **not** commit; nothing on the branch is staged. Awaiting
  user direction on whether/when to land the Phase 0 setup as a
  commit.

## Next-session candidates

1. Decide commit shape for Phase 0 (one commit for `regression.sh` +
   working log + plan; CLAUDE.md / CODING-PROCESS.md left untracked
   if they are user-only).
2. Begin Phase 1 (frame data types: `StyleRun`, `Style`,
   `ReplacedKind`, `Style.IsZero()`). No interface changes yet.
3. As prep for Phase 1, scout the prior `frame/` package work on
   branches like `unify-frame-interface` for tests that qualify
   under §13.1 reuse criteria — *read-only*; do not cherry-pick.

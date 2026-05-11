# Edwood — Claude Instructions

## Coding workflow

**Before writing any code, read [CODING-PROCESS.md](./CODING-PROCESS.md).**

It defines the four-stage workflow (Design → Tests → Implement → Bug
Classification) used in this project, with explicit gates between
stages.

## How design docs and plans map to the workflow

The repo already follows this workflow:

- **Design (Stage 1)** lives in `docs/designs/features/<feature>.md`.
  Each design doc contains purpose, requirements, signatures, edge
  cases, and explicit non-goals — the inputs CODING-PROCESS calls
  for.
- **Plan** lives in `docs/plans/PLAN_<feature>.md`. Each row of a
  plan table is one CODING-PROCESS pass on a specific deliverable:
  - `[ ] Design`  — confirm the relevant slice of the design doc
  - `[ ] Tests`   — write tests against the requirements
  - `[ ] Iterate` — implement red→green→review (Stage 3)
  - `[ ] Commit`  — commit with the message specified in the row
- **Bug Classification (Stage 4)** applies whenever a test fails:
  classify (implementation accident / undefined behavior / wrong
  design) **before** fixing. The fix starts at the earliest
  affected stage, not at the code.

When working from a plan, treat each row as the entire scope of one
sitting: do not skip the test stage, do not stage-jump on
implementation, and do not skip the commit.

## Reading order when starting work on a module

Per CODING-PROCESS § "Context Management":

1. The feature's design doc (`docs/designs/features/<feature>.md`) —
   tells you what's specified.
2. The feature's plan (`docs/plans/PLAN_<feature>.md`) — tells you
   what stage we're in and what's next.
3. The relevant tests — tells you what's been verified.
4. The implementation — tells you what's been built.

If a plan row is unchecked, you are at Stage 1 (Design review) or
Stage 2 (Tests) for that row, depending on whether the design slice
has been confirmed.

## Project conventions

- Files: 500 LOC max; functions: 30 LOC max (per CODING-PROCESS).
- Tests required for every numbered requirement in a design doc.
- Working logs on long-lived feature branches:
  `docs/working-log.md` — read at start of session, update at end.
- Commits use conventional, verb-first messages. The message for
  each plan row is given in that row's `[ ] Commit` cell.

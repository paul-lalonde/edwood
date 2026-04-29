# Development Workflow: Design → Test → Implement

This project uses a strict design-first, test-driven workflow. Every module flows through four stages in order. **Do not skip stages.**

## Stage 1: Design

Before any code exists for a module, write a `.design.md` file co-located with where the implementation will live:

```
packages/byom-sdk/src/byom/transform/identity.design.md
packages/ingestion-auto/src/ingestion_auto/testing/runner.design.md
```

A design file contains:
- **Purpose** — what this module does and why (one paragraph)
- **Requirements** — numbered list (R1, R2, ...) of specific, testable behaviors
- **Signatures** — function/class signatures with types
- **Edge cases** — what happens with empty input, nulls, invalid data, etc.
- **Not in scope** — what this module explicitly does NOT do

Requirements must be specific enough to write a test from. "Handles errors gracefully" is not a requirement. "Returns ByomTransformError with the column name and available columns when key_column is not found" is a requirement.

The design file draws from the architecture docs (if they exist) but is **self-contained for its module**. Subsequent stages read the design file, not the architecture docs.

**Gate:** The design file must be reviewed before proceeding to tests.

## Stage 2: Tests

Write tests that verify the requirements in the design file. Each test references the requirement it covers:

```python
def test_derive_node_id_deterministic():
    """R3: Same key_column value + source_id produces same UUID across runs."""
```

Every numbered requirement in the design file must have at least one test. If you find you cannot write a test for a requirement, the requirement is underspecified — go back to Stage 1 and fix the design.

**Gate:** Tests must be reviewed before proceeding to implementation. Tests will fail (no implementation yet) — that is correct.

## Stage 3: Implement

Write the implementation to make the tests pass.

During implementation, you read the design file and the test file. You do NOT need to re-read the architecture docs — the design file contains everything relevant to this module.

**Rules:**
- Only write code that makes a failing test pass
- If you realize a test is missing, add it first (go back to Stage 2), then implement
- If you realize the design is underspecified, update the design first (go back to Stage 1), add the test (Stage 2), then implement
- Do not add behaviors that aren't in the design

## Stage 4: Bug Classification

When a test fails or a bug is found during review, **classify it before fixing it**:

**Implementation accident** — the design and tests are correct, the code is wrong:
1. If no test catches this specific failure, add a regression test first
2. Fix the code to pass the test
3. No design changes needed

**Undefined behavior** — the design didn't specify what should happen:
1. Update the design file with a new requirement (R_n_)
2. Add a test for the new requirement
3. Fix the code to pass the new test

**Wrong design** — the design specified the wrong behavior:
1. Update the requirement in the design file
2. Update the test to match the corrected requirement
3. Fix the code to pass the corrected test

**Never fix code without knowing which category the bug is in.** The fix starts at the earliest affected stage, not at the code.

## Context Management

To minimize re-reading across stages:
- The `.design.md` file is the **single source of truth** for a module. It's the compiled, module-specific version of the architecture decisions.
- Tests reference requirement IDs from the design file.
- Implementation reads the design file and tests, not the architecture docs.
- Architecture docs are consulted only when writing new design files.

When starting work on a module, read in this order:
1. The module's `.design.md` (if it exists — tells you what stage you're in)
2. The module's test file (if it exists — tells you what's been specified)
3. The module's implementation (if it exists — tells you what's been built)

If none exist, you're at Stage 1. Read the relevant architecture doc and write the design file.

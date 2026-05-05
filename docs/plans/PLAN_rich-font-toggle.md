# Rich Text Font Toggle

Make the Font command (fix/var/path) work correctly in styled/span-rendered windows. Currently `initStyledMode()` hardcodes `global.tagfont`, so styled windows always render in the variable-width font regardless of the user's font choice. After this change, styled windows respect `w.body.font` and rebuild when the user toggles fonts.

**Base design doc**: `docs/designs/features/rich-font-toggle.md`

**Key design decisions**:
- Font tables are cached per-window in a `map[string]*richFontTable`, keyed by font path
- Lazy build on first use; at most 2–3 entries per window (fix, var, custom)
- No `SetFont()` API on `rich.Frame`, so font changes require teardown + rebuild of `richBody`
- Scroll position (Origin + YOffset) is saved before teardown and restored after rebuild

**Files touched**:
- `wind.go` — new type, new fields, new methods, update `initStyledMode()` and `Close()`
- `exec.go` — update `fontx()` to handle styled mode
- `wind_styled_test.go` — new tests

---

## Phase 1: Foundation — Font Table Type and Caching

This phase introduces the `richFontTable` type and the per-window cache with lazy build. These are pure additions with no behavioral changes to existing code, so they can be built and tested in isolation.

### 1.1 richFontTable Type and Builder

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill richFontTable type and buildRichFontTable from base doc | `docs/designs/features/rich-font-toggle.md` | Output: `docs/designs/features/rich-font-toggle.md` (already self-contained) |
| [x] Tests | Write tests for `buildRichFontTable` and `getOrBuildFontTable` | `docs/designs/features/rich-font-toggle.md` | In `wind_styled_test.go`. Test: (1) build returns non-nil table with base font set, (2) bold/italic/boldItalic populated when variants exist (GoRegular, GoMono families), (3) returns nil when fontget fails, (4) `getOrBuildFontTable` returns same pointer on second call (cache hit), (5) `getOrBuildFontTable` lazily initializes the map. Use `edwoodtest.NewDisplay` + `makeStyledWindow` pattern. |
| [x] Iterate | Red/green/review: add `richFontTable` struct, `fontTables` field to `Window`, implement `buildRichFontTable()` and `getOrBuildFontTable()` | `docs/designs/features/rich-font-toggle.md` | Add to `wind.go`. Type: `richFontTable` with fields `basePath string`, `base draw.Font`, `bold draw.Font`, `italic draw.Font`, `boldItalic draw.Font`. Method `buildRichFontTable(display draw.Display, fontPath string) *richFontTable` calls `fontget` + `tryLoadFontVariant`. Method `(w *Window) getOrBuildFontTable(fontPath string) *richFontTable` does lazy-init of `w.fontTables` map and caches results. Add `fontTables map[string]*richFontTable` field to Window struct near the `spanStore`/`styledMode` fields (line ~94). |
| [x] Commit | Commit font table type and caching | — | Message: `Add richFontTable type and per-window font table cache` |

### 1.2 Cache Cleanup on Window Close

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Tests | Write test: after building font tables, `Close()` nils the map | `docs/designs/features/rich-font-toggle.md` | In `wind_styled_test.go`. Create window, call `getOrBuildFontTable`, verify non-nil, then call `Close()`, verify `w.fontTables == nil`. |
| [x] Iterate | Red/green/review: add `w.fontTables = nil` to `Window.Close()` | `docs/designs/features/rich-font-toggle.md` | In `wind.go` `Close()` method (line ~430), add `w.fontTables = nil` alongside existing `w.richBody = nil`. |
| [x] Commit | Commit cache cleanup | — | Message: `Clear font table cache on window close` |

---

## Phase 2: Wire initStyledMode to Use Body Font

This phase changes `initStyledMode()` to read `w.body.font` instead of `global.tagfont`. This is the core behavioral change — styled windows now respect the user's current font choice.

### 2.1 initStyledMode Uses w.body.font

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Tests | Write tests for `initStyledMode` using body font | `docs/designs/features/rich-font-toggle.md` | In `wind_styled_test.go`. Tests: (1) Set `w.body.font` to a fixed-width font path, call `initStyledMode()`, verify `richBody` was initialized (styledMode == true, richBody != nil). (2) Set `w.body.font` to var font, call `initStyledMode()`, verify same. (3) Verify that `getOrBuildFontTable` was called (font table cache has entry for `w.body.font`). The existing `TestInitStyledMode_SetsFlag` should continue to pass (it doesn't check which font was used). |
| [x] Iterate | Red/green/review: update `initStyledMode()` to use `w.body.font` via `getOrBuildFontTable` | `docs/designs/features/rich-font-toggle.md` | In `wind.go` `initStyledMode()` (line 2349). Replace the 4 lines that use `global.tagfont` with: `fontPath := w.body.font; if fontPath == "" { fontPath = global.tagfont }; ft := w.getOrBuildFontTable(fontPath)`. Then use `ft.base`, `ft.bold`, `ft.italic`, `ft.boldItalic` for the RichText options. Keep the fallback to `global.tagfont` when `w.body.font` is empty (e.g. during initial window setup before font is assigned). |
| [x] Commit | Commit initStyledMode body font support | — | Message: `Use window body font instead of global.tagfont in initStyledMode` |

---

## Phase 3: Font Toggle Rebuilds Styled Mode

This phase adds `rebuildStyledFont()` and wires the `fontx` command to call it when the window is in styled mode. This completes the feature.

### 3.1 rebuildStyledFont Method

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Tests | Write tests for `rebuildStyledFont` | `docs/designs/features/rich-font-toggle.md` | In `wind_styled_test.go`. Tests: (1) Window in styled mode, call `rebuildStyledFont()`, verify styledMode still true and richBody non-nil (rebuilt). (2) Window NOT in styled mode, call `rebuildStyledFont()`, verify no-op (no panic, styledMode still false). (3) Window in styled mode with richBody nil (edge case), verify no-op. (4) **Scroll preservation**: init styled mode, set content with spans, set origin to non-zero value, set YOffset to non-zero, call `rebuildStyledFont()`, verify origin and YOffset are restored. |
| [x] Iterate | Red/green/review: implement `rebuildStyledFont()` | `docs/designs/features/rich-font-toggle.md` | In `wind.go`. New method `(w *Window) rebuildStyledFont()`. Guard: `if !w.styledMode \|\| w.richBody == nil { return }`. Save `origin := w.richBody.Origin()` and `yOffset := w.richBody.GetOriginYOffset()`. Teardown: set `w.styledMode = false; w.richBody = nil`. Rebuild: call `w.initStyledMode()`. Restore: if rebuild succeeded (`w.styledMode && w.richBody != nil`), call `w.richBody.SetContent(w.buildStyledContent())`, `w.richBody.SetOrigin(origin)`, `w.richBody.SetOriginYOffset(yOffset)`, `w.richBody.Render(w.body.all)`, flush display if non-nil. |
| [x] Commit | Commit rebuildStyledFont | — | Message: `Add rebuildStyledFont to teardown/rebuild richBody with new font` |

### 3.2 Wire fontx to rebuildStyledFont

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Tests | Write tests for `fontx` with styled mode | `docs/designs/features/rich-font-toggle.md` | In `wind_styled_test.go` (or `exec_test.go` if fontx tests already live there — check existing patterns). Tests: (1) Window in styled mode with var font; simulate fontx toggle; verify `w.body.font` changed and `w.styledMode` still true (richBody rebuilt). (2) Window in styled mode; call fontx with explicit path; verify font table cache has entry for that path. (3) Window NOT in styled mode; fontx should work as before (plain frame reinit). Note: fontx calls `fontget` which needs a display, and `global.row.display` must be set. Use the `configureGlobals` pattern from existing tests. fontx also calls `t.w.col.Grow` — may need a mock Column or nil-guard. |
| [x] Iterate | Red/green/review: update `fontx()` to call `rebuildStyledFont()` | `docs/designs/features/rich-font-toggle.md` | In `exec.go` `fontx()` (line 874). After `t.font = file`, add: `if t.w.styledMode { t.w.rebuildStyledFont() } else { /* existing frame.Init + dir handling */ }`. The `t.w.col.Grow(t.w, -1)` call at the end should remain outside the if/else since it applies in both modes. Keep the `global.row.display.ScreenImage().Draw(...)` call before the if/else — it clears the window background regardless of mode. |
| [x] Commit | Commit fontx styled mode support | — | Message: `Wire Font command to rebuild styled mode with new font` |

---

## Phase 4: Edge Case Hardening

### 4.1 Font Toggle Before Styled Mode

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Tests | Write test: toggle font to fixed, then trigger first span write | `docs/designs/features/rich-font-toggle.md` | In `wind_styled_test.go`. Set `w.body.font` to fixed font path. Do NOT enter styled mode. Then call `initStyledMode()`. Verify that the font table was built for the fixed font path (check `w.fontTables` has entry keyed by fixed font path). This validates the "font toggle before styled mode" edge case. |
| [x] Iterate | Verify test passes with existing implementation | `docs/designs/features/rich-font-toggle.md` | This should already pass if Phase 2 was implemented correctly. If not, investigate and fix. |
| [x] Commit | Commit edge case test | — | Message: `Add test for font toggle before styled mode entry` |

### 4.2 Zerox (Window Clone) Font Inheritance

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Tests | Write test: cloned window uses parent's body font for styled mode | `docs/designs/features/rich-font-toggle.md` | In `wind_styled_test.go`. This is a lightweight test: create a window with `w.body.font` set to fixed, simulate clone setup (set clone's `body.font` from parent's), call `initStyledMode()` on clone, verify font table has entry for fixed font path. Font table cache should NOT be shared between windows. |
| [x] Iterate | Verify test passes | `docs/designs/features/rich-font-toggle.md` | Should pass since `fontTables` is per-window and `initStyledMode` reads `w.body.font`. |
| [x] Commit | Commit clone font test | — | Message: `Add test for Zerox font inheritance in styled mode` |

---

## Open Questions

1. **`w.body.font` empty string**: When a window first opens, `w.body.font` is set from `clone.body.font` or `global.tagfont` (wind.go:186-190). It should never be empty by the time `initStyledMode()` is called. But should `getOrBuildFontTable` have a defensive fallback to `global.tagfont` when given an empty string, or should the guard be in `initStyledMode` only?

2. **Testing fontx end-to-end**: The `fontx` function calls `t.w.col.Grow(t.w, -1)` which requires a non-nil `Column` with a functioning `Grow` method. Existing tests may not have this mocked. Should we (a) add a mock Column, (b) add a nil guard in fontx, or (c) test `rebuildStyledFont` in isolation and skip the fontx integration test? The design doc doesn't address this. A nil guard in fontx (checking `t.w.col != nil`) seems simplest and safest.

3. **Display.Flush in rebuildStyledFont**: The design calls for `w.display.Flush()` at the end. In tests, `w.display` is sometimes set to nil to skip redraw. Should `rebuildStyledFont` guard `Flush` behind a nil check (already shown in the design), or should all test helpers ensure a display is present?

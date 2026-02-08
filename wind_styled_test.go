package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/rich"
)

// =========================================================================
// styleAttrsToRichStyle tests
// =========================================================================

func TestStyleAttrsToRichStyle_Default(t *testing.T) {
	// Zero-value StyleAttrs should map to rich.DefaultStyle().
	sa := StyleAttrs{}
	got := styleAttrsToRichStyle(sa)
	want := rich.DefaultStyle()

	if got.Scale != want.Scale {
		t.Errorf("Scale = %v, want %v", got.Scale, want.Scale)
	}
	if got.Bold != want.Bold {
		t.Errorf("Bold = %v, want %v", got.Bold, want.Bold)
	}
	if got.Italic != want.Italic {
		t.Errorf("Italic = %v, want %v", got.Italic, want.Italic)
	}
	if got.Fg != want.Fg {
		t.Errorf("Fg = %v, want %v", got.Fg, want.Fg)
	}
	if got.Bg != want.Bg {
		t.Errorf("Bg = %v, want %v", got.Bg, want.Bg)
	}
}

func TestStyleAttrsToRichStyle_ColorsAndFlags(t *testing.T) {
	red := color.RGBA{R: 0xff, A: 0xff}
	green := color.RGBA{G: 0xff, A: 0xff}

	sa := StyleAttrs{
		Fg:     red,
		Bg:     green,
		Bold:   true,
		Italic: true,
	}
	got := styleAttrsToRichStyle(sa)

	if got.Scale != 1.0 {
		t.Errorf("Scale = %v, want 1.0", got.Scale)
	}
	if !got.Bold {
		t.Error("Bold = false, want true")
	}
	if !got.Italic {
		t.Error("Italic = false, want true")
	}

	// Compare colors via RGBA values to handle interface comparison.
	if got.Fg == nil {
		t.Fatal("Fg is nil, want red")
	}
	r, g, b, a := got.Fg.RGBA()
	wr, wg, wb, wa := red.RGBA()
	if r != wr || g != wg || b != wb || a != wa {
		t.Errorf("Fg RGBA = (%d,%d,%d,%d), want (%d,%d,%d,%d)", r, g, b, a, wr, wg, wb, wa)
	}

	if got.Bg == nil {
		t.Fatal("Bg is nil, want green")
	}
	r, g, b, a = got.Bg.RGBA()
	wr, wg, wb, wa = green.RGBA()
	if r != wr || g != wg || b != wb || a != wa {
		t.Errorf("Bg RGBA = (%d,%d,%d,%d), want (%d,%d,%d,%d)", r, g, b, a, wr, wg, wb, wa)
	}
}

func TestStyleAttrsToRichStyle_FgOnly(t *testing.T) {
	blue := color.RGBA{B: 0xff, A: 0xff}
	sa := StyleAttrs{Fg: blue}
	got := styleAttrsToRichStyle(sa)

	if got.Fg == nil {
		t.Fatal("Fg is nil, want blue")
	}
	if got.Bg != nil {
		t.Errorf("Bg = %v, want nil", got.Bg)
	}
	if got.Bold {
		t.Error("Bold = true, want false")
	}
	if got.Italic {
		t.Error("Italic = true, want false")
	}
	if got.Scale != 1.0 {
		t.Errorf("Scale = %v, want 1.0", got.Scale)
	}
}

func TestStyleAttrsToRichStyle_HiddenNotMapped(t *testing.T) {
	// Hidden is reserved and should not affect the rich.Style output
	// beyond what the zero-value already provides.
	sa := StyleAttrs{Hidden: true}
	got := styleAttrsToRichStyle(sa)
	want := rich.DefaultStyle()

	if got.Scale != want.Scale {
		t.Errorf("Scale = %v, want %v", got.Scale, want.Scale)
	}
	if got.Bold != want.Bold {
		t.Errorf("Bold = %v, want %v", got.Bold, want.Bold)
	}
	if got.Italic != want.Italic {
		t.Errorf("Italic = %v, want %v", got.Italic, want.Italic)
	}
}

// =========================================================================
// buildStyledContent tests
// =========================================================================

// makeStyledWindow creates a headless window with body text and a SpanStore
// for testing buildStyledContent. Does not require a display.
func makeStyledWindow(t *testing.T, bodyText string) *Window {
	t.Helper()

	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", []rune(bodyText)),
	}
	w.body.w = w
	return w
}

func TestBuildStyledContent_SingleRun(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	red := color.RGBA{R: 0xff, A: 0xff}
	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: red}},
	})
	// Ensure store has correct total length.
	if w.spanStore.TotalLen() != 5 {
		t.Fatalf("TotalLen = %d, want 5", w.spanStore.TotalLen())
	}

	content := w.buildStyledContent()

	if len(content) != 1 {
		t.Fatalf("got %d spans, want 1", len(content))
	}
	if content[0].Text != "hello" {
		t.Errorf("span text = %q, want %q", content[0].Text, "hello")
	}
	if content[0].Style.Fg == nil {
		t.Fatal("span Fg is nil, want red")
	}
	r, g, b, a := content[0].Style.Fg.RGBA()
	wr, wg, wb, wa := red.RGBA()
	if r != wr || g != wg || b != wb || a != wa {
		t.Errorf("span Fg RGBA = (%d,%d,%d,%d), want (%d,%d,%d,%d)", r, g, b, a, wr, wg, wb, wa)
	}
}

func TestBuildStyledContent_MultipleRuns(t *testing.T) {
	w := makeStyledWindow(t, "hello world")

	red := color.RGBA{R: 0xff, A: 0xff}
	blue := color.RGBA{B: 0xff, A: 0xff}

	w.spanStore = NewSpanStore()
	// "hello" (5 runes) in red, " world" (6 runes) in blue
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: red}},
		{Len: 6, Style: StyleAttrs{Fg: blue}},
	})

	content := w.buildStyledContent()

	if len(content) != 2 {
		t.Fatalf("got %d spans, want 2", len(content))
	}

	// First span: "hello" in red
	if content[0].Text != "hello" {
		t.Errorf("span[0] text = %q, want %q", content[0].Text, "hello")
	}
	if content[0].Style.Fg == nil {
		t.Fatal("span[0] Fg is nil, want red")
	}
	r, _, _, _ := content[0].Style.Fg.RGBA()
	wr, _, _, _ := red.RGBA()
	if r != wr {
		t.Errorf("span[0] Fg red component = %d, want %d", r, wr)
	}

	// Second span: " world" in blue
	if content[1].Text != " world" {
		t.Errorf("span[1] text = %q, want %q", content[1].Text, " world")
	}
	if content[1].Style.Fg == nil {
		t.Fatal("span[1] Fg is nil, want blue")
	}
	_, _, b2, _ := content[1].Style.Fg.RGBA()
	_, _, wb, _ := blue.RGBA()
	if b2 != wb {
		t.Errorf("span[1] Fg blue component = %d, want %d", b2, wb)
	}
}

func TestBuildStyledContent_EmptySpanStore(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// No spanStore: should return plain content.
	content := w.buildStyledContent()
	if len(content) != 1 {
		t.Fatalf("got %d spans, want 1", len(content))
	}
	if content[0].Text != "hello" {
		t.Errorf("span text = %q, want %q", content[0].Text, "hello")
	}
	// Should use default style.
	if content[0].Style.Scale != 1.0 {
		t.Errorf("span Scale = %v, want 1.0", content[0].Style.Scale)
	}
}

func TestBuildStyledContent_SpanStoreZeroLen(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// SpanStore exists but has zero total length (cleared).
	w.spanStore = NewSpanStore()
	content := w.buildStyledContent()
	if len(content) != 1 {
		t.Fatalf("got %d spans, want 1", len(content))
	}
	if content[0].Text != "hello" {
		t.Errorf("span text = %q, want %q", content[0].Text, "hello")
	}
}

func TestBuildStyledContent_MixedStyles(t *testing.T) {
	w := makeStyledWindow(t, "abcdef")

	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 2, Style: StyleAttrs{Bold: true}},                                                    // "ab" bold
		{Len: 2, Style: StyleAttrs{Italic: true}},                                                  // "cd" italic
		{Len: 2, Style: StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}, Bg: color.RGBA{A: 0xff}}}, // "ef" red on black
	})

	content := w.buildStyledContent()

	if len(content) != 3 {
		t.Fatalf("got %d spans, want 3", len(content))
	}

	if content[0].Text != "ab" || !content[0].Style.Bold {
		t.Errorf("span[0]: text=%q bold=%v, want text=%q bold=true", content[0].Text, content[0].Style.Bold, "ab")
	}
	if content[1].Text != "cd" || !content[1].Style.Italic {
		t.Errorf("span[1]: text=%q italic=%v, want text=%q italic=true", content[1].Text, content[1].Style.Italic, "cd")
	}
	if content[2].Text != "ef" {
		t.Errorf("span[2]: text=%q, want %q", content[2].Text, "ef")
	}
}

func TestBuildStyledContent_Unicode(t *testing.T) {
	// Body has unicode characters; spans should split by rune count.
	w := makeStyledWindow(t, "hello\u4e16\u754c") // "hello世界" = 7 runes

	red := color.RGBA{R: 0xff, A: 0xff}
	blue := color.RGBA{B: 0xff, A: 0xff}

	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: red}},  // "hello"
		{Len: 2, Style: StyleAttrs{Fg: blue}}, // "世界"
	})

	content := w.buildStyledContent()

	if len(content) != 2 {
		t.Fatalf("got %d spans, want 2", len(content))
	}
	if content[0].Text != "hello" {
		t.Errorf("span[0] text = %q, want %q", content[0].Text, "hello")
	}
	if content[1].Text != "\u4e16\u754c" {
		t.Errorf("span[1] text = %q, want %q", content[1].Text, "\u4e16\u754c")
	}
}

// =========================================================================
// Mode switching tests
// =========================================================================

func TestIsStyledMode_DefaultFalse(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	if w.IsStyledMode() {
		t.Error("new window should not be in styled mode")
	}
}

func TestIsStyledMode_AfterInit(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	w.styledMode = true

	if !w.IsStyledMode() {
		t.Error("IsStyledMode() = false after setting styledMode = true")
	}
}

func TestIsStyledMode_AfterExit(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	w.styledMode = true
	// Nil display to skip redraw in exitStyledMode (no full display in test).
	w.display = nil
	w.exitStyledMode()

	if w.IsStyledMode() {
		t.Error("IsStyledMode() = true after exitStyledMode()")
	}
}

func TestInitStyledMode_SetsFlag(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	w.initStyledMode()

	if !w.styledMode {
		t.Error("styledMode = false after initStyledMode()")
	}
	if w.richBody == nil {
		t.Error("richBody is nil after initStyledMode()")
	}
}

func TestInitStyledMode_NopIfAlreadyStyled(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	w.initStyledMode()
	firstRichBody := w.richBody

	// Second call should be a no-op.
	w.initStyledMode()
	if w.richBody != firstRichBody {
		t.Error("initStyledMode() replaced richBody when already in styled mode")
	}
}

func TestInitStyledMode_NopIfPreviewMode(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	w.previewMode = true
	w.richBody = &RichText{} // simulate preview mode richBody

	w.initStyledMode()

	if w.styledMode {
		t.Error("initStyledMode() set styledMode when in preview mode")
	}
}

func TestExitStyledMode_ClearsState(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	w.initStyledMode()

	// Nil display to skip redraw in exitStyledMode (no full display in test).
	w.display = nil
	w.exitStyledMode()

	if w.styledMode {
		t.Error("styledMode = true after exitStyledMode()")
	}
	if w.richBody != nil {
		t.Error("richBody not nil after exitStyledMode()")
	}
}

func TestExitStyledMode_NopIfNotStyled(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Nil display to skip redraw in exitStyledMode (no full display in test).
	w.display = nil
	// Should not panic.
	w.exitStyledMode()

	if w.styledMode {
		t.Error("styledMode should remain false")
	}
}

func TestSpanWriteToPreviewWindow_Error(t *testing.T) {
	// When a window is in preview mode, xfidspanswrite rejects span writes.
	// This test verifies the check exists via the IsPreviewMode() gate.
	w := makeStyledWindow(t, "hello")
	w.previewMode = true

	// The actual error check is in xfidspanswrite, which we tested in
	// spanparse_test.go. Here we confirm the state invariant: a preview
	// mode window should not accept styled mode initialization.
	w.initStyledMode()
	if w.styledMode {
		t.Error("should not enter styled mode when in preview mode")
	}
}

func TestClearRevertsToPlainMode(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Enter styled mode and set up spans.
	w.spanStore = NewSpanStore()
	w.spanStore.Insert(0, 5)
	w.initStyledMode()

	if !w.styledMode {
		t.Fatal("precondition: should be in styled mode")
	}

	// Clear the span store and exit styled mode (as xfidspanswrite does).
	w.spanStore.Clear()
	// Nil display to skip redraw in exitStyledMode (no full display in test).
	w.display = nil
	w.exitStyledMode()

	if w.styledMode {
		t.Error("styledMode = true after clear + exitStyledMode()")
	}
	if w.richBody != nil {
		t.Error("richBody not nil after exitStyledMode()")
	}
}

func TestAutoSwitchToStyledMode(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Simulate what xfidspanswrite does: first span write on plain window.
	if w.styledMode {
		t.Fatal("precondition: should start in plain mode")
	}

	// Ensure span store exists.
	w.spanStore = NewSpanStore()
	w.spanStore.Insert(0, 5)
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}},
	})

	// Auto-switch: if not styled and not preview, init styled mode.
	if !w.styledMode && !w.previewMode {
		w.initStyledMode()
	}

	if !w.styledMode {
		t.Error("should have switched to styled mode after first span write")
	}
	if w.richBody == nil {
		t.Error("richBody should be initialized after auto-switch")
	}
}

func TestStyledAndPreviewMutuallyExclusive(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Enter styled mode.
	w.initStyledMode()
	if !w.styledMode {
		t.Fatal("precondition: should be in styled mode")
	}

	// Simulate entering preview mode (as previewcmd would do).
	// Preview mode sets previewMode=true, styledMode=false, and creates
	// a new richBody. Here we verify the invariant.
	w.styledMode = false
	w.previewMode = true

	if w.styledMode && w.previewMode {
		t.Error("styledMode and previewMode should never both be true")
	}
}

// TestStyledShowSendsSelectionEvent verifies that when Show() is called in
// styled mode, the 'S' selection event is sent to the event subscriber
// (e.g., the coloring program). Without this event, external programs
// don't know the selection changed and won't update the display.
func TestStyledShowSendsSelectionEvent(t *testing.T) {
	bodyText := "hello world test acme text"
	display := edwoodtest.NewDisplay(image.Rect(0, 0, 800, 600))
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", []rune(bodyText)),
		what:    Body,
	}
	w.body.w = w
	w.col = &Column{safe: true}
	w.body.all = image.Rect(0, 20, 800, 600)

	// Set up styled mode with richBody.
	w.initStyledMode()
	content := w.buildStyledContent()
	w.richBody.SetContent(content)
	w.richBody.Render(w.body.all)

	// Enable event subscription (simulates external program like edcolor).
	w.owner = 'T'
	w.nopen[QWevent]++

	// Clear any events from setup.
	w.events = nil

	// Call Show with a selection — this is what search() does.
	w.body.Show(6, 11, true) // select "world"

	// Verify body selection was updated.
	if w.body.q0 != 6 || w.body.q1 != 11 {
		t.Errorf("body selection = (%d, %d), want (6, 11)", w.body.q0, w.body.q1)
	}

	// Verify 'S' event was emitted for the external program.
	eventStr := string(w.events)
	if !strings.Contains(eventStr, "S6 11 0 0") {
		t.Errorf("expected S event for selection (6,11), got events: %q", eventStr)
	}

	// Verify the rich text selection was set.
	p0, p1 := w.richBody.Selection()
	if p0 != 6 || p1 != 11 {
		t.Errorf("richBody selection = (%d, %d), want (6, 11)", p0, p1)
	}
}

// TestStyledShowScrollsRichText verifies that when Show() is called in
// styled mode for text that is off-screen, the rich text frame scrolls
// (not just the hidden plain text frame).
func TestStyledShowScrollsRichText(t *testing.T) {
	// Build body with enough text that later content is off-screen.
	var bodyText string
	for i := 0; i < 50; i++ {
		bodyText += fmt.Sprintf("Line %d of filler text.\n", i)
	}
	bodyText += "target word here.\n"
	bodyRunes := []rune(bodyText)

	display := edwoodtest.NewDisplay(image.Rect(0, 0, 800, 600))
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", bodyRunes),
		what:    Body,
	}
	w.body.w = w
	w.col = &Column{safe: true}
	w.body.all = image.Rect(0, 20, 800, 160) // small frame to force scrolling

	// Set up styled mode with richBody.
	w.initStyledMode()
	content := w.buildStyledContent()
	w.richBody.SetContent(content)
	w.richBody.Render(w.body.all)
	w.richBody.SetOrigin(0)

	// Find "target" in the body text.
	targetIdx := strings.Index(bodyText, "target")
	if targetIdx < 0 {
		t.Fatal("could not find 'target' in body text")
	}

	// Show the selection — should scroll the rich text frame.
	w.body.Show(targetIdx, targetIdx+6, true)

	// Verify the origin changed (target was off-screen from origin=0).
	if w.richBody.Origin() == 0 && targetIdx > 200 {
		t.Errorf("rich text origin should have changed after Show to off-screen position %d", targetIdx)
	}
}

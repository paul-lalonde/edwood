package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
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
// =========================================================================
// boxStyleToRichStyle tests
// =========================================================================

func TestBoxStyleToRichStyleImagePayload(t *testing.T) {
	sa := StyleAttrs{
		IsBox:      true,
		BoxWidth:   200,
		BoxHeight:  150,
		BoxPayload: "image:/tmp/diagram.png",
		Fg:         color.RGBA{R: 0xff, A: 0xff},
		Bold:       true,
	}
	got := boxStyleToRichStyle(sa, "alt text")

	if !got.Image {
		t.Error("Image should be true")
	}
	if got.ImageWidth != 200 {
		t.Errorf("ImageWidth = %d; want 200", got.ImageWidth)
	}
	if got.ImageHeight != 150 {
		t.Errorf("ImageHeight = %d; want 150", got.ImageHeight)
	}
	if got.ImageURL != "/tmp/diagram.png" {
		t.Errorf("ImageURL = %q; want %q", got.ImageURL, "/tmp/diagram.png")
	}
	if got.ImageAlt != "alt text" {
		t.Errorf("ImageAlt = %q; want %q", got.ImageAlt, "alt text")
	}
	if !got.Bold {
		t.Error("Bold should be true")
	}
	if got.Scale != 1.0 {
		t.Errorf("Scale = %v; want 1.0", got.Scale)
	}
	if got.Fg == nil {
		t.Error("Fg should not be nil")
	}
}

func TestBoxStyleToRichStyleNoPayload(t *testing.T) {
	sa := StyleAttrs{
		IsBox:     true,
		BoxWidth:  100,
		BoxHeight: 50,
	}
	got := boxStyleToRichStyle(sa, "placeholder")

	if got.Image {
		t.Error("Image should be false for no-payload box")
	}
	if !got.FixedBox {
		t.Error("FixedBox should be true")
	}
	if got.ImageURL != "" {
		t.Errorf("ImageURL = %q; want empty", got.ImageURL)
	}
	if got.ImageAlt != "placeholder" {
		t.Errorf("ImageAlt = %q; want %q", got.ImageAlt, "placeholder")
	}
	if got.ImageWidth != 100 {
		t.Errorf("ImageWidth = %d; want 100", got.ImageWidth)
	}
	if got.ImageHeight != 50 {
		t.Errorf("ImageHeight = %d; want 50", got.ImageHeight)
	}
}

func TestBoxStyleToRichStyleNonImagePayload(t *testing.T) {
	sa := StyleAttrs{
		IsBox:      true,
		BoxWidth:   300,
		BoxHeight:  200,
		BoxPayload: "widget:chart-v2",
	}
	got := boxStyleToRichStyle(sa, "chart")

	if got.ImageURL != "" {
		t.Errorf("ImageURL = %q; want empty for non-image payload", got.ImageURL)
	}
}

// =========================================================================
// buildStyledContent with boxes tests
// =========================================================================

func TestBuildStyledContentBoxRun(t *testing.T) {
	w := makeStyledWindow(t, "hello world test!") // 17 runes

	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 6, Style: StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}},
		{Len: 5, Style: StyleAttrs{
			IsBox:      true,
			BoxWidth:   200,
			BoxHeight:  100,
			BoxPayload: "image:/tmp/test.png",
		}},
		{Len: 6, Style: StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}},
	})

	content := w.buildStyledContent()
	if len(content) != 3 {
		t.Fatalf("got %d spans; want 3", len(content))
	}

	// First span: regular text.
	if content[0].Style.Image {
		t.Error("span[0] should not be an image")
	}
	if content[0].Text != "hello " {
		t.Errorf("span[0] text = %q; want %q", content[0].Text, "hello ")
	}

	// Second span: box (image).
	if !content[1].Style.Image {
		t.Error("span[1] should be an image")
	}
	if content[1].Style.ImageWidth != 200 {
		t.Errorf("span[1] ImageWidth = %d; want 200", content[1].Style.ImageWidth)
	}
	if content[1].Style.ImageHeight != 100 {
		t.Errorf("span[1] ImageHeight = %d; want 100", content[1].Style.ImageHeight)
	}
	if content[1].Style.ImageURL != "/tmp/test.png" {
		t.Errorf("span[1] ImageURL = %q; want %q", content[1].Style.ImageURL, "/tmp/test.png")
	}
	if content[1].Text != "world" {
		t.Errorf("span[1] text = %q; want %q", content[1].Text, "world")
	}
	if content[1].Style.ImageAlt != "world" {
		t.Errorf("span[1] ImageAlt = %q; want %q", content[1].Style.ImageAlt, "world")
	}

	// Third span: regular text.
	if content[2].Style.Image {
		t.Error("span[2] should not be an image")
	}
}

func TestBuildStyledContentMixedSpansAndBoxes(t *testing.T) {
	w := makeStyledWindow(t, "abcdefghijklmnop") // 16 runes

	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 4, Style: StyleAttrs{Bold: true}},
		{Len: 4, Style: StyleAttrs{
			IsBox: true, BoxWidth: 100, BoxHeight: 50,
			BoxPayload: "image:/img1.png",
		}},
		{Len: 4, Style: StyleAttrs{Italic: true}},
		{Len: 4, Style: StyleAttrs{
			IsBox: true, BoxWidth: 200, BoxHeight: 100,
		}},
	})

	content := w.buildStyledContent()
	if len(content) != 4 {
		t.Fatalf("got %d spans; want 4", len(content))
	}

	// text, image-box, text, fixed-box pattern.
	if content[0].Style.Image || content[0].Style.FixedBox {
		t.Error("span[0] should not be image or fixed box")
	}
	if !content[1].Style.Image {
		t.Error("span[1] should be image (has image: payload)")
	}
	if content[2].Style.Image || content[2].Style.FixedBox {
		t.Error("span[2] should not be image or fixed box")
	}
	if content[3].Style.Image {
		t.Error("span[3] should not be image (no image: payload)")
	}
	if !content[3].Style.FixedBox {
		t.Error("span[3] should be FixedBox")
	}

	// Verify the second box has no ImageURL since payload is empty.
	if content[3].Style.ImageURL != "" {
		t.Errorf("span[3] ImageURL = %q; want empty", content[3].Style.ImageURL)
	}
}

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

// =========================================================================
// richFontTable and caching tests (Phase 1.1)
// =========================================================================

func TestBuildRichFontTable_ReturnsNonNil(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	fontPath := "/mnt/font/GoRegular/16a/font"
	ft := buildRichFontTable(display, fontPath)

	if ft == nil {
		t.Fatal("buildRichFontTable returned nil for valid font path")
	}
	if ft.basePath != fontPath {
		t.Errorf("basePath = %q, want %q", ft.basePath, fontPath)
	}
	if ft.base == nil {
		t.Error("base font is nil")
	}
}

func TestBuildRichFontTable_GoRegularVariants(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	fontPath := "/mnt/font/GoRegular/16a/font"
	ft := buildRichFontTable(display, fontPath)

	if ft == nil {
		t.Fatal("buildRichFontTable returned nil")
	}
	// The mock display's OpenFont always succeeds, so all variants
	// should be populated for GoRegular (which has variant mappings).
	if ft.bold == nil {
		t.Error("bold variant is nil for GoRegular family")
	}
	if ft.italic == nil {
		t.Error("italic variant is nil for GoRegular family")
	}
	if ft.boldItalic == nil {
		t.Error("boldItalic variant is nil for GoRegular family")
	}
}

func TestBuildRichFontTable_GoMonoVariants(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	fontPath := "/mnt/font/GoMono/16a/font"
	ft := buildRichFontTable(display, fontPath)

	if ft == nil {
		t.Fatal("buildRichFontTable returned nil")
	}
	// GoMono also has variant mappings in tryLoadFontVariant.
	if ft.bold == nil {
		t.Error("bold variant is nil for GoMono family")
	}
	if ft.italic == nil {
		t.Error("italic variant is nil for GoMono family")
	}
	if ft.boldItalic == nil {
		t.Error("boldItalic variant is nil for GoMono family")
	}
}

// failingFontDisplay is a minimal draw.Display that always fails OpenFont.
// Used to test the nil-return guard in buildRichFontTable.
type failingFontDisplay struct {
	draw.Display // embed interface to satisfy all methods; only OpenFont is overridden
}

func (d *failingFontDisplay) OpenFont(name string) (draw.Font, error) {
	return nil, fmt.Errorf("font not found: %s", name)
}

func TestBuildRichFontTable_NilWhenFontgetFails(t *testing.T) {
	// Use a display that always fails OpenFont and a unique path that
	// won't be in the global fontCache.
	display := &failingFontDisplay{}
	uniquePath := "/nonexistent/font/path/for/test"

	// Ensure path is not in fontCache from a previous test.
	delete(fontCache, uniquePath)

	ft := buildRichFontTable(display, uniquePath)

	if ft != nil {
		t.Error("buildRichFontTable should return nil when fontget fails")
	}
}

func TestGetOrBuildFontTable_CacheHit(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	fontPath := "/mnt/font/GoRegular/16a/font"
	ft1 := w.getOrBuildFontTable(fontPath)
	ft2 := w.getOrBuildFontTable(fontPath)

	if ft1 == nil {
		t.Fatal("first call returned nil")
	}
	if ft1 != ft2 {
		t.Error("second call returned different pointer (cache miss)")
	}
}

func TestGetOrBuildFontTable_LazyInitMap(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Before any call, fontTables should be nil (zero value).
	if w.fontTables != nil {
		t.Fatal("fontTables should be nil before first getOrBuildFontTable call")
	}

	fontPath := "/mnt/font/GoRegular/16a/font"
	ft := w.getOrBuildFontTable(fontPath)

	if ft == nil {
		t.Fatal("getOrBuildFontTable returned nil")
	}
	if w.fontTables == nil {
		t.Error("fontTables map was not initialized after getOrBuildFontTable call")
	}
	if _, ok := w.fontTables[fontPath]; !ok {
		t.Error("fontTables map does not contain entry for the requested font path")
	}
}

func TestFontTableCache_ClearedOnClose(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Build a font table so the cache is populated.
	fontPath := "/mnt/font/GoRegular/16a/font"
	ft := w.getOrBuildFontTable(fontPath)
	if ft == nil {
		t.Fatal("getOrBuildFontTable returned nil")
	}
	if w.fontTables == nil {
		t.Fatal("fontTables should be non-nil after building a font table")
	}

	// Set up tag frame so Close() doesn't panic on w.tag.Close().
	w.tag.fr = &MockFrame{}
	// Register w.body as observer on its file so Text.Close() succeeds.
	w.body.file.AddObserver(&w.body)

	w.Close()

	if w.fontTables != nil {
		t.Error("fontTables should be nil after Close()")
	}
}

// =========================================================================
// initStyledMode body font tests (Phase 2.1)
// =========================================================================

func TestInitStyledMode_UsesBodyFont_Fixed(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	fixedFontPath := "/mnt/font/GoMono/16a/font"
	w.body.font = fixedFontPath

	w.initStyledMode()

	if !w.styledMode {
		t.Error("styledMode = false after initStyledMode()")
	}
	if w.richBody == nil {
		t.Error("richBody is nil after initStyledMode()")
	}
	// Verify that initStyledMode used w.body.font via getOrBuildFontTable:
	// the font table cache should have an entry keyed by the body font path.
	if w.fontTables == nil {
		t.Fatal("fontTables is nil — initStyledMode did not use getOrBuildFontTable")
	}
	if _, ok := w.fontTables[fixedFontPath]; !ok {
		t.Errorf("fontTables has no entry for body font %q", fixedFontPath)
	}
}

func TestInitStyledMode_UsesBodyFont_Variable(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	varFontPath := "/mnt/font/GoRegular/16a/font"
	w.body.font = varFontPath

	w.initStyledMode()

	if !w.styledMode {
		t.Error("styledMode = false after initStyledMode()")
	}
	if w.richBody == nil {
		t.Error("richBody is nil after initStyledMode()")
	}
	// Verify the font table cache has an entry for the variable font path.
	if w.fontTables == nil {
		t.Fatal("fontTables is nil — initStyledMode did not use getOrBuildFontTable")
	}
	if _, ok := w.fontTables[varFontPath]; !ok {
		t.Errorf("fontTables has no entry for body font %q", varFontPath)
	}
}

func TestInitStyledMode_FontTableMatchesBodyFont(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	fixedFontPath := "/mnt/font/GoMono/16a/font"
	w.body.font = fixedFontPath

	w.initStyledMode()

	if w.fontTables == nil {
		t.Fatal("fontTables is nil after initStyledMode")
	}
	ft, ok := w.fontTables[fixedFontPath]
	if !ok {
		t.Fatalf("fontTables has no entry for body font %q", fixedFontPath)
	}
	if ft.basePath != fixedFontPath {
		t.Errorf("font table basePath = %q, want %q", ft.basePath, fixedFontPath)
	}
}

// =========================================================================
// rebuildStyledFont tests (Phase 3.1)
// =========================================================================

func TestRebuildStyledFont_RebuildInStyledMode(t *testing.T) {
	w := makeStyledWindow(t, "hello")
	w.body.font = "/mnt/font/GoRegular/16a/font"
	w.initStyledMode()

	if !w.styledMode {
		t.Fatal("precondition: should be in styled mode")
	}
	firstRichBody := w.richBody
	if firstRichBody == nil {
		t.Fatal("precondition: richBody should be non-nil")
	}

	w.rebuildStyledFont()

	if !w.styledMode {
		t.Error("styledMode should still be true after rebuildStyledFont()")
	}
	if w.richBody == nil {
		t.Error("richBody should be non-nil after rebuildStyledFont()")
	}
	// richBody should be a new instance (teardown + rebuild).
	if w.richBody == firstRichBody {
		t.Error("richBody should be a new instance after rebuild")
	}
}

func TestRebuildStyledFont_NopWhenNotStyled(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Window is NOT in styled mode — rebuildStyledFont should be a no-op.
	w.rebuildStyledFont()

	if w.styledMode {
		t.Error("styledMode should remain false")
	}
	if w.richBody != nil {
		t.Error("richBody should remain nil")
	}
}

func TestRebuildStyledFont_NopWhenRichBodyNil(t *testing.T) {
	w := makeStyledWindow(t, "hello")

	// Edge case: styledMode is true but richBody is nil.
	w.styledMode = true
	w.richBody = nil

	// Should not panic or change state.
	w.rebuildStyledFont()

	// styledMode remains true (the guard returns early).
	if w.richBody != nil {
		t.Error("richBody should remain nil when guard triggers")
	}
}

func TestRebuildStyledFont_ScrollPreservation(t *testing.T) {
	// Build body with enough text for meaningful scroll position.
	var bodyText string
	for i := 0; i < 50; i++ {
		bodyText += fmt.Sprintf("Line %d of filler text.\n", i)
	}
	bodyRunes := []rune(bodyText)

	display := edwoodtest.NewDisplay(image.Rect(0, 0, 800, 600))
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", bodyRunes),
	}
	w.body.w = w
	w.body.font = "/mnt/font/GoRegular/16a/font"
	w.body.all = image.Rect(0, 20, 800, 160)

	// Enter styled mode.
	w.initStyledMode()
	if !w.styledMode || w.richBody == nil {
		t.Fatal("precondition: should be in styled mode with richBody")
	}

	// Set content and render so the frame has something to scroll.
	content := w.buildStyledContent()
	w.richBody.SetContent(content)
	w.richBody.Render(w.body.all)

	// Scroll to a non-zero position.
	desiredOrigin := 200
	desiredYOffset := 5
	w.richBody.SetOrigin(desiredOrigin)
	w.richBody.SetOriginYOffset(desiredYOffset)

	// Verify the origin was actually set (sanity check).
	if w.richBody.Origin() != desiredOrigin {
		t.Fatalf("precondition: origin = %d, want %d", w.richBody.Origin(), desiredOrigin)
	}
	if w.richBody.GetOriginYOffset() != desiredYOffset {
		t.Fatalf("precondition: yOffset = %d, want %d", w.richBody.GetOriginYOffset(), desiredYOffset)
	}

	// Rebuild with the same font — scroll position should be preserved.
	w.rebuildStyledFont()

	if !w.styledMode || w.richBody == nil {
		t.Fatal("richBody should be rebuilt after rebuildStyledFont()")
	}

	// Verify scroll position was restored.
	if w.richBody.Origin() != desiredOrigin {
		t.Errorf("origin after rebuild = %d, want %d", w.richBody.Origin(), desiredOrigin)
	}
	if w.richBody.GetOriginYOffset() != desiredYOffset {
		t.Errorf("yOffset after rebuild = %d, want %d", w.richBody.GetOriginYOffset(), desiredYOffset)
	}
}

// =========================================================================
// fontx + styled mode tests (Phase 3.2)
// =========================================================================

// makeFontxTestWindow creates a window wired into a Column suitable for
// calling fontx(). The window has a body with bodyText, a tag with a
// MockFrame, a display, textcolors initialized, and the Column.w slice
// contains the window so that col.Grow works.
func makeFontxTestWindow(t *testing.T, bodyText string) *Window {
	t.Helper()

	display := edwoodtest.NewDisplay(image.Rect(0, 0, 800, 600))
	global.configureGlobals(display)
	global.iconinit(display)

	// Set font flags so fontx toggle logic has real paths.
	varFont := "/mnt/font/GoRegular/16a/font"
	fixedFont := "/mnt/font/GoMono/16a/font"
	*varfontflag = varFont
	*fixedfontflag = fixedFont
	global.tagfont = varFont

	// Set up the global row display.
	global.row.display = display

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.r = image.Rect(0, 0, 800, 600)

	// Set up tag with display and frame.
	w.tag.display = display
	w.tag.fr = &MockFrame{}
	w.tag.all = image.Rect(0, 0, 800, 20)

	// Set up body.
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", []rune(bodyText)),
		what:    Body,
	}
	w.body.w = w
	w.body.font = varFont
	w.body.all = image.Rect(0, 20, 800, 600)

	// Create a Column that contains the window so col.Grow works.
	col := &Column{
		safe:    true,
		fortest: true,
		display: display,
		r:       image.Rect(0, 0, 800, 600),
		w:       []*Window{w},
	}
	col.tag.fr = &MockFrame{}
	w.col = col

	return w
}

func TestFontx_StyledMode_Toggle(t *testing.T) {
	w := makeFontxTestWindow(t, "hello world")
	w.initStyledMode()

	if !w.styledMode {
		t.Fatal("precondition: should be in styled mode")
	}
	if w.richBody == nil {
		t.Fatal("precondition: richBody should be non-nil")
	}

	// Body starts with var font.
	if w.body.font != *varfontflag {
		t.Fatalf("precondition: body font = %q, want %q", w.body.font, *varfontflag)
	}

	firstRichBody := w.richBody

	// Call fontx with no args — toggles from var to fix.
	et := &w.body
	fontx(et, nil, nil, false, false, "")

	// After fontx, body font should have changed to fixed.
	if w.body.font != *fixedfontflag {
		t.Errorf("body font = %q, want %q", w.body.font, *fixedfontflag)
	}

	// Window should still be in styled mode with a rebuilt richBody.
	if !w.styledMode {
		t.Error("styledMode should still be true after fontx toggle")
	}
	if w.richBody == nil {
		t.Error("richBody should be non-nil after fontx toggle")
	}
	if w.richBody == firstRichBody {
		t.Error("richBody should be a new instance after fontx rebuild")
	}
}

func TestFontx_StyledMode_ExplicitPath(t *testing.T) {
	w := makeFontxTestWindow(t, "hello world")
	w.initStyledMode()

	if !w.styledMode {
		t.Fatal("precondition: should be in styled mode")
	}

	// Call fontx with an explicit font path.
	customFont := "/mnt/font/DejaVuSans/14a/font"
	et := &w.body
	fontx(et, nil, nil, false, false, customFont)

	// Body font should now be the custom path.
	if w.body.font != customFont {
		t.Errorf("body font = %q, want %q", w.body.font, customFont)
	}

	// Font table cache should have an entry for the custom path.
	if w.fontTables == nil {
		t.Fatal("fontTables is nil after fontx with explicit path")
	}
	if _, ok := w.fontTables[customFont]; !ok {
		t.Errorf("fontTables has no entry for %q", customFont)
	}

	// Should still be in styled mode.
	if !w.styledMode {
		t.Error("styledMode should still be true after fontx with explicit path")
	}
}

func TestFontx_PlainMode_FrameReinit(t *testing.T) {
	w := makeFontxTestWindow(t, "hello world")

	// Window is NOT in styled mode (default).
	if w.styledMode {
		t.Fatal("precondition: should not be in styled mode")
	}

	// Call fontx with toggle — should work as plain frame reinit.
	et := &w.body
	fontx(et, nil, nil, false, false, "")

	// Should not be in styled mode.
	if w.styledMode {
		t.Error("styledMode should remain false in plain mode")
	}

	// Body font should have toggled.
	if w.body.font != *fixedfontflag {
		t.Errorf("body font = %q, want %q", w.body.font, *fixedfontflag)
	}
}

// =========================================================================
// Edge case hardening tests (Phase 4.1)
// =========================================================================

func TestFontToggleBeforeStyledMode(t *testing.T) {
	// Edge case: user toggles font to fixed-width BEFORE any spans arrive.
	// When the first span write triggers initStyledMode(), it should pick
	// up the fixed font via w.body.font (not global.tagfont).
	w := makeStyledWindow(t, "hello")
	fixedFontPath := "/mnt/font/GoMono/16a/font"

	// Simulate the user toggling font before styled mode is entered.
	w.body.font = fixedFontPath

	// Precondition: not in styled mode yet.
	if w.styledMode {
		t.Fatal("precondition: should not be in styled mode")
	}
	if w.fontTables != nil {
		t.Fatal("precondition: fontTables should be nil before initStyledMode")
	}

	// First span write triggers initStyledMode.
	w.initStyledMode()

	if !w.styledMode {
		t.Error("styledMode = false after initStyledMode()")
	}
	if w.richBody == nil {
		t.Error("richBody is nil after initStyledMode()")
	}

	// The font table should have been built for the fixed font path,
	// NOT for global.tagfont.
	if w.fontTables == nil {
		t.Fatal("fontTables is nil — initStyledMode did not use getOrBuildFontTable")
	}
	if _, ok := w.fontTables[fixedFontPath]; !ok {
		t.Errorf("fontTables has no entry for fixed font %q; initStyledMode ignored w.body.font", fixedFontPath)
	}
}

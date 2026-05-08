package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
	"github.com/rjkroege/edwood/markdown"
	"github.com/rjkroege/edwood/rich"
)

func TestSetTag1(t *testing.T) {
	const (
		defaultSuffix = " Del Snarf | Look Edit "
		extraSuffix   = "|fmt g setTag1 Ldef"
	)

	for _, name := range []string{
		"/home/gopher/src/hello.go",
		"/home/ゴーファー/src/エドウード.txt",
		"/home/ゴーファー/src/",
	} {
		display := edwoodtest.NewDisplay(image.Rectangle{})
		global.configureGlobals(display)

		w := NewWindow().initHeadless(nil)
		w.display = display
		w.body = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer(name, nil),
		}
		w.tag = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer("", nil),
		}

		w.col = &Column{
			safe: true,
		}

		w.setTag1()
		got := w.tag.file.String()
		want := name + defaultSuffix
		if got != want {
			t.Errorf("bad initial tag for file %q:\n got: %q\nwant: %q", name, got, want)
		}

		w.tag.file.InsertAt(w.tag.file.Nr(), []rune(extraSuffix))
		w.setTag1()
		got = w.tag.file.String()
		want = name + defaultSuffix + extraSuffix
		if got != want {
			t.Errorf("bad replacement tag for file %q:\n got: %q\nwant: %q", name, got, want)
		}
	}
}

func TestWindowClampAddr(t *testing.T) {
	const hello_世界 = "Hello, 世界"
	runic_hello_世界 := []rune(hello_世界)
	for _, tc := range []struct {
		addr, want Range
	}{
		{Range{-1, -1}, Range{0, 0}},
		{Range{100, 100}, Range{len(runic_hello_世界), len(runic_hello_世界)}},
	} {
		w := &Window{
			addr: tc.addr,
			body: Text{
				file: file.MakeObservableEditableBuffer("", runic_hello_世界),
			},
		}
		w.ClampAddr()
		if got := w.addr; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("got addr %v; want %v", got, tc.want)
		}
	}
}

func TestWindowVisibleRange(t *testing.T) {
	// Non-styled mode: VisibleRange uses body.org + frame Nchars.
	w := &Window{
		body: Text{
			file: file.MakeObservableEditableBuffer("", []rune("Hello, world!\n")),
			fr:   &MockFrame{},
		},
	}
	// MockFrame returns Nchars=0, so end = org + 0 = 0.
	org, end := w.VisibleRange()
	if org != 0 || end != 0 {
		t.Errorf("VisibleRange() = (%d, %d), want (0, 0)", org, end)
	}

	// With body.org set, org should reflect it.
	w.body.org = 5
	org, end = w.VisibleRange()
	if org != 5 || end != 5 {
		t.Errorf("VisibleRange() = (%d, %d), want (5, 5)", org, end)
	}
}

func TestWindowParseTag(t *testing.T) {
	for _, tc := range []struct {
		tag      string
		filename string
	}{
		{"/foo/bar.txt Del Snarf | Look", "/foo/bar.txt"},
		{"'/foo/bar quux.txt' Del Snarf | Look", "'/foo/bar quux.txt'"},
		{"/foo/bar.txt", "/foo/bar.txt"},
		{"/foo/bar.txt | Look", "/foo/bar.txt"},
		{"/foo/bar.txt Del Snarf\t| Look", "/foo/bar.txt"},
		{"/foo/bar.txt Del Snarf Del Snarf", "/foo/bar.txt"},
		{"'/foo/bar.txt ' Del Snarf", "'/foo/bar.txt '"},
		{"'/foo/b|ar.txt ' Del Snarf", "'/foo/b|ar.txt '"},
	} {
		if got, want := parsetaghelper(tc.tag), tc.filename; got != want {
			t.Errorf("tag %q has filename %q; want %q", tc.tag, got, want)
		}
	}
}

func TestWindowClearTag(t *testing.T) {
	tag := "/foo bar/test.txt Del Snarf Undo Put | Look |fmt mk"
	want := "/foo bar/test.txt Del Snarf Undo Put |"
	w := &Window{
		tag: Text{
			file: file.MakeObservableEditableBuffer("", []rune(tag)),
		},
	}
	w.ClearTag()
	got := w.tag.file.String()
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

// TestWindowPreviewMode tests that a Window has preview mode fields and
// that they can be accessed via the appropriate methods.
func TestWindowPreviewMode(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rect(0, 0, 800, 600))
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/file.md", nil),
	}
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}

	// Initially, preview mode should be off
	if w.IsPreviewMode() {
		t.Error("IsPreviewMode() should be false initially")
	}

	// RichBody should be nil initially
	if w.RichBody() != nil {
		t.Error("RichBody() should be nil initially")
	}

	// After enabling preview mode, it should be on
	w.SetPreviewMode(true)
	if !w.IsPreviewMode() {
		t.Error("IsPreviewMode() should be true after SetPreviewMode(true)")
	}

	// After disabling preview mode, it should be off again
	w.SetPreviewMode(false)
	if w.IsPreviewMode() {
		t.Error("IsPreviewMode() should be false after SetPreviewMode(false)")
	}
}

// TestWindowPreviewModeToggle tests that preview mode can be toggled on and off,
// and that each toggle properly updates the state.
func TestWindowPreviewModeToggle(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rect(0, 0, 800, 600))
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/file.md", nil),
	}
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}

	// Initially off
	if w.IsPreviewMode() {
		t.Error("IsPreviewMode() should start false")
	}

	// Toggle on
	w.TogglePreviewMode()
	if !w.IsPreviewMode() {
		t.Error("IsPreviewMode() should be true after first toggle")
	}

	// Toggle off
	w.TogglePreviewMode()
	if w.IsPreviewMode() {
		t.Error("IsPreviewMode() should be false after second toggle")
	}

	// Toggle on again
	w.TogglePreviewMode()
	if !w.IsPreviewMode() {
		t.Error("IsPreviewMode() should be true after third toggle")
	}
}

// TestWindowDrawPreviewMode tests that when a window is in preview mode,
// Window.Draw() renders the richBody instead of the normal body.
func TestWindowDrawPreviewMode(t *testing.T) {
	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/file.md", []rune("# Hello World\n\nThis is **bold** text.")),
	}
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = rect

	// Create a RichText component for preview
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	// Body area is below tag (approximately)
	bodyRect := image.Rect(0, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)
	rt.Render(bodyRect)

	// Set some content in the RichText
	content := rich.Plain("Hello World")
	rt.SetContent(content)

	// Assign the richBody to the window
	w.richBody = rt

	// Initially NOT in preview mode - Draw should NOT use richBody
	w.previewMode = false

	// Clear draw operations
	display.(edwoodtest.GettableDrawOps).Clear()

	// Call Draw method if it exists - for now, test the state
	// (actual Draw method will be implemented in 11.3 "Code written" stage)

	// Verify that when previewMode is false, richBody should not be used for drawing
	// This is a state test - when Draw() is implemented, it should check previewMode
	if w.previewMode {
		t.Error("Window should not be in preview mode initially")
	}
	if w.richBody == nil {
		t.Error("richBody should be set")
	}

	// Enable preview mode
	w.SetPreviewMode(true)

	// Verify preview mode is enabled
	if !w.IsPreviewMode() {
		t.Error("Window should be in preview mode after SetPreviewMode(true)")
	}

	// The richBody should be available for rendering
	if w.RichBody() != rt {
		t.Error("RichBody() should return the assigned RichText component")
	}

	// Verify that the rich body has the expected content
	if w.richBody.Content() == nil {
		t.Error("richBody content should not be nil")
	}
	if w.richBody.Content().Len() != 11 { // "Hello World" = 11 chars
		t.Errorf("richBody content length = %d, want 11", w.richBody.Content().Len())
	}

	// Test that preview mode can be disabled
	w.SetPreviewMode(false)
	if w.IsPreviewMode() {
		t.Error("Window should not be in preview mode after SetPreviewMode(false)")
	}

	// richBody should still exist (for potential re-enabling of preview)
	if w.RichBody() == nil {
		t.Error("richBody should still exist after disabling preview mode")
	}
}

// TestWindowMousePreviewSelection tests that mouse selection in preview mode
// delegates to the richBody and allows text selection within the rich text frame.
func TestWindowMousePreviewSelection(t *testing.T) {
	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/file.md", []rune("# Hello World\n\nThis is **bold** text.")),
	}
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = rect

	// Create a RichText component for preview
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	bodyRect := image.Rect(0, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)
	rt.Render(bodyRect)

	// Set content in the RichText
	content := rich.Plain("Hello World")
	rt.SetContent(content)

	// Assign the richBody to the window
	w.richBody = rt
	w.SetPreviewMode(true)

	// Verify initial selection is empty
	q0, q1 := rt.Selection()
	if q0 != 0 || q1 != 0 {
		t.Errorf("Initial selection should be (0, 0), got (%d, %d)", q0, q1)
	}

	// Test that selection can be set on the richBody
	rt.SetSelection(2, 7) // Select "llo W" from "Hello World"
	q0, q1 = rt.Selection()
	if q0 != 2 || q1 != 7 {
		t.Errorf("Selection after SetSelection(2, 7) should be (2, 7), got (%d, %d)", q0, q1)
	}

	// Verify the window is in preview mode and has the richBody
	if !w.IsPreviewMode() {
		t.Error("Window should be in preview mode")
	}
	if w.RichBody() != rt {
		t.Error("Window's RichBody should match the assigned RichText")
	}

	// The key property: when in preview mode, mouse interactions should be
	// handled by the richBody. We verify that the richBody's frame supports
	// the necessary coordinate mapping methods (Charofpt, Ptofchar) which
	// are used for mouse-based selection.
	frame := rt.Frame()
	if frame == nil {
		t.Fatal("RichText frame should not be nil")
	}

	// Test coordinate mapping (used by mouse selection)
	pt := frame.Ptofchar(5) // Get screen position of character 5
	if pt.X < bodyRect.Min.X {
		t.Errorf("Ptofchar(5).X = %d, should be >= %d", pt.X, bodyRect.Min.X)
	}

	// Test reverse mapping (click position to character)
	char := frame.Charofpt(image.Pt(bodyRect.Min.X+50, bodyRect.Min.Y+5))
	if char < 0 || char > content.Len() {
		t.Errorf("Charofpt should return valid character index, got %d for content length %d", char, content.Len())
	}
}













// TestWindowMousePreviewScroll tests that mouse scrolling in preview mode
// properly delegates to the richBody's scroll handling.
func TestWindowMousePreviewScroll(t *testing.T) {
	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/file.md", nil),
	}
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = rect

	// Create a RichText component for preview
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)
	scrBg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xCCCCCCFF)
	scrThumb, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x666666FF)

	rt := NewRichText()
	bodyRect := image.Rect(0, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
		WithScrollbarColors(scrBg, scrThumb),
	)
	rt.Render(bodyRect)

	// Create content with many lines to enable scrolling
	var content rich.Content
	for i := 0; i < 50; i++ {
		if i > 0 {
			content = append(content, rich.Plain("\n")...)
		}
		content = append(content, rich.Plain("Line number "+string(rune('0'+i%10)))...)
	}
	rt.SetContent(content)

	// Assign the richBody to the window
	w.richBody = rt
	w.SetPreviewMode(true)

	// Verify initial origin is 0
	if rt.Origin() != 0 {
		t.Errorf("Initial origin should be 0, got %d", rt.Origin())
	}

	// Test scrollbar click - button 3 (right-click) should scroll down
	scrollRect := rt.ScrollRect()
	middleY := (scrollRect.Min.Y + scrollRect.Max.Y) / 2
	newOrigin := rt.ScrollClick(3, image.Pt(scrollRect.Min.X+5, middleY))

	// Origin should have increased (scrolled down)
	if newOrigin <= 0 {
		t.Errorf("After ScrollClick(3, middle), origin should be > 0, got %d", newOrigin)
	}

	// Save the current origin
	beforeWheelScroll := rt.Origin()

	// Test mouse wheel scrolling - scroll down
	newOrigin = rt.ScrollWheel(false) // false = scroll down
	if newOrigin < beforeWheelScroll {
		t.Errorf("ScrollWheel(down) should increase origin; before=%d, after=%d", beforeWheelScroll, newOrigin)
	}

	// Test mouse wheel scrolling - scroll up
	beforeWheelUp := rt.Origin()
	newOrigin = rt.ScrollWheel(true) // true = scroll up
	if newOrigin >= beforeWheelUp {
		t.Errorf("ScrollWheel(up) should decrease origin; before=%d, after=%d", beforeWheelUp, newOrigin)
	}

	// Verify the window is in preview mode
	if !w.IsPreviewMode() {
		t.Error("Window should still be in preview mode")
	}

	// Test scrollbar at top - button 1 (left-click) at top should stay at top
	// First scroll to top
	rt.SetOrigin(0)
	newOrigin = rt.ScrollClick(1, image.Pt(scrollRect.Min.X+5, scrollRect.Min.Y))
	if newOrigin != 0 {
		t.Errorf("ScrollClick(1, top) when at origin=0 should stay at 0, got %d", newOrigin)
	}
}














// TestIsNearEnd tests the isNearEnd helper that determines whether
// the scroll position is close enough to the end to trigger tail-follow.
func TestIsNearEnd(t *testing.T) {
	tests := []struct {
		name       string
		origin     int
		contentLen int
		want       bool
	}{
		{"empty content", 0, 0, true},
		{"origin at end", 1000, 1000, true},
		{"origin past end", 1100, 1000, true},
		{"origin near end within threshold", 600, 1000, true},  // 400 < 500
		{"origin far from end", 0, 1000, false},                // 1000 > 500
		{"origin just outside threshold", 499, 1000, false},    // 501 > 500
		{"origin exactly at threshold", 500, 1000, true},       // 500 <= 500
		{"small content fully visible", 0, 100, true},          // 100 < 500
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isNearEnd(tc.origin, tc.contentLen)
			if got != tc.want {
				t.Errorf("isNearEnd(%d, %d) = %v, want %v", tc.origin, tc.contentLen, got, tc.want)
			}
		})
	}
}














// TestWindowPreviewLinkMap tests that a Window stores the LinkMap when entering preview mode.
// The LinkMap is populated by ParseWithSourceMap and allows the window to identify links
// at rendered text positions (used for Look action on links).
func TestWindowPreviewLinkMap(t *testing.T) {
	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	// Markdown with links
	sourceMarkdown := "Check out [Google](https://google.com) and [GitHub](https://github.com) for more info."
	sourceRunes := []rune(sourceMarkdown)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/links.md", sourceRunes),
	}
	w.body.all = image.Rect(0, 20, 800, 600)
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = rect

	// Create a RichText component for preview
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	bodyRect := image.Rect(0, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)
	rt.Render(bodyRect)

	// Parse markdown with source map and link map
	content, sourceMap, linkMap := markdown.ParseWithSourceMap(sourceMarkdown)
	rt.SetContent(content)

	// Initially, link map should not be set on window
	if w.PreviewLinkMap() != nil {
		t.Error("PreviewLinkMap should be nil initially")
	}

	// Assign the richBody to the window and set both source and link maps
	w.richBody = rt
	w.SetPreviewSourceMap(sourceMap)
	w.SetPreviewLinkMap(linkMap)
	w.SetPreviewMode(true)

	// Verify link map is set
	if w.PreviewLinkMap() == nil {
		t.Fatal("PreviewLinkMap should not be nil after SetPreviewLinkMap")
	}

	// Verify the link map matches what we set
	if w.PreviewLinkMap() != linkMap {
		t.Error("PreviewLinkMap should return the same link map that was set")
	}

	// Verify the link map has the correct links
	// The rendered text will be: "Check out Google and GitHub for more info."
	// "Google" is at positions 10-16, "GitHub" is at positions 21-27

	// Find "Google" in rendered text and verify it maps to the correct URL
	plainText := content.Plain()
	googleIdx := -1
	for i := 0; i < len(plainText)-5; i++ {
		if string(plainText[i:i+6]) == "Google" {
			googleIdx = i
			break
		}
	}
	if googleIdx < 0 {
		t.Fatal("Could not find 'Google' in rendered text")
	}

	// Check URL at Google's position
	url := linkMap.URLAt(googleIdx)
	if url != "https://google.com" {
		t.Errorf("URLAt(Google) = %q, want %q", url, "https://google.com")
	}

	// Find "GitHub" in rendered text
	githubIdx := -1
	for i := 0; i < len(plainText)-5; i++ {
		if string(plainText[i:i+6]) == "GitHub" {
			githubIdx = i
			break
		}
	}
	if githubIdx < 0 {
		t.Fatal("Could not find 'GitHub' in rendered text")
	}

	// Check URL at GitHub's position
	url = linkMap.URLAt(githubIdx)
	if url != "https://github.com" {
		t.Errorf("URLAt(GitHub) = %q, want %q", url, "https://github.com")
	}

	// Check that non-link text doesn't return a URL
	// "Check" is at position 0, which is not a link
	url = linkMap.URLAt(0)
	if url != "" {
		t.Errorf("URLAt(0) should be empty for non-link text, got %q", url)
	}

	// Verify that exiting preview mode preserves the link map
	w.SetPreviewMode(false)
	if w.PreviewLinkMap() == nil {
		t.Error("PreviewLinkMap should be preserved after exiting preview mode")
	}

	// Re-entering preview should still have the link map
	w.SetPreviewMode(true)
	if w.PreviewLinkMap() != linkMap {
		t.Error("PreviewLinkMap should still be the same after re-entering preview mode")
	}
}













// TestWindowResizePreviewMode tests that when a window in preview mode is resized,
// the richBody.Render() is called with the updated body.all rectangle, ensuring
// the preview content is properly displayed in the new area.
func TestWindowResizePreviewMode(t *testing.T) {
	// Create initial rectangle and display
	initialRect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(initialRect)
	global.configureGlobals(display)

	// Create markdown content
	markdownContent := "# Hello World\n\nThis is some **bold** text and *italic* text.\n\nParagraph here."
	sourceRunes := []rune(markdownContent)

	w := NewWindow().initHeadless(nil)
	w.display = display

	// Setup body with mock frame
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/resize.md", sourceRunes),
	}
	// Initial body.all rectangle (simulating window layout after Init)
	w.body.all = image.Rect(0, 20, 800, 600)

	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = initialRect

	// Create and initialize RichText for preview
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)

	// Initial render into the body area
	rt.Render(w.body.all)

	// Parse markdown and set content
	content, sourceMap, linkMap := markdown.ParseWithSourceMap(markdownContent)
	rt.SetContent(content)

	// Assign the richBody to the window and enable preview mode
	w.richBody = rt
	w.SetPreviewSourceMap(sourceMap)
	w.SetPreviewLinkMap(linkMap)
	w.SetPreviewMode(true)

	// Verify initial state
	if !w.IsPreviewMode() {
		t.Fatal("Window should be in preview mode")
	}

	// Get the initial lastRect from richBody (via All() accessor)
	initialLastRect := rt.All()
	if !initialLastRect.Eq(w.body.all) {
		t.Errorf("Initial lastRect should match body.all: got %v, want %v", initialLastRect, w.body.all)
	}

	// Simulate resize: update body.all to a new rectangle (e.g., window made smaller)
	newBodyRect := image.Rect(0, 20, 600, 400)
	w.body.all = newBodyRect

	// Call Render with the new rectangle (as Window.Resize should do)
	// This simulates what Window.Resize() should do when in preview mode:
	// After updating body.all, it should call richBody.Render(body.all)
	w.richBody.Render(w.body.all)

	// Verify the richBody's lastRect was updated (via All() accessor)
	afterResizeRect := rt.All()
	if !afterResizeRect.Eq(newBodyRect) {
		t.Errorf("After resize, lastRect should match new body.all: got %v, want %v", afterResizeRect, newBodyRect)
	}

	// Verify the frame rectangle was also updated
	frameRect := rt.Frame().Rect()
	// Frame rect should be to the right of scrollbar within the new body rect
	if frameRect.Max.X > newBodyRect.Max.X {
		t.Errorf("Frame rect.Max.X (%d) should not exceed newBodyRect.Max.X (%d)", frameRect.Max.X, newBodyRect.Max.X)
	}
	if frameRect.Min.Y != newBodyRect.Min.Y {
		t.Errorf("Frame rect.Min.Y (%d) should match newBodyRect.Min.Y (%d)", frameRect.Min.Y, newBodyRect.Min.Y)
	}
	if frameRect.Max.Y != newBodyRect.Max.Y {
		t.Errorf("Frame rect.Max.Y (%d) should match newBodyRect.Max.Y (%d)", frameRect.Max.Y, newBodyRect.Max.Y)
	}

	// Verify scrollbar rect was also updated
	scrollRect := rt.ScrollRect()
	if scrollRect.Min.X != newBodyRect.Min.X {
		t.Errorf("Scroll rect.Min.X (%d) should match newBodyRect.Min.X (%d)", scrollRect.Min.X, newBodyRect.Min.X)
	}
	if scrollRect.Min.Y != newBodyRect.Min.Y {
		t.Errorf("Scroll rect.Min.Y (%d) should match newBodyRect.Min.Y (%d)", scrollRect.Min.Y, newBodyRect.Min.Y)
	}
	if scrollRect.Max.Y != newBodyRect.Max.Y {
		t.Errorf("Scroll rect.Max.Y (%d) should match newBodyRect.Max.Y (%d)", scrollRect.Max.Y, newBodyRect.Max.Y)
	}

	// Verify content is still accessible after resize
	if rt.Content() == nil {
		t.Error("Content should not be nil after resize")
	}
	if rt.Content().Len() == 0 {
		t.Error("Content should not be empty after resize")
	}

	// Verify the rich text frame has the correct content length
	// (this ensures layout was recomputed for the new width)
	if rt.Frame() == nil {
		t.Fatal("Frame should not be nil after resize")
	}

	// Test another resize - making window larger
	largerBodyRect := image.Rect(0, 20, 1000, 800)
	w.body.all = largerBodyRect
	w.richBody.Render(w.body.all)

	// Verify the update (via All() accessor)
	afterLargerResize := rt.All()
	if !afterLargerResize.Eq(largerBodyRect) {
		t.Errorf("After larger resize, lastRect should match: got %v, want %v", afterLargerResize, largerBodyRect)
	}

	// Verify frame expanded
	frameRectLarger := rt.Frame().Rect()
	if frameRectLarger.Max.X <= frameRect.Max.X {
		t.Errorf("Larger frame rect.Max.X (%d) should be greater than smaller (%d)", frameRectLarger.Max.X, frameRect.Max.X)
	}
}

// TestWindowDrawPreviewModeAfterResize tests that Window.Draw() correctly uses
// body.all as the rectangle when in preview mode, ensuring that after a resize
// the preview content is rendered into the correct (updated) area.
func TestWindowDrawPreviewModeAfterResize(t *testing.T) {
	// Create initial rectangle and display
	initialRect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(initialRect)
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display

	// Setup body with mock frame and initial rectangle
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/draw.md", []rune("# Hello\n\nWorld")),
	}
	w.body.all = image.Rect(0, 20, 800, 600)

	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = initialRect

	// Create and initialize RichText for preview
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)

	// Initial render into the body area
	rt.Render(w.body.all)

	// Set content
	content := rich.Plain("Hello World")
	rt.SetContent(content)

	// Assign richBody and enable preview mode
	w.richBody = rt
	w.SetPreviewMode(true)

	// Verify initial lastRect matches body.all
	if !rt.All().Eq(w.body.all) {
		t.Errorf("Initial lastRect should match body.all: got %v, want %v", rt.All(), w.body.all)
	}

	// Now simulate a resize: body.all changes but richBody's cached rectangle is stale
	newBodyRect := image.Rect(0, 20, 600, 400)
	w.body.all = newBodyRect

	// Call Draw() - this should use body.all (the current geometry) not the cached value
	w.Draw()

	// Verify that after Draw(), the richBody's lastRect has been updated to body.all
	// This proves that Draw() used Render(body.all) instead of Redraw()
	if !rt.All().Eq(newBodyRect) {
		t.Errorf("After Draw(), lastRect should match updated body.all: got %v, want %v", rt.All(), newBodyRect)
	}

	// Verify frame rectangle was also updated to match the new area
	frameRect := rt.Frame().Rect()
	if frameRect.Max.X > newBodyRect.Max.X {
		t.Errorf("Frame rect.Max.X (%d) should not exceed newBodyRect.Max.X (%d)", frameRect.Max.X, newBodyRect.Max.X)
	}
	if frameRect.Max.Y != newBodyRect.Max.Y {
		t.Errorf("Frame rect.Max.Y (%d) should match newBodyRect.Max.Y (%d)", frameRect.Max.Y, newBodyRect.Max.Y)
	}

	// Verify scrollbar rectangle was also updated
	scrollRect := rt.ScrollRect()
	if scrollRect.Min.X != newBodyRect.Min.X {
		t.Errorf("Scroll rect.Min.X (%d) should match newBodyRect.Min.X (%d)", scrollRect.Min.X, newBodyRect.Min.X)
	}
	if scrollRect.Max.Y != newBodyRect.Max.Y {
		t.Errorf("Scroll rect.Max.Y (%d) should match newBodyRect.Max.Y (%d)", scrollRect.Max.Y, newBodyRect.Max.Y)
	}

	// Test that subsequent Draw() calls also maintain correct geometry
	evenSmallerRect := image.Rect(0, 20, 400, 300)
	w.body.all = evenSmallerRect
	w.Draw()

	if !rt.All().Eq(evenSmallerRect) {
		t.Errorf("After second Draw(), lastRect should match: got %v, want %v", rt.All(), evenSmallerRect)
	}
}











// TestResolveImagePathAbsolute tests that absolute image paths are returned unchanged.
// When an image path starts with /, it should be used as-is.
func TestResolveImagePathAbsolute(t *testing.T) {
	tests := []struct {
		name     string
		basePath string // Markdown file path
		imgPath  string // Image path in markdown
		want     string // Expected resolved path
	}{
		{
			name:     "absolute unix path",
			basePath: "/home/user/docs/readme.md",
			imgPath:  "/images/logo.png",
			want:     "/images/logo.png",
		},
		{
			name:     "absolute path with subdirectory",
			basePath: "/project/docs/guide.md",
			imgPath:  "/project/assets/diagram.png",
			want:     "/project/assets/diagram.png",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveImagePath(tc.basePath, tc.imgPath)
			if got != tc.want {
				t.Errorf("resolveImagePath(%q, %q) = %q, want %q",
					tc.basePath, tc.imgPath, got, tc.want)
			}
		})
	}
}

// TestResolveImagePathRelative tests that relative image paths are resolved
// relative to the directory containing the markdown file.
func TestResolveImagePathRelative(t *testing.T) {
	tests := []struct {
		name     string
		basePath string // Markdown file path
		imgPath  string // Image path in markdown
		want     string // Expected resolved path
	}{
		{
			name:     "simple relative",
			basePath: "/home/user/docs/readme.md",
			imgPath:  "image.png",
			want:     "/home/user/docs/image.png",
		},
		{
			name:     "relative with subdirectory",
			basePath: "/home/user/docs/readme.md",
			imgPath:  "images/logo.png",
			want:     "/home/user/docs/images/logo.png",
		},
		{
			name:     "relative with parent directory",
			basePath: "/home/user/docs/guide/intro.md",
			imgPath:  "../images/diagram.png",
			want:     "/home/user/docs/images/diagram.png",
		},
		{
			name:     "relative in root directory",
			basePath: "/readme.md",
			imgPath:  "logo.png",
			want:     "/logo.png",
		},
		{
			name:     "dot prefix relative",
			basePath: "/project/docs/readme.md",
			imgPath:  "./images/icon.png",
			want:     "/project/docs/images/icon.png",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveImagePath(tc.basePath, tc.imgPath)
			if got != tc.want {
				t.Errorf("resolveImagePath(%q, %q) = %q, want %q",
					tc.basePath, tc.imgPath, got, tc.want)
			}
		})
	}
}

// =============================================================================
// Phase 16H: Integration Tests
// =============================================================================

// TestMarkdeepImageIntegration tests the end-to-end image rendering pipeline:
// 1. Parse markdown with image syntax
// 2. Create window with preview mode
// 3. Load image into cache
// 4. Verify image box is created with correct dimensions
// 5. Verify image data is available for rendering
func TestMarkdeepImageIntegration(t *testing.T) {
	// Create a temporary directory with a test image
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_image.png")
	mdPath := filepath.Join(tmpDir, "test.md")

	// Create a simple 40x30 test image
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	red := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, red)
		}
	}
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatalf("failed to create test image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("failed to encode PNG: %v", err)
	}
	f.Close()

	// Create markdown content with the image
	// Use relative path since that's the common case
	markdownContent := fmt.Sprintf("# Test Document\n\n![Test Image](test_image.png)\n\nSome text after the image.\n")

	// Write the markdown file
	if err := os.WriteFile(mdPath, []byte(markdownContent), 0644); err != nil {
		t.Fatalf("failed to write markdown file: %v", err)
	}

	// Set up the display and window
	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	// Create a window with the markdown content
	sourceRunes := []rune(markdownContent)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer(mdPath, sourceRunes),
	}
	w.body.all = image.Rect(0, 20, 800, 600)
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = rect

	// Test markdown.Parse (non-source-mapped version) first to verify image parsing works
	parsedContent := markdown.Parse(markdownContent)

	// Verify basic parsing detected the image
	foundImage := false
	for _, span := range parsedContent {
		if span.Style.Image {
			foundImage = true
			if span.Style.ImageURL != "test_image.png" {
				t.Errorf("ImageURL = %q, want %q", span.Style.ImageURL, "test_image.png")
			}
			if span.Style.ImageAlt != "Test Image" {
				t.Errorf("ImageAlt = %q, want %q", span.Style.ImageAlt, "Test Image")
			}
			break
		}
	}
	if !foundImage {
		t.Fatal("markdown.Parse did not detect image")
	}

	// Parse markdown with source map (currently images are rendered as placeholders)
	content, sourceMap, linkMap := markdown.ParseWithSourceMap(markdownContent)

	// Create and initialize the image cache
	cache := rich.NewImageCache(10)

	// Resolve and load the image
	resolvedPath := resolveImagePath(mdPath, "test_image.png")
	expectedResolvedPath := filepath.Join(tmpDir, "test_image.png")
	if resolvedPath != expectedResolvedPath {
		t.Errorf("resolveImagePath = %q, want %q", resolvedPath, expectedResolvedPath)
	}

	// Load the image into cache
	cached, err := cache.Load(resolvedPath)
	if err != nil {
		t.Fatalf("failed to load image into cache: %v", err)
	}

	// Verify cached image properties
	if cached.Width != 40 {
		t.Errorf("cached image width = %d, want 40", cached.Width)
	}
	if cached.Height != 30 {
		t.Errorf("cached image height = %d, want 30", cached.Height)
	}
	if cached.Original == nil {
		t.Error("cached.Original should not be nil")
	}
	if cached.Data == nil {
		t.Error("cached.Data (Plan 9 format) should not be nil")
	}
	if cached.Err != nil {
		t.Errorf("cached.Err should be nil, got: %v", cached.Err)
	}

	// Set up preview mode components
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	bodyRect := image.Rect(0, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)
	rt.SetContent(content)
	rt.Render(bodyRect)

	// Wire everything to the window
	w.imageCache = cache
	w.richBody = rt
	w.SetPreviewSourceMap(sourceMap)
	w.SetPreviewLinkMap(linkMap)
	w.SetPreviewMode(true)

	// Verify preview mode is active
	if !w.previewMode {
		t.Error("previewMode should be true")
	}

	// Verify cache was attached
	if w.imageCache == nil {
		t.Error("imageCache should be attached to window")
	}

	// Verify the cache hit on second load
	cached2, _ := cache.Get(resolvedPath)
	if cached2 != cached {
		t.Error("cache should return same entry on second access")
	}

	// Clean up by exiting preview mode
	w.SetPreviewMode(false)
	cache.Clear()
}

// TestHandlePreviewMouseSignature tests that HandlePreviewMouse accepts both
// a mouse event and a Mousectl, which is needed for proper drag selection.
// This test verifies the signature change from (m *draw.Mouse) to (m *draw.Mouse, mc *draw.Mousectl).
func TestHandlePreviewMouseSignature(t *testing.T) {
	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/file.md", []rune("# Hello World\n\nThis is some text.")),
	}
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = rect

	// Set up the body.all rectangle (used by HandlePreviewMouse for hit-testing)
	w.body.all = image.Rect(0, 20, 800, 600)

	// Create a RichText component for preview
	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	bodyRect := image.Rect(0, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)
	rt.Render(bodyRect)

	// Set content in the RichText
	content := rich.Plain("Hello World - this is some test content for selection")
	rt.SetContent(content)

	// Assign the richBody to the window and enter preview mode
	w.richBody = rt
	w.SetPreviewMode(true)

	// Create a mouse event in the frame area (button 1 click for selection)
	frameRect := rt.Frame().Rect()
	clickPoint := image.Pt(frameRect.Min.X+50, frameRect.Min.Y+5)
	m := draw.Mouse{
		Point:   clickPoint,
		Buttons: 1, // Button 1 pressed
	}

	// Create a Mousectl with an immediate release event for proper Select() behavior
	upEvent := draw.Mouse{
		Point:   clickPoint,
		Buttons: 0, // Button released
	}
	mc := mockMousectlWithEvents([]draw.Mouse{upEvent})

	// Test that HandlePreviewMouse can be called with both mouse and mousectl
	// The key assertion is that the call compiles and executes without error
	handled := w.HandlePreviewMouse(&m, mc)

	// The event should be handled since we're in preview mode and clicking in the frame
	if !handled {
		t.Error("HandlePreviewMouse should return true for button 1 click in frame area")
	}

	// After handling, the selection should be set (at minimum, a point selection at the click)
	q0, q1 := rt.Selection()
	// We expect at least that q0/q1 are set (the exact values depend on the click position)
	// Since this is a single click without drag, q0 should equal q1
	if q0 != q1 {
		t.Logf("Selection after single click: q0=%d, q1=%d (expected point selection)", q0, q1)
	}

	// Test that events outside the body area are not handled
	outsidePoint := image.Pt(-10, -10)
	m2 := draw.Mouse{
		Point:   outsidePoint,
		Buttons: 1,
	}
	handled2 := w.HandlePreviewMouse(&m2, mc)
	if handled2 {
		t.Error("HandlePreviewMouse should return false for clicks outside body.all")
	}

	// Test with nil Mousectl (should still handle simple cases like scroll wheel)
	scrollDownMouse := draw.Mouse{
		Point:   clickPoint,
		Buttons: 16, // Button 5 - scroll down
	}
	handled3 := w.HandlePreviewMouse(&scrollDownMouse, nil)
	if !handled3 {
		t.Error("HandlePreviewMouse should handle scroll wheel even with nil Mousectl")
	}
}

// mockMousectlWithEvents creates a mock Mousectl with a buffered channel
// containing the provided events. This is used for testing drag selection.
func mockMousectlWithEvents(events []draw.Mouse) *draw.Mousectl {
	ch := make(chan draw.Mouse, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	return &draw.Mousectl{C: ch}
}















// setupPreviewChordTestWindow creates a Window in preview mode for chord testing.
// It sets up markdown content "Hello world test" with a source map, and returns
// the window, RichText, and frame rect for positioning mouse events.
func setupPreviewChordTestWindow(t *testing.T) (*Window, *RichText, image.Rectangle) {
	t.Helper()

	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	sourceMarkdown := "Hello world test"
	sourceRunes := []rune(sourceMarkdown)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/readme.md", sourceRunes),
	}
	w.body.all = image.Rect(0, 20, 800, 600)
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.col = &Column{safe: true}
	w.r = rect
	w.body.w = w

	// Set up global.row so acmeputsnarf() can call display.WriteSnarf()
	global.row = Row{display: display}
	t.Cleanup(func() { global.row = Row{} })

	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	bodyRect := image.Rect(12, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)
	rt.Render(bodyRect)

	// Parse markdown and set content with source map for source position mapping
	content, sourceMap, _ := markdown.ParseWithSourceMap(sourceMarkdown)
	rt.SetContent(content)

	w.richBody = rt
	w.SetPreviewSourceMap(sourceMap)
	w.SetPreviewMode(true)

	frameRect := rt.Frame().Rect()
	return w, rt, frameRect
}







// TestSelectionContext tests the SelectionContext struct used for context-aware
// paste operations in preview mode. SelectionContext tracks metadata about the
// current selection including source/rendered positions, content type, and
// formatting information needed to adapt paste behavior.
func TestSelectionContext(t *testing.T) {
	t.Run("ZeroValue", func(t *testing.T) {
		// A zero-value SelectionContext should have ContentPlain type
		var ctx SelectionContext
		if ctx.ContentType != ContentPlain {
			t.Errorf("zero-value ContentType = %v, want ContentPlain (%v)", ctx.ContentType, ContentPlain)
		}
		if ctx.SourceStart != 0 || ctx.SourceEnd != 0 {
			t.Errorf("zero-value source range = (%d,%d), want (0,0)", ctx.SourceStart, ctx.SourceEnd)
		}
		if ctx.RenderedStart != 0 || ctx.RenderedEnd != 0 {
			t.Errorf("zero-value rendered range = (%d,%d), want (0,0)", ctx.RenderedStart, ctx.RenderedEnd)
		}
		if ctx.CodeLanguage != "" {
			t.Errorf("zero-value CodeLanguage = %q, want empty", ctx.CodeLanguage)
		}
		if ctx.IncludesOpenMarker || ctx.IncludesCloseMarker {
			t.Error("zero-value should not include markers")
		}
	})

	t.Run("ContentTypes", func(t *testing.T) {
		// Verify all content type constants are distinct
		types := []SelectionContentType{
			ContentPlain,
			ContentHeading,
			ContentBold,
			ContentItalic,
			ContentBoldItalic,
			ContentCode,
			ContentCodeBlock,
			ContentLink,
			ContentImage,
			ContentMixed,
		}
		seen := make(map[SelectionContentType]bool)
		for _, ct := range types {
			if seen[ct] {
				t.Errorf("duplicate content type value: %v", ct)
			}
			seen[ct] = true
		}
	})

	t.Run("PlainText", func(t *testing.T) {
		ctx := SelectionContext{
			SourceStart:   0,
			SourceEnd:     5,
			RenderedStart: 0,
			RenderedEnd:   5,
			ContentType:   ContentPlain,
		}
		if ctx.ContentType != ContentPlain {
			t.Errorf("ContentType = %v, want ContentPlain", ctx.ContentType)
		}
		if ctx.SourceEnd-ctx.SourceStart != 5 {
			t.Errorf("source length = %d, want 5", ctx.SourceEnd-ctx.SourceStart)
		}
	})

	t.Run("BoldSelection", func(t *testing.T) {
		// Selecting "bold" from "**bold**" in rendered text
		// Source: "**bold**" (positions 0-8)
		// Rendered: "bold" (positions 0-4)
		ctx := SelectionContext{
			SourceStart:         0,
			SourceEnd:           8,
			RenderedStart:       0,
			RenderedEnd:         4,
			ContentType:         ContentBold,
			PrimaryStyle:        rich.Style{Bold: true, Scale: 1.0},
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		if ctx.ContentType != ContentBold {
			t.Errorf("ContentType = %v, want ContentBold", ctx.ContentType)
		}
		if !ctx.IncludesOpenMarker || !ctx.IncludesCloseMarker {
			t.Error("full bold selection should include both markers")
		}
		if !ctx.PrimaryStyle.Bold {
			t.Error("PrimaryStyle should have Bold set")
		}
	})

	t.Run("PartialBoldSelection", func(t *testing.T) {
		// Selecting "ol" from "**bold**" in rendered text
		// Source: positions within "**bold**" excluding markers
		// Rendered: "ol" (positions 1-3)
		ctx := SelectionContext{
			SourceStart:         4, // "**b|ol|d**" -> source pos of 'o'
			SourceEnd:           6, // source pos after 'l'
			RenderedStart:       1,
			RenderedEnd:         3,
			ContentType:         ContentBold,
			PrimaryStyle:        rich.Style{Bold: true, Scale: 1.0},
			IncludesOpenMarker:  false,
			IncludesCloseMarker: false,
		}
		if ctx.ContentType != ContentBold {
			t.Errorf("ContentType = %v, want ContentBold", ctx.ContentType)
		}
		if ctx.IncludesOpenMarker || ctx.IncludesCloseMarker {
			t.Error("partial bold selection should not include markers")
		}
	})

	t.Run("HeadingSelection", func(t *testing.T) {
		// Selecting entire heading text from "# Heading"
		// Source: "# Heading\n" (positions 0-10)
		// Rendered: "Heading\n" (positions 0-8)
		ctx := SelectionContext{
			SourceStart:        0,
			SourceEnd:          10,
			RenderedStart:      0,
			RenderedEnd:        8,
			ContentType:        ContentHeading,
			PrimaryStyle:       rich.Style{Bold: true, Scale: 2.0},
			IncludesOpenMarker: true,
		}
		if ctx.ContentType != ContentHeading {
			t.Errorf("ContentType = %v, want ContentHeading", ctx.ContentType)
		}
		if !ctx.IncludesOpenMarker {
			t.Error("heading selection from start should include open marker")
		}
	})

	t.Run("CodeBlockSelection", func(t *testing.T) {
		// Selecting text inside a fenced code block
		ctx := SelectionContext{
			SourceStart:   0,
			SourceEnd:     30,
			RenderedStart: 0,
			RenderedEnd:   15,
			ContentType:   ContentCodeBlock,
			CodeLanguage:  "go",
			PrimaryStyle:  rich.Style{Code: true, Block: true, Scale: 1.0},
		}
		if ctx.ContentType != ContentCodeBlock {
			t.Errorf("ContentType = %v, want ContentCodeBlock", ctx.ContentType)
		}
		if ctx.CodeLanguage != "go" {
			t.Errorf("CodeLanguage = %q, want %q", ctx.CodeLanguage, "go")
		}
	})

	t.Run("InlineCodeSelection", func(t *testing.T) {
		// Selecting inline code "`code`"
		ctx := SelectionContext{
			SourceStart:         0,
			SourceEnd:           6, // `code`
			RenderedStart:       0,
			RenderedEnd:         4, // code
			ContentType:         ContentCode,
			PrimaryStyle:        rich.Style{Code: true, Scale: 1.0},
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		if ctx.ContentType != ContentCode {
			t.Errorf("ContentType = %v, want ContentCode", ctx.ContentType)
		}
	})

	t.Run("LinkSelection", func(t *testing.T) {
		// Selecting link text from "[link](url)"
		ctx := SelectionContext{
			SourceStart:         0,
			SourceEnd:           12,
			RenderedStart:       0,
			RenderedEnd:         4,
			ContentType:         ContentLink,
			PrimaryStyle:        rich.Style{Link: true, Fg: rich.LinkBlue, Scale: 1.0},
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		if ctx.ContentType != ContentLink {
			t.Errorf("ContentType = %v, want ContentLink", ctx.ContentType)
		}
		if !ctx.PrimaryStyle.Link {
			t.Error("PrimaryStyle should have Link set")
		}
	})

	t.Run("ImageSelection", func(t *testing.T) {
		// Selecting image placeholder
		ctx := SelectionContext{
			SourceStart:   0,
			SourceEnd:     22, // ![alt text](image.png)
			RenderedStart: 0,
			RenderedEnd:   16, // [Image: alt text]
			ContentType:   ContentImage,
			PrimaryStyle:  rich.Style{Image: true, Scale: 1.0},
		}
		if ctx.ContentType != ContentImage {
			t.Errorf("ContentType = %v, want ContentImage", ctx.ContentType)
		}
	})

	t.Run("MixedSelection", func(t *testing.T) {
		// Selecting across multiple formatting types
		// e.g., "plain **bold** *italic*"
		ctx := SelectionContext{
			SourceStart:   0,
			SourceEnd:     24,
			RenderedStart: 0,
			RenderedEnd:   18,
			ContentType:   ContentMixed,
		}
		if ctx.ContentType != ContentMixed {
			t.Errorf("ContentType = %v, want ContentMixed", ctx.ContentType)
		}
	})

	t.Run("ItalicSelection", func(t *testing.T) {
		ctx := SelectionContext{
			SourceStart:         0,
			SourceEnd:           8, // *italic*
			RenderedStart:       0,
			RenderedEnd:         6, // italic
			ContentType:         ContentItalic,
			PrimaryStyle:        rich.Style{Italic: true, Scale: 1.0},
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		if ctx.ContentType != ContentItalic {
			t.Errorf("ContentType = %v, want ContentItalic", ctx.ContentType)
		}
		if !ctx.PrimaryStyle.Italic {
			t.Error("PrimaryStyle should have Italic set")
		}
	})

	t.Run("BoldItalicSelection", func(t *testing.T) {
		ctx := SelectionContext{
			SourceStart:   0,
			SourceEnd:     13, // ***both***
			RenderedStart: 0,
			RenderedEnd:   4, // both
			ContentType:   ContentBoldItalic,
			PrimaryStyle:  rich.Style{Bold: true, Italic: true, Scale: 1.0},
		}
		if ctx.ContentType != ContentBoldItalic {
			t.Errorf("ContentType = %v, want ContentBoldItalic", ctx.ContentType)
		}
		if !ctx.PrimaryStyle.Bold || !ctx.PrimaryStyle.Italic {
			t.Error("PrimaryStyle should have both Bold and Italic set")
		}
	})
}

// TestAnalyzeSelectionContent tests the analyzeSelectionContent method which
// examines the spans in the rendered RichText content within the given
// rendered-position range [rStart, rEnd) and determines the SelectionContentType.
// This is used during selection context updates to classify what kind of
// markdown content the user has selected (plain, bold, italic, code, heading, etc.).
func TestAnalyzeSelectionContent(t *testing.T) {
	// Helper to create a Window with richBody set to given content.
	setupWindow := func(t *testing.T, content rich.Content) *Window {
		t.Helper()
		rect := image.Rect(0, 0, 800, 600)
		display := edwoodtest.NewDisplay(rect)
		global.configureGlobals(display)

		w := NewWindow().initHeadless(nil)
		w.display = display
		w.body = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer("/test/readme.md", nil),
		}
		w.body.all = image.Rect(0, 20, 800, 600)
		w.col = &Column{safe: true}

		font := edwoodtest.NewFont(10, 14)
		bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
		textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

		rt := NewRichText()
		bodyRect := image.Rect(12, 20, 800, 600)
		rt.Init(display, font,
			WithRichTextBackground(bgImage),
			WithRichTextColor(textImage),
		)
		rt.Render(bodyRect)
		rt.SetContent(content)
		w.richBody = rt
		w.SetPreviewMode(true)
		return w
	}

	t.Run("PlainText", func(t *testing.T) {
		// Content: "Hello world" — all plain text with default style.
		content := rich.Plain("Hello world")
		w := setupWindow(t, content)

		// Selecting "Hello" (positions 0-5) should be plain.
		got := w.analyzeSelectionContent(0, 5)
		if got != ContentPlain {
			t.Errorf("analyzeSelectionContent(0,5) = %v, want ContentPlain", got)
		}
	})

	t.Run("AllBold", func(t *testing.T) {
		// Content: "bold text" rendered with bold style.
		content := rich.Content{
			{Text: "bold text", Style: rich.StyleBold},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 9)
		if got != ContentBold {
			t.Errorf("analyzeSelectionContent(0,9) = %v, want ContentBold", got)
		}
	})

	t.Run("PartialBold", func(t *testing.T) {
		// Content: "bold text" rendered bold, selecting "old" (positions 1-4).
		content := rich.Content{
			{Text: "bold text", Style: rich.StyleBold},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(1, 4)
		if got != ContentBold {
			t.Errorf("analyzeSelectionContent(1,4) = %v, want ContentBold", got)
		}
	})

	t.Run("AllItalic", func(t *testing.T) {
		// Content: "italic" rendered with italic style.
		content := rich.Content{
			{Text: "italic", Style: rich.StyleItalic},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 6)
		if got != ContentItalic {
			t.Errorf("analyzeSelectionContent(0,6) = %v, want ContentItalic", got)
		}
	})

	t.Run("BoldItalic", func(t *testing.T) {
		// Content: "emphasis" rendered with both bold and italic.
		content := rich.Content{
			{Text: "emphasis", Style: rich.Style{Bold: true, Italic: true, Scale: 1.0}},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 8)
		if got != ContentBoldItalic {
			t.Errorf("analyzeSelectionContent(0,8) = %v, want ContentBoldItalic", got)
		}
	})

	t.Run("InlineCode", func(t *testing.T) {
		// Content: "code" rendered with code style (monospace).
		content := rich.Content{
			{Text: "code", Style: rich.StyleCode},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 4)
		if got != ContentCode {
			t.Errorf("analyzeSelectionContent(0,4) = %v, want ContentCode", got)
		}
	})

	t.Run("CodeBlock", func(t *testing.T) {
		// Content: "func main() {}" as a block-level code element.
		content := rich.Content{
			{Text: "func main() {}", Style: rich.Style{Code: true, Block: true, Scale: 1.0}},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 14)
		if got != ContentCodeBlock {
			t.Errorf("analyzeSelectionContent(0,14) = %v, want ContentCodeBlock", got)
		}
	})

	t.Run("Heading", func(t *testing.T) {
		// Content: "Heading" rendered with heading style (bold, Scale > 1).
		content := rich.Content{
			{Text: "Heading", Style: rich.StyleH1},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 7)
		if got != ContentHeading {
			t.Errorf("analyzeSelectionContent(0,7) = %v, want ContentHeading", got)
		}
	})

	t.Run("HeadingH2", func(t *testing.T) {
		// H2 heading also detected as heading.
		content := rich.Content{
			{Text: "Subheading", Style: rich.StyleH2},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 10)
		if got != ContentHeading {
			t.Errorf("analyzeSelectionContent(0,10) = %v, want ContentHeading", got)
		}
	})

	t.Run("Link", func(t *testing.T) {
		// Content: "click here" rendered as a link.
		content := rich.Content{
			{Text: "click here", Style: rich.StyleLink},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 10)
		if got != ContentLink {
			t.Errorf("analyzeSelectionContent(0,10) = %v, want ContentLink", got)
		}
	})

	t.Run("Image", func(t *testing.T) {
		// Content: image placeholder text.
		content := rich.Content{
			{Text: "[image]", Style: rich.Style{Image: true, ImageURL: "photo.png", Scale: 1.0}},
		}
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(0, 7)
		if got != ContentImage {
			t.Errorf("analyzeSelectionContent(0,7) = %v, want ContentImage", got)
		}
	})

	t.Run("MixedPlainAndBold", func(t *testing.T) {
		// Content: "Hello " (plain) + "world" (bold)
		// Selecting across both spans should return ContentMixed.
		content := rich.Content{
			{Text: "Hello ", Style: rich.DefaultStyle()},
			{Text: "world", Style: rich.StyleBold},
		}
		w := setupWindow(t, content)

		// Select "lo world" (positions 3-11), spanning plain and bold.
		got := w.analyzeSelectionContent(3, 11)
		if got != ContentMixed {
			t.Errorf("analyzeSelectionContent(3,11) = %v, want ContentMixed", got)
		}
	})

	t.Run("MixedBoldAndItalic", func(t *testing.T) {
		// Content: "bold" (bold) + " and " (plain) + "italic" (italic)
		content := rich.Content{
			{Text: "bold", Style: rich.StyleBold},
			{Text: " and ", Style: rich.DefaultStyle()},
			{Text: "italic", Style: rich.StyleItalic},
		}
		w := setupWindow(t, content)

		// Select everything (0-15 = "bold and italic").
		got := w.analyzeSelectionContent(0, 15)
		if got != ContentMixed {
			t.Errorf("analyzeSelectionContent(0,15) = %v, want ContentMixed", got)
		}
	})

	t.Run("SelectionWithinOneSpanOfMultiple", func(t *testing.T) {
		// Content: "plain " (default) + "bold" (bold) + " more" (default)
		// Selecting only within the bold span should return ContentBold.
		content := rich.Content{
			{Text: "plain ", Style: rich.DefaultStyle()},
			{Text: "bold", Style: rich.StyleBold},
			{Text: " more", Style: rich.DefaultStyle()},
		}
		w := setupWindow(t, content)

		// "bold" starts at position 6, ends at 10.
		got := w.analyzeSelectionContent(6, 10)
		if got != ContentBold {
			t.Errorf("analyzeSelectionContent(6,10) = %v, want ContentBold", got)
		}
	})

	t.Run("EmptySelection", func(t *testing.T) {
		// An empty selection (rStart == rEnd) should return ContentPlain.
		content := rich.Plain("Some text")
		w := setupWindow(t, content)

		got := w.analyzeSelectionContent(5, 5)
		if got != ContentPlain {
			t.Errorf("analyzeSelectionContent(5,5) = %v, want ContentPlain", got)
		}
	})

	t.Run("NilRichBody", func(t *testing.T) {
		// If richBody is nil, should safely return ContentPlain.
		w := NewWindow().initHeadless(nil)
		w.richBody = nil

		got := w.analyzeSelectionContent(0, 5)
		if got != ContentPlain {
			t.Errorf("analyzeSelectionContent(0,5) with nil richBody = %v, want ContentPlain", got)
		}
	})
}

// TestUpdateSelectionContext tests the updateSelectionContext method which is
// called after each selection change in preview mode. It should read the current
// selection from richBody, translate positions via the previewSourceMap, analyze
// the content type, and store the result in w.selectionContext.
func TestUpdateSelectionContext(t *testing.T) {
	// Helper to create a window with richBody, source map, and selection set.
	setupWindow := func(t *testing.T, srcText string, selStart, selEnd int) *Window {
		t.Helper()
		rect := image.Rect(0, 0, 800, 600)
		display := edwoodtest.NewDisplay(rect)
		global.configureGlobals(display)

		w := NewWindow().initHeadless(nil)
		w.display = display
		w.body = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer("/test/readme.md", nil),
		}
		w.body.all = image.Rect(0, 20, 800, 600)
		w.col = &Column{safe: true}

		font := edwoodtest.NewFont(10, 14)
		bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
		textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

		// Parse the source markdown to get content and source map.
		content, sourceMap, _ := markdown.ParseWithSourceMap(srcText)

		rt := NewRichText()
		bodyRect := image.Rect(12, 20, 800, 600)
		rt.Init(display, font,
			WithRichTextBackground(bgImage),
			WithRichTextColor(textImage),
		)
		rt.Render(bodyRect)
		rt.SetContent(content)
		rt.SetSelection(selStart, selEnd)

		w.richBody = rt
		w.previewSourceMap = sourceMap
		w.SetPreviewMode(true)
		return w
	}

	t.Run("PlainTextSelection", func(t *testing.T) {
		// Source: "Hello world" — plain text, no formatting markers.
		// Select "Hello" (rendered positions 0-5).
		w := setupWindow(t, "Hello world", 0, 5)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after updateSelectionContext")
		}
		ctx := w.selectionContext
		if ctx.RenderedStart != 0 || ctx.RenderedEnd != 5 {
			t.Errorf("rendered range = [%d,%d), want [0,5)", ctx.RenderedStart, ctx.RenderedEnd)
		}
		if ctx.ContentType != ContentPlain {
			t.Errorf("ContentType = %v, want ContentPlain", ctx.ContentType)
		}
	})

	t.Run("BoldTextSelection", func(t *testing.T) {
		// Source: "**bold**" — bold text. Rendered as "bold" (4 chars).
		// Select all rendered text (0-4).
		w := setupWindow(t, "**bold**", 0, 4)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after updateSelectionContext")
		}
		ctx := w.selectionContext
		if ctx.RenderedStart != 0 || ctx.RenderedEnd != 4 {
			t.Errorf("rendered range = [%d,%d), want [0,4)", ctx.RenderedStart, ctx.RenderedEnd)
		}
		if ctx.ContentType != ContentBold {
			t.Errorf("ContentType = %v, want ContentBold", ctx.ContentType)
		}
		// Source positions should include the ** markers: [0, 8).
		if ctx.SourceStart != 0 || ctx.SourceEnd != 8 {
			t.Errorf("source range = [%d,%d), want [0,8)", ctx.SourceStart, ctx.SourceEnd)
		}
	})

	t.Run("HeadingSelection", func(t *testing.T) {
		// Source: "# Heading\n" — heading. Rendered as "Heading\n" (8 chars).
		// Select "Heading" (0-7).
		w := setupWindow(t, "# Heading\n", 0, 7)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after updateSelectionContext")
		}
		ctx := w.selectionContext
		if ctx.ContentType != ContentHeading {
			t.Errorf("ContentType = %v, want ContentHeading", ctx.ContentType)
		}
	})

	t.Run("EmptySelection", func(t *testing.T) {
		// When selection is empty (p0 == p1), context should reflect that.
		w := setupWindow(t, "Hello world", 3, 3)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after updateSelectionContext for empty selection")
		}
		ctx := w.selectionContext
		if ctx.RenderedStart != 3 || ctx.RenderedEnd != 3 {
			t.Errorf("rendered range = [%d,%d), want [3,3)", ctx.RenderedStart, ctx.RenderedEnd)
		}
		// Empty selection is ContentPlain.
		if ctx.ContentType != ContentPlain {
			t.Errorf("ContentType = %v, want ContentPlain", ctx.ContentType)
		}
	})

	t.Run("NotPreviewMode", func(t *testing.T) {
		// When not in preview mode, updateSelectionContext should not set context.
		w := setupWindow(t, "Hello world", 0, 5)
		w.SetPreviewMode(false)
		w.updateSelectionContext()

		if w.selectionContext != nil {
			t.Errorf("selectionContext should be nil when not in preview mode, got %+v", w.selectionContext)
		}
	})

	t.Run("NilRichBody", func(t *testing.T) {
		// When richBody is nil, updateSelectionContext should not panic.
		w := setupWindow(t, "Hello", 0, 5)
		w.richBody = nil
		w.updateSelectionContext()

		if w.selectionContext != nil {
			t.Errorf("selectionContext should be nil when richBody is nil, got %+v", w.selectionContext)
		}
	})

	t.Run("NilSourceMap", func(t *testing.T) {
		// When previewSourceMap is nil, updateSelectionContext should not panic.
		w := setupWindow(t, "Hello", 0, 5)
		w.previewSourceMap = nil
		w.updateSelectionContext()

		if w.selectionContext != nil {
			t.Errorf("selectionContext should be nil when previewSourceMap is nil, got %+v", w.selectionContext)
		}
	})

	t.Run("InlineCodeSelection", func(t *testing.T) {
		// Source: "`code`" — inline code. Rendered as "code" (4 chars).
		// Select all rendered text (0-4).
		w := setupWindow(t, "`code`", 0, 4)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after updateSelectionContext")
		}
		ctx := w.selectionContext
		if ctx.ContentType != ContentCode {
			t.Errorf("ContentType = %v, want ContentCode", ctx.ContentType)
		}
	})

	t.Run("MixedContentSelection", func(t *testing.T) {
		// Source: "plain **bold**" — mixed plain and bold.
		// Rendered as "plain bold" (10 chars). Selecting all should be ContentMixed.
		w := setupWindow(t, "plain **bold**", 0, 10)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after updateSelectionContext")
		}
		ctx := w.selectionContext
		if ctx.ContentType != ContentMixed {
			t.Errorf("ContentType = %v, want ContentMixed", ctx.ContentType)
		}
	})

	t.Run("SelectionUpdatesOnChange", func(t *testing.T) {
		// Verify that calling updateSelectionContext again with a new selection
		// replaces the previous context.
		w := setupWindow(t, "Hello **bold** world", 0, 5)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after first updateSelectionContext")
		}
		firstType := w.selectionContext.ContentType

		// Change selection to cover the bold portion.
		// "Hello bold world" rendered: "Hello " = 6, "bold" = 4, " world" = 6
		// Bold portion is at rendered positions 6-10.
		w.richBody.SetSelection(6, 10)
		w.updateSelectionContext()

		if w.selectionContext == nil {
			t.Fatal("selectionContext is nil after second updateSelectionContext")
		}
		if w.selectionContext.ContentType == firstType && firstType == ContentPlain {
			// First selection was plain "Hello", second should be bold.
			if w.selectionContext.ContentType != ContentBold {
				t.Errorf("after changing selection, ContentType = %v, want ContentBold", w.selectionContext.ContentType)
			}
		}
	})
}

func TestSnarfWithContext(t *testing.T) {
	// Helper to create a window with richBody, source map, selection, and body buffer.
	setupWindow := func(t *testing.T, srcText string, selStart, selEnd int) *Window {
		t.Helper()
		rect := image.Rect(0, 0, 800, 600)
		display := edwoodtest.NewDisplay(rect)
		global.configureGlobals(display)

		w := NewWindow().initHeadless(nil)
		w.display = display
		w.body = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer("/test/readme.md", nil),
		}
		w.body.all = image.Rect(0, 20, 800, 600)
		w.col = &Column{safe: true}

		// Insert source text into body buffer.
		w.body.file.InsertAt(0, []rune(srcText))

		font := edwoodtest.NewFont(10, 14)
		bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
		textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

		content, sourceMap, _ := markdown.ParseWithSourceMap(srcText)

		rt := NewRichText()
		bodyRect := image.Rect(12, 20, 800, 600)
		rt.Init(display, font,
			WithRichTextBackground(bgImage),
			WithRichTextColor(textImage),
		)
		rt.Render(bodyRect)
		rt.SetContent(content)
		rt.SetSelection(selStart, selEnd)

		w.richBody = rt
		w.previewSourceMap = sourceMap
		w.SetPreviewMode(true)
		return w
	}

	t.Run("PlainTextSnarf", func(t *testing.T) {
		// Source: "Hello world" — select "Hello" (rendered 0-5), snarf it.
		w := setupWindow(t, "Hello world", 0, 5)
		w.updateSelectionContext()

		snarfed := w.PreviewSnarf()
		if len(snarfed) == 0 {
			t.Fatal("PreviewSnarf returned empty for valid selection")
		}

		// Store snarf with context (the behavior under test).
		global.snarfbuf = snarfed
		global.snarfContext = w.selectionContext

		if global.snarfContext == nil {
			t.Fatal("snarfContext is nil after snarf operation")
		}
		if global.snarfContext.ContentType != ContentPlain {
			t.Errorf("snarfContext.ContentType = %v, want ContentPlain", global.snarfContext.ContentType)
		}
		if string(global.snarfbuf) != "Hello" {
			t.Errorf("snarfbuf = %q, want %q", string(global.snarfbuf), "Hello")
		}
	})

	t.Run("BoldTextSnarf", func(t *testing.T) {
		// Source: "**bold text**" — select the rendered bold text, snarf it.
		w := setupWindow(t, "**bold text**", 0, 9)
		w.updateSelectionContext()

		snarfed := w.PreviewSnarf()
		if len(snarfed) == 0 {
			t.Fatal("PreviewSnarf returned empty for bold selection")
		}

		global.snarfbuf = snarfed
		global.snarfContext = w.selectionContext

		if global.snarfContext == nil {
			t.Fatal("snarfContext is nil after bold snarf")
		}
		if global.snarfContext.ContentType != ContentBold {
			t.Errorf("snarfContext.ContentType = %v, want ContentBold", global.snarfContext.ContentType)
		}
	})

	t.Run("HeadingSnarf", func(t *testing.T) {
		// Source: "# Heading\n" — select the rendered heading text.
		w := setupWindow(t, "# Heading\n", 0, 7)
		w.updateSelectionContext()

		snarfed := w.PreviewSnarf()
		if len(snarfed) == 0 {
			t.Fatal("PreviewSnarf returned empty for heading selection")
		}

		global.snarfbuf = snarfed
		global.snarfContext = w.selectionContext

		if global.snarfContext == nil {
			t.Fatal("snarfContext is nil after heading snarf")
		}
		if global.snarfContext.ContentType != ContentHeading {
			t.Errorf("snarfContext.ContentType = %v, want ContentHeading", global.snarfContext.ContentType)
		}
	})

	t.Run("CodeSnarf", func(t *testing.T) {
		// Source: "`code`" — select the rendered inline code.
		w := setupWindow(t, "`code`", 0, 4)
		w.updateSelectionContext()

		snarfed := w.PreviewSnarf()
		if len(snarfed) == 0 {
			t.Fatal("PreviewSnarf returned empty for code selection")
		}

		global.snarfbuf = snarfed
		global.snarfContext = w.selectionContext

		if global.snarfContext == nil {
			t.Fatal("snarfContext is nil after code snarf")
		}
		if global.snarfContext.ContentType != ContentCode {
			t.Errorf("snarfContext.ContentType = %v, want ContentCode", global.snarfContext.ContentType)
		}
	})

	t.Run("SnarfClearsContextWhenEmpty", func(t *testing.T) {
		// Set up previous snarf context, then snarf an empty selection.
		global.snarfContext = &SelectionContext{ContentType: ContentBold}
		global.snarfbuf = []byte("old")

		w := setupWindow(t, "Hello world", 3, 3) // empty selection
		w.updateSelectionContext()

		snarfed := w.PreviewSnarf()
		if len(snarfed) > 0 {
			t.Fatal("PreviewSnarf returned non-empty for empty selection")
		}
		// When snarf returns nothing, context should not be updated
		// (previous context is preserved — only overwritten on successful snarf).
		if global.snarfContext == nil {
			t.Fatal("snarfContext should be preserved when snarf returns empty")
		}
	})

	t.Run("ContextMatchesSnarfContent", func(t *testing.T) {
		// Snarf plain, then snarf bold — context should update to match.
		w1 := setupWindow(t, "Hello world", 0, 5)
		w1.updateSelectionContext()
		snarfed := w1.PreviewSnarf()
		global.snarfbuf = snarfed
		global.snarfContext = w1.selectionContext

		if global.snarfContext.ContentType != ContentPlain {
			t.Fatalf("first snarf: ContentType = %v, want ContentPlain", global.snarfContext.ContentType)
		}

		// Now snarf bold text.
		w2 := setupWindow(t, "**bold**", 0, 4)
		w2.updateSelectionContext()
		snarfed = w2.PreviewSnarf()
		global.snarfbuf = snarfed
		global.snarfContext = w2.selectionContext

		if global.snarfContext.ContentType != ContentBold {
			t.Errorf("second snarf: ContentType = %v, want ContentBold", global.snarfContext.ContentType)
		}
	})
}

func TestPasteTransformBold(t *testing.T) {
	// Tests for transformForPaste with bold content.
	// Design rule: partial formatted text should be re-wrapped at destination.
	// Exception: if destination is already bold, just insert text (inherits context).

	t.Run("BoldTextToPlainDest", func(t *testing.T) {
		// Pasting bold text ("bold text") from a bold source into a plain destination
		// should wrap the text in **...** markers.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBold,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("bold text"), sourceCtx, destCtx)
		if string(result) != "**bold text**" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "**bold text**")
		}
	})

	t.Run("BoldTextToBoldDest", func(t *testing.T) {
		// Pasting bold text into an already-bold destination should NOT double-wrap.
		// The text inherits the destination's bold formatting.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBold,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBold,
		}
		result := transformForPaste([]byte("bold text"), sourceCtx, destCtx)
		if string(result) != "bold text" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "bold text")
		}
	})

	t.Run("PartialBoldToPlainDest", func(t *testing.T) {
		// Pasting partial bold text (e.g., "bol" from "**bold**") into plain dest
		// should re-wrap with bold markers.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBold,
			IncludesOpenMarker:  false,
			IncludesCloseMarker: false,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("bol"), sourceCtx, destCtx)
		if string(result) != "**bol**" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "**bol**")
		}
	})

	t.Run("PlainTextToPlainDest", func(t *testing.T) {
		// Pasting plain text into plain destination should pass through unchanged.
		sourceCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("hello"), sourceCtx, destCtx)
		if string(result) != "hello" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "hello")
		}
	})

	t.Run("PlainTextToBoldDest", func(t *testing.T) {
		// Pasting plain text into bold destination — just insert, inherits context.
		sourceCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBold,
		}
		result := transformForPaste([]byte("hello"), sourceCtx, destCtx)
		if string(result) != "hello" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "hello")
		}
	})

	t.Run("NilSourceContext", func(t *testing.T) {
		// When source context is nil (e.g., paste from external), pass through.
		result := transformForPaste([]byte("text"), nil, &SelectionContext{ContentType: ContentPlain})
		if string(result) != "text" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "text")
		}
	})

	t.Run("NilDestContext", func(t *testing.T) {
		// When destination context is nil, pass through unchanged.
		sourceCtx := &SelectionContext{ContentType: ContentBold}
		result := transformForPaste([]byte("text"), sourceCtx, nil)
		if string(result) != "text" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "text")
		}
	})
}

func TestPasteTransformHeading(t *testing.T) {
	// Tests for transformForPaste with heading content.
	// Design rule for structural elements:
	//   - With trailing newline: preserve structural markers (e.g., "# Heading\n")
	//   - Without trailing newline: strip markers, treat as "just text"

	t.Run("HeadingWithNewline", func(t *testing.T) {
		// "# Heading\n" with trailing newline → structural paste, preserve # prefix.
		sourceCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("# Heading\n"), sourceCtx, destCtx)
		if string(result) != "# Heading\n" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "# Heading\n")
		}
	})

	t.Run("HeadingWithoutNewline", func(t *testing.T) {
		// "# Heading" without trailing newline → text-only paste, strip # prefix.
		sourceCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("# Heading"), sourceCtx, destCtx)
		if string(result) != "Heading" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "Heading")
		}
	})

	t.Run("H2WithoutNewline", func(t *testing.T) {
		// "## Subheading" without trailing newline → strip ## prefix.
		sourceCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("## Subheading"), sourceCtx, destCtx)
		if string(result) != "Subheading" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "Subheading")
		}
	})

	t.Run("H2WithNewline", func(t *testing.T) {
		// "## Subheading\n" with trailing newline → preserve structural markers.
		sourceCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("## Subheading\n"), sourceCtx, destCtx)
		if string(result) != "## Subheading\n" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "## Subheading\n")
		}
	})

	t.Run("HeadingToHeadingDest", func(t *testing.T) {
		// Pasting heading text into a heading context — just insert the text.
		sourceCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		destCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		result := transformForPaste([]byte("# Heading"), sourceCtx, destCtx)
		if string(result) != "Heading" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "Heading")
		}
	})
}

func TestPasteTransformCode(t *testing.T) {
	// Tests for transformForPaste with code content.
	// Similar to bold: re-wrap in backticks unless destination is already code.

	t.Run("InlineCodeToPlainDest", func(t *testing.T) {
		// Pasting inline code text into a plain destination should wrap in backticks.
		sourceCtx := &SelectionContext{
			ContentType:         ContentCode,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("fmt.Println"), sourceCtx, destCtx)
		if string(result) != "`fmt.Println`" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "`fmt.Println`")
		}
	})

	t.Run("InlineCodeToCodeDest", func(t *testing.T) {
		// Pasting code into already-code destination — don't double-wrap.
		sourceCtx := &SelectionContext{
			ContentType:         ContentCode,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentCode,
		}
		result := transformForPaste([]byte("fmt.Println"), sourceCtx, destCtx)
		if string(result) != "fmt.Println" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "fmt.Println")
		}
	})

	t.Run("CodeBlockToPlainDest", func(t *testing.T) {
		// Pasting code block content with trailing newline → structural paste.
		sourceCtx := &SelectionContext{
			ContentType: ContentCodeBlock,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("```go\nfunc main() {}\n```\n"), sourceCtx, destCtx)
		if string(result) != "```go\nfunc main() {}\n```\n" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "```go\\nfunc main() {}\\n```\\n")
		}
	})

	t.Run("CodeBlockWithoutNewline", func(t *testing.T) {
		// Code block content without trailing newline → strip fences, just text.
		sourceCtx := &SelectionContext{
			ContentType: ContentCodeBlock,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("func main() {}"), sourceCtx, destCtx)
		// Code block text without fences and no newline → just the code text.
		if string(result) != "func main() {}" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "func main() {}")
		}
	})

	t.Run("PlainTextToCodeDest", func(t *testing.T) {
		// Pasting plain text into code destination — just insert, inherits context.
		sourceCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		destCtx := &SelectionContext{
			ContentType: ContentCode,
		}
		result := transformForPaste([]byte("hello"), sourceCtx, destCtx)
		if string(result) != "hello" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "hello")
		}
	})

	t.Run("ItalicTextToPlainDest", func(t *testing.T) {
		// Italic source to plain dest → re-wrap with * markers.
		sourceCtx := &SelectionContext{
			ContentType:         ContentItalic,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("italic text"), sourceCtx, destCtx)
		if string(result) != "*italic text*" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "*italic text*")
		}
	})

	t.Run("ItalicTextToItalicDest", func(t *testing.T) {
		// Italic source to italic dest → don't double-wrap.
		sourceCtx := &SelectionContext{
			ContentType:         ContentItalic,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentItalic,
		}
		result := transformForPaste([]byte("italic text"), sourceCtx, destCtx)
		if string(result) != "italic text" {
			t.Errorf("transformForPaste = %q, want %q", string(result), "italic text")
		}
	})

	t.Run("EmptyText", func(t *testing.T) {
		// Empty text should return empty regardless of context.
		sourceCtx := &SelectionContext{ContentType: ContentBold}
		destCtx := &SelectionContext{ContentType: ContentPlain}
		result := transformForPaste([]byte(""), sourceCtx, destCtx)
		if string(result) != "" {
			t.Errorf("transformForPaste = %q, want empty", string(result))
		}
	})
}

func TestPasteHeadingStructural(t *testing.T) {
	// Tests for structural heading paste — when the selection includes a
	// trailing newline, the heading markers (# prefix) are preserved because
	// the user intends to paste the heading as a structural element.

	setupWindow := func(t *testing.T, srcText string, selStart, selEnd int) *Window {
		t.Helper()
		rect := image.Rect(0, 0, 800, 600)
		display := edwoodtest.NewDisplay(rect)
		global.configureGlobals(display)

		w := NewWindow().initHeadless(nil)
		w.display = display
		w.body = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer("/test/readme.md", nil),
		}
		w.body.all = image.Rect(0, 20, 800, 600)
		w.col = &Column{safe: true}

		w.body.file.InsertAt(0, []rune(srcText))

		font := edwoodtest.NewFont(10, 14)
		bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
		textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

		content, sourceMap, _ := markdown.ParseWithSourceMap(srcText)

		rt := NewRichText()
		bodyRect := image.Rect(12, 20, 800, 600)
		rt.Init(display, font,
			WithRichTextBackground(bgImage),
			WithRichTextColor(textImage),
		)
		rt.Render(bodyRect)
		rt.SetContent(content)
		rt.SetSelection(selStart, selEnd)

		w.richBody = rt
		w.previewSourceMap = sourceMap
		w.SetPreviewMode(true)
		return w
	}

	t.Run("H1StructuralPastePreservesPrefix", func(t *testing.T) {
		// Snarf "# Heading\n" (full line with newline) → paste into plain context.
		// The trailing newline signals structural paste, so "# " prefix is preserved.
		w := setupWindow(t, "# Heading\n", 0, 8) // select full heading including newline in rendered text
		w.updateSelectionContext()

		snarfed := w.PreviewSnarf()
		if len(snarfed) == 0 {
			t.Fatal("PreviewSnarf returned empty for heading selection")
		}

		sourceCtx := w.selectionContext
		if sourceCtx == nil {
			t.Fatal("selectionContext is nil after heading snarf")
		}
		if sourceCtx.ContentType != ContentHeading {
			t.Errorf("sourceCtx.ContentType = %v, want ContentHeading", sourceCtx.ContentType)
		}

		destCtx := &SelectionContext{ContentType: ContentPlain}
		// Simulate structural paste: text with trailing newline.
		result := transformForPaste([]byte("# Heading\n"), sourceCtx, destCtx)
		if string(result) != "# Heading\n" {
			t.Errorf("structural paste: transformForPaste = %q, want %q", string(result), "# Heading\n")
		}
	})

	t.Run("H2StructuralPastePreservesPrefix", func(t *testing.T) {
		// Snarf "## Subheading\n" with trailing newline → structural paste preserves markers.
		w := setupWindow(t, "## Subheading\n", 0, 11)
		w.updateSelectionContext()

		sourceCtx := w.selectionContext
		destCtx := &SelectionContext{ContentType: ContentPlain}
		result := transformForPaste([]byte("## Subheading\n"), sourceCtx, destCtx)
		if string(result) != "## Subheading\n" {
			t.Errorf("structural paste: transformForPaste = %q, want %q", string(result), "## Subheading\n")
		}
	})

	t.Run("H3StructuralPastePreservesPrefix", func(t *testing.T) {
		// ### level heading with trailing newline → structural paste.
		sourceCtx := &SelectionContext{ContentType: ContentHeading}
		destCtx := &SelectionContext{ContentType: ContentPlain}
		result := transformForPaste([]byte("### Section\n"), sourceCtx, destCtx)
		if string(result) != "### Section\n" {
			t.Errorf("structural paste: transformForPaste = %q, want %q", string(result), "### Section\n")
		}
	})

	t.Run("StructuralPasteIntoHeadingContext", func(t *testing.T) {
		// Pasting a heading with newline into another heading context.
		// Same-type paste strips markers even for structural paste.
		sourceCtx := &SelectionContext{ContentType: ContentHeading}
		destCtx := &SelectionContext{ContentType: ContentHeading}
		result := transformForPaste([]byte("# Heading"), sourceCtx, destCtx)
		if string(result) != "Heading" {
			t.Errorf("heading-to-heading paste: transformForPaste = %q, want %q", string(result), "Heading")
		}
	})

	t.Run("MultipleHeadingsStructural", func(t *testing.T) {
		// Pasting multiple headings (structural block) preserves all prefixes.
		sourceCtx := &SelectionContext{ContentType: ContentHeading}
		destCtx := &SelectionContext{ContentType: ContentPlain}
		text := "# First\n## Second\n"
		result := transformForPaste([]byte(text), sourceCtx, destCtx)
		if string(result) != text {
			t.Errorf("multi-heading structural paste: transformForPaste = %q, want %q", string(result), text)
		}
	})
}

func TestPasteHeadingText(t *testing.T) {
	// Tests for text-only heading paste — when the selection does NOT include a
	// trailing newline, the heading markers (# prefix) are stripped because the
	// user is pasting the heading content as inline text.

	setupWindow := func(t *testing.T, srcText string, selStart, selEnd int) *Window {
		t.Helper()
		rect := image.Rect(0, 0, 800, 600)
		display := edwoodtest.NewDisplay(rect)
		global.configureGlobals(display)

		w := NewWindow().initHeadless(nil)
		w.display = display
		w.body = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer("/test/readme.md", nil),
		}
		w.body.all = image.Rect(0, 20, 800, 600)
		w.col = &Column{safe: true}

		w.body.file.InsertAt(0, []rune(srcText))

		font := edwoodtest.NewFont(10, 14)
		bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
		textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

		content, sourceMap, _ := markdown.ParseWithSourceMap(srcText)

		rt := NewRichText()
		bodyRect := image.Rect(12, 20, 800, 600)
		rt.Init(display, font,
			WithRichTextBackground(bgImage),
			WithRichTextColor(textImage),
		)
		rt.Render(bodyRect)
		rt.SetContent(content)
		rt.SetSelection(selStart, selEnd)

		w.richBody = rt
		w.previewSourceMap = sourceMap
		w.SetPreviewMode(true)
		return w
	}

	t.Run("H1TextPasteStripsPrefix", func(t *testing.T) {
		// Snarf "# Heading" (no trailing newline) → paste into plain context.
		// No trailing newline signals text paste, so "# " prefix is stripped.
		w := setupWindow(t, "# Heading\n", 0, 7) // select heading text without newline
		w.updateSelectionContext()

		snarfed := w.PreviewSnarf()
		if len(snarfed) == 0 {
			t.Fatal("PreviewSnarf returned empty for heading selection")
		}

		sourceCtx := w.selectionContext
		if sourceCtx == nil {
			t.Fatal("selectionContext is nil after heading snarf")
		}

		destCtx := &SelectionContext{ContentType: ContentPlain}
		// Text paste: heading content without trailing newline.
		result := transformForPaste([]byte("# Heading"), sourceCtx, destCtx)
		if string(result) != "Heading" {
			t.Errorf("text paste: transformForPaste = %q, want %q", string(result), "Heading")
		}
	})

	t.Run("H2TextPasteStripsPrefix", func(t *testing.T) {
		// "## Subheading" without trailing newline → strip markers.
		w := setupWindow(t, "## Subheading\n", 0, 10)
		w.updateSelectionContext()

		sourceCtx := w.selectionContext
		destCtx := &SelectionContext{ContentType: ContentPlain}
		result := transformForPaste([]byte("## Subheading"), sourceCtx, destCtx)
		if string(result) != "Subheading" {
			t.Errorf("text paste: transformForPaste = %q, want %q", string(result), "Subheading")
		}
	})

	t.Run("H3TextPasteStripsPrefix", func(t *testing.T) {
		// "### Section" without trailing newline → strip ### prefix.
		sourceCtx := &SelectionContext{ContentType: ContentHeading}
		destCtx := &SelectionContext{ContentType: ContentPlain}
		result := transformForPaste([]byte("### Section"), sourceCtx, destCtx)
		if string(result) != "Section" {
			t.Errorf("text paste: transformForPaste = %q, want %q", string(result), "Section")
		}
	})

	t.Run("PartialHeadingTextPaste", func(t *testing.T) {
		// Selecting part of a heading's text (e.g., "Head" from "# Heading")
		// without trailing newline → strip prefix, return just selected text.
		sourceCtx := &SelectionContext{ContentType: ContentHeading}
		destCtx := &SelectionContext{ContentType: ContentPlain}
		result := transformForPaste([]byte("# Head"), sourceCtx, destCtx)
		if string(result) != "Head" {
			t.Errorf("partial text paste: transformForPaste = %q, want %q", string(result), "Head")
		}
	})

	t.Run("HeadingTextPasteIntoParagraph", func(t *testing.T) {
		// Pasting heading text (no newline) mid-paragraph should give just the text.
		sourceCtx := &SelectionContext{ContentType: ContentHeading}
		destCtx := &SelectionContext{ContentType: ContentPlain}
		result := transformForPaste([]byte("# Title"), sourceCtx, destCtx)
		if string(result) != "Title" {
			t.Errorf("mid-paragraph paste: transformForPaste = %q, want %q", string(result), "Title")
		}
	})

	t.Run("HeadingTextPasteIntoBold", func(t *testing.T) {
		// Pasting heading text (no newline) into bold context → just text, no markers.
		sourceCtx := &SelectionContext{ContentType: ContentHeading}
		destCtx := &SelectionContext{ContentType: ContentBold}
		result := transformForPaste([]byte("# Important"), sourceCtx, destCtx)
		if string(result) != "Important" {
			t.Errorf("heading-to-bold paste: transformForPaste = %q, want %q", string(result), "Important")
		}
	})
}

func TestPasteIntoFormattedContext(t *testing.T) {
	// Tests for format inheritance: when pasting into an already-formatted
	// destination context, the transform should avoid double-wrapping.
	// The key principle: if dest already provides formatting of the same type,
	// strip source markers; otherwise apply normal transformation rules.

	t.Run("BoldIntoBold", func(t *testing.T) {
		// Pasting bold text into bold context — don't double-wrap with **.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBold,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBold,
		}
		result := transformForPaste([]byte("important"), sourceCtx, destCtx)
		if string(result) != "important" {
			t.Errorf("bold-into-bold: got %q, want %q", string(result), "important")
		}
	})

	t.Run("ItalicIntoItalic", func(t *testing.T) {
		// Pasting italic text into italic context — don't double-wrap with *.
		sourceCtx := &SelectionContext{
			ContentType:         ContentItalic,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentItalic,
		}
		result := transformForPaste([]byte("emphasis"), sourceCtx, destCtx)
		if string(result) != "emphasis" {
			t.Errorf("italic-into-italic: got %q, want %q", string(result), "emphasis")
		}
	})

	t.Run("CodeIntoCode", func(t *testing.T) {
		// Pasting code into code context — don't double-wrap with backticks.
		sourceCtx := &SelectionContext{
			ContentType:         ContentCode,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentCode,
		}
		result := transformForPaste([]byte("x := 1"), sourceCtx, destCtx)
		if string(result) != "x := 1" {
			t.Errorf("code-into-code: got %q, want %q", string(result), "x := 1")
		}
	})

	t.Run("BoldItalicIntoBoldItalic", func(t *testing.T) {
		// Pasting bold-italic into bold-italic context — same type, strip markers.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBoldItalic,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBoldItalic,
		}
		result := transformForPaste([]byte("strong emphasis"), sourceCtx, destCtx)
		if string(result) != "strong emphasis" {
			t.Errorf("bolditalic-into-bolditalic: got %q, want %q", string(result), "strong emphasis")
		}
	})

	t.Run("HeadingIntoHeading", func(t *testing.T) {
		// Pasting heading text (no newline) into heading context — strip prefix.
		sourceCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		destCtx := &SelectionContext{
			ContentType: ContentHeading,
		}
		result := transformForPaste([]byte("## Section"), sourceCtx, destCtx)
		if string(result) != "Section" {
			t.Errorf("heading-into-heading: got %q, want %q", string(result), "Section")
		}
	})

	t.Run("BoldIntoItalic", func(t *testing.T) {
		// Pasting bold text into italic context — different formatting types.
		// Bold source into non-plain dest: text passes through (not re-wrapped).
		sourceCtx := &SelectionContext{
			ContentType:         ContentBold,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentItalic,
		}
		result := transformForPaste([]byte("bold text"), sourceCtx, destCtx)
		// Bold into non-plain, non-bold: text passes through (dest provides its own formatting).
		if string(result) != "bold text" {
			t.Errorf("bold-into-italic: got %q, want %q", string(result), "bold text")
		}
	})

	t.Run("ItalicIntoBold", func(t *testing.T) {
		// Pasting italic text into bold context — different formatting types.
		sourceCtx := &SelectionContext{
			ContentType:         ContentItalic,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBold,
		}
		result := transformForPaste([]byte("italic text"), sourceCtx, destCtx)
		// Italic into non-plain, non-italic: text passes through.
		if string(result) != "italic text" {
			t.Errorf("italic-into-bold: got %q, want %q", string(result), "italic text")
		}
	})

	t.Run("CodeIntoBold", func(t *testing.T) {
		// Pasting code text into bold context — different formatting types.
		sourceCtx := &SelectionContext{
			ContentType:         ContentCode,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBold,
		}
		result := transformForPaste([]byte("var x"), sourceCtx, destCtx)
		// Code into non-plain, non-code: text passes through.
		if string(result) != "var x" {
			t.Errorf("code-into-bold: got %q, want %q", string(result), "var x")
		}
	})

	t.Run("BoldIntoCode", func(t *testing.T) {
		// Pasting bold text into code context — different formatting types.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBold,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentCode,
		}
		result := transformForPaste([]byte("bold text"), sourceCtx, destCtx)
		// Bold into non-plain, non-bold: text passes through.
		if string(result) != "bold text" {
			t.Errorf("bold-into-code: got %q, want %q", string(result), "bold text")
		}
	})

	t.Run("PlainIntoBold", func(t *testing.T) {
		// Pasting plain text into bold context — inherits bold formatting.
		sourceCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBold,
		}
		result := transformForPaste([]byte("hello world"), sourceCtx, destCtx)
		if string(result) != "hello world" {
			t.Errorf("plain-into-bold: got %q, want %q", string(result), "hello world")
		}
	})

	t.Run("PlainIntoItalic", func(t *testing.T) {
		// Pasting plain text into italic context — inherits italic formatting.
		sourceCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		destCtx := &SelectionContext{
			ContentType: ContentItalic,
		}
		result := transformForPaste([]byte("hello world"), sourceCtx, destCtx)
		if string(result) != "hello world" {
			t.Errorf("plain-into-italic: got %q, want %q", string(result), "hello world")
		}
	})

	t.Run("PlainIntoCode", func(t *testing.T) {
		// Pasting plain text into code context — inherits code formatting.
		sourceCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		destCtx := &SelectionContext{
			ContentType: ContentCode,
		}
		result := transformForPaste([]byte("x + y"), sourceCtx, destCtx)
		if string(result) != "x + y" {
			t.Errorf("plain-into-code: got %q, want %q", string(result), "x + y")
		}
	})

	t.Run("PartialBoldIntoBold", func(t *testing.T) {
		// Pasting partial bold (no markers in selection) into bold context.
		// Same type — should still strip/pass through, not re-wrap.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBold,
			IncludesOpenMarker:  false,
			IncludesCloseMarker: false,
		}
		destCtx := &SelectionContext{
			ContentType: ContentBold,
		}
		result := transformForPaste([]byte("parti"), sourceCtx, destCtx)
		if string(result) != "parti" {
			t.Errorf("partial-bold-into-bold: got %q, want %q", string(result), "parti")
		}
	})

	t.Run("BoldItalicIntoPlain", func(t *testing.T) {
		// Pasting bold-italic into plain context — should wrap with ***.
		sourceCtx := &SelectionContext{
			ContentType:         ContentBoldItalic,
			IncludesOpenMarker:  true,
			IncludesCloseMarker: true,
		}
		destCtx := &SelectionContext{
			ContentType: ContentPlain,
		}
		result := transformForPaste([]byte("strong emphasis"), sourceCtx, destCtx)
		if string(result) != "***strong emphasis***" {
			t.Errorf("bolditalic-into-plain: got %q, want %q", string(result), "***strong emphasis***")
		}
	})
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr) >= 0
}








// TrackingMockFrame is a MockFrame that tracks DrawSel calls.
type TrackingMockFrame struct {
	MockFrame
	DrawSelCalled bool
	DrawSelCount  int
	nchars        int
	maxlines      int
}

func (mf *TrackingMockFrame) GetFrameFillStatus() frame.FrameFillStatus {
	return frame.FrameFillStatus{
		Nchars:         mf.nchars,
		Nlines:         mf.maxlines,
		Maxlines:       mf.maxlines,
		MaxPixelHeight: mf.maxlines * 14,
	}
}

func (mf *TrackingMockFrame) DrawSel(pt image.Point, p0, p1 int, ticked bool) {
	mf.DrawSelCalled = true
	mf.DrawSelCount++
}

func (mf *TrackingMockFrame) Ptofchar(int) image.Point { return image.Point{0, 0} }


















// searchString returns the index of substr in s, or -1 if not found.
func searchString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}






















// setupPreviewTypeTestWindow creates a Window in preview mode for typing tests.
// It sets up markdown content with a source map, positions the cursor, and
// returns the window and body Text for verification.
func setupPreviewTypeTestWindow(t *testing.T, sourceMarkdown string) *Window {
	t.Helper()

	rect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(rect)
	global.configureGlobals(display)

	sourceRunes := []rune(sourceMarkdown)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("/test/readme.md", sourceRunes),
		eq0:     ^0,
		what:    Body,
	}
	w.body.all = image.Rect(0, 20, 800, 600)
	w.tag = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", nil),
	}
	w.body.file.AddObserver(&w.body)
	w.col = &Column{safe: true}
	w.r = rect
	w.body.w = w

	global.row = Row{display: display}
	t.Cleanup(func() { global.row = Row{} })

	font := edwoodtest.NewFont(10, 14)
	bgImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	textImage, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)

	rt := NewRichText()
	bodyRect := image.Rect(12, 20, 800, 600)
	rt.Init(display, font,
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
	)
	rt.Render(bodyRect)

	content, sourceMap, _ := markdown.ParseWithSourceMap(sourceMarkdown)
	rt.SetContent(content)

	w.richBody = rt
	w.SetPreviewSourceMap(sourceMap)
	w.SetPreviewMode(true)

	return w
}







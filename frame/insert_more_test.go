package frame

import (
	"image"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TODO(rjk): Test having a height that's not a multiple of the font
// height. Particularly relevant for supporting lines of differing
// heights.

// TestInsertAligned is a high-level Insert test that uses a frame where
// the character edge aligns with the width of the text region.
func TestInsertAligned(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
	}

	*validate = true

	tests := []struct {
		name        string
		fn          func(t *testing.T, fr Frame, iv *invariants)
		want        []string
		textarea    image.Rectangle
		knowntofail bool
	}{
		{
			// Insert text that doesn't fit.
			name: "insertPastEnd",
			fn:   insertPastEnd,
			// B5: see frame/testdata/.../*_trial.html.
			knowntofail: true,
			want: []string{
				"fill (20,10)-(59,20) [0,0],[3,1]",
				"fill (20,20)-(59,40) [0,1],[3,2]",
				"fill (20,40)-(20,50) [0,3],[0,1]",
				`screen-800x600 <- string "a本ポ" atpoint: (20,10) [0,0] fill: black`,
				`screen-800x600 <- string "ポポポ" atpoint: (20,20) [0,1] fill: black`,
				`screen-800x600 <- string "ポポh" atpoint: (20,30) [0,2] fill: black`},
			textarea: image.Rect(20, 10, 59, 40),
		},
		{
			// Split a wrapped line by inserting a newline.
			name:     "splitWrappedLine",
			fn:       splitWrappedLine,
			textarea: image.Rect(20, 10, 59, 60),
			// B5: see frame/testdata/.../*_trial.html.
			knowntofail: true,
			want: []string{
				"fill (20,10)-(59,20) [0,0],[3,1]",
				"fill (20,20)-(59,50) [0,1],[3,3]",
				"fill (20,50)-(33,60) [0,4],[1,1]",
				`screen-800x600 <- string "a本ポ" atpoint: (20,10) [0,0] fill: black`,
				`screen-800x600 <- string "ポポポ" atpoint: (20,20) [0,1] fill: black`,
				`screen-800x600 <- string "ポポh" atpoint: (20,30) [0,2] fill: black`,
				`screen-800x600 <- string "ell" atpoint: (20,40) [0,3] fill: black`,
				`screen-800x600 <- string "o" atpoint: (20,50) [0,4] fill: black`,
				// The previously failing insertion starts here. We didn't have to do
				// anything in this case. But we still fill blank space at the end of the
				// line over again. This is (hopefully) harmless.
				// TODO(rjk): Elide the 0-width draws.
				"fill (58,10)-(59,20) [-,0],[-,1]",
				"fill (20,20)-(20,30) [0,1],[0,1]",
			},
		},
		{
			// Insert a single character that forces conversion of non-wrapped to
			// wrapped with wripple to end.
			// B2.3 R7: see TestInsert/insertForcesWrap.
			name:     "insertForcesWrap",
			fn:       insertForcesWrap,
			textarea: image.Rect(20, 10, 59, 60),
			want: []string{
				"fill (20,10)-(59,60) [0,0],[3,5]",
				"fill (20,10)-(59,20) [0,0],[3,1]",
				`screen-800x600 <- string "0ab" atpoint: (20,10) [0,0] fill: black`,
				"fill (20,20)-(59,30) [0,1],[3,1]",
				`screen-800x600 <- string "1cd" atpoint: (20,20) [0,1] fill: black`,
				"fill (20,30)-(59,40) [0,2],[3,1]",
				`screen-800x600 <- string "2ef" atpoint: (20,30) [0,2] fill: black`,
				"fill (20,40)-(59,50) [0,3],[3,1]",
				`screen-800x600 <- string "3gh" atpoint: (20,40) [0,3] fill: black`,
				"fill (20,50)-(59,60) [0,4],[3,1]",
				`screen-800x600 <- string "4ij" atpoint: (20,50) [0,4] fill: black`,
				"blit (20,20)-(59,50) [0,1],[3,3], to (20,30)-(59,60) [0,2],[3,3]",
				"fill (20,10)-(59,30) [0,0],[3,2]",
				"fill (20,10)-(59,20) [0,0],[3,1]",
				`screen-800x600 <- string "0aX" atpoint: (20,10) [0,0] fill: black`,
				"fill (20,20)-(33,30) [0,1],[1,1]",
				`screen-800x600 <- string "b" atpoint: (20,20) [0,1] fill: black`,
			},
		},
		{
			// Append a pair of characters at the end of the otherwise full text
			// area.
			// B2.3 R7: appended "XX" lands past rect.Max.Y →
			// truncateOffscreen drops it; no ops emitted.
			name:     "appendAtEnd",
			fn:       appendAtEnd,
			textarea: image.Rect(20, 10, 59, 60),
			want: []string{
				"fill (20,10)-(59,60) [0,0],[3,5]",
				"fill (20,10)-(59,20) [0,0],[3,1]",
				`screen-800x600 <- string "0ab" atpoint: (20,10) [0,0] fill: black`,
				"fill (20,20)-(59,30) [0,1],[3,1]",
				`screen-800x600 <- string "1cd" atpoint: (20,20) [0,1] fill: black`,
				"fill (20,30)-(59,40) [0,2],[3,1]",
				`screen-800x600 <- string "2ef" atpoint: (20,30) [0,2] fill: black`,
				"fill (20,40)-(59,50) [0,3],[3,1]",
				`screen-800x600 <- string "3gh" atpoint: (20,40) [0,3] fill: black`,
				"fill (20,50)-(59,60) [0,4],[3,1]",
				`screen-800x600 <- string "4ij" atpoint: (20,50) [0,4] fill: black`,
			},
		},

		{
			// Append a multibox string that hangs off the end. TODO(rjk): Draws a
			// zero-width fill off the end of text area. This is conceivably wrong.
			// It would (for example) make some drawing stacks unhappy.
			name: "appendHangingLongAtEnd",
			// B5: see frame/testdata/.../*_trial.html.
			knowntofail: true,
			fn:          appendHangingLongAtEnd,
			textarea:    image.Rect(20, 10, 59, 60),
			want: []string{
				"fill (20,10)-(59,20) [0,0],[3,1]",
				"fill (20,20)-(59,60) [0,1],[3,4]",
				"fill (20,60)-(20,70) [0,5],[0,1]",
				`screen-800x600 <- string "0" atpoint: (20,10) [0,0] fill: black`,
				`screen-800x600 <- string "1" atpoint: (20,20) [0,1] fill: black`,
				`screen-800x600 <- string "2" atpoint: (20,30) [0,2] fill: black`,
				`screen-800x600 <- string "3" atpoint: (20,40) [0,3] fill: black`,
				`screen-800x600 <- string "4" atpoint: (20,50) [0,4] fill: black`,
				"fill (33,50)-(59,60) [1,4],[2,1]",
				"fill (20,60)-(20,70) [0,5],[0,1]",
				`screen-800x600 <- string "XX" atpoint: (33,50) [1,4] fill: black`,
			},
		},
		{
			// Insert a multibox string that forces ripple past the end.
			name: "insertWrappedThatForcesRipple",
			// B5: see frame/testdata/.../*_trial.html.
			knowntofail: true,
			fn:          insertWrappedThatForcesRipple,
			textarea:    image.Rect(20, 10, 59, 60),
			want: []string{
				"fill (20,10)-(59,20) [0,0],[3,1]",
				"fill (20,20)-(59,60) [0,1],[3,4]",
				"fill (20,60)-(20,70) [0,5],[0,1]",
				`screen-800x600 <- string "0" atpoint: (20,10) [0,0] fill: black`,
				`screen-800x600 <- string "1" atpoint: (20,20) [0,1] fill: black`,
				`screen-800x600 <- string "2" atpoint: (20,30) [0,2] fill: black`,
				`screen-800x600 <- string "3b" atpoint: (20,40) [0,3] fill: black`,
				`screen-800x600 <- string "4" atpoint: (20,50) [0,4] fill: black`,
				"fill (59,50)-(59,60) [3,4],[0,1]",
				"blit (33,40)-(46,50) [1,3],[1,1], to (46,50)-(59,60) [2,4],[1,1]",
				"fill (33,40)-(59,50) [1,3],[2,1]",
				"fill (20,50)-(46,60) [0,4],[2,1]",
				`screen-800x600 <- string "ij" atpoint: (33,40) [1,3] fill: black`,
				`screen-800x600 <- string "XX" atpoint: (20,50) [0,4] fill: black`,
			},
		},
		{
			// Insert a string that pushes a blank line off the end.
			name: "insertPushesBlankLineOffEnd",
			// B5: see frame/testdata/.../*_trial.html.
			knowntofail: true,
			fn:          insertPushesBlankLineOffEnd,
			textarea:    image.Rect(20, 10, 59, 60),
			want: []string{
				"fill (20,10)-(59,20) [0,0],[3,1]",
				"fill (20,20)-(59,60) [0,1],[3,4]",
				"fill (20,60)-(20,70) [0,5],[0,1]",
				`screen-800x600 <- string "0ab" atpoint: (20,10) [0,0] fill: black`,
				`screen-800x600 <- string "1cd" atpoint: (20,20) [0,1] fill: black`,
				`screen-800x600 <- string "2ef" atpoint: (20,30) [0,2] fill: black`,
				`screen-800x600 <- string "3gh" atpoint: (20,40) [0,3] fill: black`,
				"blit (20,30)-(59,50) [0,2],[3,2], to (20,40)-(59,60) [0,3],[3,2]",
				"blit (59,20)-(59,30) [3,1],[0,1], to (59,30)-(59,40) [3,2],[0,1]",
				"blit (20,20)-(59,30) [0,1],[3,1], to (20,30)-(59,40) [0,2],[3,1]",
				"fill (33,20)-(59,30) [1,1],[2,1]",
				"blit (46,10)-(59,20) [2,0],[1,1], to (20,20)-(33,30) [0,1],[1,1]",
				"blit (33,10)-(46,20) [1,0],[1,1], to (46,10)-(59,20) [2,0],[1,1]",
				"fill (59,10)-(59,20) [3,0],[0,1]",
				"fill (33,10)-(46,20) [1,0],[1,1]",
				`screen-800x600 <- string "X" atpoint: (33,10) [1,0] fill: black`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.knowntofail {
				return
			}

			iv.textarea = tc.textarea
			fr := setupFrame(t, iv)

			// TODO(rjk): validate here

			tc.fn(t, fr, iv)

			// TODO(rjk): validate here

			// Peek inside.
			got := gdo(t, fr).DrawOps()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("dump mismatch (-want +got):\n%s", diff)
			}

			visualizedoutputtest(t, fr)
		})
	}
}

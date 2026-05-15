package frame

import (
	"image"
	"testing"
)

type BoxModelTestResult struct {
	result     int
	boolresult bool
}

type BoxModelTest struct {
	name       string
	frame      *frameimpl
	stim       func(*frameimpl) (int, bool)
	nbox       int
	afterboxes []*frbox
	result     int
	boolresult bool
}

func (bx BoxModelTest) Try() interface{} {
	a, b := bx.stim(bx.frame)
	return BoxModelTestResult{
		result:     a,
		boolresult: b,
	}
}

func (bx BoxModelTest) Verify(t *testing.T, prefix string, result interface{}) {
	r := result.(BoxModelTestResult)

	if got, want := r.result, bx.result; got != want {
		t.Errorf("%s-%s: running stim got %d but want %d\n", prefix, bx.name, got, want)
	}
	if got, want := r.boolresult, bx.boolresult; got != want {
		t.Errorf("%s-%s: running stim bool got %v but want %v\n", prefix, bx.name, got, want)
	}

	testcore(t, prefix, bx.name, bx.frame, bx.nbox, bx.afterboxes)
}

func TestCanfit(t *testing.T) {
	newlinebox := makeBox("\n")
	tabbox := makeBox("\t")

	comparecore(t, "TestCanfit", []BoxTester{
		BoxModelTest{
			"multi-glyph box doesn't fit",
			&frameimpl{
				font: mockFont(),
				rect: image.Rect(10, 15, 10+57, 15+57),
				box:  []*frbox{makeBox("0123456789")},
			},
			func(f *frameimpl) (int, bool) {
				a, b := f.canfit(image.Pt(10+14, 15), f.box[0])
				return a, b
			},
			1,
			[]*frbox{makeBox("0123456789")},
			// 10 + 14 + 40 = 64. less than 67.
			4,
			true,
		},
		BoxModelTest{
			"multi-glyph box, fits",
			&frameimpl{
				font: mockFont(),
				rect: image.Rect(10, 15, 10+57, 15+57),
				box:  []*frbox{makeBox("0123")},
			},
			func(f *frameimpl) (int, bool) {
				a, b := f.canfit(image.Pt(10+14, 15), f.box[0])
				return a, b
			},
			1,
			[]*frbox{makeBox("0123")},
			// 10 + 14 + 40 = 64. less than 67.
			4,
			true,
		},
		BoxModelTest{
			"newline box",
			&frameimpl{
				font: mockFont(),
				rect: image.Rect(10, 15, 10+57, 15+57),
				box:  []*frbox{newlinebox},
			},
			func(f *frameimpl) (int, bool) {
				a, b := f.canfit(image.Pt(10+57, 15), f.box[0])
				return a, b
			},
			1,
			[]*frbox{newlinebox},
			// newline fits up to the edge.
			1,
			true,
		},
		BoxModelTest{
			"tab box",
			&frameimpl{
				font: mockFont(),
				rect: image.Rect(10, 15, 10+57, 15+57),
				box:  []*frbox{tabbox},
			},
			func(f *frameimpl) (int, bool) {
				a, b := f.canfit(image.Pt(10+48, 15), f.box[0])
				return a, b
			},
			1,
			[]*frbox{tabbox},
			// tab at edge doesn't  fit
			0,
			false,
		},
		BoxModelTest{
			"multi-glyph box, doesn't fit",
			&frameimpl{
				font: mockFont(),
				rect: image.Rect(10, 15, 10+57, 15+57),
				box:  []*frbox{makeBox("本a")},
			},
			func(f *frameimpl) (int, bool) {
				a, b := f.canfit(image.Pt(10+57-11, 15), f.box[0])
				return a, b
			},
			1,
			[]*frbox{makeBox("本a")},
			// 10 + 14 + 40 = 64. less than 67.
			1,
			true,
		},
	})
}

// B2.3 R11 deleted TestClean along with the clean() helper.
// R1's eager-coalesce inside relayoutFrom replaces clean's
// merge functionality; eager-coalesce is tested in
// frame/line_summary_test.go (R1.13–R1.16, R1.18).

func TestNewwid0(t *testing.T) {
	f := &frameimpl{
		rect:   image.Rect(4, 15, 4+57, 15+61),
		maxtab: 32,
	}

	testtab := []struct {
		name string
		box  *frbox
		pt   image.Point
		want int
	}{
		{
			name: "normal character",
			box: &frbox{
				Nrune: 0,
				Wid:   11,
			},
			pt:   image.Pt(10, 15),
			want: 11,
		},
		{
			name: "newline character",
			box: &frbox{
				Nrune: -1,
				Wid:   1000,
				Bc:    '\n',
			},
			pt:   image.Pt(10, 15),
			want: 1000,
		},
		{
			name: "tab character, left edge",
			box: &frbox{
				Nrune:  -1,
				Wid:    10000,
				Bc:     '\t',
				Minwid: 10,
			},
			pt:   image.Pt(4, 15),
			want: f.maxtab,
		},
		{
			name: "tab character, less than maxtab",
			box: &frbox{
				Nrune:  -1,
				Wid:    10000,
				Bc:     '\t',
				Minwid: 10,
			},
			pt: image.Pt(10, 15),
			// In 0th tab cell, 6 pixels over so maxtab - 6 over to next tab stop.
			want: f.maxtab - 6,
		},
		{
			name: "tab character, start of second tabstop, doesn't fit so trimmed",
			box: &frbox{
				Nrune:  -1,
				Wid:    10000,
				Bc:     '\t',
				Minwid: 5,
			},
			pt:   image.Pt(4+32, 15),
			want: 5,
		},
		{
			name: "tab character, minwidth doesn't fit so size as if start of next line",
			box: &frbox{
				Nrune:  -1,
				Wid:    10000,
				Bc:     '\t',
				Minwid: 5,
			},
			pt:   image.Pt(4+56, 15),
			want: 32,
		},
	}

	for _, test := range testtab {
		t.Run(test.name, func(t *testing.T) {
			// write me a test here
			if got, want := f.newwid0(test.pt, test.box), test.want; got != want {
				t.Errorf("got %d, want %d", got, want)
			}
		})
	}

}

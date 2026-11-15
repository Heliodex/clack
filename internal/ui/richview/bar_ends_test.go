package richview

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	rt "github.com/some-du6e/clack/internal/richtext"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/ui/theme/pixeltest"
)

// Radii that don't fit a side scale down together the way CSS does
// (Backgrounds 3, corner curve overlap); radii that fit are left alone.
func TestCSSRadii(t *testing.T) {
	for _, c := range []struct {
		r      image.Rectangle
		in     [4]float32
		want   [4]float32
		scaled bool
	}{
		// .c-message_attachment__border: 4px wide, border-radius 8px → 2px ends
		{image.Rect(0, 0, 4, 40), [4]float32{8, 8, 8, 8}, [4]float32{2, 2, 2, 2}, true},
		// at 1.25x: 5px wide, radius 10 → 2.5
		{image.Rect(0, 0, 5, 50), [4]float32{10, 10, 10, 10}, [4]float32{2.5, 2.5, 2.5, 2.5}, true},
		// shorter than wide too: the smaller factor wins (3/16)
		{image.Rect(0, 0, 4, 3), [4]float32{8, 8, 8, 8}, [4]float32{1.5, 1.5, 1.5, 1.5}, true},
		// per-corner: only the left corners, the top side limits (4/8)
		{image.Rect(0, 0, 4, 40), [4]float32{8, 0, 0, 8}, [4]float32{4, 0, 0, 4}, true},
		// a card, a pill: unchanged
		{image.Rect(0, 0, 300, 80), [4]float32{8, 8, 8, 8}, [4]float32{8, 8, 8, 8}, false},
		{image.Rect(0, 0, 60, 20), [4]float32{10, 10, 10, 10}, [4]float32{10, 10, 10, 10}, false},
		{image.Rect(0, 0, 4, 40), [4]float32{}, [4]float32{}, false},
	} {
		got, scaled := cssRadii(c.r, c.in)
		if got != c.want || scaled != c.scaled {
			t.Errorf("cssRadii(%v, %v) = %v, %v; want %v, %v", c.r, c.in, got, scaled, c.want, c.scaled)
		}
	}
}

// The coloured bar beside an attachment (a Dokploy/Uptime-Kuma style bot
// post: green bar, a "Time" field) and a quote's bar are 4px boxes with
// border-radius 8px, which Slack draws as a 4px stadium (2px round ends,
// flush with the block's top and bottom). clip.RRect with radius 8 on a
// 4px box crossed itself: horns past both ends of the bar.
func TestSideBarEnds(t *testing.T) {
	green := color.NRGBA{R: 0x2e, G: 0xb8, B: 0x86, A: 0xff}
	st := newTestStyle()
	// a quote bar dark enough to tell a rounded end pixel from a solid one
	// (#ddd on white is only 34 levels apart)
	st.Palette.QuoteBar = color.NRGBA{R: 0x12, G: 0x64, B: 0xa3, A: 0xff}
	quote := doc(rt.Block{Kind: rt.Quote, Children: []rt.Block{para(txt("quoted line one")), para(txt("quoted line two"))}})
	for _, c := range []struct {
		name string
		d    rt.Doc
		col  color.NRGBA
	}{
		{"attachment fields", attDoc("", slack.Attachment{ID: 1, Color: "#2eb886", Fields: []slack.AttachmentField{{Title: "Time", Value: "Sat, 15 Nov 2026 10:00:00"}}}), green},
		{"attachment text", attDoc("", slack.Attachment{ID: 1, Color: "#2eb886", Title: "Server restarted", Text: "all good"}), green},
		{"quote", quote, st.Palette.QuoteBar},
	} {
		t.Run(c.name, func(t *testing.T) {
			const w, pad = 400, 20
			v := layoutAt(st, c.d, Options{}, w-2*pad)
			var br image.Rectangle
			for _, b := range v.d.boxes {
				if b.kind == bFill && b.fill == c.col && b.r.Dx() == 4 {
					br = b.r
					break
				}
			}
			if br.Dy() < 16 {
				t.Fatalf("no 4px bar in %+v", v.d.boxes)
			}
			h := v.d.size.Y + 2*pad
			var ops op.Ops
			paint.FillShape(&ops, st.Palette.Panel, clip.Rect{Max: image.Pt(w, h)}.Op())
			tr := op.Offset(image.Pt(pad, pad)).Push(&ops)
			v.Layout(testGtx(&ops, w-2*pad), st, &recHandler{})
			tr.Pop()
			img := pixeltest.Render(t, w, h, &ops)
			br = br.Add(image.Pt(pad, pad))

			// nothing of the bar's colour beside it or past its ends
			for _, out := range []image.Rectangle{
				image.Rect(br.Min.X-8, br.Min.Y-8, br.Min.X, br.Max.Y+8),
				image.Rect(br.Max.X, br.Min.Y-8, br.Max.X+8, br.Max.Y+8),
				image.Rect(br.Min.X, br.Min.Y-8, br.Max.X, br.Min.Y),
				image.Rect(br.Min.X, br.Max.Y, br.Max.X, br.Max.Y+8),
			} {
				if n := pixeltest.Ink(img, out, c.col, st.Palette.Panel); n > 0 {
					t.Errorf("%d px of bar colour in %v outside the bar %v", n, out, br)
				}
			}
			// solid between the 2px ends
			body := image.Rect(br.Min.X, br.Min.Y+2, br.Max.X, br.Max.Y-2)
			if n := pixeltest.Count(img, body, c.col, 4); n != body.Dx()*body.Dy() {
				t.Errorf("bar body %v: %d of %d px solid", body, n, body.Dx()*body.Dy())
			}
			// round 2px ends: the first and last rows are partly covered
			// (no pixel solid, some ink), flush with the bar's box
			for _, row := range []image.Rectangle{
				image.Rect(br.Min.X, br.Min.Y, br.Max.X, br.Min.Y+1),
				image.Rect(br.Min.X, br.Max.Y-1, br.Max.X, br.Max.Y),
			} {
				if n := pixeltest.Count(img, row, c.col, 4); n > 0 {
					t.Errorf("end row %v: %d solid px, want a rounded end", row, n)
				}
				if n := pixeltest.Ink(img, row, c.col, st.Palette.Panel); n == 0 {
					t.Errorf("end row %v: no ink, the bar is short of its box", row)
				}
			}
		})
	}
}

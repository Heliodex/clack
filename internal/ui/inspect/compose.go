package inspect

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The image Karim pastes into #clack: the picked element as it was drawn
// (Shoot: at ShotDensity px per dp) with CropMargin dp around it, that
// margin blurred past reading so nothing else in the workspace comes
// along, the element outlined, and under it a panel with the report's
// rows. It is drawn on the CPU with the Go fonts (already in the binary
// for the demo's art), so it needs no GPU and no UI goroutine, and a test
// sees the same pixels CI does.

var (
	// outlineCol is the pick's outline: no Slack colour, so it can't be
	// read as part of the UI.
	outlineCol = color.NRGBA{R: 0xff, G: 0x2e, B: 0x93, A: 0xff}
	haloCol    = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xe6}
	panelBg    = color.NRGBA{R: 0x16, G: 0x17, B: 0x1a, A: 0xff}
	panelKey   = color.NRGBA{R: 0x9e, G: 0xa0, B: 0xa6, A: 0xff}
	panelText  = color.NRGBA{R: 0xf2, G: 0xf2, B: 0xf3, A: 0xff}
)

var fonts struct {
	once      sync.Once
	reg, bold *opentype.Font
	err       error
}

func loadFonts() error {
	fonts.once.Do(func() {
		if fonts.reg, fonts.err = opentype.Parse(goregular.TTF); fonts.err != nil {
			return
		}
		fonts.bold, fonts.err = opentype.Parse(gobold.TTF)
	})
	return fonts.err
}

// PanelMinWidth is the narrowest the report panel gets, in the panel's
// dp (its text is 12 of them): a small element's image is this wide, the
// element at its own size centred above the panel.
const PanelMinWidth = 420

// MarginBlur is how far the margin around the element is blurred, dp: a
// box blur this far each way, three times (near a Gaussian of this
// sigma), so a neighbour's 15 dp text is a smear.
const MarginBlur = 5

// Compose draws shot (the element and the margin around it, px) with
// elem (px, in shot) outlined and everything outside it blurred, over
// the report panel for lines; scale is the shot's px per dp, which sizes
// the outline, the blur and the panel's text.
func Compose(shot *image.RGBA, elem image.Rectangle, scale float32, lines []Line) (*image.RGBA, error) {
	if err := loadFonts(); err != nil {
		return nil, err
	}
	if scale <= 0 {
		scale = 1
	}
	px := clampF(12*scale, 11, 30)
	u := px / 12 // the panel's px per dp
	sb := shot.Bounds()
	W := max(sb.Dx(), int(PanelMinWidth*u))
	reg, err := opentype.NewFace(fonts.reg, &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer reg.Close()
	bold, err := opentype.NewFace(fonts.bold, &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer bold.Close()

	pad := int(16 * u)
	gap := int(12 * u)
	lh := int(math.Ceil(float64(px) * 1.45))
	keyW := 0
	for _, l := range lines {
		keyW = max(keyW, font.MeasureString(bold, l.Key).Ceil())
	}
	valX := pad + keyW + gap
	valW := max(W-valX-pad, 8*int(px))
	type row struct {
		key   string
		parts []string
	}
	rows := make([]row, 0, len(lines))
	n := 0
	for _, l := range lines {
		parts := wrap(reg, l.Value, valW)
		rows = append(rows, row{l.Key, parts})
		n += len(parts)
	}
	panelH := 2*pad + n*lh
	band := max(int(3*scale), 2)

	out := image.NewRGBA(image.Rect(0, 0, W, sb.Dy()+band+panelH))
	draw.Draw(out, out.Bounds(), image.NewUniform(panelBg), image.Point{}, draw.Src)
	// the shot at its own size, centred: all of it blurred, then the
	// element itself drawn sharp again
	at := image.Pt((W-sb.Dx())/2, 0)
	dst := image.Rectangle{Min: at, Max: at.Add(sb.Size())}
	draw.Draw(out, dst, shot, sb.Min, draw.Src)
	blur(out, dst, max(int(math.Round(float64(MarginBlur*scale))), 1))
	el := elem.Sub(sb.Min).Add(at)
	if c := el.Intersect(dst); !c.Empty() {
		draw.Draw(out, c, shot, c.Min.Sub(at).Add(sb.Min), draw.Src)
	}
	outline(out, el, scale)
	top := sb.Dy()
	draw.Draw(out, image.Rect(0, top, W, top+band), image.NewUniform(outlineCol), image.Point{}, draw.Src)
	top += band

	asc := reg.Metrics().Ascent.Ceil()
	y := top + pad + (lh-int(px))/2 + asc - int(px)/8
	for _, r := range rows {
		(&font.Drawer{Dst: out, Src: image.NewUniform(panelKey), Face: bold, Dot: fixed.P(pad, y)}).DrawString(r.key)
		for _, p := range r.parts {
			(&font.Drawer{Dst: out, Src: image.NewUniform(panelText), Face: reg, Dot: fixed.P(valX, y)}).DrawString(p)
			y += lh
		}
	}
	return out, nil
}

// outline strokes r on img: a white halo, then the pick's colour, and a
// faint wash of it inside.
func outline(img *image.RGBA, r image.Rectangle, scale float32) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	sw := max(int(math.Round(float64(2*scale))), 2)
	hw := max(int(math.Round(float64(scale/2))), 1) // the halo
	wash := outlineCol
	wash.A = 0x1c
	blendRect(img, r, wash)
	ring(img, r.Inset(-sw-hw), hw, haloCol)
	ring(img, r.Inset(-sw), sw, outlineCol)
	ring(img, r, hw, haloCol)
}

// blur box-blurs img inside r three times, rad px each way, rows then
// columns (edges clamped: nothing outside r comes in).
func blur(img *image.RGBA, r image.Rectangle, rad int) {
	r = r.Intersect(img.Bounds())
	w, h := r.Dx(), r.Dy()
	if w == 0 || h == 0 || rad <= 0 {
		return
	}
	n := max(w, h)
	line := make([]uint8, 4*n)
	tmp := make([]uint8, 4*n)
	d := 2*rad + 1
	// pass blurs line's first l pixels once
	pass := func(l int) {
		for c := 0; c < 4; c++ {
			at := func(i int) int { return int(line[4*min(max(i, 0), l-1)+c]) }
			sum := 0
			for i := -rad; i <= rad; i++ {
				sum += at(i)
			}
			for i := 0; i < l; i++ {
				tmp[4*i+c] = uint8((sum + d/2) / d)
				sum += at(i+rad+1) - at(i-rad)
			}
		}
		copy(line[:4*l], tmp[:4*l])
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		o := img.PixOffset(r.Min.X, y)
		copy(line, img.Pix[o:o+4*w])
		for range 3 {
			pass(w)
		}
		copy(img.Pix[o:o+4*w], line[:4*w])
	}
	for x := r.Min.X; x < r.Max.X; x++ {
		for y := range h {
			o := img.PixOffset(x, r.Min.Y+y)
			copy(line[4*y:4*y+4], img.Pix[o:o+4])
		}
		for range 3 {
			pass(h)
		}
		for y := range h {
			o := img.PixOffset(x, r.Min.Y+y)
			copy(img.Pix[o:o+4], line[4*y:4*y+4])
		}
	}
}

// ring strokes w px inside r.
func ring(img *image.RGBA, r image.Rectangle, w int, c color.NRGBA) {
	if r.Empty() {
		return
	}
	blendRect(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), c)
	blendRect(img, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), c)
	blendRect(img, image.Rect(r.Min.X, r.Min.Y+w, r.Min.X+w, r.Max.Y-w), c)
	blendRect(img, image.Rect(r.Max.X-w, r.Min.Y+w, r.Max.X, r.Max.Y-w), c)
}

// blendRect draws c over r (source over).
func blendRect(img *image.RGBA, r image.Rectangle, c color.NRGBA) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Over)
}

// wrap breaks s into lines at most w px wide in face, at spaces, or
// anywhere inside a word wider than w.
func wrap(face font.Face, s string, w int) []string {
	if s == "" {
		return []string{""}
	}
	var out []string
	line := ""
	for _, word := range strings.Split(s, " ") {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if font.MeasureString(face, try).Ceil() <= w {
			line = try
			continue
		}
		if line != "" {
			out = append(out, line)
			line = ""
		}
		for font.MeasureString(face, word).Ceil() > w {
			// a word wider than the column: as many runes as fit
			rs := []rune(word)
			k := len(rs)
			for k > 1 && font.MeasureString(face, string(rs[:k])).Ceil() > w {
				k--
			}
			out = append(out, string(rs[:k]))
			word = string(rs[k:])
		}
		line = word
	}
	return append(out, line)
}

func clampF(v, lo, hi float32) float32 {
	return min(max(v, lo), hi)
}

package richview

import (
	"encoding/json"
	"testing"

	rt "github.com/some-du6e/clack/internal/richtext"
	"github.com/some-du6e/clack/internal/slack"
)

// An image Slack made no thumbnails for (a 828×25208 phone screenshot
// comes with no thumb_* and no original_w/h) isn't fileIsImage in Slack,
// so FileGallery draws it as a file card (SINGLE_FILE, getFileType
// "generic"): Clack lays out the same card and never loads the original
// (a blank 108px tile before: imgcache refuses a side past MaxSide). The
// same image with its size but no thumbs too; a thumbnailed one stays a
// gallery tile from its thumb.
func TestUnthumbedImageIsFileCard(t *testing.T) {
	st := newTestStyle()
	docOf := func(file string) rt.Doc {
		t.Helper()
		var f slack.File
		if err := json.Unmarshal([]byte(file), &f); err != nil {
			t.Fatal(err)
		}
		m := slack.Message{Type: "message", TS: "1762700000.000100", User: "U1", Upload: true, Files: []slack.File{f}}
		return rt.FromMessage(codeRes{}, &m)
	}
	const base = `"mimetype":"image/png","filetype":"png","pretty_type":"PNG","size":3272505,"mode":"hosted",` +
		`"url_private":"https://files.slack.com/files-pri/T1-F1/img_0001.png","permalink":"https://ws.slack.com/files/U1/F1/img_0001.png",` +
		`"media_display_type":"unknown","file_access":"visible","name":"IMG_0001.png","title":"IMG_0001"`
	for _, tc := range []struct{ name, js string }{
		{"no size", `{"id":"F1",` + base + `}`},
		{"with its size", `{"id":"F1",` + base + `,"original_w":828,"original_h":25208}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := layoutAt(st, docOf(tc.js), Options{CompactFiles: true}, 600)
			if len(v.d.imgs) != 0 {
				t.Errorf("%d images drawn (%s), want none: Slack draws a file card", len(v.d.imgs), v.d.imgs[0].url)
			}
			rs := v.FileRects()
			if len(rs) != 1 || rs[0].File.IsImage || rs[0].File.Thumb != nil {
				t.Fatalf("file rects %+v, want the one card", rs)
			}
			// genericFile: the 36px icon, 12px padding, 2px of border
			if want := 36 + 2*12 + 2; rs[0].R.Dy() != want || v.d.size.Y != want {
				t.Errorf("card %v, view %v: want %dpx tall", rs[0].R, v.d.size, want)
			}
			if rs[0].File.URL == "" || rs[0].File.Permalink == "" {
				t.Errorf("the card lost its file's links: %+v", rs[0].File)
			}
		})
	}

	// thumbnailed: the gallery tile from thumb_360, unchanged
	v := layoutAt(st, docOf(`{"id":"F2",`+base+`,"original_w":828,"original_h":1792,`+
		`"thumb_360":"https://files.slack.com/files-tmb/T1-F2/360.png","thumb_360_w":166,"thumb_360_h":360}`), Options{CompactFiles: true}, 600)
	if len(v.d.imgs) != 1 || v.d.imgs[0].url != "https://files.slack.com/files-tmb/T1-F2/360.png" {
		t.Fatalf("thumbnailed image: %+v, want its thumb_360 tile", v.d.imgs)
	}
	// gallery 574 (medium): portrait cap 276 → 127x276
	if r := v.d.imgs[0].r; r.Dx() != 127 || r.Dy() != 276 {
		t.Errorf("thumbnailed tile %v, want 127x276", r)
	}
}

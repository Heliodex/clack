package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/some-du6e/clack/internal/fakeslack"
)

// A search item's IsImage is Slack's fileIsImage (slack.FileIsImage), the
// check the Files browser and the channel Files tab open the viewer by: an
// image/* file Slack made no thumbs for is a plain file.
func TestFilesViewFileIsImage(t *testing.T) {
	for _, c := range []struct {
		name, raw string
		image     bool
		thumb     string
	}{
		{"thumbed png", `{"id":"F1","mimetype":"image/png","filetype":"png","thumb_360":"t360"}`, true, "t360"},
		{"gif with only thumb_360_gif", `{"id":"F2","mimetype":"image/gif","filetype":"gif","thumb_360_gif":"g360"}`, true, "g360"},
		{"heic with thumb_800", `{"id":"F3","mimetype":"image/heic","filetype":"heic","thumb_800":"t800"}`, true, ""},
		{"salesforce image", `{"id":"F4","mimetype":"image/png","filetype":"png","is_salesforce_file":true}`, true, ""},
		{"828x25208 screenshot", `{"id":"F5","mimetype":"image/png","filetype":"png","url_private":"orig","original_w":828,"original_h":25208}`, false, ""},
		{"pdf", `{"id":"F6","mimetype":"application/pdf","filetype":"pdf","thumb_pdf":"tpdf"}`, false, "tpdf"},
	} {
		var r browseFileRaw
		if err := json.Unmarshal([]byte(c.raw), &r); err != nil {
			t.Fatal(err)
		}
		f := r.file()
		if f.IsImage != c.image || f.Thumb != c.thumb {
			t.Errorf("%s: IsImage %v Thumb %q, want %v %q", c.name, f.IsImage, f.Thumb, c.image, c.thumb)
		}
	}
}

// Against fakeslack: the un-thumbnailed #files-gallery images come back in
// the tab's media and the browser's Images, as Slack's search returns
// them, but not as images.
func TestChannelFilesUnthumbedImages(t *testing.T) {
	a, fs := channelFilesFake(t)
	ctx := context.Background()
	unthumbed := map[string]bool{}
	for _, name := range []string{fakeslack.FilesUnthumbedImage, fakeslack.FilesUnthumbedSized} {
		for _, f := range fs.FilesCaseFiles(name) {
			unthumbed[f.ID] = true
		}
	}
	if len(unthumbed) != 2 {
		t.Fatalf("unthumbed fixtures %v", unthumbed)
	}
	check := func(what string, files []File) {
		t.Helper()
		seen, images := 0, 0
		for _, f := range files {
			switch {
			case unthumbed[f.ID]:
				seen++
				if f.IsImage || f.Thumb != "" || f.Permalink == "" {
					t.Errorf("%s: %s IsImage %v Thumb %q Permalink %q, want a plain file with its page", what, f.ID, f.IsImage, f.Thumb, f.Permalink)
				}
			case f.IsImage:
				images++
				if f.Thumb == "" {
					t.Errorf("%s: image %s has no thumb", what, f.ID)
				}
			}
		}
		if seen != len(unthumbed) || images == 0 {
			t.Errorf("%s: %d of the %d unthumbed images, %d thumbed images", what, seen, len(unthumbed), images)
		}
	}
	media, err := a.ChannelFiles(ctx, ChannelFilesOpts{Channel: fakeslack.FilesChannelID, Media: true})
	if err != nil {
		t.Fatal(err)
	}
	check("channel media", media.Files)
	browser, err := a.Files(ctx, FilesOpts{Types: []string{"images"}})
	if err != nil {
		t.Fatal(err)
	}
	check("browser Images", browser.Files)
}

package slack

import "testing"

// FileIsImage is 2HRR's fileIsImage: image/* plus thumb_360 or
// thumb_360_gif, a HEIC/HEIF's thumb_800, or a Salesforce file.
func TestFileIsImage(t *testing.T) {
	for _, c := range []struct {
		name string
		f    *File
		want bool
	}{
		{"thumbed png", &File{Mimetype: "image/png", Filetype: "png", Thumb360: "t360"}, true},
		{"gif with only its animated thumb", &File{Mimetype: "image/gif", Filetype: "gif", Thumb360GIF: "t360gif"}, true},
		{"png Slack made no thumbs for", &File{Mimetype: "image/png", Filetype: "png", URLPrivate: "orig", OriginalW: 828, OriginalH: 25208}, false},
		{"heic with thumb_800", &File{Mimetype: "image/heic", Filetype: "heic", Thumb800: "t800"}, true},
		{"heif with thumb_800", &File{Mimetype: "image/heif", Filetype: "heif", Thumb800: "t800"}, true},
		{"png with thumb_800 only", &File{Mimetype: "image/png", Filetype: "png", Thumb800: "t800"}, false},
		{"salesforce image", &File{Mimetype: "image/png", Filetype: "png", IsSalesforceFile: true}, true},
		{"pdf with a thumb_360", &File{Mimetype: "application/pdf", Filetype: "pdf", Thumb360: "t360"}, false},
		{"no mimetype", &File{Filetype: "png", Thumb360: "t360"}, false},
		{"nil", nil, false},
	} {
		if got := FileIsImage(c.f); got != c.want {
			t.Errorf("%s: FileIsImage = %v, want %v", c.name, got, c.want)
		}
	}
}

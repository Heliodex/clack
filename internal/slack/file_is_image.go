package slack

import "strings"

// FileIsImage is Slack's fileIsImage (2HRR, build c07ab895506f): an
// image/* file that is a Salesforce file, or one Slack made a thumb_360
// (or the animated thumb_360_gif) for, or a HEIC/HEIF with thumb_800.
// Slack makes no thumbs at all for some huge images (a 828×25208 phone
// screenshot has no thumb_* and no original_w/h), and every surface that
// asks fileIsImage treats them as plain files: a message's FileGallery
// draws a file card (getFileType "generic"), and getMainFileAction gives
// them "download" rather than "view", so the Files browser and a
// channel's Files tab never open them in the media viewer (which couldn't
// show an original that size anyway).
//
// Slack's own check starts with shouldShowFile; callers drop the files it
// would hide before they get here.
func FileIsImage(f *File) bool {
	if f == nil || !strings.Contains(f.Mimetype, "image/") {
		return false
	}
	t := strings.ToLower(f.Filetype)
	return f.IsSalesforceFile || f.Thumb360 != "" || f.Thumb360GIF != "" || (t == "heic" || t == "heif") && f.Thumb800 != ""
}

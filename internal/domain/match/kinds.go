package match

import (
	"path"
	"strings"
)

// kind is what a file holds, as its extension tells.
type kind int8

// A still can also be the photo of a Live Photo or of a RAW pair, and a MOV
// the video of a Live Photo; an image or another video is always an asset of
// its own. Photos does not take a source. A sidecar only stays in the cloud
// and moves with the media file of its stem.
const (
	unlisted kind = iota
	still
	image
	raw
	mov
	video
	source
	sidecar
)

// Photos takes the stills, images, RAWs and videos listed here; a file with
// an extension that is not listed stays in the cloud and is reported.
var kinds = map[string]kind{
	".heic": still, ".heif": still, ".jpg": still, ".jpeg": still,
	".png": image, ".gif": image, ".tif": image, ".tiff": image,
	".dng": raw, ".cr2": raw, ".cr3": raw, ".nef": raw, ".arw": raw,
	".mov": mov, ".mp4": video, ".m4v": video,
	".mpg": source, ".mpeg": source, ".vob": source, ".3gp": source, ".avi": source,
	".mts": source, ".m2ts": source, ".wmv": source, ".mkv": source,
	".xmp": sidecar, ".thm": sidecar, ".aae": sidecar,
	".ifo": sidecar, ".bup": sidecar, ".zip": sidecar, ".exe": sidecar, ".rar": sidecar,
}

func kindOf(name string) kind {
	return kinds[strings.ToLower(path.Ext(name))]
}

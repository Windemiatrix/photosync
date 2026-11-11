package naming

import (
	"path"
	"strings"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Companion returns the name of the file of role that goes with primary, the
// name of the original of the same asset in the same directory: the stem of
// primary with the extension of resourceName as path.Ext gives it, or with
// ".mov" for the video of a Live Photo whatever the resource is called, as
// osxphotos export names it. The result satisfies IsValidPart. It keeps the
// stem, and so the export.StemKey, of a valid primary unless the name would
// be longer than 255 bytes; then its end is cut.
func Companion(primary string, role domain.ResourceRole, resourceName string) string {
	ext := path.Ext(resourceName)
	if role == domain.ResourceLiveVideo {
		ext = ".mov"
	}
	return SanitizePart(strings.TrimSuffix(primary, path.Ext(primary)) + ext)
}

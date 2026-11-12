package match_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// Only the extension of the last element tells a sidecar, in any case; a
// media file and a file of an unknown kind are not one.
func TestIsSidecar(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		want bool
	}{
		{"IMG_0001.xmp", true},
		{"IMG_0001.XMP", true},
		{"IMG_0001.thm", true},
		{"IMG_0001.AAE", true},
		{"VTS_01_0.IFO", true},
		{"VTS_01_0.bup", true},
		{"IMG_0001.zip", true},
		{"setup.exe", true},
		{"IMG_0001.rar", true},
		{"2024/05-01 - Holidays/IMG_0001.xmp", true},
		{"IMG_0001.HEIC", false},
		{"IMG_0001.png", false},
		{"IMG_0001.DNG", false},
		{"IMG_0001.MOV", false},
		{"IMG_0001.mp4", false},
		{"MVI_0001.AVI", false},
		{"IMG_0001.txt", false},
		{"IMG_0001.xmp.HEIC", false},
		{"xmp", false},
		{"2024/05-02 - St. xmp/IMG_0001", false},
	} {
		if got := match.IsSidecar(c.name); got != c.want {
			t.Errorf("IsSidecar(%q) = %v; want %v", c.name, got, c.want)
		}
	}
}

package match_test

import (
	"cmp"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// Written from what makes one copy of a shot better than another, not from
// the code: the pixels first, then a RAW, then the bytes.
func TestCompareCopies(t *testing.T) {
	t.Parallel()
	original := func(name string, width, height int, size int64) domain.Asset {
		return domain.Asset{
			OriginalName: name, Width: width, Height: height,
			Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
		}
	}
	copyOf := func(p string, width, height int, size int64) domain.RemoteEntry {
		return domain.RemoteEntry{Root: domain.LibraryShared, Path: p, Size: size, Meta: domain.MediaMeta{Width: width, Height: height}}
	}
	livePhoto := original("IMG_0001.HEIC", 4032, 3024, 100)
	livePhoto.Resources = append([]domain.Resource{{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 300}}, livePhoto.Resources...)
	for _, c := range []struct {
		name  string
		asset domain.Asset
		file  domain.RemoteEntry
		want  int
	}{
		{"more pixels in Photos", original("IMG_0001.HEIC", 4032, 3024, 100), copyOf("a/IMG_0001.JPG", 2016, 1512, 200), 1},
		{"more pixels in the cloud", original("IMG_0001.HEIC", 2016, 1512, 200), copyOf("a/IMG_0001.JPG", 4032, 3024, 100), -1},
		{"the frame turned", original("IMG_0001.HEIC", 4032, 3024, 100), copyOf("a/IMG_0001.JPG", 3024, 4032, 200), -1},
		{"no frame in Photos", original("IMG_0001.HEIC", 0, 0, 200), copyOf("a/IMG_0001.JPG", 4032, 3024, 100), 1},
		{"no frame in the cloud", original("IMG_0001.HEIC", 4032, 3024, 100), copyOf("a/IMG_0001.JPG", 0, 0, 200), -1},
		{"a RAW in Photos", original("IMG_0001.DNG", 4032, 3024, 100), copyOf("a/IMG_0001.JPG", 4032, 3024, 200), 1},
		{"a RAW in the cloud", original("IMG_0001.JPG", 4032, 3024, 200), copyOf("a/IMG_0001.NEF", 4032, 3024, 100), -1},
		{"pixels before a RAW", original("IMG_0001.JPG", 4032, 3024, 100), copyOf("a/IMG_0001.DNG", 2016, 1512, 200), 1},
		{"two RAWs", original("IMG_0001.DNG", 4032, 3024, 100), copyOf("a/IMG_0001.dng", 4032, 3024, 200), -1},
		{"more bytes in Photos", original("IMG_0001.HEIC", 4032, 3024, 200), copyOf("a/IMG_0001.JPG", 4032, 3024, 100), 1},
		{"the size of the original", livePhoto, copyOf("a/IMG_0001.JPG", 4032, 3024, 200), -1},
		{"an image is no RAW", original("IMG_0001.TIF", 4032, 3024, 100), copyOf("a/IMG_0001.JPG", 4032, 3024, 200), -1},
		{"the area, not the sides", original("IMG_0001.HEIC", 4000, 3000, 100), copyOf("a/IMG_0001.JPG", 6000, 1500, 200), 1},
		{"the same copy", original("IMG_0001.HEIC", 4032, 3024, 100), copyOf("a/IMG_0001.HEIC", 4032, 3024, 100), 0},
	} {
		if got := match.CompareCopies(c.asset, c.file); cmp.Compare(got, 0) != c.want {
			t.Errorf("%s: CompareCopies = %d; want the sign of %d", c.name, got, c.want)
		}
	}
}

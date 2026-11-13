package domain_test

import (
	"cmp"
	"reflect"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// The plan file and the journal of a run store these strings: changing one
// breaks the plans and journals already written.
func TestOpKinds(t *testing.T) {
	t.Parallel()
	checkStored(t, []domain.OpKind{domain.OpDelete, domain.OpAlbumAdd, domain.OpAlbumRemove, domain.OpAssetDelete},
		"delete", "album_add", "album_remove", "asset_delete")
}

// The phases are the order in which a run makes its changes: a wrong phase
// runs a change before the ones that it must follow.
func TestOpKindPhase(t *testing.T) {
	t.Parallel()
	for k, want := range map[domain.OpKind]int{
		domain.OpDelete: 1, domain.OpAlbumAdd: 4, domain.OpAlbumRemove: 4, domain.OpAssetDelete: 5,
	} {
		if got := k.Phase(); got != want {
			t.Errorf("%s.Phase() = %d, want %d", k, got, want)
		}
	}
}

// Every plan lists its operations in this order and a run takes them so.
// Each pair is told apart by the first key of the order on which it differs,
// so a skipped key or a wrong sign reorders a pair. No pair here yet fixes
// Kind before Root and Src, which needs two kinds of cloud operation in one
// phase, or Remote before Kind, which needs a cloud and an iCloud operation
// in one phase, as album_create will give.
func TestOpCompare(t *testing.T) {
	t.Parallel()
	const holidays, trip = "2024/05-01 - Holidays/", "2024/05-02 - Trip/"
	ordered := []domain.Op{
		{Kind: domain.OpDelete, Remote: "alpha", Root: domain.LibraryPersonal, Src: holidays + "IMG_0002.HEIC", Asset: 8},
		{Kind: domain.OpDelete, Remote: "alpha", Root: domain.LibraryPersonal, Src: trip + "IMG_0001.HEIC", Asset: 7},
		{Kind: domain.OpDelete, Remote: "alpha", Root: domain.LibraryShared, Src: holidays + "IMG_0001.HEIC", Asset: 7},
		{Kind: domain.OpDelete, Remote: "beta", Root: domain.LibraryPersonal, Src: holidays + "IMG_0001.HEIC", Asset: 7},
		{Kind: domain.OpAlbumAdd, Asset: 7, AlbumID: 10},
		{Kind: domain.OpAlbumAdd, Asset: 7, AlbumID: 11},
		{Kind: domain.OpAlbumAdd, Asset: 8, AlbumID: 10},
		{Kind: domain.OpAlbumRemove, Asset: 7, AlbumID: 10},
		{Kind: domain.OpAssetDelete, Asset: 7},
	}
	for i, a := range ordered {
		for j, b := range ordered {
			if got, want := cmp.Compare(a.Compare(b), 0), cmp.Compare(i, j); got != want {
				t.Errorf("ops %d and %d: Compare has sign %d, want %d", i+1, j+1, got, want)
			}
		}
	}
	sorted := slices.Clone(ordered)
	slices.Reverse(sorted)
	slices.SortFunc(sorted, domain.Op.Compare)
	if !reflect.DeepEqual(sorted, ordered) {
		t.Errorf("sorted %+v, want %+v", sorted, ordered)
	}
}

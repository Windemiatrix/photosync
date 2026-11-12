package refmodel_test

import (
	"reflect"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/refmodel"
)

func TestCanon(t *testing.T) {
	t.Parallel()

	const (
		new8 = tripDir + "/NEW_0008.HEIC"
		new9 = trip2Dir + "/NEW_0009.HEIC"
	)
	uploaded := refmodel.Object{Album: trip, Name: "NEW_0008.HEIC", Size: 5_000_008}
	// other differs from its file in name and size, so that an asset made
	// anew from its file shows.
	other := refmodel.Object{Album: trip2, Name: "IMG_000999.HEIC", Size: 5_000_019}
	m := refmodel.Model{Base: before(), Now: unlinked(before())}
	m.Now.Objects[key(8)] = uploaded
	same := func(*refmodel.World) {}

	for _, c := range []struct {
		name        string
		edit, canon func(w *refmodel.World)
	}{
		{
			"assets that Now has, an upload among them",
			func(w *refmodel.World) {
				w.Objects[key(8)] = uploaded
				w.Files[new8] = refmodel.File{Key: key(8), Size: 5_000_008}
			},
			same,
		},
		{
			"an asset that Now lacks",
			func(w *refmodel.World) {
				w.Objects[key(9)] = other
				w.Files[new9] = refmodel.File{Key: key(9), Size: 5_000_009}
			},
			func(w *refmodel.World) {
				delete(w.Objects, key(9))
				w.Objects[domain.AssetKey{UUID: new9}] = other
				w.Files[new9] = refmodel.File{Key: domain.AssetKey{UUID: new9}, Size: 5_000_009}
			},
		},
		{"an asset that Now lacks and no file links", func(w *refmodel.World) { w.Objects[key(9)] = other }, same},
		{"a file without a link", func(w *refmodel.World) { w.Files[new9] = refmodel.File{Size: 5_000_009} }, same},
		{"a link to no asset", func(w *refmodel.World) { w.Files[new9] = refmodel.File{Key: key(9), Size: 5_000_009} }, same},
		{"an asset keyed by the path of its file", func(w *refmodel.World) { imported(w, new9, trip2, 5_000_009) }, same},
	} {
		w := before()
		c.edit(&w)
		was, want := cloneWorld(w), cloneWorld(w)
		c.canon(&want)
		if got := m.Canon(w); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, want)
		}
		if !reflect.DeepEqual(w, was) {
			t.Errorf("%s: Canon left w %+v; want %+v", c.name, w, was)
		}
	}
}

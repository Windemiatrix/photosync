package domain_test

import (
	"cmp"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// New assets get their IDs in this order, and every plan breaks its last tie
// by it, so a wrong sign or a skipped field reorders the IDs and the plans.
func TestAssetKeyCompare(t *testing.T) {
	t.Parallel()
	ordered := []domain.AssetKey{
		{CloudAssetGUID: "A", UUID: "9"},
		{CloudAssetGUID: "B", UUID: "1"},
		{CloudAssetGUID: "B", UUID: "2"},
		{CloudAssetGUID: "a", UUID: "0"},
		{UUID: "1"},
		{UUID: "2"},
	}
	for i, a := range ordered {
		for j, b := range ordered {
			if got, want := cmp.Compare(a.Compare(b), 0), cmp.Compare(i, j); got != want {
				t.Errorf("%+v.Compare(%+v) has sign %d, want %d", a, b, got, want)
			}
		}
	}
	sorted := slices.Clone(ordered)
	slices.Reverse(sorted)
	slices.SortFunc(sorted, domain.AssetKey.Compare)
	if !slices.Equal(sorted, ordered) {
		t.Errorf("sorted %+v, want %+v", sorted, ordered)
	}
}

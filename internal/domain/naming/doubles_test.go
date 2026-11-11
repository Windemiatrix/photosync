package naming_test

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

func TestDoubles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc  string
		paths []string
		want  [][]string
	}{
		{"none: nil", nil, nil},
		{"none: different names", []string{"a/IMG_0001.jpg", "a/IMG_0002.jpg"}, nil},
		{
			"case",
			[]string{"a/img_0001.JPG", "a/IMG_0001.jpg"},
			[][]string{{"a/IMG_0001.jpg", "a/img_0001.JPG"}},
		},
		{"extension case", []string{"a/x.jpg", "a/x.JPG"}, [][]string{{"a/x.JPG", "a/x.jpg"}}},
		{
			"NFC and NFD",
			[]string{"a/Café.jpg", "a/Café.jpg"},
			[][]string{{"a/Café.jpg", "a/Café.jpg"}},
		},
		{
			"letter without a composed capital",
			[]string{"a/t" + "̈.jpg", "a/T" + "̈.jpg"},
			[][]string{{"a/T" + "̈.jpg", "a/t" + "̈.jpg"}},
		},
		{"same path twice", []string{"a/x.jpg", "a/x.jpg"}, [][]string{{"a/x.jpg", "a/x.jpg"}}},
		{"directory case", []string{"A/x.jpg", "a/x.jpg"}, [][]string{{"A/x.jpg", "a/x.jpg"}}},
		{"same name in two directories", []string{"a/x.jpg", "b/x.jpg"}, nil},
		{
			"final sigma",
			[]string{"a/σ.jpg", "a/ς.jpg"},
			[][]string{{"a/ς.jpg", "a/σ.jpg"}},
		},
		{
			"no full case folding",
			[]string{"a/ß.jpg", "a/ss.jpg", "a/SS.jpg"},
			[][]string{{"a/SS.jpg", "a/ss.jpg"}},
		},
		{
			"groups and members in byte order",
			[]string{"b/X.jpg", "c/z.jpg", "a/y.jpg", "b/x.jpg", "a/Y.jpg", "b/x.JPG"},
			[][]string{{"a/Y.jpg", "a/y.jpg"}, {"b/X.jpg", "b/x.JPG", "b/x.jpg"}},
		},
		{
			"four groups",
			[]string{"d/x", "D/X", "c/x", "C/X", "b/x", "B/X", "a/x", "A/X"},
			[][]string{{"A/X", "a/x"}, {"B/X", "b/x"}, {"C/X", "c/x"}, {"D/X", "d/x"}},
		},
	}
	for _, tt := range tests {
		before := slices.Clone(tt.paths)
		got := naming.Doubles(tt.paths)
		if !slices.EqualFunc(got, tt.want, slices.Equal[[]string]) {
			t.Errorf("%s: Doubles(%q) = %q, want %q", tt.desc, tt.paths, got, tt.want)
		}
		if len(tt.want) == 0 && got != nil {
			t.Errorf("%s: Doubles(%q) = %#v, want nil", tt.desc, tt.paths, got)
		}
		if !slices.Equal(tt.paths, before) {
			t.Errorf("%s: Doubles changed its input to %q from %q", tt.desc, tt.paths, before)
		}
	}
}

func pathGen() *rapid.Generator[string] {
	dirs := rapid.SampledFrom([]string{"a", "A", "b"})
	names := rapid.SampledFrom([]string{
		"x.jpg", "X.JPG", "Café.jpg", "Café.jpg", "σ.jpg", "ς.jpg", "ß.jpg", "ss.jpg",
	})
	return rapid.Custom(func(t *rapid.T) string {
		return dirs.Draw(t, "dir") + "/" + names.Draw(t, "name")
	})
}

func TestDoublesProperties(t *testing.T) {
	t.Parallel()

	key := func(p string) string { return norm.NFC.String(strings.ToUpper(norm.NFD.String(p))) }

	t.Run("order of input does not matter", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			paths := rapid.SliceOfN(pathGen(), 0, 10).Draw(t, "paths")
			perm := rapid.Permutation(paths).Draw(t, "perm")
			want := naming.Doubles(paths)
			if got := naming.Doubles(perm); !slices.EqualFunc(got, want, slices.Equal[[]string]) {
				t.Fatalf("Doubles(%q) = %q, but Doubles(%q) = %q", paths, want, perm, got)
			}
		})
	})

	t.Run("complete partition", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			paths := rapid.SliceOfN(pathGen(), 0, 10).Draw(t, "paths")
			groups := naming.Doubles(paths)

			count := map[string]int{}
			for _, p := range paths {
				count[key(p)]++
			}
			wantMembers := 0
			for _, p := range paths {
				if count[key(p)] >= 2 {
					wantMembers++
				}
			}

			members := 0
			seen := map[string]bool{}
			for i, g := range groups {
				if len(g) < 2 {
					t.Fatalf("group %q of Doubles(%q) has fewer than two paths", g, paths)
				}
				if !slices.IsSorted(g) {
					t.Fatalf("group %q of Doubles(%q) is not in byte order", g, paths)
				}
				k := key(g[0])
				for _, p := range g {
					if key(p) != k {
						t.Fatalf("group %q of Doubles(%q) holds two keys", g, paths)
					}
				}
				if seen[k] {
					t.Fatalf("key %q is in two groups of Doubles(%q)", k, paths)
				}
				seen[k] = true
				if i > 0 && groups[i-1][0] >= g[0] {
					t.Fatalf("groups %q of Doubles(%q) are not in the byte order of their first paths", groups, paths)
				}
				members += len(g)
			}
			if members != wantMembers {
				t.Fatalf("Doubles(%q) = %q holds %d paths, want %d", paths, groups, members, wantMembers)
			}
		})
	})
}

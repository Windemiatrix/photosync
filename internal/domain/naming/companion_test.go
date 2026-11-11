package naming_test

import (
	"path"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

func TestCompanion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc, primary string
		role          domain.ResourceRole
		resource      string
		want          string
	}{
		{"Live takes .mov", "IMG_0001 (1).HEIC", domain.ResourceLiveVideo, "IMG_0001.MOV", "IMG_0001 (1).mov"},
		{"Live ignores the resource name", "IMG_0001.HEIC", domain.ResourceLiveVideo, "", "IMG_0001.mov"},
		{"RAW takes the resource extension", "IMG_0001 (1).JPG", domain.ResourceRAWPair, "IMG_0001.DNG", "IMG_0001 (1).DNG"},
		{"original is the primary", "IMG_0001 (1).HEIC", domain.ResourceOriginal, "IMG_0001.HEIC", "IMG_0001 (1).HEIC"},
		{"inner dots stay in the stem", "Holidays.v2.JPG", domain.ResourceRAWPair, "x.CR3", "Holidays.v2.CR3"},
		{"primary without extension", "IMG_0001", domain.ResourceRAWPair, "IMG_0001.DNG", "IMG_0001.DNG"},
		{"resource without extension", "IMG_0001.JPG", domain.ResourceRAWPair, "IMG_0001", "IMG_0001"},
		{"forbidden character", "IMG_0001.JPG", domain.ResourceRAWPair, "a.d:ng", "IMG_0001.d꞉ng"},
		{"leading dot of the resource", "IMG_0001.JPG", domain.ResourceRAWPair, ".DNG", "IMG_0001.DNG"},
		{"final dot", "IMG_0001.JPG", domain.ResourceRAWPair, "x.", "IMG_0001．"},
		{"final space", "IMG_0001.JPG", domain.ResourceRAWPair, "IMG_0001.DNG ", "IMG_0001.DNG "},
		{
			"too long",
			strings.Repeat("a", 252) + ".jp", domain.ResourceLiveVideo, "",
			strings.Repeat("a", 252) + ".mo",
		},
	}
	for _, tt := range tests {
		if !validName(tt.want) {
			t.Errorf("%s: want %q is not a valid name", tt.desc, tt.want)
		}
		if got := naming.Companion(tt.primary, tt.role, tt.resource); got != tt.want {
			t.Errorf("%s: Companion(%q, %q, %q) = %q, want %q", tt.desc, tt.primary, tt.role, tt.resource, got, tt.want)
		}
	}
}

func TestCompanionProperties(t *testing.T) {
	t.Parallel()

	roles := rapid.SampledFrom([]domain.ResourceRole{
		domain.ResourceOriginal, domain.ResourceLiveVideo, domain.ResourceRAWPair,
	})

	t.Run("result is valid", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			primary := naming.Free(nameGen().Draw(t, "primary"), nil)
			role := roles.Draw(t, "role")
			resource := nameGen().Draw(t, "resource")
			if got := naming.Companion(primary, role, resource); !validName(got) {
				t.Fatalf("Companion(%q, %q, %q) = %q, not a valid name", primary, role, resource, got)
			}
		})
	})

	t.Run("stem key is kept", func(t *testing.T) {
		t.Parallel()
		resources := rapid.SampledFrom([]string{"IMG_0001.DNG", "IMG_0001.MOV", "x.CR3", "IMG_0001.HEIC"})
		rapid.Check(t, func(t *rapid.T) {
			primary := naming.Free(nameGen().Draw(t, "primary"), nil)
			role := roles.Draw(t, "role")
			resource := resources.Draw(t, "resource")
			ext := path.Ext(resource)
			if role == domain.ResourceLiveVideo {
				ext = ".mov"
			}
			if len(strings.TrimSuffix(primary, path.Ext(primary)))+len(ext) > maxNameBytes {
				return
			}
			got := naming.Companion(primary, role, resource)
			if export.StemKey(got) != export.StemKey(primary) {
				t.Fatalf("Companion(%q, %q, %q) = %q, its stem key differs from the key of the primary",
					primary, role, resource, got)
			}
		})
	})
}

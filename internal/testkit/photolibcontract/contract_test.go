package photolibcontract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
	"github.com/Windemiatrix/photosync/internal/testkit/photolibcontract"
)

func TestFakePhotoLibrary(t *testing.T) {
	t.Parallel()
	photolibcontract.Run(t, func(t *testing.T) photolibcontract.Library {
		photo := filepath.Join(t.TempDir(), "IMG_0001.JPG")
		if err := os.WriteFile(photo, []byte("photo 1"), 0o600); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
		lib := testkit.NewFakePhotoLibrary(domain.LibrarySnapshot{})
		return photolibcontract.Library{
			PhotoLibrary: lib,
			Photo:        photo,
			Answer:       lib.Answer,
			Hold:         lib.Hold,
			SyncCloud:    lib.SyncCloud,
			Restore:      lib.Restore,
		}
	})
}

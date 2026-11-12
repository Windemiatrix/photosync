package testkit_test

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

func photoLibFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v, want nil", name, err)
	}
	return path
}

func TestFakePhotoLibraryImport(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	src := t.TempDir()
	heic := photoLibFile(t, src, "IMG_0004.HEIC", "photo 4")
	live := photoLibFile(t, src, "IMG_0004.MOV", "live video 4")
	raw := photoLibFile(t, src, "IMG_0004.DNG", "raw 4")
	video := photoLibFile(t, src, "IMG_0005.MOV", "video 5")

	reqA := domain.ImportRequest{
		Photo:        heic,
		LiveVideo:    live,
		RAWPair:      raw,
		OriginalName: "IMG_0104.HEIC",
		Taken:        time.Date(2024, 5, 2, 13, 0, 0, 0, time.FixedZone("", 3*60*60)),
		AlbumUUIDs:   []string{"album-1"},
	}
	if uuid, err := lib.Import(ctx, reqA); err != nil || uuid != "fake-uuid-1" {
		t.Fatalf("Import() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
	}
	if uuid, err := lib.Import(ctx, domain.ImportRequest{Video: video}); err != nil || uuid != "fake-uuid-2" {
		t.Fatalf("Import() of a video = %q, %v; want %q, nil", uuid, err, "fake-uuid-2")
	}
	reqA.AlbumUUIDs[0] = "album-9"
	photoLibFile(t, src, "IMG_0004.HEIC", "changed")

	got, err := lib.Snapshot(ctx, "")
	if err != nil {
		t.Fatalf("Snapshot() error = %v, want nil", err)
	}
	want := photoLibWant()
	want.Assets = append(want.Assets,
		domain.Asset{
			Key:          domain.AssetKey{UUID: "fake-uuid-1"},
			Library:      domain.LibraryPersonal,
			OriginalName: "IMG_0104.HEIC",
			Taken:        time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC),
			DateSource:   domain.DateICloud,
			Resources: []domain.Resource{
				{Role: domain.ResourceOriginal, Filename: "IMG_0104.HEIC", Size: 7},
				{Role: domain.ResourceLiveVideo, Filename: "IMG_0004.MOV", Size: 12},
				{Role: domain.ResourceRAWPair, Filename: "IMG_0004.DNG", Size: 5},
			},
			AlbumUUIDs: []string{"album-1"},
		},
		domain.Asset{
			Key:          domain.AssetKey{UUID: "fake-uuid-2"},
			Library:      domain.LibraryPersonal,
			OriginalName: "IMG_0005.MOV",
			DateSource:   domain.DateICloud,
			IsVideo:      true,
			Resources:    []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0005.MOV", Size: 7}},
		},
	)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after Import() = %+v, want %+v", got, want)
	}

	dirA := t.TempDir()
	files, err := lib.Download(ctx, "fake-uuid-1", dirA, nil)
	if err != nil {
		t.Fatalf("Download() of the photo error = %v, want nil", err)
	}
	wantFiles := []domain.LocalFile{
		{Role: domain.ResourceOriginal, Path: filepath.Join(dirA, "original")},
		{Role: domain.ResourceLiveVideo, Path: filepath.Join(dirA, "live_video")},
		{Role: domain.ResourceRAWPair, Path: filepath.Join(dirA, "raw_pair")},
	}
	if !slices.Equal(files, wantFiles) {
		t.Fatalf("Download() of the photo = %+v, want %+v", files, wantFiles)
	}
	photoLibDir(t, dirA, map[string]string{"original": "photo 4", "live_video": "live video 4", "raw_pair": "raw 4"})

	dirB := t.TempDir()
	files, err = lib.Download(ctx, "fake-uuid-2", dirB, nil)
	if err != nil {
		t.Fatalf("Download() of the video error = %v, want nil", err)
	}
	wantFiles = []domain.LocalFile{{Role: domain.ResourceOriginal, Path: filepath.Join(dirB, "original")}}
	if !slices.Equal(files, wantFiles) {
		t.Fatalf("Download() of the video = %+v, want %+v", files, wantFiles)
	}
	photoLibDir(t, dirB, map[string]string{"original": "video 5"})

	c := lib.Counters()
	if want := map[string]int{"Import": 2, "Snapshot": 1, "Download": 2}; !maps.Equal(c.Calls, want) {
		t.Errorf("Counters().Calls = %v, want %v", c.Calls, want)
	}
	if c.BytesDownloaded != 31 {
		t.Errorf("Counters().BytesDownloaded = %d, want 31", c.BytesDownloaded)
	}
}

func TestFakePhotoLibraryImportRejects(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	heic := photoLibFile(t, src, "IMG_0004.HEIC", "photo 4")
	live := photoLibFile(t, src, "IMG_0004.MOV", "live video 4")
	raw := photoLibFile(t, src, "IMG_0004.DNG", "raw 4")
	video := photoLibFile(t, src, "IMG_0005.MOV", "video 5")
	missing := filepath.Join(src, "IMG_0009.HEIC")

	for _, c := range []struct {
		name string
		req  domain.ImportRequest
	}{
		{"both", domain.ImportRequest{Photo: heic, Video: video}},
		{"neither", domain.ImportRequest{}},
		{"live_video_with_video", domain.ImportRequest{Video: video, LiveVideo: live}},
		{"raw_pair_with_video", domain.ImportRequest{Video: video, RAWPair: raw}},
		{"nul_name", domain.ImportRequest{Photo: heic, OriginalName: "IMG_\x000004.HEIC"}},
		{"invalid_utf8", domain.ImportRequest{Photo: heic, OriginalName: "IMG_\xff.HEIC"}},
		{"missing_photo", domain.ImportRequest{Photo: missing}},
		{"missing_live_video", domain.ImportRequest{Photo: heic, LiveVideo: missing}},
		{"missing_raw_pair", domain.ImportRequest{Photo: heic, RAWPair: missing}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			lib := testkit.NewFakePhotoLibrary(photoLibSeed())
			if uuid, err := lib.Import(ctx, c.req); err == nil || uuid != "" {
				t.Errorf("Import() = %q, %v; want an empty UUID and an error", uuid, err)
			}
			if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
				t.Errorf("Import created an asset: Snapshot() = %+v, want %+v", got, want)
			}
			if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic}); err != nil || uuid != "fake-uuid-1" {
				t.Errorf("next Import() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
			}
			if got, want := lib.Counters().Calls, map[string]int{"Import": 2, "Snapshot": 1}; !maps.Equal(got, want) {
				t.Errorf("Counters().Calls = %v, want %v", got, want)
			}
		})
	}
}

func TestFakePhotoLibraryImportFails(t *testing.T) {
	t.Parallel()

	heic := photoLibFile(t, t.TempDir(), "IMG_0004.HEIC", "photo 4")

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()

		lib := testkit.NewFakePhotoLibrary(photoLibSeed())
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic}); !errors.Is(err, context.Canceled) || uuid != "" {
			t.Errorf("Import() = %q, %v; want an empty UUID, %v", uuid, err, context.Canceled)
		}
		if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
			t.Errorf("Snapshot() = %+v, want %+v", got, want)
		}
		if got, want := lib.Counters().Calls, map[string]int{"Import": 1, "Snapshot": 1}; !maps.Equal(got, want) {
			t.Errorf("Counters().Calls = %v, want %v", got, want)
		}
	})

	t.Run("fail_next", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		lib := testkit.NewFakePhotoLibrary(photoLibSeed())
		boom := errors.New("boom")
		lib.FailNext("Import", boom)
		if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic}); !errors.Is(err, boom) || uuid != "" {
			t.Errorf("Import() = %q, %v; want an empty UUID, %v", uuid, err, boom)
		}
		if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
			t.Errorf("Snapshot() = %+v, want %+v", got, want)
		}
		if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic}); err != nil || uuid != "fake-uuid-1" {
			t.Errorf("second Import() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
		}
		if got, want := lib.Counters().Calls, map[string]int{"Import": 2, "Snapshot": 1}; !maps.Equal(got, want) {
			t.Errorf("Counters().Calls = %v, want %v", got, want)
		}
	})
}

func TestFakePhotoLibrarySyncCloud(t *testing.T) {
	t.Parallel()

	seed := photoLibSeed()
	seed.Assets[2].InCloud = new(false)
	lib := testkit.NewFakePhotoLibrary(seed)

	lib.SyncCloud()
	if got := lib.Counters().Calls; len(got) != 0 {
		t.Errorf("Counters().Calls after SyncCloud() = %v, want empty", got)
	}
	want := photoLibWant()
	want.Assets[1].Key.CloudAssetGUID = "fake-guid-1"
	want.Assets[1].InCloud = new(true)
	for i, key := range map[int]string{0: "fake-guid-2", 1: "fake-guid-3", 3: "fake-guid-4", 4: "fake-guid-5"} {
		want.Albums[i].Key, want.Albums[i].KeySource = key, domain.KeyCloudGUID
	}
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after SyncCloud() = %+v, want %+v", got, want)
	}
	if *seed.Assets[2].InCloud {
		t.Errorf("SyncCloud wrote through the InCloud pointer of its seed")
	}

	lib.SyncCloud()
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after the second SyncCloud() = %+v, want %+v", got, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"Snapshot": 2}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryImportConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	heic := photoLibFile(t, t.TempDir(), "IMG_0004.HEIC", "photo 4")
	dirs := make([]string, 8)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}
	var wg sync.WaitGroup
	for _, dir := range dirs {
		wg.Go(func() {
			uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic})
			if err != nil || uuid == "" {
				t.Errorf("Import() = %q, %v; want a UUID, nil", uuid, err)
				return
			}
			lib.SyncCloud()
			if _, err := lib.Download(ctx, uuid, dir, nil); err != nil {
				t.Errorf("Download() error = %v, want nil", err)
			}
		})
	}
	wg.Wait()

	s, err := lib.Snapshot(ctx, "")
	if err != nil || len(s.Assets) != 11 {
		t.Fatalf("Snapshot() = %d assets, %v; want 11 assets, nil", len(s.Assets), err)
	}
	uuids, guids := map[string]bool{}, map[string]bool{}
	for _, a := range s.Assets {
		uuids[a.Key.UUID] = true
		guids[a.Key.CloudAssetGUID] = true
	}
	if len(uuids) != 11 || len(guids) != 11 || guids[""] {
		t.Errorf("Snapshot() has %d distinct UUIDs and %d distinct GUIDs, an empty GUID %v; want 11, 11, false",
			len(uuids), len(guids), guids[""])
	}
	c := lib.Counters()
	if want := map[string]int{"Import": 8, "Download": 8, "Snapshot": 1}; !maps.Equal(c.Calls, want) {
		t.Errorf("Counters().Calls = %v, want %v", c.Calls, want)
	}
	if c.BytesDownloaded != 56 {
		t.Errorf("Counters().BytesDownloaded = %d, want 56", c.BytesDownloaded)
	}
}

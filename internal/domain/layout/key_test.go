package layout_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// The state stores keys, so they are literals here: a test that built them
// with the code would let a new form through.
func TestKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		album  domain.Album
		key    string
		source domain.KeySource
	}{
		{"zcloudguid: the cloud GUID is the key", domain.Album{UUID: "u-1", Key: "G-1", KeySource: domain.KeyCloudGUID, FolderTitles: []string{"2024"}, Title: "Trips"}, "G-1", domain.KeyCloudGUID},
		{"zcloudguid: a rename keeps the key", domain.Album{UUID: "u-1", Key: "G-1", KeySource: domain.KeyCloudGUID, FolderTitles: []string{"2024"}, Title: "Travel"}, "G-1", domain.KeyCloudGUID},
		{"zcloudguid: a move keeps the key", domain.Album{UUID: "u-1", Key: "G-1", KeySource: domain.KeyCloudGUID, FolderTitles: []string{"2024", "Summer"}, Title: "Trips"}, "G-1", domain.KeyCloudGUID},
		{"zcloudguid: a rebuilt library keeps the key", domain.Album{UUID: "u-2", Key: "G-1", KeySource: domain.KeyCloudGUID, FolderTitles: []string{"2024"}, Title: "Trips"}, "G-1", domain.KeyCloudGUID},
		{"zcloudguid: another album with the path has its own key", domain.Album{UUID: "u-3", Key: "G-2", KeySource: domain.KeyCloudGUID, FolderTitles: []string{"2024"}, Title: "Trips"}, "G-2", domain.KeyCloudGUID},
		{"phcloudid: the cloud identifier is the key", domain.Album{UUID: "u-1", Key: "C-1", KeySource: domain.KeyCloudID, FolderTitles: []string{"2024"}, Title: "Trips"}, "C-1", domain.KeyCloudID},
		{"phcloudid: a rename keeps the key", domain.Album{UUID: "u-1", Key: "C-1", KeySource: domain.KeyCloudID, FolderTitles: []string{"2024"}, Title: "Travel"}, "C-1", domain.KeyCloudID},
		{"phcloudid: a move keeps the key", domain.Album{UUID: "u-1", Key: "C-1", KeySource: domain.KeyCloudID, FolderTitles: []string{"2024", "Summer"}, Title: "Trips"}, "C-1", domain.KeyCloudID},
		{"phcloudid: a rebuilt library keeps the key", domain.Album{UUID: "u-2", Key: "C-1", KeySource: domain.KeyCloudID, FolderTitles: []string{"2024"}, Title: "Trips"}, "C-1", domain.KeyCloudID},
		{"phcloudid: another album with the path has its own key", domain.Album{UUID: "u-3", Key: "C-2", KeySource: domain.KeyCloudID, FolderTitles: []string{"2024"}, Title: "Trips"}, "C-2", domain.KeyCloudID},
		{"path: the folder titles and the title are the key", domain.Album{UUID: "u-1", FolderTitles: []string{"2024"}, Title: "Trips"}, `["2024"] "Trips"`, domain.KeyPath},
		{"path: a rename gives a new key", domain.Album{UUID: "u-1", FolderTitles: []string{"2024"}, Title: "Travel"}, `["2024"] "Travel"`, domain.KeyPath},
		{"path: a move gives a new key", domain.Album{UUID: "u-1", FolderTitles: []string{"2024", "Summer"}, Title: "Trips"}, `["2024" "Summer"] "Trips"`, domain.KeyPath},
		{"path: a rebuilt library keeps the key", domain.Album{UUID: "u-2", FolderTitles: []string{"2024"}, Title: "Trips"}, `["2024"] "Trips"`, domain.KeyPath},
		{"path: a folder with the path has the same key", domain.Album{UUID: "u-3", Kind: domain.KindFolder, FolderTitles: []string{"2024"}, Title: "Trips"}, `["2024"] "Trips"`, domain.KeyPath},
		{"path: a trashed album and a new one with its path share the key", domain.Album{UUID: "u-4", InTrash: true, FolderTitles: []string{"2024"}, Title: "Trips"}, `["2024"] "Trips"`, domain.KeyPath},
		{"a key that the state stores is kept", domain.Album{ID: 7, UUID: "u-1", Key: `["2024"] "Trips"`, KeySource: domain.KeyPath, Kind: domain.KindEvent, Title: "Trips", ParentID: 3}, `["2024"] "Trips"`, domain.KeyPath},
		{"path: a top-level album", domain.Album{Title: "Pets"}, `[] "Pets"`, domain.KeyPath},
		{"path: empty folder titles are none", domain.Album{FolderTitles: []string{}, Title: "Pets"}, `[] "Pets"`, domain.KeyPath},
		{"path: a slash in a folder title is no level", domain.Album{FolderTitles: []string{"a/b"}, Title: "c"}, `["a/b"] "c"`, domain.KeyPath},
		{"path: a slash in the title is no level", domain.Album{FolderTitles: []string{"a"}, Title: "b/c"}, `["a"] "b/c"`, domain.KeyPath},
		{"path: quotes and backslashes are escaped", domain.Album{Title: `a"b\c`}, `[] "a\"b\\c"`, domain.KeyPath},
		{"path: letters outside ASCII are escaped", domain.Album{Title: "Дом"}, "[] \"\\u0414\\u043e\\u043c\"", domain.KeyPath},
		{"path: an untitled album has a key", domain.Album{FolderTitles: []string{"2024"}, Title: ""}, `["2024"] ""`, domain.KeyPath},
		{"a source without a key gives the path key", domain.Album{KeySource: domain.KeyCloudGUID, Title: "Pets"}, `[] "Pets"`, domain.KeyPath},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			key, source := layout.Key(c.album)
			if key != c.key || source != c.source {
				t.Errorf("Key(%+v) = %q, %q; want %q, %q", c.album, key, source, c.key, c.source)
			}
		})
	}
}

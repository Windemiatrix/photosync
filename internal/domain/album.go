package domain

// Album is an album or a folder of albums of the photo library, as a
// snapshot reads it or as the state keeps it after the last applied run.
// Only the state fills ID and ParentID; only a snapshot fills ParentUUID and
// FolderTitles. The domain classifies albums and makes keys from paths, so a
// snapshot sets Kind only to KindFolder, for a folder, and sets Key and
// KeySource only from a cloud identifier that the library gives. Titles are
// in NFC.
type Album struct {
	ID        AlbumID
	UUID      string
	Key       string
	KeySource KeySource
	Kind      AlbumKind
	Title     string
	InTrash   bool

	ParentID     AlbumID  // zero at the top level
	ParentUUID   string   // empty at the top level
	FolderTitles []string // the titles of the enclosing folders, outermost first
}

// AlbumPair ties an album or a folder of the state to the directory of a
// remote root that stands for it.
type AlbumPair struct {
	AlbumID   AlbumID
	Remote    string
	Root      LibraryKind
	DirPath   string
	CreatedBy PairOrigin
}

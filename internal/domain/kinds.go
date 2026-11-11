package domain

// AssetID is the key of an asset in the state; zero is an asset that the
// state does not hold yet.
type AssetID int64

// AlbumID is the key of an album or a folder of albums in the state.
type AlbumID int64

// LinkID is the key of a link in the state.
type LinkID int64

// LibraryKind is the iCloud library an asset is in, and the root of a remote
// that mirrors that library.
type LibraryKind string

// The libraries.
const (
	LibraryShared   LibraryKind = "shared"
	LibraryPersonal LibraryKind = "personal"
)

// ResourceRole is a file of an asset in the library.
type ResourceRole string

// The files of an asset: the original, the video of a Live Photo, the RAW of
// a RAW+JPEG pair.
const (
	ResourceOriginal  ResourceRole = "original"
	ResourceLiveVideo ResourceRole = "live_video"
	ResourceRAWPair   ResourceRole = "raw_pair"
)

// LinkRole is what the file of a link holds.
type LinkRole string

// The roles of a link: a file of the asset; the cloud file in a format that
// Photos does not take, which the asset was converted from; a lesser copy of
// the asset that the remote keeps beside the original.
const (
	RoleOriginal  LinkRole = "original"
	RoleLiveVideo LinkRole = "live_video"
	RoleRAWPair   LinkRole = "raw_pair"
	RoleSource    LinkRole = "source"
	RoleDerived   LinkRole = "derived"
)

// LinkStatus says whether a link takes part in sync and, if not, why.
type LinkStatus string

// The statuses of a link.
const (
	StatusActive            LinkStatus = "active"
	StatusQuarantinedICloud LinkStatus = "quarantined_icloud"
	StatusQuarantinedRemote LinkStatus = "quarantined_remote"
	StatusDetached          LinkStatus = "detached"
)

// MatchLevel is how a link was made.
type MatchLevel string

// The levels of matching a cloud file to an asset, and the links that
// photosync makes itself: by an upload, by an import, by the import of an
// earlier state.
const (
	LevelExact         MatchLevel = "exact"
	LevelSameShot      MatchLevel = "same_shot"
	LevelSameShotWeak  MatchLevel = "same_shot_weak"
	LevelSameTimeSize  MatchLevel = "same_time_size"
	LevelProbable      MatchLevel = "probable"
	LevelPairComponent MatchLevel = "pair_component"
	LevelUploaded      MatchLevel = "uploaded"
	LevelImported      MatchLevel = "imported"
	LevelImportedState MatchLevel = "imported_state"
)

// AlbumKind is a folder of albums, an event album or a thematic album.
type AlbumKind string

// The kinds of albums.
const (
	KindFolder   AlbumKind = "folder"
	KindEvent    AlbumKind = "event"
	KindThematic AlbumKind = "thematic"
)

// KeySource is what the key of an album is made from.
type KeySource string

// The sources of an album key, in the order of preference: the cloud GUID of
// the library database, the cloud identifier of PhotoKit, the path.
const (
	KeyCloudGUID KeySource = "zcloudguid"
	KeyCloudID   KeySource = "phcloudid"
	KeyPath      KeySource = "path"
)

// PairOrigin is what created an album pair.
type PairOrigin string

// The origins of an album pair.
const (
	OriginICloud PairOrigin = "icloud"
	OriginRemote PairOrigin = "remote"
	OriginImport PairOrigin = "import"
)

// DateSource is where the capture time of an asset comes from.
type DateSource string

// The sources of a capture time.
const (
	DateICloud   DateSource = "icloud"
	DateMetadata DateSource = "metadata"
	DateFilename DateSource = "filename"
	DateFolder   DateSource = "folder"
	DateMTime    DateSource = "mtime"
)

// Mode is the direction in which a remote syncs.
type Mode string

// The modes of a remote.
const (
	ModeTwoWay       Mode = "two-way"
	ModeUploadOnly   Mode = "upload-only"
	ModeDownloadOnly Mode = "download-only"
)

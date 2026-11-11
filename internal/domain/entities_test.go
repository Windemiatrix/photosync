package domain_test

import "github.com/Windemiatrix/photosync/internal/domain"

// Each field below keeps its own type: then the compiler keeps a role,
// status, source, mode or ID of one kind out of a field of another, zero
// rather than nil stands for no album, and nil still stands for an unknown
// offset or cloud state. AssetKey stays comparable, because plans key maps by
// it.
var (
	_ = domain.Asset{
		ID:            domain.AssetID(1),
		Key:           domain.AssetKey{CloudAssetGUID: "guid-1", UUID: "uuid-1"},
		Library:       domain.LibraryPersonal,
		TargetLibrary: domain.LibraryShared,
		TZOffset:      new(int),
		DateSource:    domain.DateFilename,
		InCloud:       new(bool),
		Resources:     []domain.Resource{{Role: domain.ResourceLiveVideo}},
		Memberships:   []domain.Membership{{AlbumID: domain.AlbumID(1)}},
	}
	_ = domain.Album{
		ID:        domain.AlbumID(2),
		KeySource: domain.KeyPath,
		Kind:      domain.KindEvent,
		ParentID:  domain.AlbumID(1),
	}
	_ = domain.AlbumPair{AlbumID: domain.AlbumID(2), Root: domain.LibraryShared, CreatedBy: domain.OriginRemote}
	_ = domain.Link{
		ID:      domain.LinkID(1),
		AssetID: domain.AssetID(1),
		Root:    domain.LibraryPersonal,
		Role:    domain.RoleDerived,
		AlbumID: domain.AlbumID(2),
		Level:   domain.LevelProbable,
		Status:  domain.StatusQuarantinedRemote,
	}
	_ = domain.LibrarySnapshot{Assets: []domain.Asset{{}}, Albums: []domain.Album{{}}}
	_ = map[domain.AssetKey]domain.AssetID{}
	_ = domain.State{
		Assets:     []domain.Asset{{}},
		Albums:     []domain.Album{{}},
		AlbumPairs: []domain.AlbumPair{{}},
		Links:      []domain.Link{{}},
		History:    []domain.AssetHistory{{AssetID: domain.AssetID(1), Key: domain.AssetKey{UUID: "uuid-0"}, Deleted: true}},
	}
	_ = domain.Notice{Code: "match.probable", Params: []string{"40"}}
	_ = domain.RemoteSpec{
		Name:         "remote-1",
		Mode:         domain.ModeUploadOnly,
		Targets:      map[domain.LibraryKind]string{domain.LibraryShared: "Photos/Shared"},
		Capabilities: domain.Capabilities{ContentHash: domain.HashMD5},
	}
)

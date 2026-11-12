package domain

// SnapshotDelta is a snapshot of the photo library reconciled with the state,
// as a run writes it to the state before its operations. Assets and Albums
// are those of the snapshot, each with its ID in the state or a new ID above
// every ID of its kind there, and with the fields that only the state fills;
// an asset or an album that the snapshot lacks is not in them, and neither is
// an earlier identity of an asset that History names. History is that of the
// state, with the key that each earlier identity has in the snapshot, and
// with Deleted once that identity is in Recently Deleted or gone.
type SnapshotDelta struct {
	Assets  []Asset
	Albums  []Album
	History []AssetHistory
}

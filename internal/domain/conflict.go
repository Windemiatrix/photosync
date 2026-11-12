package domain

// Conflict is an asset that iCloud and a remote changed in different ways
// and that the conflict policy ask or skip left unsettled. Resolution is the
// conflict policy that the user chose for this conflict alone, empty until
// the user chooses.
type Conflict struct {
	AssetID    AssetID
	Remote     string
	Status     ConflictStatus
	Resolution ConflictPolicy
}

// ConflictStatus says whether a conflict waits for the user, is set aside
// without asking or is closed.
type ConflictStatus string

// The statuses of a conflict.
const (
	ConflictStatusOpen     ConflictStatus = "open"
	ConflictStatusResolved ConflictStatus = "resolved"
	ConflictStatusSkipped  ConflictStatus = "skipped"
)

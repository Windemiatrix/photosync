package domain

// LibrarySnapshot is the photo library at one moment: its assets, hidden and
// trashed ones included, and its albums and folders, trashed ones included.
// Assets refer to albums, and albums to folders, of the same snapshot by
// UUID, and no folder encloses itself, directly or through others.
type LibrarySnapshot struct {
	Assets []Asset
	Albums []Album
}

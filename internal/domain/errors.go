package domain

import "errors"

// The errors that the adapters return and app tells apart with errors.Is. They
// live in domain because app and the adapters may not import each other.
var (
	// ErrNotSupported is returned by a call that this build or this remote
	// cannot make; nothing was changed.
	ErrNotSupported = errors.New("domain: not supported")
	// ErrUserCanceled is returned when the user refuses a dialog of the photo
	// library; nothing was changed.
	ErrUserCanceled = errors.New("domain: the user canceled the change")
	// ErrOutcomeUnknown is wrapped together with the error of ctx when ctx ends
	// before the photo library reports the outcome of a change. The change may
	// still complete: read the library again before the next change.
	ErrOutcomeUnknown = errors.New("domain: the outcome of the change is unknown")
	// ErrNoTTY is returned instead of a question when there is no terminal to
	// ask it in.
	ErrNoTTY = errors.New("domain: no terminal to ask the user")
	// ErrNotFound is returned when a path, an asset or an album that a call
	// names does not exist.
	ErrNotFound = errors.New("domain: not found")
	// ErrExists is returned when the destination of a call is taken; the
	// destination is left as it was.
	ErrExists = errors.New("domain: already exists")
	// ErrNotEmpty is returned when a directory to remove is not empty; nothing
	// was removed.
	ErrNotEmpty = errors.New("domain: the directory is not empty")
)

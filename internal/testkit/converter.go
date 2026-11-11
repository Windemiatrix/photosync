package testkit

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// FakeConverter is an app.Converter whose MP4 is a copy of the source file;
// its methods are safe for concurrent use. A test sets the fields before the
// run and leaves them while the fake is in use.
type FakeConverter struct {
	// NotInstalled fails both methods with domain.ErrNotSupported, as on a
	// Mac without a converter.
	NotInstalled bool
	// Err fails ToMP4 of an installed converter, as a video that it cannot
	// read does.
	Err error

	mu    sync.Mutex
	calls []ToMP4Call
}

// ToMP4Call is a call of ToMP4.
type ToMP4Call struct {
	Src, Dst string
}

// ToMP4 writes a copy of src to dst. A dst that is there fails the call and
// is left as it was, so that a caller that would overwrite a file goes wrong
// in a test.
func (c *FakeConverter) ToMP4(_ context.Context, src, dst string) error {
	c.mu.Lock()
	c.calls = append(c.calls, ToMP4Call{Src: src, Dst: dst})
	c.mu.Unlock()
	switch {
	case c.NotInstalled:
		return domain.ErrNotSupported
	case c.Err != nil:
		return c.Err
	}
	b, err := os.ReadFile(src) //nolint:gosec // the code under test names the files, as it does for the real converter
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the code under test names the files, as it does for the real converter
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	return errors.Join(err, f.Close())
}

// Version gives "fake 1.0", the same for every fake, or "" with its error.
func (c *FakeConverter) Version(context.Context) (string, error) {
	if c.NotInstalled {
		return "", domain.ErrNotSupported
	}
	return "fake 1.0", nil
}

// ToMP4Calls gives every call of ToMP4, failed ones included, in the order
// of the calls.
func (c *FakeConverter) ToMP4Calls() []ToMP4Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
}

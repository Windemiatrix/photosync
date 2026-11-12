package testkit

import (
	"context"
	"fmt"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Hold makes the next change by the method of app.PhotoLibrary named op,
// such as "Import", wait for an outcome that comes too late, as when the user
// leaves a dialog open. The call calls waiting once the change waits and the
// fake is unlocked, then fails with domain.ErrOutcomeUnknown wrapped together
// with the error of ctx when ctx ends, and the change takes effect only by
// Answer. waiting may end ctx, so that a test needs no goroutine and no sleep.
// A call that fails before its change leaves the Hold to the next one; a nil
// waiting takes back an earlier Hold of op.
func (f *FakePhotoLibrary) Hold(op string, waiting func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held[op] = waiting
}

// Answer is the outcome, come too late, of the change of op that Hold made
// wait: with yes the change takes effect now, as it was checked when called,
// and without it the change is dropped. With no such change, Answer answers
// the next dialog of op: no fails that change with domain.ErrUserCanceled and
// nothing changed, and yes takes back an earlier no.
func (f *FakePhotoLibrary) Answer(op string, yes bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	apply := f.pending[op]
	delete(f.pending, op)
	if apply == nil {
		f.refused[op] = !yes
	} else if yes {
		apply()
	}
}

func (f *FakePhotoLibrary) commit(ctx context.Context, op string, apply func()) error {
	waiting := f.held[op]
	if waiting == nil {
		apply()
		return nil
	}
	delete(f.held, op)
	f.pending[op] = apply
	// The library goes on with other calls while a change waits for its
	// outcome; the deferred Unlock of the caller needs the fake locked again.
	f.mu.Unlock()
	defer f.mu.Lock()
	waiting()
	<-ctx.Done()
	return fmt.Errorf("%w: %w", domain.ErrOutcomeUnknown, ctx.Err())
}

package testkit

import (
	"context"
	"slices"
	"sync"
)

// FakeSpaceManager is an app.SpaceManager that answers from its fields and
// writes no file; its methods are safe for concurrent use. A test sets the
// fields before the run and leaves them while the fake is in use. A method
// that fails gives zero values with its error, so that a caller that uses
// them anyway goes wrong in a test.
type FakeSpaceManager struct {
	FreeBytes       int64 // for the volume of any path
	LibraryBytes    int64 // for any library
	OptimizeStorage bool
	// Err fails Free, LibrarySize and OptimizeStorageEnabled.
	Err error
	// PressureErr fails Pressure alone.
	PressureErr error

	mu        sync.Mutex
	pressures []PressureCall
}

// PressureCall is a call of Pressure that succeeded. Released tells whether
// its release was called: on a real disk an unreleased call leaves a file
// that takes the space.
type PressureCall struct {
	Dir             string
	Bytes, KeepFree int64
	Released        bool
}

// Free gives FreeBytes.
func (s *FakeSpaceManager) Free(context.Context, string) (int64, error) {
	if s.Err != nil {
		return 0, s.Err
	}
	return s.FreeBytes, nil
}

// LibrarySize gives LibraryBytes.
func (s *FakeSpaceManager) LibrarySize(context.Context, string) (int64, error) {
	if s.Err != nil {
		return 0, s.Err
	}
	return s.LibraryBytes, nil
}

// OptimizeStorageEnabled gives OptimizeStorage.
func (s *FakeSpaceManager) OptimizeStorageEnabled(context.Context) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	return s.OptimizeStorage, nil
}

// Pressure records its arguments and makes no file. Its release may be
// called more than once.
func (s *FakeSpaceManager) Pressure(_ context.Context, dir string, bytes, keepFree int64) (release func(), err error) {
	if s.PressureErr != nil {
		return nil, s.PressureErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := len(s.pressures)
	s.pressures = append(s.pressures, PressureCall{Dir: dir, Bytes: bytes, KeepFree: keepFree})
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.pressures[i].Released = true
	}, nil
}

// PressureCalls gives the calls of Pressure that succeeded, in the order of
// the calls.
func (s *FakeSpaceManager) PressureCalls() []PressureCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.pressures)
}

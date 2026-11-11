package domain_test

import (
	"errors"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// app tells the outcomes of a call apart with errors.Is, so no signal error
// may match another.
func TestBoundaryErrors(t *testing.T) {
	t.Parallel()
	errs := []struct {
		name string
		err  error
	}{
		{"ErrNotSupported", domain.ErrNotSupported},
		{"ErrUserCanceled", domain.ErrUserCanceled},
		{"ErrOutcomeUnknown", domain.ErrOutcomeUnknown},
		{"ErrNoTTY", domain.ErrNoTTY},
		{"ErrNotFound", domain.ErrNotFound},
		{"ErrExists", domain.ErrExists},
		{"ErrNotEmpty", domain.ErrNotEmpty},
	}
	for i, a := range errs {
		if a.err == nil {
			t.Errorf("%s is nil", a.name)
			continue
		}
		for j, b := range errs {
			if i != j && errors.Is(a.err, b.err) {
				t.Errorf("%s matches %s", a.name, b.name)
			}
		}
	}
}

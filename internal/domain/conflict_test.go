package domain_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// The state stores these strings: changing one breaks the states already
// written.
func TestConflictStatusStored(t *testing.T) {
	t.Parallel()
	checkStored(t, []domain.ConflictStatus{domain.ConflictStatusOpen, domain.ConflictStatusResolved, domain.ConflictStatusSkipped},
		"open", "resolved", "skipped")
}

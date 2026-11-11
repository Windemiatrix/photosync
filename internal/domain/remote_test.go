package domain_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// A remote that reports no hash must not be trusted with one, or a stranger's
// file of the same size would pass for the one photosync wrote.
func TestBoundaryHashNone(t *testing.T) {
	t.Parallel()
	if got := (domain.Capabilities{}).ContentHash; got != domain.HashNone {
		t.Errorf("zero Capabilities report %q, want HashNone", got)
	}
	kinds := []domain.HashKind{domain.HashNone, domain.HashMD5, domain.HashSHA256}
	for i, a := range kinds {
		for _, b := range kinds[:i] {
			if a == b {
				t.Errorf("two hash kinds are %q", a)
			}
		}
	}
}

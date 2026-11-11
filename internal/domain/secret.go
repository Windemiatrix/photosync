package domain

import (
	"fmt"
	"io"
)

// Secret is a password or a token. fmt shows it as *** for every verb but %p
// and %w, which print the value and which go vet rejects on a Secret;
// encoding/json shows it as "***", but not as a map key. A conversion, an index
// or a range gives the value, and only an adapter at the edge of I/O may write
// that. A struct field of type Secret must be exported: fmt prints an
// unexported field without calling its methods.
type Secret string

// Format writes *** whatever the flags; fmt does not call it for %p and %w.
func (Secret) Format(f fmt.State, _ rune) {
	// fmt.State writes into the buffer of fmt, which does not fail.
	_, _ = io.WriteString(f, "***")
}

// MarshalJSON gives the JSON string "***".
func (Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"***"`), nil
}

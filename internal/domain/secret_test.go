package domain_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

const maskCanary = "canary-value"

type secretHolder struct{ Value domain.Secret }

// A secret must not reach a log, a report or the text of an error in any form
// of printing.
func TestBoundarySecret(t *testing.T) {
	t.Parallel()
	s := domain.Secret(maskCanary)
	// The verbs are data: with a constant format vet, not this test, would
	// reject %d on a Secret without Format.
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
		checkMasked(t, verb, fmt.Sprintf(verb, s), "***")
	}
	checkMasked(t, "Sprint", fmt.Sprint(s), "***")
	checkMasked(t, "pointer", fmt.Sprint(&s), "***")
	checkMasked(t, "slice", fmt.Sprint([]domain.Secret{s}), "[***]")
	checkMasked(t, "field %v", fmt.Sprintf("%v", secretHolder{s}), "{***}")
	checkMasked(t, "field %+v", fmt.Sprintf("%+v", secretHolder{s}), "{Value:***}")
	checkMasked(t, "field %#v", fmt.Sprintf("%#v", secretHolder{s}), "domain_test.secretHolder{Value:***}")
	checkMasked(t, "JSON", secretJSON(t, s), `"***"`)
	checkMasked(t, "JSON pointer", secretJSON(t, &s), `"***"`)
	checkMasked(t, "JSON field", secretJSON(t, secretHolder{s}), `{"Value":"***"}`)
}

func checkMasked(t *testing.T, form, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", form, got, want)
	}
}

func secretJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Errorf("JSON of %T: %v", v, err)
	}
	return string(b)
}

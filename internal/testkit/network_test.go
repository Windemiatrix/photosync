package testkit_test

import (
	"errors"
	"testing"

	"github.com/Windemiatrix/photosync/internal/app"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

var _ app.NetworkMonitor = (*testkit.FakeNetworkMonitor)(nil)

func TestFakeNetworkMonitor(t *testing.T) {
	t.Parallel()

	errNet := errors.New("network")
	tests := []struct {
		name string
		fake testkit.FakeNetworkMonitor
		want bool
		err  error
	}{
		{name: "zero"},
		{name: "expensive", fake: testkit.FakeNetworkMonitor{Expensive: true}, want: true},
		{name: "error", fake: testkit.FakeNetworkMonitor{Err: errNet}, err: errNet},
		{name: "expensive_error", fake: testkit.FakeNetworkMonitor{Expensive: true, Err: errNet}, err: errNet},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.fake.IsExpensive(t.Context())
			if got != tc.want {
				t.Errorf("IsExpensive() = %v, want %v", got, tc.want)
			}
			if !errors.Is(err, tc.err) {
				t.Errorf("IsExpensive() error = %v, want %v", err, tc.err)
			}
		})
	}
}

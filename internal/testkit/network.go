package testkit

import "context"

// FakeNetworkMonitor is an app.NetworkMonitor that answers from its fields;
// its method is safe for concurrent use. A test sets the fields before the
// run and leaves them while the fake is in use.
type FakeNetworkMonitor struct {
	Expensive bool
	// Err fails IsExpensive, which then gives false.
	Err error
}

// IsExpensive gives Expensive.
func (n *FakeNetworkMonitor) IsExpensive(context.Context) (bool, error) {
	if n.Err != nil {
		return false, n.Err
	}
	return n.Expensive, nil
}

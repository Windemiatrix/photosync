package domain

// RemoteSpec is a remote as a sync sees it. Name is the name by which links
// and scans refer to the remote. Targets gives, for each library that the
// remote mirrors, the directory of its root in the remote. Capabilities come
// from the adapter of the remote, not from the configuration.
type RemoteSpec struct {
	Name         string
	Mode         Mode
	Targets      map[LibraryKind]string
	Capabilities Capabilities
}

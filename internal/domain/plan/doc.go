// Package plan decides what a sync does: it compares each side of a link,
// the asset in iCloud and the file in the cloud, with the base that the last
// applied run left. The package does no I/O and reads no clock.
package plan

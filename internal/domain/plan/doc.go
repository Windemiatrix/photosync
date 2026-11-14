// Package plan decides what a sync does. Build reconciles the snapshot of the
// photo library with the state that the last applied run left, reads the
// listing of each remote, decides on each link by comparing the asset in
// iCloud and the file in the cloud with its base, lays out the files of each
// asset by its albums and gives the plan of the run: its operations in the
// order that the run takes them, with the links, pairs and conflicts that the
// state takes. The package does no I/O and reads no clock.
package plan

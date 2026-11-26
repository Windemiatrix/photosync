// Package plan decides what a sync does. Build reconciles the snapshot of the
// photo library with the state that the last applied run left, reads the
// listing of each remote, decides on each link by comparing the asset in
// iCloud and the file in the cloud with its base, lays out the files of each
// asset by its albums and gives the plan of the run: its operations in the
// order that the run takes them, with the links, pairs and conflicts that the
// state takes. The package does no I/O and reads no clock.
//
// The templates of a sync bound what it touches. An album that the template
// of Photos does not read whole is no event and gets no directory; an asset
// in no event gets no file in a remote, and the files that an event placed
// for it there are surplus. A cloud file in no directory that the cloud
// template reads as an event is left out, and an asset whose file lies
// there, or has moved there as the content hash of the remote tells, stays
// as it is in that remote, so that no run takes such a file for new or for
// deleted; without that hash the move cannot be told, and the link of the
// moved file reads as deleted. A directory paired with an album follows that
// album rather than the cloud template: it is read while the album is an
// event and left alone while the album is thematic outside Recently Deleted,
// until it is an event again. An edit that no link names, a file named as an
// edited photo or lying in a directory of edits, follows the edited_files
// policy wherever it lies.
package plan

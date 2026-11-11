// Package testkit holds the fakes that the tests of app use in place of the
// systems outside photosync: the photo library, the disks, the network and
// the video converter. A fake keeps the contract of its interface in app,
// behaves as the real system did in live runs where they showed it, and lets
// a test see what was done to it.
package testkit

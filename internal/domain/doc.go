// Package domain holds the entities, rules and errors of photosync. It does
// no I/O, reads no clock and writes no log: the current time and the run ID
// are parameters, and a result does not depend on the order of its input
// (INV-9).
//
// Paths in RemoteEntry and Link use package path and are relative to the root
// of their library in the remote. A path read from a remote is kept as the remote stores it, not
// normalized, so two names that differ only in Unicode form or in case stay
// apart; a name that photosync makes is in NFC. Times are UTC, with the
// offset of the place in seconds beside them. Sizes are bytes in int64. The
// value of an enumeration constant is the string that the state and the
// configuration store: changing one breaks the states and configurations
// already written.
package domain

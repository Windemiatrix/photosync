//go:build darwin && cgo

package main

// macOS reads the bundle identifier and the Photos usage descriptions from the
// embedded Info.plist when the command line tool asks for access to the Photos
// library, and codesign takes the signing identifier from it.

/*
#cgo LDFLAGS: -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/Info.plist
*/
import "C"

//go:build darwin && cgo

package main

// The linker embeds Info.plist into the __TEXT,__info_plist section of the
// binary. macOS reads the bundle identifier and the Photos usage descriptions
// from it when the command line tool asks for access to the Photos library,
// and codesign takes the signing identifier from it.

/*
#cgo LDFLAGS: -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/Info.plist
*/
import "C"

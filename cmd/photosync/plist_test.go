//go:build darwin && cgo

package main

import (
	"bytes"
	"debug/macho"
	"os"
	"testing"
)

// TestInfoPlistSection checks that the test binary, which is linked with the
// same cgo flags as the command, carries Info.plist in __TEXT,__info_plist.
// The release binary is checked by hand with "otool -l" before a release.
func TestInfoPlistSection(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	f, err := macho.Open(exe)
	if err != nil {
		t.Fatalf("macho.Open(%s): %v", exe, err)
	}
	defer func() { _ = f.Close() }()

	sec := f.Section("__info_plist")
	if sec == nil {
		t.Fatal("section __info_plist not found: the -sectcreate flag did not reach the linker")
	}
	if sec.Seg != "__TEXT" {
		t.Errorf("section __info_plist is in segment %q, want __TEXT", sec.Seg)
	}
	got, err := sec.Data()
	if err != nil {
		t.Fatalf("read section __info_plist: %v", err)
	}
	want, err := os.ReadFile("Info.plist") // go test runs in the package directory
	if err != nil {
		t.Fatalf("read Info.plist: %v", err)
	}
	// The linker may pad a section with zero bytes.
	if !bytes.Equal(bytes.TrimRight(got, "\x00"), bytes.TrimRight(want, "\x00")) {
		t.Errorf("section __info_plist (%d bytes) differs from Info.plist (%d bytes)", len(got), len(want))
	}
	for _, key := range []string{
		"CFBundleIdentifier",
		"NSPhotoLibraryUsageDescription",
		"NSPhotoLibraryAddUsageDescription",
	} {
		if !bytes.Contains(got, []byte(key)) {
			t.Errorf("section __info_plist has no key %s", key)
		}
	}
}

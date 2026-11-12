package match

// IsSidecar reports whether the file at name, a name or a slash-separated
// path, is a sidecar by the extension of its last element, in any case: an
// XMP, THM, AAE, IFO, BUP, ZIP, EXE or RAR file, which only stays in the
// cloud and goes with the media file of its directory and stem.
func IsSidecar(name string) bool {
	return kindOf(name) == sidecar
}

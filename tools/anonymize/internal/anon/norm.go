package anon

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// nfc returns the NFC form of s.
func nfc(s string) string { return norm.NFC.String(s) }

// nfd returns the NFD form of s.
func nfd(s string) string { return norm.NFD.String(s) }

// decomposable reports whether the cluster c (NFC) has a different NFD form.
func decomposable(c string) bool { return norm.NFD.String(c) != c }

// lowerNFC returns the lower-case NFC form of s, rune by rune.
func lowerNFC(s string) string { return strings.ToLower(nfc(s)) }

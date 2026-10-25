package anon

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

func nfc(s string) string { return norm.NFC.String(s) }

func nfd(s string) string { return norm.NFD.String(s) }

func decomposable(c string) bool { return norm.NFD.String(c) != c }

func lowerNFC(s string) string { return strings.ToLower(nfc(s)) }

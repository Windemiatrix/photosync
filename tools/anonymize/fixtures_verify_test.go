//go:build fixtures_verify

package main

import (
	"flag"
	"os"
	"testing"
)

var materialsDir = flag.String("anonymize.in", "", "absolute path to the manual-sync materials")

func TestFixturesVerify(t *testing.T) {
	if st, err := os.Stat(*materialsDir); *materialsDir == "" || err != nil || !st.IsDir() {
		t.Skip("no materials directory: pass -anonymize.in")
	}
	verifyMaterials(t, *materialsDir)
}

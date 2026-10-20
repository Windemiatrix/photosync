// Command photosync keeps a macOS Photos library and cloud storage in sync.
//
// This is the repository skeleton: it implements only the version command.
// The other commands arrive together with the application packages.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
)

// version and commit are set at link time:
//
//	go build -ldflags "-X main.version=v1.2.3 -X main.commit=<sha>"
//
// When they are empty (for example after "go install"), the values that the
// Go toolchain records in the binary are used instead.
var (
	version string
	commit  string
)

// buildInfo describes how the running binary was built.
type buildInfo struct {
	version string
	commit  string
	// read returns the build information recorded by the Go toolchain.
	// It is debug.ReadBuildInfo in production and a fake in tests.
	read func() (*debug.BuildInfo, bool)
}

const usage = "usage: photosync version\n"

func main() {
	b := buildInfo{version: version, commit: commit, read: debug.ReadBuildInfo}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, b))
}

// run executes one command line and returns the process exit code.
func run(args []string, stdout, stderr io.Writer, b buildInfo) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage) // nothing more to do if stderr is broken
		return 2
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			_, _ = io.WriteString(stderr, usage) // nothing more to do if stderr is broken
			return 2
		}
		ver, rev := b.resolve()
		if _, err := fmt.Fprintf(stdout, "photosync %s (commit %s, %s)\n", ver, rev, runtime.Version()); err != nil {
			return 1
		}
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "photosync: unknown command %q\n%s", args[0], usage) // nothing more to do if stderr is broken
		return 2
	}
}

// resolve returns the version and the commit to print. Link-time values win;
// empty ones fall back to the toolchain build information, then to the
// placeholders "devel" and "unknown".
func (b buildInfo) resolve() (ver, rev string) {
	ver, rev = b.version, b.commit
	if ver != "" && rev != "" {
		return ver, rev
	}

	var info *debug.BuildInfo
	if b.read != nil {
		if bi, ok := b.read(); ok {
			info = bi
		}
	}

	if ver == "" {
		ver = "devel"
		if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
			ver = info.Main.Version
		}
	}
	if rev == "" {
		rev = "unknown"
		if info != nil {
			rev = vcsRevision(info)
		}
	}
	return ver, rev
}

// vcsRevision returns the revision the toolchain stamped into the binary,
// marked "+dirty" when the working tree had local changes, or "unknown".
func vcsRevision(info *debug.BuildInfo) string {
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if dirty {
		rev += "+dirty"
	}
	return rev
}

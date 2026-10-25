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

const usage = "usage: photosync version\n"

func main() {
	info, _ := debug.ReadBuildInfo()
	ver, rev := resolve(version, commit, info)
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, ver, rev))
}

// run executes one command line and returns the process exit code.
func run(args []string, stdout, stderr io.Writer, ver, rev string) int {
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
// empty ones fall back to the toolchain build information (nil when the
// binary has none), then to the placeholders "devel" and "unknown".
func resolve(version, commit string, info *debug.BuildInfo) (ver, rev string) {
	ver, rev = version, commit
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

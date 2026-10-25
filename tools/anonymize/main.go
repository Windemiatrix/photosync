// Command anonymize is a developer tool; it is not needed to use photosync.
// It builds test fixtures from the materials of an earlier script-based sync
// (a Photos catalog, a cloud file inventory, the reviewed plan and a rerun of
// that plan with its links) without leaking personal data, keeping the
// relations the matching tests need. It also scans fixture directories for
// leaks (--check).
//
//	anonymize --in <dir> --out <dir> --salt-file <file>
//	anonymize --check <dir> [--salt-file <file>]
//
// The build mode reads only the known files of --in through os.Root:
// icloud_catalog.json, yandex_inventory.json (yandex_inventory.prev.json
// when the current inventory does not pass the partition check),
// plan/plan.json, plan-rerun/plan.json and plan-rerun/links.json, and writes
// <out>/anon-<category>/case.json and <out>/tokens.json. The build fails
// unless the rerun plan equals the reviewed plan and the links partition the
// records (the partition check), and it fails when the leak scanner finds
// anything in its own output. --out is created when missing, must be empty
// and lie outside --in and any git working tree; the salt file (64 hex digits, mode 0600) is created when missing and must
// lie outside any git working tree.
//
// Example:
//
//	anonymize --in ~/materials/sync --out ~/materials/fixtures-out --salt-file ~/.config/photosync-dev/anonymize.key
//	anonymize --check ~/materials/fixtures-out
//
// Fixtures reach testdata/fixtures only by a manual move after --check; an
// automatic comparison with the source materials ("make fixtures-verify")
// confirms that none of the input words got in, and the maintainer reads the
// summary of that comparison.
//
// The standard output holds aggregates only (counts per level, unit and
// case); --check prints "<file>: <JSON pointer>: <rule>" per finding and a
// summary, never a value. Exit codes: 0 success, 1 data, I/O, consistency or
// scanner failure, 2 usage error.
package main

import (
	"errors"
	"flag"
	"io"
	"os"
)

const usage = `usage: anonymize --in <dir> --out <dir> --salt-file <file>
       anonymize --check <dir> [--salt-file <file>]
`

func main() {
	os.Exit(safely(func() int { return run(os.Args[1:], os.Stdout, os.Stderr) }, os.Stderr)) //nolint:forbidigo // the exit code of the command
}

// The panic value and the stack may hold input values and are never printed.
func safely(f func() int, stderr io.Writer) (code int) {
	defer func() {
		if recover() != nil {
			_, _ = io.WriteString(stderr, "anonymize: internal error\n") // nothing more to do if stderr is broken
			code = 1
		}
	}()
	return f()
}

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("anonymize", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	in := fl.String("in", "", "materials directory")
	out := fl.String("out", "", "output directory")
	salt := fl.String("salt-file", "", "salt file")
	check := fl.String("check", "", "directory to scan")
	if err := fl.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = io.WriteString(stdout, usage) // nothing more to do if stdout is broken
			return 0
		}
		return usageError(stderr)
	}
	set := map[string]bool{}
	fl.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if fl.NArg() != 0 {
		return usageError(stderr)
	}
	if set["check"] {
		if *check == "" || set["in"] || set["out"] {
			return usageError(stderr)
		}
		n, err := runCheck(*check, *salt, stdout)
		if err != nil {
			return report(stderr, err)
		}
		if n > 0 {
			return 1
		}
		return 0
	}
	if *in == "" || *out == "" || *salt == "" {
		return usageError(stderr)
	}
	if err := build(*in, *out, *salt, stdout, stderr); err != nil {
		return report(stderr, err)
	}
	return 0
}

func usageError(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, usage) // nothing more to do if stderr is broken
	return 2
}

// Any error but a toolError is printed as a fixed message because its text may
// hold a path or an input value.
func report(stderr io.Writer, err error) int {
	msg := "unexpected error"
	var te *toolError
	if errors.As(err, &te) {
		msg = te.Error()
	}
	_, _ = io.WriteString(stderr, "anonymize: "+msg+"\n") // nothing more to do if stderr is broken
	return 1
}

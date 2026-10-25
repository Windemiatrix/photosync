package main

import (
	"bytes"
	"errors"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	revision := debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef"}
	info := func(mainVersion string, settings ...debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: mainVersion}, Settings: settings}
	}

	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}

	tests := []struct {
		name    string
		version string
		commit  string
		info    *debug.BuildInfo
		want    string
	}{
		{
			name:    "link-time values win over build info",
			version: "v1.2.3",
			commit:  "abc123",
			info:    info("v9.9.9", debug.BuildSetting{Key: "vcs.revision", Value: "ffff"}),
			want:    "v1.2.3 abc123",
		},
		{
			name: "empty values fall back to build info",
			info: info("v0.4.0-alpha.1", revision),
			want: "v0.4.0-alpha.1 0123456789abcdef",
		},
		{
			name:    "only the commit is missing",
			version: "v1.2.3",
			info:    info("v9.9.9", revision),
			want:    "v1.2.3 0123456789abcdef",
		},
		{
			name:   "only the version is missing",
			commit: "abc123",
			info:   info("v0.5.0", revision),
			want:   "v0.5.0 abc123",
		},
		{
			name: "dirty tree is marked on the vcs revision",
			info: info("v0.4.0", revision, dirty),
			want: "v0.4.0 0123456789abcdef+dirty",
		},
		{
			name:   "dirty tree does not mark the link-time commit",
			commit: "abc123",
			info:   info("v0.4.0", revision, dirty),
			want:   "v0.4.0 abc123",
		},
		{
			name: "devel build without vcs data",
			info: info("(devel)"),
			want: "devel unknown",
		},
		{
			name: "build info unavailable",
			want: "devel unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ver, rev := resolve(tt.version, tt.commit, tt.info)
			if got := ver + " " + rev; got != tt.want {
				t.Errorf("resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr, "v1.2.3", "abc123"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr.String())
	}
	want := "photosync v1.2.3 (commit abc123, " + runtime.Version() + ")\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no arguments", args: nil, want: "usage: photosync version"},
		{name: "unknown command", args: []string{"sync"}, want: `unknown command "sync"`},
		{name: "extra argument", args: []string{"version", "extra"}, want: "usage: photosync version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr, "devel", "unknown")
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestVersionWriteFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	if code := run([]string{"version"}, failingWriter{}, &stderr, "devel", "unknown"); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

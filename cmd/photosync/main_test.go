package main

import (
	"bytes"
	"errors"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// fakeBuildInfo returns a read function that reports the given toolchain data.
func fakeBuildInfo(mainVersion string, settings ...debug.BuildSetting) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main:     debug.Module{Version: mainVersion},
			Settings: settings,
		}, true
	}
}

func unavailableBuildInfo() (*debug.BuildInfo, bool) { return nil, false }

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	goVersion := runtime.Version()
	revision := debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef"}

	tests := []struct {
		name string
		b    buildInfo
		want string
	}{
		{
			name: "link-time values win over build info",
			b: buildInfo{
				version: "v1.2.3",
				commit:  "abc123",
				read:    fakeBuildInfo("v9.9.9", debug.BuildSetting{Key: "vcs.revision", Value: "ffff"}),
			},
			want: "photosync v1.2.3 (commit abc123, " + goVersion + ")",
		},
		{
			name: "empty values fall back to build info",
			b:    buildInfo{read: fakeBuildInfo("v0.4.0-alpha.1", revision)},
			want: "photosync v0.4.0-alpha.1 (commit 0123456789abcdef, " + goVersion + ")",
		},
		{
			name: "only the commit is missing",
			b:    buildInfo{version: "v1.2.3", read: fakeBuildInfo("v9.9.9", revision)},
			want: "photosync v1.2.3 (commit 0123456789abcdef, " + goVersion + ")",
		},
		{
			name: "only the version is missing",
			b:    buildInfo{commit: "abc123", read: fakeBuildInfo("v0.5.0", revision)},
			want: "photosync v0.5.0 (commit abc123, " + goVersion + ")",
		},
		{
			name: "dirty tree is marked",
			b: buildInfo{read: fakeBuildInfo("v0.4.0", revision,
				debug.BuildSetting{Key: "vcs.modified", Value: "true"})},
			want: "photosync v0.4.0 (commit 0123456789abcdef+dirty, " + goVersion + ")",
		},
		{
			name: "devel build without vcs data",
			b:    buildInfo{read: fakeBuildInfo("(devel)")},
			want: "photosync devel (commit unknown, " + goVersion + ")",
		},
		{
			name: "build info unavailable",
			b:    buildInfo{read: unavailableBuildInfo},
			want: "photosync devel (commit unknown, " + goVersion + ")",
		},
		{
			name: "no read function",
			b:    buildInfo{},
			want: "photosync devel (commit unknown, " + goVersion + ")",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := run([]string{"version"}, &stdout, &stderr, tt.b)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr.String())
			}
			if got := strings.TrimSuffix(stdout.String(), "\n"); got != tt.want {
				t.Errorf("stdout = %q, want %q", got, tt.want)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
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
			code := run(tt.args, &stdout, &stderr, buildInfo{})
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
	if code := run([]string{"version"}, failingWriter{}, &stderr, buildInfo{}); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

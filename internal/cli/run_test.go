package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHelpExitsSuccessfully(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-h"}, strings.NewReader(""), &stdout, &stderr, "dev")
	if code != 0 {
		t.Fatalf("code=%d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage: reorder") {
		t.Fatalf("help output missing usage: %q", stderr.String())
	}
}

func TestRunStdin(t *testing.T) {
	t.Parallel()

	input := `package sample

type Thing struct{}
func (Thing) Public() {}
func NewThing() *Thing { return &Thing{} }
`
	want := `package sample

type Thing struct{}
func NewThing() *Thing { return &Thing{} }
func (Thing) Public() {}
`

	var stdout, stderr bytes.Buffer
	code := Run(nil, strings.NewReader(input), &stdout, &stderr, "test")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.String() != want {
		t.Fatalf("unexpected stdout\n%s", stdout.String())
	}
}

func TestRunCheckAndWriteDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	input := `package sample

type Thing struct{}
func (Thing) Public() {}
func NewThing() *Thing { return &Thing{} }
`
	want := `package sample

type Thing struct{}
func NewThing() *Thing { return &Thing{} }
func (Thing) Public() {}
`
	if err := os.WriteFile(path, []byte(input), 0o640); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-check", dir}, strings.NewReader(""), &stdout, &stderr, "test")
	if code != 1 {
		t.Fatalf("check exit=%d stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != path {
		t.Fatalf("unexpected check output %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"-write", dir}, strings.NewReader(""), &stdout, &stderr, "test")
	if code != 0 {
		t.Fatalf("write exit=%d stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("unexpected file\n%s", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%o, want 640", info.Mode().Perm())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"-check", dir}, strings.NewReader(""), &stdout, &stderr, "test")
	if code != 0 {
		t.Fatalf("post-write check exit=%d stderr=%s", code, stderr.String())
	}
}

func TestRunAbortsBatchBeforeWritingOnUnsafeFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	goodPath := filepath.Join(dir, "a.go")
	unsafePath := filepath.Join(dir, "b.go")
	good := `package sample

type Thing struct{}
func (Thing) Public() {}
func NewThing() *Thing { return &Thing{} }
`
	unsafe := `package sample

type Other struct{}
func (Other) private() {}

// standalone section

func helper() {}
func (Other) Public() {}
`
	if err := os.WriteFile(goodPath, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unsafePath, []byte(unsafe), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-write", dir}, strings.NewReader(""), &stdout, &stderr, "test")
	if code != 2 {
		t.Fatalf("exit=%d, stderr=%s", code, stderr.String())
	}
	got, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != good {
		t.Fatal("first file was modified before batch planning completed")
	}
}

func TestRunMultipleFilesRequireMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.go"), filepath.Join(dir, "b.go")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("package sample\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	code := Run(paths, strings.NewReader(""), &stdout, &stderr, "test")
	if code != 2 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stderr.String(), "multiple file targets require") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-version"}, strings.NewReader(""), &stdout, &stderr, "v1.2.3")
	if code != 0 || stdout.String() != "v1.2.3\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

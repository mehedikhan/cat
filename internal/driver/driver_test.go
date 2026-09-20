package driver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mehedikhan/cat/internal/driver"
)

const examples = "../../examples"

// Expected stdout for the deterministic examples.
var expected = map[string]string{
	"hello.cat": "hello, world!\nline 1\nline 2\nline 3\n",
	"point.cat": "a = (0, 0)\nb = (3, 4)\ndistance = 5\na moved to (1, 1)\n",
}

func TestExamplesCompile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(examples, "*.cat"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			if err := driver.Build(f, filepath.Join(t.TempDir(), "prog")); err != nil {
				t.Fatalf("build: %v", err)
			}
		})
	}
}

func TestExampleOutput(t *testing.T) {
	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			if got := runCat(t, filepath.Join(examples, name)); got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestFizzBuzzOutput(t *testing.T) {
	got := runCat(t, filepath.Join(examples, "fizzbuzz.cat"))
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 15 {
		t.Fatalf("got %d lines, want 15", len(lines))
	}
	if lines[2] != "Fizz" || lines[4] != "Buzz" || lines[14] != "FizzBuzz" {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestWorkersDrainsEveryJob(t *testing.T) {
	got := runCat(t, filepath.Join(examples, "workers.cat"))
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 9 {
		t.Fatalf("got %d result lines, want 9:\n%s", len(lines), got)
	}
}

func TestBuildErrorsPointAtCatSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.cat")
	if err := os.WriteFile(src, []byte("x = 1\ny = \"two\"\nputs x + y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := driver.Build(src, filepath.Join(dir, "prog"))
	if err == nil {
		t.Fatal("expected a compile error")
	}
	if !strings.Contains(err.Error(), "bad.cat:3") {
		t.Fatalf("error does not point at bad.cat line 3: %v", err)
	}
}

func TestEmitIsValidGo(t *testing.T) {
	src, err := driver.Emit(filepath.Join(examples, "hello.cat"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(src, "package main") {
		t.Fatalf("emitted source does not start with a package clause:\n%s", src)
	}
}

// runCat builds and runs a .cat file, returning its stdout.
func runCat(t *testing.T, path string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "prog")
	if err := driver.Build(path, bin); err != nil {
		t.Fatalf("build %s: %v", path, err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run %s: %v", path, err)
	}
	return string(out)
}

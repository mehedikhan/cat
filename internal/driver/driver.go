// Package driver compiles .cat files by generating Go and invoking the Go
// toolchain, mapping any toolchain diagnostics back onto cat source lines.
package driver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mehedikhan/cat/internal/codegen"
	"github.com/mehedikhan/cat/internal/parser"
)

// Compile parses and lowers a .cat file to Go source.
func Compile(path string) (*codegen.Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	prog, err := parser.Parse(name, string(src))
	if err != nil {
		return nil, err
	}
	return codegen.Generate(name, prog)
}

// Emit returns the generated Go source, gofmt'd for reading. The formatted
// text is for humans only; builds use the unformatted text so that line
// numbers stay meaningful.
func Emit(path string) (string, error) {
	res, err := Compile(path)
	if err != nil {
		return "", err
	}
	out, err := formatSource(res.Source)
	if err != nil {
		// Unformattable output still helps when debugging codegen.
		return res.Source, nil
	}
	return out, nil
}

// Build compiles path to a native executable at out.
func Build(path, out string) error {
	res, err := Compile(path)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "catbuild")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	if err := writeModule(dir, res.Source); err != nil {
		return err
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", abs, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", rewrite(string(output), filepath.Base(path), res.LineMap))
	}
	return nil
}

// Run builds path into a temporary directory and executes it.
func Run(path string, args []string) (int, error) {
	dir, err := os.MkdirTemp("", "catrun")
	if err != nil {
		return 1, err
	}
	defer os.RemoveAll(dir)

	bin := filepath.Join(dir, "catprog")
	if err := Build(path, bin); err != nil {
		return 1, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok {
			return ee.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

func writeModule(dir, source string) error {
	mod := "module catprog\n\ngo " + minGo + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644)
}

var goErrLine = regexp.MustCompile(`(?m)^(?:\./)?main\.go:(\d+)(?::\d+)?:`)

// rewrite maps "main.go:37:5:" prefixes in toolchain output back to cat lines.
func rewrite(output, catFile string, lineMap map[int]int) string {
	out := goErrLine.ReplaceAllStringFunc(output, func(m string) string {
		sub := goErrLine.FindStringSubmatch(m)
		n, err := strconv.Atoi(sub[1])
		if err != nil {
			return m
		}
		if catLine, ok := lineMap[n]; ok {
			return fmt.Sprintf("%s:%d:", catFile, catLine)
		}
		return fmt.Sprintf("%s (generated line %d):", catFile, n)
	})
	// Drop the "# catprog" package banner the Go toolchain prints.
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\n")
}

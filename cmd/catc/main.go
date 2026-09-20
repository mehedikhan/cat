// Command catc compiles and runs cat programs.
//
// The language is "cat" and its files are .cat; the compiler driver is named
// catc so that it does not shadow /bin/cat on anyone's PATH.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mehedikhan/cat/internal/driver"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `catc - a Ruby-flavoured language that compiles through Go

usage:
  catc run   <file.cat> [args...]      compile and run
  catc build <file.cat> [-o output]    compile to a native executable
  catc emit  <file.cat>                print the generated Go source
  catc doctor                          check that the Go toolchain is usable
  catc version

  catc <file.cat> [args...]            same as "catc run"

catc requires the Go toolchain to be installed; it generates Go and builds it.
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	// "catc foo.cat" is shorthand for "catc run foo.cat".
	if strings.HasSuffix(args[0], ".cat") {
		args = append([]string{"run"}, args...)
	}

	switch args[0] {
	case "run":
		src := needFile(args)
		requireToolchain()
		code, err := driver.Run(src, args[2:])
		if err != nil {
			fail("%v", err)
		}
		os.Exit(code)

	case "build":
		src := needFile(args)
		requireToolchain()
		out, err := outputPath(src, args[2:])
		if err != nil {
			fail("%v", err)
		}
		if err := driver.Build(src, out); err != nil {
			fail("%v", err)
		}
		fmt.Println("wrote", out)

	case "emit":
		src := needFile(args)
		go_, err := driver.Emit(src)
		if err != nil {
			fail("%v", err)
		}
		fmt.Print(go_)

	case "doctor":
		doctor()

	case "version", "-v", "--version":
		fmt.Printf("catc %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		if v, err := driver.ToolchainVersion(); err == nil {
			fmt.Println("go   " + v)
		}

	case "help", "-h", "--help":
		fmt.Print(usage)

	default:
		fail("unknown command %q\n\n%s", args[0], usage)
	}
}

func needFile(args []string) string {
	if len(args) < 2 {
		fail("%s needs a .cat file\n\n%s", args[0], usage)
	}
	return args[1]
}

// outputPath resolves "-o name", defaulting to the source's base name.
func outputPath(src string, rest []string) (string, error) {
	out := strings.TrimSuffix(filepath.Base(src), ".cat")
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "-o", "--output":
			if i+1 >= len(rest) {
				return "", fmt.Errorf("-o needs a file name")
			}
			out = rest[i+1]
			i++
		default:
			return "", fmt.Errorf("unexpected argument %q", rest[i])
		}
	}
	return out, nil
}

// requireToolchain fails with an actionable message when Go is unusable.
func requireToolchain() {
	if err := driver.CheckToolchain(); err != nil {
		fail("%v", err)
	}
}

func doctor() {
	fmt.Printf("catc      %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	if exe, err := os.Executable(); err == nil {
		fmt.Printf("installed %s\n", exe)
	}
	if err := driver.CheckToolchain(); err != nil {
		fmt.Printf("go        NOT USABLE\n\n%v\n", err)
		os.Exit(1)
	}
	v, _ := driver.ToolchainVersion()
	path, _ := driver.ToolchainPath()
	fmt.Printf("go        %s\n", v)
	fmt.Printf("          %s\n", path)
	fmt.Println("\neverything looks good")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "catc: "+format+"\n", args...)
	os.Exit(1)
}

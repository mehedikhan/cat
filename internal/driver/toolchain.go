package driver

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// minGo is the language version the generated module declares. Per-iteration
// loop variables, which cat relies on to give each block invocation its own
// binding, arrived in Go 1.22.
const minGo = "1.22"

// ToolchainPath returns the resolved path of the go command.
func ToolchainPath() (string, error) { return exec.LookPath("go") }

// ToolchainVersion returns the installed Go version, e.g. "go1.24.0".
func ToolchainVersion() (string, error) {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return strings.TrimSpace(string(out)), nil
	}
	return fields[2], nil
}

// CheckToolchain verifies that a new enough Go toolchain is available.
// cat generates Go and shells out to build it, so this is a hard requirement.
func CheckToolchain() error {
	if _, err := ToolchainPath(); err != nil {
		return fmt.Errorf(`the Go toolchain is required but "go" was not found on your PATH.

cat compiles by generating Go source and building it, so Go must be installed.
Install it from https://go.dev/dl/ (or "brew install go" on macOS), then run
"catc doctor" to verify`)
	}
	v, err := ToolchainVersion()
	if err != nil {
		return fmt.Errorf("could not run \"go version\": %w", err)
	}
	if !atLeast(v, minGo) {
		return fmt.Errorf(`Go %s or newer is required, but %s is installed.

cat relies on per-iteration loop variables, which arrived in Go %s, to give
each block invocation its own binding. Upgrade from https://go.dev/dl/`,
			minGo, v, minGo)
	}
	return nil
}

// atLeast reports whether a Go version string ("go1.24.0") is >= want ("1.22").
func atLeast(got, want string) bool {
	g := parseVersion(strings.TrimPrefix(got, "go"))
	w := parseVersion(want)
	for i := range w {
		if i >= len(g) {
			return false
		}
		if g[i] != w[i] {
			return g[i] > w[i]
		}
	}
	return true
}

func parseVersion(s string) []int {
	// Trim pre-release suffixes such as "1.24rc1".
	for i, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			s = s[:i]
			break
		}
	}
	var out []int
	for _, part := range strings.Split(s, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

package pmtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionedBuildArtifacts(t *testing.T) {
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("release build runs in the Linux compiler image with sha256sum")
	}
	for _, input := range []string{"dev", "dev-" + strings.Repeat("a", 40), "1.2.3", "v1.2.3", "v0.0.1", "2.0.0", "v2.0.0"} {
		t.Run(input, func(t *testing.T) {
			dir := t.TempDir()
			fakeGo := filepath.Join(dir, "go")
			program := `set -eu
flags=
output=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -ldflags) shift; flags=$1 ;;
    -o) shift; output=$1 ;;
  esac
  shift
done
printf '%s/%s\n%s\n' "$GOOS" "$GOARCH" "$flags" > "$output"
`
			if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\n"+program), 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("make", "--no-print-directory", "-C", "../..", "build-native", "VERSION="+input, "IMAGE=registry.example/tooling:v9", "DIST="+filepath.Join(dir, "dist"))
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "MAKEFLAGS=", "MFLAGS=", "MAKELEVEL=0")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build failed: %v: %s", err, output)
			}
			selected := input
			if !strings.HasPrefix(input, "dev") {
				selected = "v" + strings.TrimPrefix(input, "v")
			}
			outputDir := filepath.Join(dir, "dist", selected)
			for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"} {
				name := "pmtool-" + selected + "-" + target
				data, err := os.ReadFile(filepath.Join(outputDir, name))
				if err != nil || !strings.HasPrefix(string(data), strings.ReplaceAll(target, "-", "/")+"\n") || !strings.Contains(string(data), "github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/pmtool.version="+selected+" ") || !strings.Contains(string(data), "github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/pmtool.defaultImage=registry.example/tooling:v9") {
					t.Fatalf("artifact %s lost the selected version/image: %v: %s", name, err, data)
				}
			}
			license, err := os.ReadFile(filepath.Join(outputDir, "LICENSE"))
			if err != nil {
				t.Fatal(err)
			}
			sourceLicense, err := os.ReadFile("../../LICENSE")
			if err != nil {
				t.Fatal(err)
			}
			if string(license) != string(sourceLicense) || !strings.HasPrefix(string(license), "MIT License\n") {
				t.Fatal("release artifacts lost the complete MIT license")
			}
			checksums, err := os.ReadFile(filepath.Join(outputDir, "SHA256SUMS"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(checksums), "  LICENSE\n") {
				t.Fatal("release license is missing from checksums")
			}
			verify := exec.Command("sha256sum", "-c", "SHA256SUMS")
			verify.Dir = outputDir
			if output, err := verify.CombinedOutput(); err != nil {
				t.Fatalf("checksums do not verify: %v: %s", err, output)
			}
		})
	}
}

func TestInvalidReleaseVersionDoesNotBuild(t *testing.T) {
	for _, version := range []string{"", "main", "1.2", "v01.2.3", "../../outside", "v1.2.3;echo bad", "v1.2.3\n", "dev-short", "dev-" + strings.Repeat("a", 40) + "\n", "dev-" + strings.Repeat("g", 40), "v1.0.0-rc.1"} {
		t.Run(version, func(t *testing.T) {
			dist := filepath.Join(t.TempDir(), "dist")
			cmd := exec.Command("make", "--no-print-directory", "-C", "../..", "build-native", "VERSION="+version, "DIST="+dist)
			cmd.Env = append(os.Environ(), "MAKEFLAGS=", "MFLAGS=", "MAKELEVEL=0")
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "VERSION must be") {
				t.Fatalf("invalid version was accepted: %q: %s", version, output)
			}
			if _, err := os.Stat(dist); !os.IsNotExist(err) {
				t.Fatal("invalid version created build output")
			}
		})
	}
}

func TestLauncherReportsInjectedVersion(t *testing.T) {
	previous := version
	version = "v1.2.3"
	defer func() { version = previous }()
	a, _, output := testApp(t)
	if a.run([]string{"version"}) != 0 || !strings.HasPrefix(output.String(), "pmtool v1.2.3 (") {
		t.Fatalf("injected version missing: %s", output)
	}
}

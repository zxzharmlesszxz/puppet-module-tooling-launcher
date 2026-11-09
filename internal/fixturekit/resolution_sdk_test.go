package fixturekit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type sdkRelease struct {
	metadata map[string]any
	archive  []byte
}

func sdkPackage(t *testing.T, name, version, dependency string) sdkRelease {
	t.Helper()
	metadata := map[string]any{"name": name, "version": version, "author": "test",
		"license": "MIT", "summary": "Forge resolution regression", "source": "https://example.test/module",
		"dependencies": []any{}}
	if dependency != "" {
		metadata["dependencies"] = []map[string]string{{"name": dependency, "version_requirement": ">= 1.0.0 < 2.0.0"}}
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name + "-" + version + "/metadata.json", Mode: 0644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return sdkRelease{metadata: metadata, archive: buffer.Bytes()}
}

func TestOpenVoxResolvesForgeFixturesOverAuthenticatedHTTPS(t *testing.T) {
	if os.Getenv("PMTOOL_FORGE_TESTS") != "1" {
		t.Skip("requires the pinned VoxBox Puppet/puppet_fixtures toolchain")
	}
	releases := map[string][]sdkRelease{
		"team-common": {sdkPackage(t, "team-common", "1.1.0", "team-apt"), sdkPackage(t, "team-common", "2.0.0", "")},
		"team-apt":    {sdkPackage(t, "team-apt", "1.2.0", "team-stdlib"), sdkPackage(t, "team-apt", "2.0.0", "")},
		"team-stdlib": {sdkPackage(t, "team-stdlib", "1.3.0", "")},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-read-secret" {
			http.Error(w, "missing fixture authorization", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/v3/releases" {
			name := r.URL.Query().Get("module")
			var result []map[string]any
			for _, release := range releases[name] {
				hash := sha256.Sum256(release.archive)
				version := release.metadata["version"].(string)
				result = append(result, map[string]any{"metadata": release.metadata, "file_uri": "/files/" + name + "-" + version + ".tar.gz",
					"file_sha256": hex.EncodeToString(hash[:])})
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{"results": result, "pagination": map[string]any{"next": nil}}); err != nil {
				t.Error(err)
			}
			return
		}
		for name, versions := range releases {
			for _, release := range versions {
				if r.URL.Path == "/files/"+name+"-"+release.metadata["version"].(string)+".tar.gz" {
					w.Header().Set("Content-Type", "application/gzip")
					if _, err := w.Write(release.archive); err != nil {
						t.Error(err)
					}
					return
				}
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	root := t.TempDir()
	write(t, filepath.Join(root, ".fixtures.yml"), "fixtures:\n  resolve_dependencies: true\n  forge_modules:\n    common: {repo: team/common, ref: '1.1.0'}\n")
	prepared, err := Prepare(root, Options{Native: true, Getenv: environment(map[string]string{
		"FORGE_URL": server.URL, "FORGE_TOKEN": "fixture-read-secret",
	})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	})
	ca := filepath.Join(t.TempDir(), "forge-ca.pem")
	write(t, ca, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})))
	cmd := exec.CommandContext(t.Context(), "bundle", "exec", "ruby", prepared.Installer, filepath.Join(prepared.Auth, "puppet.conf"))
	cmd.Dir = root
	cmd.Env = append(sdkEnvironment(), "FIXTURES_YML="+prepared.Manifest, "SSL_CERT_FILE="+ca)
	output, err := cmd.CombinedOutput()
	if strings.Contains(string(output), "fixture-read-secret") {
		t.Fatal("installer leaked its read token")
	}
	if err != nil {
		t.Fatalf("authenticated resolution failed: %v\n%s", err, output)
	}
	for name, version := range map[string]string{"common": "1.1.0", "apt": "1.2.0", "stdlib": "1.3.0"} {
		path := filepath.Join(root, "spec", "fixtures", "modules", name, "metadata.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var metadata struct{ Version string }
		if err := json.Unmarshal(data, &metadata); err != nil || metadata.Version != version {
			t.Fatalf("resolved %s version %q, want %q: %v", name, metadata.Version, version, err)
		}
	}
	if err := prepared.RemoveCredentials(); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "spec", "fixtures", "modules")); !os.IsNotExist(err) {
		t.Fatalf("native fixture path survived cleanup: %v", err)
	}
}

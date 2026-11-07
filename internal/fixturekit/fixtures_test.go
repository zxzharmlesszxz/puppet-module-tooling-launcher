package fixturekit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/templatekit"
)

func archive(t *testing.T, headers []*tar.Header, bodies []string) []byte {
	t.Helper()
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	for i, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(bodies[i])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
func fixtureArchive(t *testing.T) []byte {
	t.Helper()
	body := `{"name":"team-common","version":"1.0.0"}`
	return archive(t, []*tar.Header{{Name: "team-common-1.0.0/metadata.json", Mode: 0644, Size: int64(len(body))}, {Name: "team-common-1.0.0/types/package.pp", Mode: 0644, Size: int64(len("type Common::Package = String\n"))}}, []string{body, "type Common::Package = String\n"})
}
func itemFor(data []byte) Entry {
	digest := sha256.Sum256(data)
	return Entry{Module: "team-common", Version: "1.0.0", SHA256: hex.EncodeToString(digest[:])}
}
func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestPrepareVerifiedFixtureAndCleanup(t *testing.T) {
	data := fixtureArchive(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer read-secret" {
			t.Error("missing scoped credential")
		}
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			root := t.TempDir()
			item := itemFor(data)
			source := server.URL + "/download"
			original := "fixtures:\n  forge_modules:\n    stdlib: puppetlabs/stdlib\n"
			write(t, filepath.Join(root, "metadata.json"), `{"name":"team-sshd"}`)
			write(t, filepath.Join(root, ".fixtures.yml"), original)
			write(t, filepath.Join(root, ".sync.yml"), fmt.Sprintf("test_fixtures:\n  common:\n    source: %q\n    module: team-common\n    version: '1.0.0'\n    auth: forge\n    sha256: %q\n", source, item.SHA256))
			options := Options{Native: native, Client: server.Client(), Getenv: func(key string) string {
				if key == "FORGE_TOKEN" {
					return "read-secret"
				}
				if key == "FORGE_TOKEN_HOST" {
					return strings.TrimPrefix(server.URL, "https://")
				}
				return ""
			}}
			prepared, err := Prepare(t.Context(), root, options)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := os.ReadFile(filepath.Join(prepared.Directory, "fixtures.yml"))
			if err != nil {
				t.Fatal(err)
			}
			values, err := templatekit.YAML(manifest)
			if err != nil {
				t.Fatal(err)
			}
			links := values["fixtures"].(map[string]any)["symlinks"].(map[string]any)
			expectedRoot := "/repo"
			if native {
				expectedRoot = root
			}
			if links["sshd"] != expectedRoot || !strings.HasPrefix(links["common"].(string), expectedRoot+"/vendor/") {
				t.Fatal(links)
			}
			if strings.Contains(string(manifest), "read-secret") || strings.Contains(string(manifest), "Authorization") {
				t.Fatal("credential persisted")
			}
			actual, err := os.ReadFile(filepath.Join(root, ".fixtures.yml"))
			if err != nil || string(actual) != original {
				t.Fatal("tracked fixture manifest changed", err)
			}
			if _, err := os.Stat(filepath.Join(prepared.Directory, "common", "types", "package.pp")); err != nil {
				t.Fatal(err)
			}
			if err := prepared.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(prepared.Directory); !os.IsNotExist(err) {
				t.Fatal("staging was not removed", err)
			}
		})
	}
}
func TestDownloadRefusesRedirectsAndUnscopedCredentials(t *testing.T) {
	hits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, "https://elsewhere.example.test/file", http.StatusFound)
	}))
	defer server.Close()
	item := itemFor(fixtureArchive(t))
	item.Source = server.URL + "/download"
	item.Auth = "forge"
	for _, host := range []string{"", "wrong.example.test"} {
		_, err := download(t.Context(), item, Options{Client: server.Client(), Getenv: func(key string) string {
			if key == "FORGE_TOKEN" {
				return "secret"
			}
			return host
		}})
		if err == nil || hits != 0 {
			t.Fatal("unscoped credential sent", err, hits)
		}
	}
	item.Auth = ""
	_, err := download(t.Context(), item, Options{Client: server.Client()})
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") || hits != 1 {
		t.Fatal(err, hits)
	}
}
func TestDownloadChecksStatusAndDigest(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				if _, err := w.Write([]byte("wrong artifact")); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			item := itemFor(fixtureArchive(t))
			item.Source = server.URL + "/download"
			if _, err := download(t.Context(), item, Options{Client: server.Client()}); err == nil {
				t.Fatal("untrusted download accepted")
			}
		})
	}
}
func TestArchiveRejectsUnsafePathsLinksDuplicatesAndMetadata(t *testing.T) {
	good := `{"name":"team-common","version":"1.0.0"}`
	for _, name := range []string{"../escape", "/tmp/escape", "team-common-1.0.0/../../escape", "team-common-1.0.0/.git/config", "team-common-1.0.0/metadata.json"} {
		t.Run(name, func(t *testing.T) {
			headers := []*tar.Header{{Name: "team-common-1.0.0/metadata.json", Mode: 0644, Size: int64(len(good))}, {Name: name, Mode: 0644, Size: 1}}
			data := archive(t, headers, []string{good, "x"})
			if err := unpack(data, itemFor(data), t.TempDir()); err == nil {
				t.Fatal("unsafe entry accepted")
			}
		})
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo} {
		data := archive(t, []*tar.Header{{Name: "team-common-1.0.0/link", Typeflag: kind, Linkname: "/tmp/escape"}}, []string{""})
		if err := unpack(data, itemFor(data), t.TempDir()); err == nil {
			t.Fatal("special entry accepted")
		}
	}
	for _, body := range []string{`{"name":"other-common","version":"1.0.0"}`, `{"name":"team-common","version":"2.0.0"}`, good + good} {
		data := archive(t, []*tar.Header{{Name: "team-common-1.0.0/metadata.json", Mode: 0644, Size: int64(len(body))}}, []string{body})
		if err := unpack(data, itemFor(data), t.TempDir()); err == nil {
			t.Fatal("incorrect metadata accepted")
		}
	}
	data := fixtureArchive(t)
	for _, broken := range [][]byte{data[:len(data)-4], append(bytes.Clone(data), []byte("garbage")...)} {
		if err := unpack(broken, itemFor(broken), t.TempDir()); err == nil {
			t.Fatal("corrupt gzip accepted")
		}
	}
}
func TestConfigurationAndStagingGuards(t *testing.T) {
	root := t.TempDir()
	if prepared, err := Prepare(context.Background(), root, Options{}); err != nil || prepared != nil {
		t.Fatal(prepared, err)
	}
	for _, source := range []string{"http://forge.test/a", "https://user:secret@forge.test/a", "https://forge.test/a?token=secret", "https://forge.test/a#fragment"} {
		if _, err := sourceURL(source); err == nil {
			t.Fatal("unsafe source accepted", source)
		}
	}
	for _, body := range []string{"test_fixtures: []\n", "test_fixtures: null\n", "test_fixtures:\n  ../bad: {}\n", "test_fixtures:\n  common: {unknown: value}\n", "test_fixtures: {}\ntest_fixtures: {}\n"} {
		write(t, filepath.Join(root, ".sync.yml"), body)
		if _, err := entries(root); err == nil {
			t.Fatal("invalid settings accepted", body)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "vendor")); err != nil {
		t.Fatal(err)
	}
	if _, err := staging(root); err == nil {
		t.Fatal("vendor symlink accepted")
	}
	if err := rejectCollision(map[string]any{"forge_modules": map[string]any{"common": "team/common"}}, map[string]any{}, "common"); err == nil {
		t.Fatal("duplicate fixture accepted")
	}
}

func TestFailedPreparationRemovesStaging(t *testing.T) {
	data := []byte("wrong archive")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	write(t, filepath.Join(root, "metadata.json"), `{"name":"team-sshd"}`)
	write(t, filepath.Join(root, ".fixtures.yml"), "fixtures: {}\n")
	digest := itemFor(data).SHA256
	write(t, filepath.Join(root, ".sync.yml"), fmt.Sprintf("test_fixtures:\n  common:\n    source: %q\n    module: team-common\n    version: '1.0.0'\n    sha256: %q\n", server.URL+"/download", digest))
	if prepared, err := Prepare(t.Context(), root, Options{Client: server.Client()}); err == nil || prepared != nil {
		t.Fatal("bad archive accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, "vendor"))
	if err != nil || len(entries) != 0 {
		t.Fatal("failed staging remains", entries, err)
	}
}

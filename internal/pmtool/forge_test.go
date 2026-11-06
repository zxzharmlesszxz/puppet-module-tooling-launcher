package pmtool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func forgeFixture(t *testing.T, metadata string, copies int) []byte {
	t.Helper()
	var output bytes.Buffer
	compressed := gzip.NewWriter(&output)
	archive := tar.NewWriter(compressed)
	for range copies {
		body := []byte(metadata)
		if err := archive.WriteHeader(&tar.Header{Name: "team-sample-1.2.3/metadata.json", Size: int64(len(body)), Mode: 0644}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestForgeArchiveRequiresExactMetadataAndRegularLocalArtifact(t *testing.T) {
	a, _, _ := testApp(t)
	metadata := `{"name":"team-sample","version":"1.2.3"}`
	if err := os.WriteFile(filepath.Join(a.cwd, "metadata.json"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(a.cwd, "pkg"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.cwd, "pkg", "team-sample-1.2.3.tar.gz")
	for _, fixture := range []struct {
		body   []byte
		accept bool
	}{
		{forgeFixture(t, metadata, 1), true},
		{forgeFixture(t, metadata, 0), false},
		{forgeFixture(t, metadata, 2), false},
		{forgeFixture(t, `{"name":"team-other","version":"1.2.3"}`, 1), false},
		{[]byte("broken"), false},
	} {
		if err := os.WriteFile(path, fixture.body, 0600); err != nil {
			t.Fatal(err)
		}
		_, got, err := forgeArchive(a.cwd)
		if (err == nil) != fixture.accept || (fixture.accept && !bytes.Equal(got, fixture.body)) {
			t.Fatal("archive validation changed", err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(a.cwd, "metadata.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forgeArchive(a.cwd); err == nil {
		t.Fatal("symlink archive accepted")
	}
}

func TestForgeUploadPreservesV3BytesTokenScopeAndFailure(t *testing.T) {
	data := []byte{'b', 0, 255, '\n'}
	for _, status := range []int{201, 302, 401, 409, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/prefix/v3/releases" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("wrong upload contract")
				}
				var body map[string][]byte
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !bytes.Equal(body["file"], data) || len(body) != 1 {
					t.Error("V3 JSON/base64 bytes changed", err)
				}
				w.Header().Set("Location", "/credential-leak")
				w.WriteHeader(status)
			}))
			defer server.Close()
			err := uploadForge(t.Context(), server.Client(), server.URL+"/prefix/v3/releases", "fixture-token", data)
			if (err == nil) != (status == 201) || calls.Load() != 1 {
				t.Fatal("upload retried, redirected or accepted failure", calls.Load(), err)
			}
		})
	}
	for _, address := range []string{"", "http://forge.example", "https://user:secret@forge.example", "https://forge.example/?token=x", "https://forge.example/#fragment"} {
		if _, err := forgeEndpoint(address); err == nil {
			t.Fatal("unsafe Forge URL accepted")
		}
	}
	endpoint, err := forgeEndpoint("https://forge.example/prefix/")
	if err != nil || !strings.HasSuffix(endpoint, "/prefix/v3/releases") {
		t.Fatal(endpoint, err)
	}
}

package pmtool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maximumForgeArchive = 256 << 20

type forgeMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func forgeEndpoint(address string) (string, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("forge URL must be a credential-free HTTPS base URL without query or fragment")
	}
	return strings.TrimRight(u.String(), "/") + "/v3/releases", nil
}

func forgeArchive(root string) (string, []byte, error) {
	body, err := os.ReadFile(filepath.Join(root, "metadata.json")) // #nosec G304 -- User-selected module directory.
	if err != nil {
		return "", nil, err
	}
	var metadata forgeMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return "", nil, err
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*-[a-z][a-z0-9_]*$`).MatchString(metadata.Name) || !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(metadata.Version) {
		return "", nil, fmt.Errorf("publication requires owner-module metadata and a stable X.Y.Z version")
	}
	slug := metadata.Name + "-" + metadata.Version
	path := filepath.Join(root, "pkg", slug+".tar.gz")
	physical, err := filepath.EvalSymlinks(path)
	if err != nil || physical != path {
		return "", nil, fmt.Errorf("publication requires a regular local archive at pkg/%s.tar.gz", slug)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximumForgeArchive {
		return "", nil, fmt.Errorf("publication requires a regular archive of at most 256 MiB")
	}
	archive, err := os.Open(path) // #nosec G304 -- Metadata-derived path with validated name/version and no symlinks.
	if err != nil {
		return "", nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(archive, maximumForgeArchive+1))
	closeErr := archive.Close()
	if readErr != nil {
		return "", nil, readErr
	}
	if closeErr != nil {
		return "", nil, closeErr
	}
	if len(data) == 0 || len(data) > maximumForgeArchive {
		return "", nil, fmt.Errorf("archive is empty or exceeds 256 MiB")
	}
	if err := verifyForgeArchive(data, slug, metadata); err != nil {
		return "", nil, err
	}
	return path, data, nil
}

func verifyForgeArchive(data []byte, slug string, expected forgeMetadata) (caught error) {
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer func() { caught = errors.Join(caught, compressed.Close()) }()
	bounded := &io.LimitedReader{R: compressed, N: maximumForgeArchive + 1}
	reader := tar.NewReader(bounded)
	matches := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid module archive: %w", err)
		}
		if header.Name != slug+"/metadata.json" {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size > 1<<20 || header.Size <= 0 {
			return fmt.Errorf("archive metadata must be a regular JSON file of at most 1 MiB")
		}
		var got forgeMetadata
		decoder := json.NewDecoder(reader)
		if err := decoder.Decode(&got); err != nil || got != expected {
			return fmt.Errorf("archive metadata differs from the module; rebuild with module-build")
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return fmt.Errorf("archive metadata must contain one JSON document")
		}
		matches++
	}
	if _, err := io.Copy(io.Discard, bounded); err != nil {
		return fmt.Errorf("invalid module archive: %w", err)
	}
	if bounded.N == 0 {
		return fmt.Errorf("unpacked archive exceeds 256 MiB")
	}
	if matches != 1 {
		return fmt.Errorf("archive must contain exactly one matching metadata.json")
	}
	return nil
}

func uploadForge(ctx context.Context, client *http.Client, endpoint, token string, data []byte) error {
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("forge publish token is required and must not contain newlines")
	}
	body, err := json.Marshal(map[string][]byte{"file": data})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "pmtool/"+version)
	isolated := *client
	isolated.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := isolated.Do(request)
	if err != nil {
		return fmt.Errorf("forge upload failed; verify server state before retrying")
	}
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("forge upload refused: HTTP %d (no redirect or automatic retry)", response.StatusCode)
	}
	return closeErr
}

func (a *app) publishForge(args []string, dry bool) int {
	o := a.parseOperation(args)
	check(a.preflightForge(dry))
	endpoint := must(forgeEndpoint(a.getenv("FORGE_URL")))
	path, data, err := forgeArchive(o.Module)
	check(err)
	if dry {
		must(fmt.Fprintf(a.stdout, "Would upload %s to %s\n", path, endpoint))
		return 0
	}
	a.notice("RUN", "forge-publish", "1;36")
	check(uploadForge(context.Background(), &http.Client{Timeout: 2 * time.Minute}, endpoint, a.getenv("FORGE_PUBLISH_TOKEN"), data))
	a.notice("OK", "Forge accepted "+filepath.Base(path), "1;32")
	a.notice("NEXT", "Verify the published version and canary before cleanup.", "36")
	return 0
}

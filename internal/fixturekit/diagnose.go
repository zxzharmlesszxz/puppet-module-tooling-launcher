package fixturekit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	publicForge       = "https://forgeapi.puppet.com"
	maxCatalogPages   = 100
	maxDoctorCatalogs = 64
)

var (
	forgeModule = regexp.MustCompile(`^[A-Za-z0-9_]+[-/][A-Za-z0-9_]+$`)
	exactRef    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
)

type forgeTarget struct{ Name, Ref string }

type forgeRelease struct {
	FileURI string `json:"file_uri"`
	SHA256  string `json:"file_sha256"`
	MD5     string `json:"file_md5"`
	Version string `json:"version"`
	Meta    struct {
		Name         string `json:"name"`
		Version      string `json:"version"`
		Dependencies []struct {
			Name string `json:"name"`
			Ref  string `json:"version_requirement"`
		} `json:"dependencies"`
	} `json:"metadata"`
}

type forgePage struct {
	Results    []forgeRelease `json:"results"`
	Pagination *struct {
		Next *string `json:"next"`
	} `json:"pagination"`
}

type forgeProbe struct {
	base   *url.URL
	token  string
	client *http.Client
}

type ForgeReport struct {
	Catalogs, Archives, Unpinned int
}

// DiagnoseForge checks the API shape expected by the SDK and probes the gzip header
// of each exactly selected archive. OpenVox remains responsible for resolving ranges.
func DiagnoseForge(ctx context.Context, root string, getenv func(string) string) (ForgeReport, error) {
	return diagnoseForge(ctx, root, getenv, nil)
}

func diagnoseForge(ctx context.Context, root string, getenv func(string) string, transport http.RoundTripper) (ForgeReport, error) {
	if err := rejectRetiredSettings(root); err != nil {
		return ForgeReport{}, err
	}
	data, modules, err := fixtureManifest(root)
	if err != nil || len(modules) == 0 {
		return ForgeReport{}, err
	}
	resolve, err := resolveDependencies(data, modules)
	if err != nil {
		return ForgeReport{}, err
	}
	repository, token, err := forgeSettings(getenv)
	if err != nil {
		return ForgeReport{}, err
	}
	if repository == "" {
		repository = publicForge
	}
	targets, err := diagnosticTargets(data["fixtures"].(map[string]any), modules)
	if err != nil {
		return ForgeReport{}, err
	}
	base, err := url.Parse(repository)
	if err != nil {
		return ForgeReport{}, fmt.Errorf("invalid Forge repository URL")
	}
	return newForgeProbe(base, token, transport).diagnose(ctx, targets, resolve)
}

func diagnosticTargets(fixtures, modules map[string]any) ([]forgeTarget, error) {
	groups, _ := fixtures["defaults"].(map[string]any)
	defaults, _ := groups["forge_modules"].(map[string]any)
	var targets []forgeTarget
	for name, raw := range modules {
		var entry map[string]any
		switch value := raw.(type) {
		case string:
			entry = map[string]any{"repo": value}
		case map[string]any:
			entry = value
		default:
			return nil, fmt.Errorf("fixture %s must be a Forge module name or mapping", name)
		}
		repo, _ := entry["repo"].(string)
		if !forgeModule.MatchString(repo) {
			return nil, fmt.Errorf("fixture %s requires a Forge owner/module name", name)
		}
		ref, err := diagnosticRef(entry, defaults)
		if err != nil {
			return nil, fmt.Errorf("fixture %s: %w", name, err)
		}
		targets = append(targets, forgeTarget{strings.ReplaceAll(repo, "/", "-"), ref})
	}
	slices.SortFunc(targets, func(a, b forgeTarget) int { return strings.Compare(a.Name+"@"+a.Ref, b.Name+"@"+b.Ref) })
	return targets, nil
}

func diagnosticRef(entry, defaults map[string]any) (string, error) {
	for _, key := range []string{"ref", "tag"} {
		value, ok := entry[key]
		if !ok {
			value = defaults[key]
		}
		if value == nil {
			continue
		}
		ref, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("%s must be a string", key)
		}
		if ref != "" {
			return ref, nil
		}
	}
	return "", nil
}

func newForgeProbe(base *url.URL, token string, transport http.RoundTripper) *forgeProbe {
	p := &forgeProbe{base: base, token: token}
	p.client = &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return http.ErrUseLastResponse
		}
		if !strings.EqualFold(req.URL.Host, base.Host) {
			req.Header.Del("Authorization")
		}
		// Release metadata must come from the configured proxy. Archive
		// downloads may use a signed CDN redirect.
		if via[0].Header.Get("Range") == "" && !strings.EqualFold(req.URL.Host, base.Host) {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	return p
}

func (p *forgeProbe) get(ctx context.Context, address *url.URL, archive bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid Forge request URL")
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	if archive {
		req.Header.Set("Range", "bytes=0-1")
		req.Header.Set("Accept-Encoding", "identity")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// HTTP errors can include signed redirect URLs. Do not expose them.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("could not reach the Forge endpoint")
	}
	return resp, nil
}

func forgeHTTPError(status int, operation string) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%s: authorization failed (HTTP %d); check FORGE_TOKEN and its read scope", operation, status)
	case http.StatusNotFound:
		return fmt.Errorf("%s: not found (HTTP %d)", operation, status)
	default:
		return fmt.Errorf("%s: HTTP %d", operation, status)
	}
}

func (p *forgeProbe) relative(path string) (*url.URL, error) {
	u, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") ||
		u.Host != "" || u.Scheme != "" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("expected a relative Forge URI starting with /")
	}
	return p.base.ResolveReference(u), nil
}

func (p *forgeProbe) catalog(ctx context.Context, name string) (map[string]forgeRelease, error) {
	address := *p.base
	address.Path = strings.TrimRight(address.Path, "/") + "/v3/releases"
	address.RawPath = ""
	address.RawQuery = url.Values{"module": {name}, "sort_by": {"version"}}.Encode()
	visited := map[string]bool{}
	releases := map[string]forgeRelease{}
	for {
		if len(visited) >= maxCatalogPages || visited[address.String()] {
			return nil, fmt.Errorf("release catalog has repeated or excessive pagination")
		}
		visited[address.String()] = true
		page, err := p.readPage(ctx, &address)
		if err != nil {
			return nil, err
		}
		for _, release := range page.Results {
			if err := p.addRelease(name, releases, release); err != nil {
				return nil, err
			}
		}
		if page.Pagination.Next == nil {
			break
		}
		next, err := p.relative(*page.Pagination.Next)
		if err != nil {
			return nil, fmt.Errorf("invalid release catalog pagination: %w", err)
		}
		address = *next
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}
	return releases, nil
}

func (p *forgeProbe) readPage(ctx context.Context, address *url.URL) (forgePage, error) {
	var page forgePage
	resp, err := p.get(ctx, address, false)
	if err != nil {
		return page, err
	}
	defer closeProbeBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return page, forgeHTTPError(resp.StatusCode, "release catalog")
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&page); err != nil || page.Results == nil || page.Pagination == nil {
		return page, fmt.Errorf("invalid or incomplete release catalog response")
	}
	return page, nil
}

func closeProbeBody(body io.Closer) {
	_ = body.Close() // Closing a read-only HTTP response does not change the probe result.
}

func (p *forgeProbe) addRelease(name string, releases map[string]forgeRelease, release forgeRelease) error {
	version := release.Meta.Version
	if strings.ReplaceAll(release.Meta.Name, "/", "-") != name || !exactRef.MatchString(version) ||
		(release.Version != "" && release.Version != version) || (release.SHA256 == "" && release.MD5 == "") {
		return fmt.Errorf("incomplete or inconsistent release metadata (name, version or archive checksum)")
	}
	if _, err := p.relative(release.FileURI); err != nil {
		return fmt.Errorf("invalid release archive URI: %w", err)
	}
	releases[version] = release
	return nil
}

func (p *forgeProbe) archive(ctx context.Context, uri string) error {
	address, err := p.relative(uri)
	if err != nil {
		return err
	}
	resp, err := p.get(ctx, address, true)
	if err != nil {
		return err
	}
	defer closeProbeBody(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return forgeHTTPError(resp.StatusCode, "archive download")
	}
	var header [2]byte
	if _, err := io.ReadFull(resp.Body, header[:]); err != nil {
		return fmt.Errorf("archive download is empty or unreadable")
	}
	if header != [2]byte{0x1f, 0x8b} {
		return fmt.Errorf("archive download does not start with a gzip header")
	}
	return nil
}

func (p *forgeProbe) diagnose(ctx context.Context, targets []forgeTarget, resolve bool) (ForgeReport, error) {
	var report ForgeReport
	catalogs := map[string]map[string]forgeRelease{}
	checked := map[forgeTarget]bool{}
	for len(targets) > 0 {
		target := targets[0]
		targets = targets[1:]
		if checked[target] {
			continue
		}
		checked[target] = true
		catalog, found := catalogs[target.Name]
		if !found {
			if len(catalogs) >= maxDoctorCatalogs {
				return report, fmt.Errorf("diagnostic exceeded %d Forge module catalogs", maxDoctorCatalogs)
			}
			var err error
			catalog, err = p.catalog(ctx, target.Name)
			if err != nil {
				return report, fmt.Errorf("%s: %w", target.Name, err)
			}
			catalogs[target.Name] = catalog
			report.Catalogs++
		}
		if !exactRef.MatchString(target.Ref) {
			report.Unpinned++
			continue
		}
		release, ok := catalog[target.Ref]
		if !ok {
			return report, fmt.Errorf("%s@%s: requested release not found", target.Name, target.Ref)
		}
		if err := p.archive(ctx, release.FileURI); err != nil {
			return report, fmt.Errorf("%s@%s: archive unavailable: %w", target.Name, target.Ref, err)
		}
		report.Archives++
		if resolve {
			for _, dependency := range release.Meta.Dependencies {
				if !forgeModule.MatchString(dependency.Name) || dependency.Ref == "" {
					return report, fmt.Errorf("%s@%s: incomplete dependency metadata", target.Name, target.Ref)
				}
				targets = append(targets, forgeTarget{strings.ReplaceAll(dependency.Name, "/", "-"), dependency.Ref})
			}
		}
	}
	return report, nil
}

package pmtool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	documentation "github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/docs"
)

func TestEmbeddedDocumentationCoversCommandCatalog(t *testing.T) {
	entries, err := documentation.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var documents []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			documents = append(documents, documentation.Read(entry.Name()))
		}
	}
	body := strings.Join(documents, "\n")
	for name := range launcherManifest().Commands {
		pattern := regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(name) + `([^A-Za-z0-9_-]|$)`)
		if !pattern.MatchString(body) {
			t.Errorf("catalog command missing from embedded documentation: %s", name)
		}
	}
	for _, name := range launcherManifest().Options {
		if !strings.Contains(body, "--"+name) {
			t.Errorf("catalog option missing from embedded documentation: --%s", name)
		}
	}
}

func TestDocumentedSectionCommandsResolveLocally(t *testing.T) {
	entries, err := documentation.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`pmtool docs ([a-z]+) --section (?:'([^']+)'|"([^"]+)"|([a-z]+))`)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		for _, match := range pattern.FindAllStringSubmatch(documentation.Read(entry.Name()), -1) {
			t.Run(entry.Name()+"/"+match[0], func(t *testing.T) {
				section := match[2] + match[3] + match[4]
				a, driver, output := testApp(t)
				if code := a.run([]string{"docs", match[1], "--section", section}); code != 0 || output.Len() == 0 {
					t.Fatal("documented section command failed")
				}
				if len(driver.calls)+len(driver.plans)+len(driver.queries) != 0 {
					t.Fatal("documentation contacted external tools")
				}
			})
		}
	}
}

func TestPublicMakeTargetsHaveSourceDocumentation(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	targets := regexp.MustCompile(`(?m)^([A-Za-z0-9_-]+):[^\n]*## `).FindAllStringSubmatch(string(body), -1)
	if len(targets) == 0 {
		t.Fatal("Make help target inventory is empty")
	}
	guide := documentation.Read("DEVELOPMENT.md")
	for _, target := range targets {
		name := target[1]
		pattern := regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(name) + `([^A-Za-z0-9_-]|$)`)
		if !pattern.MatchString(guide) {
			t.Errorf("public Make target missing from development guide: %s", name)
		}
	}
}

func TestReadmeArtifactVersionsMatchBuildExamples(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	builds := regexp.MustCompile(`(?m)^make build VERSION=(v[0-9]+\.[0-9]+\.[0-9]+)$`).FindAllStringSubmatch(string(body), -1)
	versions := map[string]bool{}
	for _, build := range builds {
		versions[build[1]] = true
	}
	paths := regexp.MustCompile("`dist/(v[0-9]+\\.[0-9]+\\.[0-9]+)/`").FindAllStringSubmatch(string(body), -1)
	for _, path := range paths {
		if !versions[path[1]] {
			t.Errorf("README artifact path %s has no matching build VERSION example", path[0])
		}
	}
	if len(builds) == 0 || len(paths) == 0 {
		t.Fatal("README must demonstrate a versioned build and artifact directory")
	}
}

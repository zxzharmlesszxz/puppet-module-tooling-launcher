package pmtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnitFixtureFailureStopsBeforeSDK(t *testing.T) {
	a, d, output := testApp(t)
	if err := os.WriteFile(filepath.Join(a.cwd, ".sync.yml"), []byte("test_fixtures: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code := a.executeSequence(config{Docker: "docker"}, imageID, []string{"voxbox", "--stage", "unit"}, false, false)
	if code == 0 || len(d.plans) != 0 || !strings.Contains(output.String(), "test fixtures") {
		t.Fatal(code, d.plans, output.String())
	}
}
func TestValidationOnlyDoesNotFetchFixtures(t *testing.T) {
	a, d, _ := testApp(t)
	if err := os.WriteFile(filepath.Join(a.cwd, ".sync.yml"), []byte("test_fixtures: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code := a.executeSequence(config{Docker: "docker"}, imageID, []string{"voxbox", "--stage", "validate"}, false, false)
	if code != 0 || len(d.plans) != 1 {
		t.Fatal(code, d.plans)
	}
}
func TestUnitFixturePreviewDoesNotReadOrFetch(t *testing.T) {
	a, d, _ := testApp(t)
	if err := os.WriteFile(filepath.Join(a.cwd, ".sync.yml"), []byte("test_fixtures: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := a.executeSequence(config{Docker: "docker"}, imageID, []string{"voxbox", "--stage", "unit"}, false, true); code != 0 {
		t.Fatal(code)
	}
	if len(d.plans) != 0 {
		t.Fatal("preview executed a worker")
	}
	if _, err := os.Stat(filepath.Join(a.cwd, "vendor")); !os.IsNotExist(err) {
		t.Fatal("preview created staging", err)
	}
}
func TestFixtureManifestAddsOnlyTheNonsecretWorkerSetting(t *testing.T) {
	p := plan{Args: []string{"run", "--rm", "sdk:tag", "spec"}}
	p = withFixtureManifest(p, "/repo/vendor/.pmtool-fixtures-test/fixtures.yml", false)
	joined := strings.Join(p.Args, " ")
	if !strings.Contains(joined, "--env FIXTURES_YML=/repo/vendor/") || strings.Contains(joined, "FORGE_TOKEN") {
		t.Fatal(p.Args)
	}
	p = withFixtureManifest(plan{}, "/checkout/vendor/fixture.yml", true)
	if p.Environment["FIXTURES_YML"] != "/checkout/vendor/fixture.yml" {
		t.Fatal(p.Environment)
	}
}

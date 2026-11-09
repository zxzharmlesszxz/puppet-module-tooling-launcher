package pmtool

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsIndependentChecksWithoutWritingConfig(t *testing.T) {
	a, driver, out := testApp(t)
	driver.rendered = []byte(imageID + "\n")
	path := filepath.Join(a.cwd, "config.json")
	if status := a.run([]string{"--config", path, "doctor", "--provider", "gitlab"}); status != 1 {
		t.Fatalf("status %d, output %q", status, out)
	}
	for _, line := range []string{"[OK] Origin: gitlab team/puppet-sample", "[FAIL] Template:", "[OK] VoxBox:", "[SKIP] Forge:"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("missing %q in %s", line, out)
		}
	}
	if len(driver.plans) != 0 || len(driver.queries) < 2 || driver.queries[0].Args[0] != "info" {
		t.Fatalf("unexpected execution plans: %+v %+v", driver.plans, driver.queries)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("doctor created a config: %v", err)
	}
}

func TestDoctorWithInvalidSettingsStillChecksVoxBoxAndForge(t *testing.T) {
	a, driver, out := testApp(t)
	driver.rendered = []byte(imageID + "\n")
	if err := os.WriteFile(filepath.Join(a.cwd, ".sync.yml"), []byte("template: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if status := a.run([]string{"doctor"}); status != 1 {
		t.Fatalf("status %d, output %q", status, out)
	}
	for _, line := range []string{"[FAIL] Origin: .sync.yml", "[FAIL] Template: .sync.yml", "[OK] VoxBox:", "[SKIP] Forge:"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("missing %q in %s", line, out)
		}
	}
}

func TestDoctorDryRunAndHelpSkipExternalChecks(t *testing.T) {
	a, driver, out := testApp(t)
	if status := a.run([]string{"--dry-run", "doctor"}); status != 0 || !strings.Contains(out.String(), "Would check origin") {
		t.Fatalf("dry run status %d, output %q", status, out)
	}
	out.Reset()
	if status := a.run([]string{"doctor", "--help"}); status != 0 || !strings.Contains(out.String(), doctorUsage) {
		t.Fatalf("help status %d, output %q", status, out)
	}
	if len(driver.plans)+len(driver.queries)+len(driver.calls) != 0 {
		t.Fatal("doctor dry run or help called the driver")
	}
}

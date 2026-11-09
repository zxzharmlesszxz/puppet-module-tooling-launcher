package pmtool

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type inspectingFixtureDriver struct {
	*fakeDriver
	inspect func(plan, int)
}

func (d *inspectingFixtureDriver) execute(p plan) int {
	d.inspect(p, len(d.plans))
	return d.fakeDriver.execute(p)
}

func configuredForge(t *testing.T, a *app) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(a.cwd, ".fixtures.yml"), []byte("fixtures:\n  forge_modules:\n    common: {repo: team/common, ref: '1.0.0'}\n    stdlib: puppetlabs/stdlib\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a.getenv = func(key string) string {
		return map[string]string{"FORGE_URL": "https://forge.example.test", "FORGE_TOKEN": "not-printed-secret"}[key]
	}
}

func authDirectory(t *testing.T, p plan, native bool) string {
	t.Helper()
	if native {
		return filepath.Dir(p.Environment["FIXTURES_YML"])
	}
	return fixtureMountSource(t, p, "/pmtool-forge")
}

func fixtureMountSource(t *testing.T, p plan, destination string) string {
	t.Helper()
	for i, arg := range p.Args {
		if arg != "--mount" || i+1 == len(p.Args) {
			continue
		}
		fields, err := csv.NewReader(strings.NewReader(p.Args[i+1])).Read()
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(fields, "dst="+destination) {
			for _, field := range fields {
				if source, ok := strings.CutPrefix(field, "src="); ok {
					return source
				}
			}
		}
	}
	t.Fatal("missing fixture mount", destination)
	return ""
}

func TestAuthenticatedForgePreparationAndSecretRemoval(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "docker", true: "native"}[native], func(t *testing.T) {
			a, d, output := testApp(t)
			configuredForge(t, &a)
			var auth, modules string
			a.driver = &inspectingFixtureDriver{fakeDriver: d, inspect: func(p plan, index int) {
				joined := strings.Join(p.Args, " ")
				if strings.Contains(joined, "not-printed-secret") || len(p.Input) != 0 {
					t.Fatal("credential exposed in worker arguments or stdin")
				}
				for key, value := range p.Environment {
					if strings.HasPrefix(key, "FORGE_") && value != "" {
						t.Fatal("Forge credentials forwarded to worker environment")
					}
				}
				if index == 0 {
					if !strings.Contains(joined, "fixtures:prep") {
						t.Fatal("standard preparation task missing", p.Args)
					}
					auth = authDirectory(t, p, native)
					config, err := os.ReadFile(filepath.Join(auth, "puppet.conf"))
					if err != nil || !strings.Contains(string(config), "forge_authorization = Bearer not-printed-secret") {
						t.Fatal("native installer config missing", err)
					}
					if !native {
						modules = fixtureMountSource(t, p, "/repo/spec/fixtures/modules")
						if err := os.WriteFile(filepath.Join(modules, "prepared"), []byte("installed dependencies"), 0600); err != nil {
							t.Fatal(err)
						}
					}
				} else {
					if !strings.HasSuffix(joined, "spec:standalone fixtures:clean") || strings.Contains(joined, "dst=/pmtool-forge") || p.Environment["FIXTURES_YML"] != "" {
						t.Fatal("spec worker retained preparation inputs or downloads fixtures again", p)
					}
					if _, err := os.Stat(auth); !os.IsNotExist(err) {
						t.Fatal("credentials still exist when specs start", err)
					}
					if !native {
						if fixtureMountSource(t, p, "/repo/spec/fixtures/modules") != modules {
							t.Fatal("specs use a different dependency directory")
						}
						data, err := os.ReadFile(filepath.Join(modules, "prepared"))
						if err != nil || string(data) != "installed dependencies" {
							t.Fatal("prepared dependencies are unavailable to specs", err)
						}
					}
				}
			}}
			if code := a.executeSequence(config{Docker: "docker"}, imageID, []string{"voxbox", "--stage", "unit"}, native, false); code != 0 || len(d.plans) != 2 {
				t.Fatal(code, d.plans, output.String())
			}
			if !native {
				for _, p := range d.plans {
					joined := strings.Join(p.Args, " ")
					if !strings.Contains(joined, "dst=/repo/spec/fixtures/modules") || strings.Contains(joined, "--tmpfs /repo/spec/fixtures/modules") || strings.Contains(joined, "docker.sock") {
						t.Fatal("prep/spec do not share the isolated fixtures directory", p.Args)
					}
				}
			}
			if _, err := os.Stat(filepath.Dir(auth)); !os.IsNotExist(err) {
				t.Fatal("private installer staging remains", err)
			}
			if strings.Contains(output.String(), "not-printed-secret") {
				t.Fatal("credential leaked to output")
			}
		})
	}
}

func TestFailedForgePreparationStopsSpecsAndCleansCredentials(t *testing.T) {
	for _, codes := range [][]int{{17}, {0, 23}} {
		a, d, _ := testApp(t)
		configuredForge(t, &a)
		d.results = slices.Clone(codes)
		var auth string
		a.driver = &inspectingFixtureDriver{fakeDriver: d, inspect: func(p plan, i int) {
			if i == 0 {
				auth = authDirectory(t, p, true)
			}
		}}
		if code := a.executeSequence(config{}, "", []string{"voxbox", "--stage", "unit"}, true, false); code != codes[len(codes)-1] || len(d.plans) != len(codes) {
			t.Fatal("failure was hidden or specs ran after failed prep", code, d.plans)
		}
		if _, err := os.Stat(filepath.Dir(auth)); !os.IsNotExist(err) {
			t.Fatal("failed worker left private installer inputs", err)
		}
	}
}

func TestValidationAndPreviewDoNotPrepareForgeFixtures(t *testing.T) {
	for _, preview := range []bool{false, true} {
		a, d, _ := testApp(t)
		configuredForge(t, &a)
		if err := os.WriteFile(filepath.Join(a.cwd, ".fixtures.yml"), []byte("fixtures: []\n"), 0600); err != nil {
			t.Fatal(err)
		}
		stage := "validate"
		if preview {
			stage = "unit"
		}
		if code := a.executeSequence(config{Docker: "docker"}, imageID, []string{"voxbox", "--stage", stage}, false, preview); code != 0 {
			t.Fatal(code)
		}
		expected := 1
		if preview {
			expected = 0
		}
		if len(d.plans) != expected {
			t.Fatal("validation or preview prepared fixtures", d.plans)
		}
	}
}

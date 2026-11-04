package templatekit

import (
	"reflect"
	"strings"
	"testing"
)

func TestMigrationRemovesOnlyFixtureSymlinks(t *testing.T) {
	for _, body := range []string{
		"# Keep fixture notes\nfixtures:\n  forge_modules:\n    stdlib: puppetlabs/stdlib\n  symlinks:\n    sample: source\nother:\n  symlinks: keep\n",
		"fixtures: {forge_modules: {stdlib: puppetlabs/stdlib}, symlinks: {sample: source}}\nother: {symlinks: keep}\n",
		"fixtures:\n    symlinks: {sample: source}\n    forge_modules:\n        stdlib: puppetlabs/stdlib\nother: {symlinks: keep}\n",
	} {
		t.Run(body, func(t *testing.T) {
			before, err := YAML([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			delete(mapping(before["fixtures"]), "symlinks")
			updated, err := migrateFixtures([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			after, err := YAML(updated)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("migration changed unrelated fixture data: %s: %v", updated, err)
			}
			if strings.HasPrefix(body, "#") && !strings.Contains(string(updated), "# Keep fixture notes") {
				t.Fatal("migration lost fixture comments")
			}
			repeated, err := migrateFixtures(updated)
			if err != nil || string(repeated) != string(updated) {
				t.Fatal("migration is not idempotent", err)
			}
		})
	}
}

func TestFixtureValidationRejectsMalformedMappingsWithoutWrites(t *testing.T) {
	for _, body := range []string{
		"fixtures: []\n", "fixtures: broken\n",
		"fixtures: {symlinks: []}\n", "fixtures: {symlinks: broken}\n",
		"fixtures: {symlinks: {sample: source}, symlinks: {}}\n",
	} {
		t.Run(body, func(t *testing.T) {
			_, e := fixture(t)
			syncFixture(t, e)
			put(t, e.Root, ".fixtures.yml", body)
			put(t, e.Root, "metadata.json", `{"name":"team-sample","version":"1.2.3","template-ref":"v1"}`)
			before := snapshot(t, e.Root)
			requireError(t, e.Run(false, false), ".fixtures.yml")
			if err := Migrate(e.Root); err == nil {
				t.Fatal("migration accepted malformed fixture mappings")
			}
			if !reflect.DeepEqual(before, snapshot(t, e.Root)) {
				t.Fatal("failed fixture validation wrote files")
			}
		})
	}
}

func TestMigrationPreservesUntouchedFixturesAndRejectsSharedAliases(t *testing.T) {
	for _, body := range []string{"", "fixtures: {}\n", "fixtures: null\n", "# Keep this layout\nfixtures:\n    forge_modules: {stdlib: puppetlabs/stdlib}\n"} {
		updated, err := migrateFixtures([]byte(body))
		if err != nil || string(updated) != body {
			t.Fatal("migration rewrote untouched fixtures", err)
		}
	}
	body := "shared: &shared {symlinks: {sample: source}}\nfixtures: *shared\n"
	_, err := migrateFixtures([]byte(body))
	requireError(t, err, "expand the fixtures alias")
	body = "fixtures: {symlinks: &shared {sample: source}}\nother: *shared\n"
	_, err = migrateFixtures([]byte(body))
	requireError(t, err, "expand shared aliases")
	body = "shared: &shared {symlinks: {sample: source}}\nfixtures: {<<: *shared}\n"
	_, err = migrateFixtures([]byte(body))
	requireError(t, err, "expand inherited fixture symlinks")
}

func TestMigrationPreservesMetadataFormattingWithoutPDKFields(t *testing.T) {
	root := t.TempDir()
	metadata := "{ \"name\": \"team-sample\", \"version\": \"1.2.3\" }\n"
	put(t, root, "metadata.json", metadata)
	if err := Migrate(root); err != nil {
		t.Fatal(err)
	}
	if read(t, root, "metadata.json") != metadata {
		t.Fatal("migration reformatted unrelated metadata")
	}
}

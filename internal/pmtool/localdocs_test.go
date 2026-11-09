package pmtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShortHelpAndDefaultFullManualAreLocal(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		a, d, out := testApp(t)
		if a.run(args) != 0 || !strings.Contains(out.String(), "Usage:\n  pmtool [global flags]") || strings.Count(out.String(), "\n") > 56 || strings.Contains(out.String(), "## Responsibilities") || strings.Contains(out.String(), "# pmtool") {
			t.Fatal("help is not concise", args, out.String())
		}
		for _, command := range []string{"sync", "audit", "check", "mr-create", "mr-merge", "pmtool docs tokens", "pmtool docs inputs"} {
			if !strings.Contains(out.String(), "\n  "+command+"  ") {
				t.Fatal("help command lacks its own row", command, out.String())
			}
		}
		for _, flag := range []string{"--config", "--image", "--docker", "--executor", "--color", "--dry-run", "-h, --help", "--version"} {
			if !strings.Contains(out.String(), "\n  "+flag+" ") {
				t.Fatal("help flag lacks its own row", flag, out.String())
			}
		}
		if !strings.HasSuffix(out.String(), "Use \"pmtool docs usage\" for the full command index.\n") {
			t.Fatal("help footer omitted the command index", out.String())
		}
		if !strings.HasPrefix(out.String(), "pmtool "+version+" — ") {
			t.Fatal("help header omitted the build version", out.String())
		}
		if len(d.calls)+len(d.plans)+len(d.queries) != 0 {
			t.Fatal("help contacted an external tool")
		}
	}
	for _, args := range [][]string{{"docs"}, {"docs", "launcher"}} {
		a, d, out := testApp(t)
		if a.run(args) != 0 || out.String() != manual {
			t.Fatal("default docs did not return the complete former help manual")
		}
		if len(d.calls)+len(d.plans)+len(d.queries) != 0 {
			t.Fatal("docs contacted an external tool")
		}
		if _, err := os.Stat(filepath.Join(a.home, ".config")); !os.IsNotExist(err) {
			t.Fatal("docs created configuration", err)
		}
	}
}
func TestNewDocumentationTopicsAndCompletionRemainOffline(t *testing.T) {
	for _, topic := range []string{"tokens", "inputs", "fixtures"} {
		a, d, out := testApp(t)
		if a.run([]string{"docs", topic}) != 0 || out.Len() == 0 {
			t.Fatal("new docs topic unavailable", topic)
		}
		if len(d.calls)+len(d.plans)+len(d.queries) != 0 {
			t.Fatal("documentation required tools or credentials")
		}
		a, d, out = testApp(t)
		if a.run([]string{"__complete", "docs", topic[:2]}) != 0 || out.String() != topic+"\n" || len(d.calls)+len(d.plans)+len(d.queries) != 0 {
			t.Fatal("new docs completion unavailable", topic, out.String())
		}
	}
	a, _, out := testApp(t)
	if a.run([]string{"docs", "--section", "list"}) != 0 || !strings.Contains(out.String(), "Responsibilities") {
		t.Fatal("default manual section index failed")
	}
}

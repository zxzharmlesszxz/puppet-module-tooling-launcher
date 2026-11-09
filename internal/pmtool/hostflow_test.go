package pmtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/workflow"
)

func TestNativeHostStatusWithoutImageOrConfig(t *testing.T) {
	a, d, out := testApp(t)
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"remote", "add", "origin", "https://git.example.invalid/team/puppet-sample.git"}} {
		command := exec.Command("git", append([]string{"-C", a.cwd}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatal(string(output), err)
		}
	}
	if a.run([]string{"status"}) != 0 || len(d.calls) != 0 || len(d.plans) != 0 {
		t.Fatal("status used a tool container")
	}
	if !strings.Contains(out.String(), "[OK] status completed") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(filepath.Join(a.home, ".config", "puppet-module-tooling", "config.json")); !os.IsNotExist(err) {
		t.Fatal("host Git command created image config")
	}
}
func TestHostCommandHelpAndQuotedHints(t *testing.T) {
	a, d, out := testApp(t)
	if a.run([]string{"release-tag", "--help"}) != 0 || len(d.calls) != 0 || out.Len() == 0 {
		t.Fatal("host help requires tools")
	}
	got := quotedCommand([]string{"pmtool", "mr", "--title", "[REL] Release 1.2.3", "--module", "/tmp/a b"})
	if got != "pmtool mr --title '[REL] Release 1.2.3' --module '/tmp/a b'" {
		t.Fatal(got)
	}
}
func TestRubyLifecycleIsNotEmbedded(t *testing.T) {
	for _, name := range []string{"gitlab.rb", "github.rb", "hosting.rb", "providers.rb", "module_release.rb", "module_main_sync.rb", "module_cleanup.rb", "runner_api_plans.rb"} {
		if _, err := workflow.Files.ReadFile("tools/" + name); err == nil {
			t.Fatalf("obsolete Ruby lifecycle is embedded: %s", name)
		}
	}
}

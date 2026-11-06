package pmtool

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v2/internal/scaffold"
)

const templateInitHelp = `Usage: pmtool template-init --name NAME --output NEW_DIRECTORY [--provider gitlab|github|both]
Create a neutral schema-1 template with ERB, tests, authoring docs and validation CI.
The destination must not exist. No Git, Docker, network, config, commit or push.
Default provider: both. --dry-run lists files without creating them.
CI pins this stable launcher version; development builds require --launcher-version vX.Y.Z.
`

func (a *app) initTemplate(args []string, dry bool) int {
	flags := flag.NewFlagSet("template-init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	o := scaffold.Options{Provider: "both", Version: version, VoxBox: voxboxImage}
	var output string
	var help bool
	flags.StringVar(&o.Name, "name", "", "template name")
	flags.StringVar(&output, "output", "", "new template directory")
	flags.StringVar(&o.Provider, "provider", "both", "gitlab, github or both")
	flags.StringVar(&o.Version, "launcher-version", version, "stable launcher version for generated CI")
	flags.BoolVar(&help, "help", false, "help")
	flags.BoolVar(&help, "h", false, "help")
	check(flags.Parse(args))
	if help {
		must(fmt.Fprint(a.stdout, templateInitHelp))
		return 0
	}
	if flags.NArg() != 0 || o.Name == "" || output == "" {
		fail("use template-init --name NAME --output NEW_DIRECTORY [--provider gitlab|github|both]")
	}
	if !filepath.IsAbs(output) {
		output = filepath.Join(a.cwd, output)
	}
	files := must(scaffold.Files(o))
	if dry {
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		must(fmt.Fprintln(a.stdout, string(must(json.MarshalIndent(struct {
			Operation, Output string
			Files             []string
		}{"template-init", output, names}, "", "  ")))))
		return 0
	}
	a.notice("RUN", "template-init", "1;36")
	for _, name := range must(scaffold.Generate(output, files)) {
		must(fmt.Fprintln(a.stdout, "create "+name))
	}
	a.notice("OK", "template created at "+output, "1;32")
	a.printGuidance("Review the starter, add team policies and run its self-tests:", []string{"pmtool", "template-test", "--module", output})
	return 0
}

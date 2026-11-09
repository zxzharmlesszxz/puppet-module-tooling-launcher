package hostflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/templatekit"
)

func templateHostCommand(command string) bool {
	switch command {
	case "status", "diff", "mr", "mr-create", "mr-merge", "main-sync", "pipeline-jobs", "pipeline-logs":
		return true
	}
	return false
}

func templateRepository(g *Git) (bool, error) {
	if _, err := os.Lstat(filepath.Join(g.Root, "metadata.json")); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if _, tracked := g.Optional("ls-files", "--error-unmatch", "--", "template.yml"); !tracked {
		return false, nil
	}
	revision, err := g.Text("rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	if _, err := templatekit.Load(g.Root, "", revision); err != nil {
		return false, fmt.Errorf("invalid template checkout: %w", err)
	}
	return true, nil
}

func (e *Engine) requestTitle(branch string) (string, error) {
	if e.Project.Template {
		if version, release := strings.CutPrefix(branch, "release/"); release {
			if err := validVersion(version); err != nil {
				return "", err
			}
			return "[REL] Release template " + version, nil
		}
		return "[UPD] Update template", nil
	}
	m, err := e.metadata("")
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(branch, "bootstrap/") {
		return "[UPD] Update module", nil
	}
	return "[REL] Release " + m.Version, nil
}

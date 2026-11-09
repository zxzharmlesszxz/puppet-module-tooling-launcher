package fixturekit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/templatekit"
)

func (p *Prepared) linkNativeModules(root string) error {
	target, err := templatekit.SafePath(root, "spec/fixtures/modules")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	// Remove can only replace an empty directory. Leave an existing fixture
	// tree (or another pmtool run's symlink) untouched.
	info, err := os.Lstat(target)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("dependency resolution requires an empty spec/fixtures/modules directory")
		}
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("dependency resolution requires an empty spec/fixtures/modules directory: %w", err)
		}
		p.restoreDirectory = true
		p.restoreMode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Symlink(p.Modules, target); err != nil {
		if p.restoreDirectory {
			restore := os.Mkdir(target, p.restoreMode)
			if !errors.Is(restore, os.ErrExist) {
				err = errors.Join(err, restore)
			}
		}
		return err
	}
	p.link = target
	return nil
}

func (p *Prepared) removeNativeLink() error {
	if p.link == "" {
		return nil
	}
	destination, err := os.Readlink(p.link)
	if err == nil {
		if destination != p.Modules {
			return fmt.Errorf("refusing to remove a replaced fixture link")
		}
		if err := os.Remove(p.link); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if p.restoreDirectory {
		if err := os.Mkdir(p.link, p.restoreMode); err != nil {
			return err
		}
	}
	p.link = ""
	return nil
}

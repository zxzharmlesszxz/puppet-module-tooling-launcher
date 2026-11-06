package templatekit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (e *Engine) Run(sync, noop bool) error {
	p, err := e.Plan(sync)
	if err != nil {
		return err
	}
	if sync {
		for _, change := range p.Changes {
			action := "add"
			if change.Existed {
				action = "replace"
			}
			if change.Remove {
				action = "remove"
			}
			if noop {
				action = "would " + action
			}
			if _, err := fmt.Fprintf(e.Out, "%s\t%s\n", action, change.Path); err != nil {
				return err
			}
			if change.Existed && e.Git != nil {
				if err := e.diff(change); err != nil {
					return err
				}
			}
		}
	}
	if len(p.Errors) > 0 {
		return fmt.Errorf("%s", strings.Join(p.Errors, "\n"))
	}
	if !sync {
		if err := e.runChecks(p.Managed); err != nil {
			return err
		}
		m, _, err := e.metadata()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(e.Out, "module audit passed: %s %s\n", str(m["name"]), str(m["version"]))
		return err
	}
	if noop {
		return nil
	}
	if e.BeforeApply != nil {
		e.BeforeApply()
	}
	for _, c := range p.Changes {
		if err := e.verify(c); err != nil {
			return err
		}
	}
	for _, c := range p.Changes {
		if err := e.apply(c); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(e.Out, "Baseline installed; module-owned metadata, fixtures and domain code are preserved. Run audit next.")
	return err
}
func (e *Engine) verify(c Change) error {
	path, err := SafePath(e.Root, c.Path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- SafePath verified the module target.
	if c.Existed {
		if err != nil || string(data) != string(c.Original) {
			return fmt.Errorf("file changed during sync: %s; preview again", c.Path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("file appeared during sync: %s; preview again", c.Path)
	}
	return nil
}
func (e *Engine) apply(c Change) (caught error) {
	path, err := SafePath(e.Root, c.Path)
	if err != nil {
		return err
	}
	if err := e.verify(c); err != nil {
		return err
	}
	if c.Remove {
		return os.Remove(path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { // #nosec G301 -- Managed module directories must remain traversable.
		return err
	}
	if _, err := SafePath(e.Root, c.Path); err != nil {
		return err
	}
	if !c.Existed {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644) // #nosec G302 G304 -- SafePath and exclusive creation protect generated source files; 0644 is intentional.
		if err != nil {
			return err
		}
		_, writeErr := file.Write(c.Content)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".sync-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer func() {
		err := os.Remove(tmp)
		if caught == nil && !os.IsNotExist(err) {
			caught = err
		}
	}()
	if _, err := file.Write(c.Content); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("write: %w; close: %w", err, closeErr)
		}
		return err
	}
	if err := file.Chmod(c.Mode); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("chmod: %w; close: %w", err, closeErr)
		}
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := e.verify(c); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func (e *Engine) diff(c Change) (caught error) {
	file, err := os.CreateTemp("", "pmtool-sync-diff-*")
	if err != nil {
		return err
	}
	defer func() {
		err := os.Remove(file.Name())
		if caught == nil && !os.IsNotExist(err) {
			caught = err
		}
	}()
	if _, err := file.Write(c.Content); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return closeErr
		}
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	color := "never"
	if e.Color {
		color = "always"
	}
	output, err := e.Git("--no-pager", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--color="+color, "--", filepath.Join(e.Root, c.Path), file.Name())
	if err != nil {
		if code, ok := errors.AsType[*exec.ExitError](err); !ok || code.ExitCode() != 1 {
			return err
		}
	}
	_, err = fmt.Fprint(e.Out, output)
	return err
}
func (e *Engine) runChecks(expected map[string]string) error {
	checks, _ := e.Template.Audit["checks"].([]any)
	for _, raw := range checks {
		c := mapping(raw)
		target := str(c["path"])
		data, err := readFile(e.Root, target)
		if err != nil {
			return err
		}
		if expected[target] == "" || hash(data) != expected[target] {
			return fmt.Errorf("audit script is not a verified managed file: %s", target)
		}
		if e.Check == nil {
			return fmt.Errorf("audit check executor is required")
		}
		if err := e.Check(e.Context, e.Root, target, str(c["runtime"])); err != nil {
			return fmt.Errorf("%s: %w", str(c["name"]), err)
		}
	}
	return nil
}
func Migrate(root string) error {
	var changes []Change
	for _, name := range []string{"metadata.json", ".fixtures.yml"} {
		path, err := SafePath(root, name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- SafePath verified the module-owned migration target.
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		updated := data
		if name == "metadata.json" {
			var metadata Mapping
			if err := json.Unmarshal(data, &metadata); err != nil {
				return err
			}
			if metadata == nil {
				return fmt.Errorf("metadata.json: expected a mapping")
			}
			_, hasURL := metadata["template-url"]
			_, hasRef := metadata["template-ref"]
			_, hasVersion := metadata["pdk-version"]
			if hasURL || hasRef || hasVersion {
				delete(metadata, "template-url")
				delete(metadata, "template-ref")
				delete(metadata, "pdk-version")
				updated, err = jsonBytes(metadata)
			}
		} else {
			updated, err = migrateFixtures(data)
		}
		if err != nil {
			return err
		}
		changes = append(changes, Change{Path: name, Original: data, Content: updated})
	}
	for _, change := range changes {
		if err := replaceMigratedFile(root, change.Path, change.Original, change.Content); err != nil {
			return err
		}
	}
	return nil
}

func replaceMigratedFile(root, name string, original, updated []byte) error {
	if bytes.Equal(original, updated) {
		return nil
	}
	path, err := SafePath(root, name)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", name)
	}
	engine := &Engine{Root: root}
	return engine.apply(Change{Path: name, Original: original, Content: updated, Existed: true, Mode: info.Mode().Perm()})
}

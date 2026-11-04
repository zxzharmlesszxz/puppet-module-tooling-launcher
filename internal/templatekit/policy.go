package templatekit

import "fmt"

func (t *Template) validateAudit() error {
	if !allowed(t.Audit, "metadata", "tracked_ignored", "coverage", "checks", "allow_custom_files") {
		return fmt.Errorf("unknown template audit policy")
	}
	for _, k := range []string{"tracked_ignored", "allow_custom_files"} {
		if raw, ok := t.Audit[k]; ok {
			if _, valid := raw.(bool); !valid {
				return fmt.Errorf("invalid Boolean audit policy %s", k)
			}
		}
	}
	m := Mapping{}
	if raw, ok := t.Audit["metadata"]; ok {
		m = mapping(raw)
		if m == nil || !allowed(m, "semantic_version", "changelog", "forbidden_keys", "url_fields", "no_self_symlinks") {
			return fmt.Errorf("invalid metadata audit policy")
		}
	}
	for _, k := range []string{"semantic_version", "no_self_symlinks"} {
		if raw, ok := m[k]; ok {
			if _, valid := raw.(bool); !valid {
				return fmt.Errorf("invalid Boolean metadata policy %s", k)
			}
		}
	}
	for _, k := range []string{"forbidden_keys", "url_fields"} {
		if _, ok := listField(m, k); !ok {
			return fmt.Errorf("invalid metadata %s", k)
		}
	}
	if raw, ok := m["changelog"]; ok {
		c := mapping(raw)
		if c == nil {
			return fmt.Errorf("invalid changelog policy")
		}
		if _, ok := c["heading"].(string); !ok {
			return fmt.Errorf("invalid changelog heading")
		}
		if err := Path(str(c["file"])); err != nil {
			return err
		}
	}
	if raw, ok := t.Audit["coverage"]; ok {
		c := mapping(raw)
		n, valid := integer(c["minimum"])
		_, exists := t.Defaults[str(c["file"])]
		if c == nil || !valid || n < 0 || n > 100 || !exists {
			return fmt.Errorf("invalid coverage policy")
		}
		for _, k := range []string{"enabled_key", "minimum_key"} {
			if _, ok := c[k].(string); !ok {
				return fmt.Errorf("invalid coverage keys")
			}
		}
	}
	if raw, ok := t.Audit["checks"]; ok {
		checks, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("audit checks must be an array")
		}
		for _, raw := range checks {
			c := mapping(raw)
			if c == nil || !contains([]string{"ruby", "voxbox"}, str(c["runtime"])) {
				return fmt.Errorf("invalid audit check")
			}
			if _, ok := c["name"].(string); !ok {
				return fmt.Errorf("invalid audit check name")
			}
			target := str(c["path"])
			if err := Path(target); err != nil {
				return err
			}
			if _, ok := t.Defaults[target]; !ok {
				return fmt.Errorf("audit check must be a managed template file")
			}
		}
	}
	return nil
}

type Selection struct {
	Enabled   []string
	Overrides map[string]Mapping
	Disabled  map[string]bool
}

func (t *Template) Select(overrides Mapping, namespace string) (Selection, error) {
	s := Selection{Overrides: map[string]Mapping{}, Disabled: map[string]bool{}}
	for key, value := range overrides {
		if _, group := t.Groups[key]; group || key == "ci" || key == "template" {
			continue
		}
		target, err := namespacePath(key, namespace)
		if err != nil {
			return s, err
		}
		if _, exists := s.Overrides[target]; exists {
			return s, fmt.Errorf("duplicate configuration for %s", target)
		}
		v := mapping(value)
		if v == nil {
			return s, fmt.Errorf("expected a mapping for %s", target)
		}
		s.Overrides[target] = merge(v, nil)
	}
	for _, name := range t.groupNames() {
		g := t.Groups[name]
		raw, explicit := overrides[name]
		selected := contains(t.Contracts, name)
		if explicit {
			var ok bool
			selected, ok = raw.(bool)
			if !ok {
				return s, fmt.Errorf("%s must be true or false", name)
			}
			if !t.AllowOverrides {
				return s, fmt.Errorf("contract selection belongs to the selected template")
			}
		}
		var targets, previous []string
		for _, target := range g.Targets {
			p, err := namespacePath(target, namespace)
			if err != nil {
				return s, err
			}
			targets = append(targets, p)
			if !contains(g.LegacyOptional, target) {
				previous = append(previous, p)
			}
		}
		legacyDisabled := !explicit && len(previous) > 0
		for _, p := range previous {
			legacyDisabled = legacyDisabled && truth(s.Overrides[p]["unmanaged"])
		}
		var state *bool
		for _, target := range targets {
			c := s.Overrides[target]
			if truth(c["custom"]) {
				return s, fmt.Errorf("baseline target cannot be custom: %s", target)
			}
			v, exists := c["unmanaged"]
			if exists && !t.AllowOverrides {
				return s, fmt.Errorf("contract selection belongs to the selected template")
			}
			if exists && explicit {
				return s, fmt.Errorf("%s conflicts with %s.unmanaged", name, target)
			}
			unmanaged := legacyDisabled || !contains(t.Contracts, name)
			if exists {
				var ok bool
				unmanaged, ok = v.(bool)
				if !ok {
					return s, fmt.Errorf("%s.unmanaged must be Boolean", target)
				}
			}
			if state != nil && *state != unmanaged {
				return s, fmt.Errorf("incomplete contract group %s", name)
			}
			value := unmanaged
			state = &value
		}
		if !explicit && state != nil {
			selected = !*state
		}
		if selected {
			s.Enabled = append(s.Enabled, name)
		}
		for _, p := range targets {
			s.Overrides[p] = merge(s.Overrides[p], Mapping{"unmanaged": !selected})
			if !selected && (!contains(t.Contracts, name) || explicit && !truth(raw)) {
				s.Disabled[p] = true
			}
		}
	}
	return s, nil
}
func (t *Template) groupNames() []string {
	if len(t.GroupOrder) > 0 {
		return t.GroupOrder
	}
	return keysGroups(t.Groups)
}

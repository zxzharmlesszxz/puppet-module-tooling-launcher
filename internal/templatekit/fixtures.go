package templatekit

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

func fixtureSymlinks(document Mapping) (Mapping, error) {
	raw := document["fixtures"]
	if raw == nil {
		return nil, nil
	}
	fixtures := mapping(raw)
	if fixtures == nil {
		return nil, fmt.Errorf(".fixtures.yml: fixtures must be a mapping")
	}
	raw = fixtures["symlinks"]
	if raw == nil {
		return nil, nil
	}
	symlinks := mapping(raw)
	if symlinks == nil {
		return nil, fmt.Errorf(".fixtures.yml: fixtures.symlinks must be a mapping")
	}
	return symlinks, nil
}

func migrateFixtures(data []byte) ([]byte, error) {
	values, err := YAML(data)
	if err != nil {
		return nil, fmt.Errorf(".fixtures.yml: %w", err)
	}
	symlinks, err := fixtureSymlinks(values)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if len(document.Content) == 0 {
		return data, nil
	}
	root := document.Content[0]
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value != "fixtures" {
			continue
		}
		fixtures := root.Content[index+1]
		if fixtures.Kind == yaml.AliasNode {
			return nil, fmt.Errorf(".fixtures.yml: expand the fixtures alias before migration")
		}
		for field := 0; field+1 < len(fixtures.Content); field += 2 {
			if fixtures.Content[field].Value != "symlinks" {
				continue
			}
			fixtures.Content = append(fixtures.Content[:field], fixtures.Content[field+2:]...)
			var output bytes.Buffer
			encoder := yaml.NewEncoder(&output)
			encoder.SetIndent(2)
			if err := encoder.Encode(&document); err != nil {
				return nil, err
			}
			if err := encoder.Close(); err != nil {
				return nil, err
			}
			if _, err := YAML(output.Bytes()); err != nil {
				return nil, fmt.Errorf(".fixtures.yml: expand shared aliases before migration: %w", err)
			}
			return output.Bytes(), nil
		}
	}
	if len(symlinks) > 0 {
		return nil, fmt.Errorf(".fixtures.yml: expand inherited fixture symlinks before migration")
	}
	return data, nil
}

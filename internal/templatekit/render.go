package templatekit

import (
	"context"
	"embed"
	"encoding/json"
	"maps"
	"strings"
)

//go:embed assets/profile.rb assets/tooling_group_selection.rb
var Assets embed.FS

//go:embed assets/erb_worker.rb
var RendererScript string

type Document struct {
	Source     string   `json:"source"`
	Config     Mapping  `json:"config"`
	Metadata   Mapping  `json:"metadata"`
	GroupOrder []string `json:"group_order"`
}
type Render func(context.Context, []Document) ([][]byte, error)

func RenderRequest(documents []Document) ([]byte, error) {
	return json.Marshal(struct {
		Documents []Document `json:"documents"`
	}{documents})
}
func RenderResponse(data []byte) ([][]byte, error) {
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	result := make([][]byte, len(values))
	for i, v := range values {
		result[i] = []byte(v)
	}
	return result, nil
}
func profileSource() (string, error) {
	helper, err := Assets.ReadFile("assets/tooling_group_selection.rb")
	if err != nil {
		return "", err
	}
	profile, err := Assets.ReadFile("assets/profile.rb")
	if err != nil {
		return "", err
	}
	var lines []string
	for _, line := range strings.SplitAfter(string(profile), "\n") {
		if strings.HasPrefix(line, "require_relative 'tooling_group_selection'") || strings.HasPrefix(line, "# frozen_string_literal:") {
			continue
		}
		lines = append(lines, line)
	}
	return string(helper) + "\n" + strings.Join(lines, ""), nil
}
func (t *Template) document(source []byte, config, metadata Mapping) (Document, error) {
	profile, err := profileSource()
	if err != nil {
		return Document{}, err
	}
	groups := map[string]Group{}
	maps.Copy(groups, t.Groups)
	config = merge(config, Mapping{"contract_groups": groups, "contracts": t.Contracts, "allow_contract_overrides": t.AllowOverrides, "profile_source": profile})
	return Document{string(source), config, metadata, t.groupNames()}, nil
}

// Fixture exposes only template data and the module-side RSpec compatibility helper.
func Fixture(t *Template) (Mapping, error) {
	source, err := profileSource()
	if err != nil {
		return nil, err
	}
	return Mapping{"defaults": t.Defaults, "group_order": t.groupNames(), "groups": t.Groups, "contracts": t.Contracts, "allow_contract_overrides": t.AllowOverrides, "profile_source": source}, nil
}

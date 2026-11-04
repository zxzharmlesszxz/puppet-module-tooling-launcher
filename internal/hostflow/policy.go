package hostflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type PolicyLevels struct{ Merge, Unprotect, Tag int }

func (c *Client) Policy(ctx context.Context, noop, deleteMaster bool, out io.Writer, levels PolicyLevels) (caught error) {
	var writeErr error
	defer func() {
		if caught == nil {
			caught = writeErr
		}
	}()
	row := func(message string) {
		if writeErr == nil {
			_, writeErr = fmt.Fprintf(out, "%s\t%s\n", c.Project.Name, message)
		}
	}
	main, err := c.Branch(ctx, "main")
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	mainExists := err == nil
	master, err := c.Branch(ctx, "master")
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	masterExists := err == nil
	var project record
	if err := c.call(ctx, http.MethodGet, c.prefix(), nil, nil, &project); err != nil {
		return err
	}
	if !mainExists {
		if !masterExists {
			return fmt.Errorf("neither main nor master exists")
		}
		main = Branch{Name: "main", SHA: master.SHA}
		row("create main from " + master.SHA)
		if !noop {
			path := c.prefix() + "/repository/branches"
			body := record{"branch": "main", "ref": master.SHA}
			if c.Project.Provider == "github" {
				path = c.prefix() + "/git/refs"
				body = record{"ref": "refs/heads/main", "sha": master.SHA}
			}
			if err := c.call(ctx, http.MethodPost, path, nil, body, nil); err != nil {
				return err
			}
		}
	}
	if text(project, "default_branch") != "main" {
		if text(project, "default_branch") != "master" || !masterExists || main.SHA != master.SHA {
			return fmt.Errorf("refusing to replace divergent default branch")
		}
		row("default branch requires main")
		if !noop {
			method := http.MethodPut
			if c.Project.Provider == "github" {
				method = http.MethodPatch
			}
			if err := c.call(ctx, method, c.prefix(), nil, record{"default_branch": "main"}, nil); err != nil {
				return err
			}
		}
	} else {
		row("default branch main")
	}
	if c.Project.Provider == "github" {
		err = c.githubProtection(ctx, noop, row)
	} else {
		err = c.gitlabProtection(ctx, noop, row, levels)
	}
	if err != nil {
		return err
	}
	if !deleteMaster || !masterExists {
		return nil
	}
	if main.SHA != master.SHA {
		return fmt.Errorf("refusing to delete master because it differs from main")
	}
	requests, err := c.Requests(ctx, "opened", "", "master")
	if err != nil {
		return err
	}
	if len(requests) != 0 {
		return fmt.Errorf("refusing to delete master with open requests")
	}
	if c.Project.Provider == "github" && master.Protected {
		return fmt.Errorf("refusing to remove a protected master")
	}
	if noop {
		row("master requires deletion")
		return nil
	}
	path := c.prefix() + "/repository/branches/master"
	if c.Project.Provider == "github" {
		path = c.prefix() + "/git/refs/heads/master"
	} else if master.Protected {
		if err := c.call(ctx, http.MethodDelete, c.prefix()+"/protected_branches/master", nil, nil, nil); err != nil {
			return err
		}
	}
	if err := c.call(ctx, http.MethodDelete, path, nil, nil, nil); err != nil {
		return err
	}
	row("master deleted")
	return nil
}
func exactLevel(values []any, level int) bool {
	if len(values) != 1 {
		return false
	}
	r, ok := values[0].(map[string]any)
	return ok && number(r, "access_level") == int64(level)
}
func accessUpdates(values []any, level int) []record {
	var result []record
	for i, v := range values {
		r, _ := v.(map[string]any)
		entry := record{"id": number(r, "id")}
		if i == 0 {
			entry["access_level"] = level
		} else {
			entry["_destroy"] = true
		}
		result = append(result, entry)
	}
	if len(result) == 0 {
		result = append(result, record{"access_level": level})
	}
	return result
}
func (c *Client) gitlabProtection(ctx context.Context, noop bool, row func(string), levels PolicyLevels) error {
	var protection record
	path := c.prefix() + "/protected_branches/main"
	err := c.call(ctx, http.MethodGet, path, nil, nil, &protection)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	valid := err == nil && !boolean(protection, "allow_force_push") && exactLevel(array(protection, "push_access_levels"), 0) && exactLevel(array(protection, "merge_access_levels"), levels.Merge) && exactLevel(array(protection, "unprotect_access_levels"), levels.Unprotect)
	if valid {
		row("main protected: merge only")
	} else {
		row("main protection requires update")
		if !noop {
			method := http.MethodPatch
			body := record{"allowed_to_push": accessUpdates(array(protection, "push_access_levels"), 0), "allowed_to_merge": accessUpdates(array(protection, "merge_access_levels"), levels.Merge), "allowed_to_unprotect": accessUpdates(array(protection, "unprotect_access_levels"), levels.Unprotect), "allow_force_push": false}
			if errors.Is(err, ErrNotFound) {
				method = http.MethodPost
				path = c.prefix() + "/protected_branches"
				body = record{"name": "main", "push_access_level": 0, "merge_access_level": levels.Merge, "unprotect_access_level": levels.Unprotect, "allow_force_push": false}
			}
			if err := c.call(ctx, method, path, nil, body, nil); err != nil {
				return err
			}
		}
	}
	tags, err := c.pages(ctx, c.prefix()+"/protected_tags", "", nil)
	if err != nil {
		return err
	}
	var tag record
	for _, r := range tags {
		if text(r, "name") == "v*" {
			tag = r
		}
	}
	if tag != nil && exactLevel(array(tag, "create_access_levels"), levels.Tag) {
		row("release tags protected")
		return nil
	}
	row("release tag protection requires update")
	if noop {
		return nil
	}
	if tag != nil {
		if err := c.call(ctx, http.MethodDelete, c.prefix()+"/protected_tags/"+url.PathEscape("v*"), nil, nil, nil); err != nil {
			return err
		}
	}
	return c.call(ctx, http.MethodPost, c.prefix()+"/protected_tags", nil, record{"name": "v*", "create_access_level": levels.Tag}, nil)
}
func (c *Client) githubProtection(ctx context.Context, noop bool, row func(string)) error {
	sets, err := c.pages(ctx, c.prefix()+"/rulesets", "", nil)
	if err != nil {
		return err
	}
	pull := record{"type": "pull_request", "parameters": record{"required_approving_review_count": 0, "dismiss_stale_reviews_on_push": true, "require_code_owner_review": false, "require_last_push_approval": false, "required_review_thread_resolution": true}}
	for _, rule := range []struct {
		name, target, ref string
		rules             []record
	}{{"puppet-tooling-main", "branch", "refs/heads/main", []record{{"type": "deletion"}, {"type": "non_fast_forward"}, pull}}, {"puppet-tooling-release-tags", "tag", "refs/tags/v*", []record{{"type": "deletion"}, {"type": "update"}}}} {
		var matching []record
		for _, set := range sets {
			if text(set, "name") == rule.name {
				matching = append(matching, set)
			}
		}
		if len(matching) > 1 {
			return fmt.Errorf("multiple tooling rulesets; review before changing policy")
		}
		row(rule.name + ": enforce ruleset")
		if noop {
			continue
		}
		body := record{"name": rule.name, "target": rule.target, "enforcement": "active", "bypass_actors": []record{}, "conditions": record{"ref_name": record{"include": []string{rule.ref}, "exclude": []string{}}}, "rules": rule.rules}
		path, method := c.prefix()+"/rulesets", http.MethodPost
		if len(matching) == 1 {
			path += "/" + strconv.FormatInt(number(matching[0], "id"), 10)
			method = http.MethodPut
		}
		if err := c.call(ctx, method, path, nil, body, nil); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) policyLevels() (PolicyLevels, error) {
	levels := PolicyLevels{Merge: 40, Unprotect: 40, Tag: 40}
	for _, field := range []struct {
		name   string
		target *int
	}{{"MAIN_MERGE_ACCESS_LEVEL", &levels.Merge}, {"MAIN_UNPROTECT_ACCESS_LEVEL", &levels.Unprotect}, {"RELEASE_TAG_ACCESS_LEVEL", &levels.Tag}} {
		raw := e.Getenv(field.name)
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 60 {
			return levels, fmt.Errorf("invalid %s", field.name)
		}
		*field.target = value
	}
	return levels, nil
}

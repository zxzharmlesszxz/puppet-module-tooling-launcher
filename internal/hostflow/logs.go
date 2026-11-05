package hostflow

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// CI text is untrusted terminal input. Strip escape sequences and control bytes
// before redaction, tail selection and either console or file output.
var logEscapes = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)
var logCredentials = regexp.MustCompile(`(https?://)[^\s/@]+:[^\s/@]+@`)

func (e *Engine) cleanLog(body []byte) string {
	s := logEscapes.ReplaceAllString(string(body), "")
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, s)
	secrets := []string{e.Git.Token}
	if e.Getenv != nil {
		for _, key := range strings.Fields("GITLAB_TOKEN GITHUB_TOKEN GH_TOKEN CI_JOB_TOKEN TOOLING_TEMPLATE_TOKEN") {
			secrets = append(secrets, e.Getenv(key))
		}
	}
	sort.SliceStable(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		for _, form := range []string{url.QueryEscape(secret), url.PathEscape(secret), base64.StdEncoding.EncodeToString([]byte(e.Git.Username + ":" + secret)), base64.StdEncoding.EncodeToString([]byte("oauth2:" + secret)), base64.StdEncoding.EncodeToString([]byte("gitlab-ci-token:" + secret)), secret} {
			s = strings.ReplaceAll(s, form, "[REDACTED]")
		}
	}
	return logCredentials.ReplaceAllString(s, "${1}[REDACTED]@")
}

func tailLog(body string, lines int) string {
	if lines == 0 {
		return body
	}
	parts := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n") + "\n"
}

func (e *Engine) logPipeline(o Options) (Pipeline, error) {
	if o.PipelineID != 0 {
		p, err := e.API.Pipeline(e.Context, o.PipelineID)
		if err != nil {
			return Pipeline{}, err
		}
		if p.ID != o.PipelineID || p.SHA == "" || o.Ref != "" && p.Ref != o.Ref {
			return Pipeline{}, fmt.Errorf("pipeline identity changed or ref does not match")
		}
		return p, nil
	}
	ref := o.Ref
	if ref == "" {
		var err error
		ref, err = e.branch()
		if err != nil {
			return Pipeline{}, err
		}
	}
	sha := ""
	branch, err := e.API.Branch(e.Context, ref)
	if err == nil {
		sha = branch.SHA
	} else if !errors.Is(err, ErrNotFound) {
		return Pipeline{}, err
	}
	p, err := e.latest(PipelineQuery{Ref: ref, SHA: sha, Source: "push"})
	if err != nil {
		return Pipeline{}, err
	}
	if p.Ref != ref || p.SHA == "" || sha != "" && p.SHA != sha {
		return Pipeline{}, fmt.Errorf("pipeline identity changed")
	}
	return p, nil
}

func (e *Engine) collectLogs(p Pipeline, o Options) ([]byte, error) {
	jobs, err := e.API.Jobs(e.Context, p.ID)
	if err != nil {
		return nil, err
	}
	var result bytes.Buffer
	selected := 0
	seen := map[int64]bool{}
	for _, job := range jobs {
		if o.JobID != 0 && job.ID != o.JobID || o.Failed && job.Status != "failed" {
			continue
		}
		if job.ID <= 0 || seen[job.ID] {
			return nil, fmt.Errorf("pipeline has an invalid or duplicate job ID")
		}
		seen[job.ID] = true
		body, err := e.API.JobLog(e.Context, job.ID)
		if err != nil {
			return nil, fmt.Errorf("job %d log: %w", job.ID, err)
		}
		text := tailLog(e.cleanLog(body), o.Tail)
		header := fmt.Sprintf("=== pipeline %d job %d %s (%s) ===\n", p.ID, job.ID, e.cleanLog([]byte(job.Name)), e.cleanLog([]byte(job.Status)))
		if result.Len()+len(header)+len(text) > 32<<20 {
			return nil, fmt.Errorf("combined logs exceed 32 MiB limit; select a job or smaller tail")
		}
		result.WriteString(header)
		result.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			result.WriteByte('\n')
		}
		selected++
	}
	if o.JobID != 0 && selected == 0 {
		return nil, fmt.Errorf("job %d is not selected in pipeline %d", o.JobID, p.ID)
	}
	if selected == 0 {
		return []byte(fmt.Sprintf("pipeline %d: no matching jobs\n", p.ID)), nil
	}
	return result.Bytes(), nil
}

func writeLogFile(path string, body []byte) (caught error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- Explicit local output path, exclusive creation refuses existing files and symlinks.
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); caught == nil {
			caught = err
		}
	}()
	_, err = file.Write(body)
	return err
}

func (e *Engine) pipelineLogs(o Options) error {
	if err := e.token(); err != nil {
		return err
	}
	p, err := e.logPipeline(o)
	if err != nil {
		return err
	}
	body, err := e.collectLogs(p, o)
	if err != nil {
		return err
	}
	if o.Output != "" {
		if err := writeLogFile(o.Output, body); err != nil {
			return err
		}
		e.row("logs saved to %s", o.Output)
	} else {
		e.print("%s", body)
	}
	return nil
}

func (e *Engine) failedLogs(p Pipeline) {
	body, err := e.collectLogs(p, Options{Failed: true, Tail: 100})
	if err != nil {
		e.print("[WARN] Failed-job logs unavailable for pipeline %d; use pipeline-logs --pipeline %d\n", p.ID, p.ID)
		return
	}
	e.print("%s", body)
}

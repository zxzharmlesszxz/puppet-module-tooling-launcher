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

const maxLogBytes = 16 << 20

// JobLog downloads plain text. Only GitHub's signed log download may redirect;
// no provider headers, cookies or credentials are sent to the download host.
func (c *Client) JobLog(ctx context.Context, id int64) ([]byte, error) {
	if id <= 0 {
		return nil, fmt.Errorf("job must have a positive ID")
	}
	path := c.prefix() + "/jobs/" + strconv.FormatInt(id, 10) + "/trace"
	if c.Project.Provider == "github" {
		path = c.prefix() + "/actions/jobs/" + strconv.FormatInt(id, 10) + "/logs"
	}
	address := c.Project.Endpoint + path
	client := *c.HTTP
	client.Jar = nil
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	for redirects := 0; ; redirects++ {
		body, location, err := c.logRequest(ctx, &client, address, redirects == 0)
		if err != nil {
			return nil, err
		}
		if location == "" {
			return body, nil
		}
		if redirects >= 5 {
			return nil, fmt.Errorf("job log redirect limit exceeded")
		}
		next, err := url.Parse(location)
		if err != nil || next.Scheme != "https" || next.Hostname() == "" || next.User != nil || next.Fragment != "" {
			return nil, fmt.Errorf("job log redirect must be an absolute credential-free HTTPS URL")
		}
		address = next.String()
	}
}

func (c *Client) logRequest(ctx context.Context, client *http.Client, address string, authenticated bool) (body []byte, location string, caught error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, "", fmt.Errorf("invalid job log URL")
	}
	if authenticated {
		if c.Project.Provider == "github" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
			req.Header.Set("Accept", "application/vnd.github+json")
			req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		} else {
			req.Header.Set("PRIVATE-TOKEN", c.Token)
		}
	}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", fmt.Errorf("job log request failed")
	}
	defer func() {
		if err := response.Body.Close(); caught == nil {
			caught = err
		}
	}()
	if c.Project.Provider == "github" && response.StatusCode == http.StatusFound {
		location = response.Header.Get("Location")
		if location == "" {
			return nil, "", fmt.Errorf("job log redirect has no location")
		}
		return nil, location, nil
	}
	body, err = readLogResponse(response)
	return body, "", err
}

func readLogResponse(response *http.Response) ([]byte, error) {
	if response.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job log unavailable or expired: %w", ErrNotFound)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("job log: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxLogBytes+1))
	if err != nil {
		return nil, errors.New("could not read job log")
	}
	if len(body) > maxLogBytes {
		return nil, fmt.Errorf("job log exceeds 16 MiB limit")
	}
	return body, nil
}

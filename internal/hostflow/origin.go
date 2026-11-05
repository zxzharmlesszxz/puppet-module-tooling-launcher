package hostflow

import (
	"fmt"
	"strings"
)

// ConfiguredOrigin reads repository identity without applying transport URL rewrites.
func ConfiguredOrigin(run func(...string) (string, error)) (string, error) {
	output, err := run("config", "--local", "--get-all", "remote.origin.url")
	if err != nil {
		return "", err
	}
	origin := strings.TrimSuffix(output, "\n")
	if origin == "" {
		return "", fmt.Errorf("missing origin URL")
	}
	if strings.ContainsAny(origin, "\r\n") {
		return "", fmt.Errorf("origin must contain exactly one URL on a single line")
	}
	return origin, nil
}

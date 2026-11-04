package pmtool

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
)

func containerIdentity(permission profile) []string {
	if !permission.Module && !permission.Socket {
		return nil
	}
	args := []string{"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())}
	if !permission.Socket {
		return args
	}
	groups := must(os.Getgroups())
	for _, group := range containerGroupIDs(runtime.GOOS, os.Getgid(), groups) {
		args = append(args, "--group-add", group)
	}
	return args
}

func containerGroupIDs(platform string, primary int, groups []int) []string {
	if platform == "darwin" {
		groups = []int{0} // Docker Desktop exposes its Linux socket with group root.
	}
	seen := map[int]bool{primary: true}
	var result []string
	for _, group := range groups {
		if !seen[group] {
			result = append(result, strconv.Itoa(group))
			seen[group] = true
		}
	}
	return result
}

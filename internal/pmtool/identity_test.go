package pmtool

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestContainerIdentityFollowsManifestScope(t *testing.T) {
	identity := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	for _, permission := range []profile{{}, {Module: true}, {Module: true, Socket: true}} {
		args := containerIdentity(permission)
		if !permission.Module {
			if len(args) != 0 {
				t.Fatal("metadata command received host identity/groups")
			}
			continue
		}
		if len(args) < 2 || args[0] != "--user" || args[1] != identity {
			t.Fatal("module command lost the invoking user's UID/GID")
		}
		if !permission.Socket && len(args) != 2 {
			t.Fatal("command without socket received supplementary groups")
		}
	}
}

func TestContainerGroupIDs(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		primary  int
		groups   []int
		want     []string
	}{
		{"linux docker group", "linux", 1000, []int{1000, 998, 998, 27}, []string{"998", "27"}},
		{"linux no supplementary groups", "linux", 1000, nil, nil},
		{"desktop Linux socket group", "darwin", 20, []int{20, 80}, []string{"0"}},
		{"desktop root primary", "darwin", 0, []int{0, 80}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := containerGroupIDs(tc.platform, tc.primary, tc.groups); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("groups = %v, want %v", got, tc.want)
			}
		})
	}
}

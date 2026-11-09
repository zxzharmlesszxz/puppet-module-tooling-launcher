package templatekit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/scaffold"

	"go.yaml.in/yaml/v3"
)

func TestRealStarterCIBootstrapChecksDownloadedBinary(t *testing.T) {
	if os.Getenv("PMTOOL_RUBY_TESTS") != "1" {
		t.Skip("bootstrap Ruby runs in workflow-check SDK")
	}
	files, err := scaffold.Files(scaffold.Options{Name: "team", Provider: "gitlab", Version: "v2.0.0", VoxBox: "voxbox:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var document Mapping
	if err := yaml.Unmarshal(files[".gitlab-ci.yml"], &document); err != nil {
		t.Fatal(err)
	}
	steps := document["template-test"].(map[string]any)["before_script"].([]any)
	_, code, ok := strings.Cut(steps[0].(string), "<<'RUBY'\n")
	if !ok {
		t.Fatal("CI bootstrap has no Ruby entrypoint")
	}
	code, _, ok = strings.Cut(code, "\nRUBY\n")
	if !ok {
		t.Fatal("CI bootstrap heredoc is unterminated")
	}
	for _, mode := range []string{"success", "bad-checksum", "duplicate", "http-redirect", "redirect-loop"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "http_fixture.rb", starterHTTPFixture)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			cmd := exec.CommandContext(ctx, "ruby", "-r", filepath.Join(root, "http_fixture.rb"), "-", root)
			cmd.Stdin = strings.NewReader(code)
			cmd.Env = append(cleanEnvironment(), "FIXTURE_MODE="+mode)
			output, err := cmd.CombinedOutput()
			path := filepath.Join(root, "pmtool")
			if mode != "success" {
				if err == nil {
					t.Fatalf("unsafe download accepted: %s", output)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("unsafe binary installed")
				}
				return
			}
			if err != nil {
				t.Fatalf("valid bootstrap failed: %s %v", output, err)
			}
			body, err := os.ReadFile(path)
			if err != nil || string(body) != "fixture binary\n" {
				t.Fatal("wrong installed bytes", err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatal("unsafe binary mode", err)
			}
		})
	}
}

const starterHTTPFixture = `require 'digest'
require 'net/http'
Net::HTTP.define_singleton_method(:start) do |_host, _port, **_options, &block|
  client = Object.new
  client.define_singleton_method(:get) do |path|
    mode = ENV.fetch('FIXTURE_MODE')
    if %w[http-redirect redirect-loop].include?(mode)
      response = Net::HTTPFound.new('1.1', '302', 'Found')
      response['location'] = mode == 'http-redirect' ? 'http://example.invalid/download' : 'https://example.invalid/download'
      next response
    end
    bytes = "fixture binary\n"
    checksum = mode == 'bad-checksum' ? '0' * 64 : Digest::SHA256.hexdigest(bytes)
    binary = "pmtool-v2.0.0-linux-#{RbConfig::CONFIG.fetch('host_cpu').match?(/aarch64|arm64/) ? 'arm64' : 'amd64'}"
    entry = "#{checksum}  #{binary}\n"
    body = path.end_with?('/SHA256SUMS') ? entry * (mode == 'duplicate' ? 2 : 1) : bytes
    response = Net::HTTPOK.new('1.1', '200', 'OK')
    response.define_singleton_method(:body) { body }
    response
  end
  block.call(client)
end
`

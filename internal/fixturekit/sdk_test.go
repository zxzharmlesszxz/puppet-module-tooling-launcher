package fixturekit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This suite runs under the pinned stock VoxBox in make workflow-check.
func TestStandardForgeInstallerSDK(t *testing.T) {
	if os.Getenv("PMTOOL_FORGE_TESTS") != "1" {
		t.Skip("requires the pinned VoxBox Puppet/puppet_fixtures toolchain")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, ".fixtures.yml"), "fixtures:\n  forge_modules:\n    common: {repo: team/common, ref: '1.0.0'}\n    apt: {repo: puppetlabs/apt, ref: '11.4.0'}\n    stdlib: {repo: puppetlabs/stdlib, ref: '10.1.0'}\n")
	prepared, err := Prepare(root, Options{Native: true, Getenv: authenticated()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	})
	const script = `require 'json'
require 'puppet'
require 'puppet_fixtures'
ENV['FIXTURES_YML'] = ARGV.fetch(1)
Dir.chdir(ARGV.fetch(0)) do
  fixtures = PuppetFixtures::Fixtures.new
  calls = []
  fixtures.define_singleton_method(:run_command) do |command, **_options|
    raise 'wrong installer' unless command.first(3) == %w[puppet module install]
    raise 'token exposed in argv' if command.any? { |arg| arg.include?('fixture-read-secret') }
    calls << command
    true
  end
  fixtures.forge_modules.each { |remote, options| fixtures.download_module(remote, **options) }
  raise 'not all dependencies use Forge modules' unless calls.length == 3
  config_paths = calls.map { |command| command.fetch(command.index('--config') + 1) }.uniq
  raise 'dependencies do not share the proxy config' unless config_paths.length == 1
  expected = { 'team/common' => '1.0.0', 'puppetlabs/apt' => '11.4.0', 'puppetlabs/stdlib' => '10.1.0' }
  calls.each do |command|
    remote = command.fetch(command.index('--target-dir') + 2)
    version = command.fetch(command.index('--version') + 1)
    raise 'pinned dependency version lost' unless expected.fetch(remote) == version
  end
  Puppet.initialize_settings(['--config', config_paths.first])
  raise 'native Forge repository setting was ignored' unless Puppet[:module_repository] == 'https://forge.example.test/proxy'
  raise 'native Forge authorization setting was ignored' unless Puppet[:forge_authorization] == 'Bearer fixture-read-secret'
  raise 'read token leaked through environment' if ENV['FORGE_TOKEN'] || ENV['FORGE_PUBLISH_TOKEN']
  puts 'Standard puppet_fixtures argv and native Puppet Forge authorization verified'
end
`
	cmd := exec.Command("bundle", "exec", "ruby", "-e", script, root, prepared.Manifest)
	cmd.Env = sdkEnvironment()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native installer regression failed: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "fixture-read-secret") || !strings.Contains(string(output), "native Puppet Forge authorization verified") {
		t.Fatal("native installer regression leaked credentials or did not finish")
	}
}

func sdkEnvironment() []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "FORGE_") || strings.HasPrefix(key, "BUNDLE_") || key == "RUBYOPT" || key == "RUBYLIB" || key == "FIXTURES_YML" || key == "SSL_CERT_FILE" {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "BUNDLE_IGNORE_CONFIG=1", "BUNDLE_GEMFILE=/opt/voxbox/Gemfile", "BUNDLE_PATH=/opt/voxbox/vendor/bundle")
	return env
}

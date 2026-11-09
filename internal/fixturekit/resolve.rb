# frozen_string_literal: true

require 'puppet'
require 'puppet_fixtures'

# Keep the SDK's fixture parsing, symlinks and repository handling. Opted-in
# Forge installs share a single modulepath and run in series.
class ResolvingForgeFixtures < PuppetFixtures::Fixtures
  def initialize(config)
    super(max_thread_limit: 1)
    @config = config
  end

  def install
    forge_modules.each { |remote, options| validate(remote, options) }
    download
    forge_modules.each { |remote, options| verify(remote, options) }
  end

  def download_module(remote, ref:, target:, flags:)
    args = %w[puppet module install] + flags
    args += ['--version', ref] if ref
    directory = File.realpath(File.dirname(target))
    args += ['--modulepath', directory, '--target-dir', directory]
    Dir.mktmpdir do |working_dir|
      args += ['--module_working_dir', working_dir, remote]
      raise "Failed to install Forge fixture #{remote} with dependencies" unless system(*args)
    end
  end

  private

  def validate(remote, options)
    unless remote.match?(%r{\A[a-zA-Z0-9_]+[-/][a-zA-Z0-9_]+\z}) &&
           options[:target] == File.join(module_target_dir, remote.split(%r{[-/]}).last)
      raise "Dependency resolution requires the standard target for Forge fixture #{remote}"
    end

    flags = options[:flags]
    allowed = %w[--debug --trace --verbose]
    return if flags && flags.last(2) == ['--config', @config] && (flags[0...-2] - allowed).empty?

    raise "Dependency resolution only supports logging flags for Forge fixture #{remote}"
  end

  def verify(remote, options)
    metadata = JSON.parse(File.read(File.join(options[:target], 'metadata.json')))
    version = SemanticPuppet::Version.parse(metadata.fetch('version'))
    name = metadata.fetch('name').tr('/', '-')
    range = Puppet::Module.parse_range(options[:ref] || '>= 0.0.0')
    return if name == remote.tr('/', '-') && range.include?(version)

    raise "Resolved Forge fixtures do not satisfy the requested #{remote} #{options[:ref]}"
  end
end

ResolvingForgeFixtures.new(ARGV.fetch(0)).install if $PROGRAM_NAME == __FILE__

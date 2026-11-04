# frozen_string_literal: true

require_relative 'tooling_group_selection'

# One profile definition is shared by sync, audit and the generated AST checker.
class ToolingContractProfile
  GROUPS = {}.freeze
  ENABLED = [].freeze
  ALLOW_OVERRIDES = false
  KEY_CHECKSUM_TARGETS = [].freeze

  def self.configure(groups, enabled, allow_overrides)
    { GROUPS: groups, ENABLED: enabled, ALLOW_OVERRIDES: allow_overrides,
      KEY_CHECKSUM_TARGETS: groups.values.flat_map do |group|
        group.fetch(:legacy_optional_targets, [])
      end }.each do |name, value|
      remove_const(name)
      const_set(name, value.freeze)
    end
    self
  end

  def self.path(target, namespace = nil)
    raise '.sync.yml: file target must be a String' unless target.is_a?(String)
    return target unless target.match?(/%\{namespace\}/) && namespace
    unless namespace.is_a?(String) && namespace.match?(/\A[a-z][a-z0-9_]*\z/)
      raise '.sync.yml: invalid module namespace for baseline path'
    end

    target.gsub(/%\{(?<name>namespace)\}/, namespace)
  end

  def self.targets(group, namespace = nil)
    GROUPS.fetch(group).fetch(:targets).map { |target| path(target, namespace) }
  end

  def self.resolve(overrides, namespace = nil)
    raise '.sync.yml: expected a mapping' unless overrides.is_a?(Hash)

    normalized = normalize_overrides(overrides, namespace)
    validate_override_authority(overrides, normalized, namespace)
    enabled = GROUPS.keys.select do |name|
      ToolingGroupSelection.new(self, name, namespace, normalized).resolve(overrides)
    end
    [enabled, normalized]
  end

  def self.normalize_overrides(overrides, namespace)
    values = overrides.reject { |key, _value| GROUPS.key?(key) || %w[ci template].include?(key) }
    values.each_with_object({}) do |(key, value), normalized|
      target = path(key, namespace)
      raise ".sync.yml: duplicate configuration for #{target}" if normalized.key?(target)
      raise ".sync.yml: unknown contract group #{target.inspect}" if [true, false].include?(value)
      raise ".sync.yml: expected a mapping for #{target}" unless value.is_a?(Hash)

      normalized[target] = value
    end
  end

  def self.validate_override_authority(overrides, normalized, namespace)
    explicit_group = overrides.keys.any? { |key| GROUPS.key?(key) }
    target_override = GROUPS.values.flat_map { |group| group.fetch(:targets) }.any? do |target|
      normalized.fetch(path(target, namespace), {}).key?('unmanaged')
    end
    return if ALLOW_OVERRIDES || (!explicit_group && !target_override)

    raise '.sync.yml: contract selection belongs to the selected template'
  end
end

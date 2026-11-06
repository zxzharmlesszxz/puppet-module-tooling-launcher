# frozen_string_literal: true

# Resolves one complete contract group without permitting partial file selection.
class ToolingGroupSelection
  BOOLEAN_VALUES = [true, false].freeze

  def initialize(profile, name, namespace, normalized)
    @profile = profile
    @name = name
    @namespace = namespace
    @normalized = normalized
    @targets = profile.targets(name, namespace)
  end

  def resolve(overrides)
    explicit = overrides.key?(@name)
    selected = overrides[@name]
    raise ".sync.yml: #{@name} must be true or false" if explicit && !BOOLEAN_VALUES.include?(selected)

    states = legacy_states(explicit, legacy_disabled?(explicit))
    selected = legacy_selected?(states) unless explicit
    @targets.each { |target| @normalized[target] = @normalized.fetch(target, {}).merge('unmanaged' => !selected) }
    selected
  end

  private

  def legacy_disabled?(explicit)
    group = @profile.const_get(:GROUPS).fetch(@name)
    optional = group.fetch(:legacy_optional_targets, []).map { |target| @profile.path(target, @namespace) }
    previous = @targets - optional
    !explicit && !previous.empty? && previous.all? { |target| @normalized.fetch(target, {})['unmanaged'] == true }
  end

  def legacy_states(explicit, disabled)
    @targets.map do |target|
      config = @normalized.fetch(target, {})
      validate_legacy_config(target, config, explicit)
      value = config.fetch('unmanaged', disabled || !@profile.const_get(:ENABLED).include?(@name))
      raise ".sync.yml: #{target}.unmanaged must be true or false" unless BOOLEAN_VALUES.include?(value)

      value
    end
  end

  def validate_legacy_config(target, config, explicit)
    raise ".sync.yml: baseline target cannot be custom: #{target}" if config['custom'] == true
    return unless explicit && config.key?('unmanaged')

    raise ".sync.yml: #{@name} conflicts with #{target}.unmanaged; remove legacy per-file flags"
  end

  def legacy_selected?(states)
    raise ".sync.yml: incomplete contract group #{@targets.join(', ')}" if states.uniq.length > 1

    !states.all?
  end
end

# frozen_string_literal: true

require 'json'
require 'minitest/autorun'

class StarterTemplateTest < Minitest::Test
  def setup
    @fixture = JSON.parse(File.read(ENV.fetch('PMTOOL_TEMPLATE_FIXTURE')))
  end

  def test_editor_configuration_is_managed_and_configurable
    settings = @fixture.fetch('defaults').fetch('.editorconfig')
    assert_equal false, settings.fetch('unmanaged')
    assert_equal 2, settings.fetch('indent_size')
  end

  def test_starter_has_no_team_contracts_or_module_switches
    assert_empty @fixture.fetch('contracts')
    assert_empty @fixture.fetch('groups')
    assert_equal false, @fixture.fetch('allow_contract_overrides')
  end
end

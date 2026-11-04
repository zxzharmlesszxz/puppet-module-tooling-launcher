# frozen_string_literal: true

require 'json'
require 'erb'
request = JSON.parse($stdin.read)
result = request.fetch('documents').map do |doc|
  config = doc.fetch('config')
  groups = config.fetch('contract_groups', {})
  config['contract_groups'] = doc.fetch('group_order', []).to_h do |name|
    [name, groups.fetch(name).transform_keys(&:to_sym)]
  end
  context = Object.new
  context.instance_variable_set(:@configs, config)
  metadata = doc.fetch('metadata')
  context.instance_variable_set(:@metadata, metadata.merge(module_name: metadata.fetch('name').split('-', 2).last))
  ERB.new(doc.fetch('source'), trim_mode: '-').result(context.instance_eval { binding })
end
$stdout.write(JSON.generate(result))

# Template authoring contract

The package root must contain `template.yml` and the defaults file it selects.
Schema 1 accepts these top-level fields only:

- `schema`: integer 1.
- `name`: descriptive package name.
- `files_root`: relative directory containing `OUTPUT.erb` sources.
- `defaults`: relative YAML file mapping each output path to its parameter mapping.
- `contract_groups`: named groups with `files`, optional `types` and optional
  `legacy_optional_files`. Group files must exist in defaults and cannot overlap.
- `contracts`: enabled group names; omitted means all, an empty array means none.
- `allow_contract_overrides`: Boolean, default false. Prefer team-owned profiles.
- `ci`: optional provider mapping, `gitlab` and/or `github`, to existing YAML files.
- `audit`: optional metadata, tracked_ignored, coverage, checks, allow_custom_files.

Paths must stay inside the package/module, contain no traversal or symlinks.
The paths `.sync.yml`, `.sync.lock.json`, `.gitlab-ci.yml` and
`.github/workflows/puppet-module.yml` are reserved and cannot be managed defaults.
The module author selects settings in .sync.yml; the engine writes ownership
inventory and the selected provider caller.

## ERB and defaults

Each output `path/file` has a source at `files_root/path/file.erb`.
`%{namespace}` in an output path expands from the module metadata name.
ERB receives merged per-file `@configs` and module `@metadata`; the engine supplies
the module namespace as `@metadata[:module_name]`. Data is JSON, not a Ruby eval.
Per-file overrides replace top-level defaults keys. Nested hashes and arrays
are replaced as values, not deep-merged; this is separate from Puppet Hiera policy.
Use explicit per-file defaults for every parameter the ERB expects.

```yaml
types/package.pp:
  unmanaged: false
```

```puppet
type <%= @metadata[:module_name].capitalize %>::Package = Struct[{
  'version' => String[1],
}]
```

Add the output/source pair together; test the contract in a real Puppet catalogue
and add template regressions. Merely listing a type does not verify its usage.

## Audit and coverage

Metadata policy supports semantic_version, no_self_symlinks, forbidden_keys,
url_fields, and changelog with `file` and `heading: 'Release %{version}'`.
`tracked_ignored: true` rejects generated ignored files committed to Git.

Checks are entries with `name`, `path` and `runtime: ruby` or `runtime: voxbox`.
The path must be a managed file. The module directory is passed as its argument;
nonzero exit fails audit. Template code is trusted executable code, not a sandbox.

Coverage policy names a managed helper via `file`, its `enabled_key`,
`minimum_key`, and `minimum` (0..100). The helper must implement the measurement
and enforce that threshold; setting the YAML alone does not measure coverage.
Use specs for every public entity and real 100% Puppet resource coverage when
your team's profile requires them. See `pmtool docs templates` for a full example.

## Test interface

`pmtool template-test` first renders/synchronizes a temporary fixture module and
checks baseline integrity, then runs `test/run.rb`. The environment variable
`PMTOOL_TEMPLATE_FIXTURE` points to JSON with defaults, group_order, groups,
contracts, allow_contract_overrides and profile_source. Group entries have
targets, types and legacy_optional_targets. Do not import launcher source files.
Test both rendering and behavior of additional Puppet/Ruby helpers you introduce.

The built-in smoke test selects GitLab when that entry exists, otherwise GitHub;
it is not a provider CI matrix. Extend test/run.rb and validate the chosen provider
in its own CI. Starter self-tests do not qualify additional contracts automatically.

The smoke baseline self-check does not execute audit.checks scripts automatically.
Tests in test/run.rb must exercise custom audit/helper behavior explicitly.

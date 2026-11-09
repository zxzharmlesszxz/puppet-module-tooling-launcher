# pmtool — Puppet module and template workflows

Usage:
  pmtool [global flags] COMMAND [command options]

Modules:
  sync                        Synchronize the module baseline
  audit                       Check the baseline and template policy
  check                       Run all module checks
  release-prepare             Create a module release branch
  release-submit              Commit and push a module release
  release-tag                 Tag the release and wait for publication
  cleanup                     Remove a merged module branch

Templates:
  template-init               Generate a team template
  template-test               Validate a team template

Requests and CI:
  mr                          Open or reuse, wait for CI and review, merge, sync main
  mr-create                   Open or reuse a merge request
  mr-merge                    Wait for CI and review, then merge an existing request
  main-sync                   Synchronize local main
  pipeline-jobs               Inspect or watch a CI run
  pipeline-logs               Read CI job logs

Diagnostics:
  doctor                      Check origin, template access, VoxBox and Forge
  status                      Show checkout status
  diff                        Show uncommitted changes
  settings                    Show resolved module settings

Global flags:
  --config PATH               Read launcher settings from PATH
  --image IMAGE               Select the tool image for baseline operations
  --docker PATH               Select the Docker executable
  --executor docker|native    Run tools in Docker or use the installed SDK
  --color auto|always|never   Control colored output
  --dry-run                   Preview the command
  -h, --help                  Show this help
  --version                   Show the launcher version

Run repository commands from the checkout or pass --module PATH.

Documentation and setup:
  pmtool docs                 Full manual
  pmtool docs --section list  Manual contents
  pmtool docs tokens          Token requirements
  pmtool docs inputs          Required inputs and optional overrides
  config                      Manage launcher settings
  completion                  Generate Bash or Zsh completion
  ai                          Export instructions for AI tools

Use "pmtool docs usage" for the full command index.

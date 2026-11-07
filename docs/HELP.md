# pmtool

Usage: pmtool [global options] COMMAND [command options]

Module workflow:
  release-prepare --version X.Y.Z → sync → check → release-submit → mr → check → release-tag

Commands:
  status, diff, settings       Inspect the current module
  sync, audit, check           Synchronize the baseline and run checks
  voxbox, release-checks       Run a selected validation phase
  mr, pipeline-jobs, pipeline-logs, cleanup
  template-init, template-test, module-new, module-convert, module-build
  docs, completion, ai, config, version

Global options: --config PATH, --image IMAGE, --executor docker|native,
                --color auto|always|never, --dry-run, --help, --version
Run from the module directory or pass --module PATH after the command.

Documentation:
  pmtool docs                 Full manual
  pmtool docs tokens          Developer and CI token requirements
  pmtool docs inputs          Required command inputs and optional overrides
  pmtool docs --section list  Manual contents

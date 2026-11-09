# Command inputs

This table describes pmtool v3.0.0. All test dependencies use standard
.fixtures.yml; authenticated proxy installation requires the new launcher.
Run from the module directory; `--module` is an optional directory override.
SDK operations require the selected Docker environment or an explicit native SDK,
but do not require image/tool version environment variables for normal use.

| Commands                                                                     | Required explicit input                                           | Authentication                                                            |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `help`, `docs`, `version`, `manifest`, `completion`, `ai`, `config`          | Only requested subcommand/output arguments                        | None                                                                      |
| `status`, `diff`, `settings`, `migrate`                                      | None inside the module clone                                      | None                                                                      |
| `template-init`                                                              | `--name`, `--output`; dev binary also needs `--launcher-version`  | None; `--origin` is optional and creates no hosted project                |
| `module-new`                                                                 | `--name`                                                          | None                                                                      |
| `module-convert`, `module-build`, `template-test`, `release-checks`          | Module/template directory; current directory by default           | None                                                                      |
| `voxbox --stage validate`                                                    | None                                                              | None                                                                      |
| `sync`, `audit`, `check`                                                     | `template.use` in `.sync.yml` or explicit `--template`            | Saved Git helper; optional template token override                        |
| `voxbox --stage unit`, unit phase of `check`                                 | Dependency names/versions in `.fixtures.yml` `forge_modules`      | Configured proxy: `FORGE_URL` + `FORGE_TOKEN`; neither for public default |
| `bootstrap-prepare`                                                          | `--branch bootstrap/NAME`                                         | Selected provider token                                                   |
| `release-prepare`                                                            | `--version X.Y.Z`                                                 | Selected provider token                                                   |
| `bootstrap-commit`, `release-commit`                                         | None; version and changed files are inferred                      | Git author/config; no API token                                           |
| `release-submit`, `release-tag`                                              | None; version is read from module metadata                        | Selected provider token + Git author/config                               |
| `bootstrap-push`, `release-push`, `main-sync`, `mr`, `mr-create`, `mr-merge` | None; branch inferred from current state                          | Selected provider token                                                   |
| `pipeline-jobs`, `pipeline-logs`, `policy`, `policy-delete-master`           | None; ref/provider/project are inferred                           | Selected provider token                                                   |
| `cleanup`                                                                    | `--branch release/X.Y.Z` or the actual completed bootstrap branch | Selected provider token                                                   |
| `forge-publish`                                                              | `FORGE_URL` and built `pkg/OWNER-MODULE-VERSION.tar.gz`           | `FORGE_PUBLISH_TOKEN`; omitted with `--dry-run`                           |

See [developer and CI token scopes/roles](TOKENS.md) and `pmtool docs tokens`.

The selected provider token means **one** of `GITLAB_TOKEN` or `GITHUB_TOKEN`,
according to origin/explicit provider. Git helper template authentication does
not replace provider REST API authentication. API operations need permissions
appropriate to the action; a repository-read-only token cannot merge or change
protection. Ordinary Git author settings come from Git config; GIT_AUTHOR_* and
GIT_COMMITTER_* are optional overrides, not mandatory setup.

Template token override variables are `TOOLING_TEMPLATE_TOKEN` plus matching
`TOOLING_TEMPLATE_TOKEN_HOST`; `TOOLING_TEMPLATE_USERNAME` is optional. With saved
Git helper credentials, none of these template variables is required. The team
GitLab CI setup explicitly maps its authorized CI_JOB_TOKEN to these overrides;
users do not create a separate CI_JOB_TOKEN for local work.

`FORGE_URL` + `FORGE_TOKEN` configure authenticated standard fixture reads for
all declared Forge modules. The URL may select a proxy that caches upstream
modules. Neither variable keeps the public default; one alone fails preflight.
`FORGE_URL` + `FORGE_PUBLISH_TOKEN` configure publication separately.
Read, template and publish tokens have separate scopes; tests never use the
publishing token. The read token is supplied through a private preparation config
and removed before specs, not passed in worker argv/environment.

Optional selectors: `--provider`, `--ci` and `--template` override `.sync.yml`;
provider otherwise comes from origin and CI defaults to the selected template.
`--branch`, `--ref`, `--title`, `--files`, `--watch`, `--pipeline`, `--job`,
`--failed`, `--tail`, `--output` and `--section` apply only to commands accepting
them. They are not a universal checklist. Root `--image`, `--config`, `--docker`,
`--executor`, `--color`, `--dry-run` and `--version` are optional; command-local
`release-prepare --version` is the release version, not the root version display.

`*-noop` variants have the same inputs as their apply command and remain optional
previews; requesting a preview does not authorize apply. `bootstrap-local` maps
to `sync`, `bootstrap-local-noop` to `sync-noop`, and `audit-local` to `audit`.
For normal release work use `release-submit` → `mr` → `check` → `release-tag`;
standalone commit/push/MR/main-sync/watch commands are available for recovery.

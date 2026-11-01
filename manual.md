# Puppet module tooling launcher

Запускати з каталогу звичайного Git clone модуля. Потрібні Docker CLI та запущений
Docker daemon; для коммітів автор береться з GIT_AUTHOR_NAME/GIT_AUTHOR_EMAIL або
host git config user.name/user.email. Project визначає контейнер із origin.

```sh
pmtool config init --image registry.example/tooling@sha256:IMAGE_DIGEST
pmtool audit
pmtool pdk
pmtool sync-noop
pmtool docs onboarding
```

## Локальні команди

Без Docker:

```sh
pmtool help
pmtool docs launcher
pmtool version
pmtool config path
pmtool config show
```

## Конфігурація

Конфігураційний файл: ~/.config/puppet-module-tooling/config.json на Linux і macOS.
Каталог створюється з правами 0700, файл — 0600. config init зберігає наявний файл.
При першій runtime-команді відсутній конфігураційний файл створюється автоматично. Tag-збірка
launcher-а має image за SHA того самого source commit; dev-збірка — локальний image.

```json
{
  "image": "registry.example/tooling@sha256:IMAGE_DIGEST",
  "docker": "docker",
  "socket": "/var/run/docker.sock"
}
```

Обов'язковий лише image. Docker і socket мають показані defaults. CI/provider
зберігаються в .sync.yml самого модуля: ci.use та optional ci.provider.
Пріоритет: CLI → .sync.yml → provider з origin. Токени залишаються лише в environment.
--config обирає тільки host image/docker/socket config, без налаштувань модулів.

## Опції запуску

Глобальні опції ставляться ПЕРЕД командою, runtime-опції — ПІСЛЯ:

```sh
pmtool --image tooling:local --dry-run check --module /absolute/clone
pmtool --config /absolute/config.json audit
pmtool help runtime
pmtool docs usage
pmtool pipeline-jobs --ref main
```

--dry-run читає manifest image і друкує Docker argv, але не запускає команду модуля.
Для image, відсутнього локально, launcher виконає pull. Наявний локальний tag
автоматично не оновлюється: для оновлення явно виконайте docker pull або змініть
image у конфігурації. Рекомендовано pin через digest чи immutable SHA tag.

## Image та доступи

Команди та вимоги до доступів читаються з manifest protocol 1 самого image.
Manifest і команда запускаються за одним image ID навіть при паралельному pull.
Старий image без manifest потрібно оновити; launcher не вгадує його permissions.

Clone монтується за його фізичним абсолютним шляхом; --module вибирає інший clone.
Child PDK/VoxBox отримують той самий host path. --host-module задає сам launcher.
Потрібен локальний Docker daemon із доступом до цього шляху; remote Docker host
не переносить файли з вашого комп'ютера. Linked Git worktrees не підтримуються runtime.

Docker socket передається тільки check, pdk, pdk-convert[-noop], voxbox і release-checks.
GITLAB_TOKEN або GITHUB_TOKEN передається через env лише remote/API-командам.
Launcher обирає тільки токен потрібного провайдера; для GitHub Enterprise задайте
--provider github. GITLAB_BASE_URL/GITLAB_USERNAME та GITHUB_API_URL опційні. Значення токена
не потрапляє в Docker argv, конфігурацію або --dry-run. SSH keys і host Git config не монтуються.

Приклади наступних кроків і guards релізу: docs onboarding. Повна довідка команд
постачається з runtime image: help runtime. Launcher не додає прихованих workflow-кроків: комбіновані release-submit, mr
та release-tag виконують саме описані для них послідовності. Exit code контейнера зберігається.

## Менше ручних кроків

Для нового релізу вже підключеного модуля:

```text
release-prepare → sync → зміни коду/metadata/changelog → check
→ release-submit → mr → check → release-tag → Forge/canary
```

Sync показує план і відмовляє на конфлікті до записів. Перегляньте diff і повторіть
із reviewed --files; sync-noop опційний. Migrate теж веде одразу до sync.
Check послідовно виконує audit, PDK, VoxBox і release checks.

Release-submit створює локальний комміт і пушить гілку. Файли обираються автоматично,
--files — опційний. Mr створює/знаходить MR, чекає CI його SHA, мерджить та синхронізує
local main. Далі — одразу check. Release-tag чекає exact-main CI, пушить тег та чекає
tag pipeline і forge-publish. На появу tag pipeline — до 60 секунд, потім фіксується
його ID; очікування кожні 5 секунд із WAIT. Forge artifact/canary перевіряються окремо.

Commit/push/tag/sync noops доступні за потреби; обов'язкових preview або окремих
watcher-кроків немає. Якщо preview обрано явно, перегляньте його перед apply.
Bootstrap зберігає окремі bootstrap-commit і bootstrap-push, після них — mr.
Окремі release-commit/release-push та mr-create/mr-merge також доступні.

### Якщо частина операції вже завершилася

- Commit успішний, push впав: комміт збережено; після виправлення — лише release-push.
- Merge успішний, main-sync впав: MR уже merged; виправте local state й повторіть
  лише main-sync, потім check. Жодного reset/stash або відкидання коммітів.
- Tag запушено, publication впала/очікування перервано: тег збережено; перевірте
  pipeline-jobs --ref vX.Y.Z --watch 5. Не створюйте й не пересувайте тег повторно.

Збій першого кроку не запускає наступний. Коди помилок зберігаються. Standalone
release-commit local-only, standalone mr-merge потребує окремого main-sync.
Mr використовує writable clone і token без socket/author; release-submit —
writable clone, token та author без socket. Audit у check вже звіряє baseline:
після main-sync sync потрібний лише при drift на робочій гілці.

## Вивід і кольори

Runtime друкує [RUN], кроки [STEP]/[PASS], підсумок [OK] або [FAIL] із exit code.
Підсумки й діагностика йдуть у stderr, дані команди — у stdout. Навіть тихий
release-checks має видимий підсумок. Manifest і --dry-run лишаються чистим JSON.
У терміналі помилки червоні, успіх зелений, кроки блакитні; Git diff кольоровий.
NO_COLOR вимикає ANSI; --color auto|always|never ставиться перед командою.

```sh
pmtool --color always sync-noop
pmtool --color never audit
pmtool docs onboarding --section list
pmtool docs onboarding --section 'Підготовка'
pmtool docs usage > tooling-usage.md
```

У терміналі docs має заголовки, перенесені абзаци й виділені блоки коду.
Зміст: --section list. Вибір розділу: --section частина-назви; при кількох збігах
уточніть назву. При перенаправленні stdout зберігається вихідний Markdown без ANSI
та службових підсумків. Працює також docs launcher --section list без Docker.
Pager не запускається. Довгі рядки коду не переносяться, щоб їх можна було копіювати.

## Інструкції для ШІ

`ai` export генерує універсальні інструкції для будь-якого ШІ: launcher, наш workflow,
контракти, Hiera, репозиторії/ключі/пакети, файли/секрети, тести та релізи.
Формати: agents (типово, AGENTS.md), skill (SKILL.md із YAML frontmatter), prompt
(текст для нового чату). Назва формату не встановлює інструкції у ваш редактор.

```sh
pmtool ai help
pmtool ai export --format prompt
pmtool ai export --output ./AGENTS.md
pmtool ai export --format skill --output ./puppet-module-workflow/SKILL.md
pmtool --dry-run ai export --output ./AGENTS.md
```

Без --output або з --output - виводиться чистий Markdown у stdout навіть у TTY.
З --output створюються потрібні батьківські каталоги й новий файл; наявні файли та
symlink не перезаписуються. Наявний AGENTS.md чи skill потрібно доповнювати свідомо,
переглянувши експорт в окремому файлі. Автоматичного встановлення/комміту немає.

Інструкції беруться з docs ai вибраного image; його ID, protocol, launcher version
та доступні runtime-команди додаються в результат. Manifest і docs читаються за
одним image ID без clone, token, socket чи мережі контейнера. При відсутньому image
можливий registry pull. Старий image без docs ai потребує оновлення; неповний файл
при такій помилці не створюється. Після зміни image перегенеруйте інструкції та
перегляньте diff. Глобальний --dry-run друкує JSON-план експорту без запису файлу.

## Published branch cleanup

Runtime commands `cleanup-noop --branch release/X.Y.Z` and `cleanup --branch release/X.Y.Z`
preview/remove one explicitly selected completed release or bootstrap branch after
publication. Run from clean synchronized main; the selected provider token is required, Docker socket
and commit author are not. No tags or unrelated refs are removed. Runtime documentation
contains the MR, ancestry, publication and concurrent-change guards. Rebuild the image
to obtain new runtime commands; rebuild the launcher only to refresh this embedded manual.

## Runtime next steps

Runtime commands print an advisory `[NEXT]` action to stderr. No action is executed
automatically and command exit codes are unchanged. The displayed command uses the
canonical `pmtool` executable name; replace it with your binary path
when necessary. Runtime docs describe conditional CI hints and review-before-apply.
Arguments containing spaces or shell metacharacters use readable single quotes, for example `--title '[REL] Release 3.0.3'`. Embedded apostrophes use adjacent quoted segments (`'O'"'"'Reilly'`) so copying preserves the exact argument without shell expansion. Safe paths and options remain unquoted.
Manifest JSON, documentation stdout and AI exports keep their original format.

## GitLab, GitHub and CI selection

The same binary works with both providers. Public github.com origins select GitHub;
other hosts default to GitLab. Select Enterprise explicitly after the command:

```sh
pmtool mr --provider github
pmtool sync --ci 'github.com/team_b/pipelines/.github/workflows/puppet.yml@v1.0.0'
pmtool sync --ci 'git.example.com/team_a/pipelines//ci/puppet-module.yml@v1.0.0'
```

--ci carries repository/file/version together. Module defaults live in .sync.yml:
ci.use and optional ci.provider. CLI overrides win per field; if no provider is
selected, origin determines it. GitLab uses // to separate repository and nested file.
The host JSON stores image/docker/socket only. Legacy --ci-project remains available.
Read pmtool docs ci for permissions, reusable workflow requirements,
GitHub Actions push-token limitations and release recovery.

CI and template selection are independent; template.use selects generated files and audit rules.

## Module CI settings

Normally select only template.use in the module .sync.yml; CI comes from that
template's provider-specific entrypoint at the same resolved SHA. Set ci.use or
--ci only to override it. ci.provider or --provider overrides origin autodetection.
Host config.json contains image/docker/socket only; tokens remain environment-only.
The launcher's read-only, network-free settings query returns CI/provider/template
metadata without fetching or evaluating a template. Default CI is resolved later
by the runtime, preserving isolation of provider credentials.

## Module template selection

The launcher and container are separate repositories. Teams publish their own
template packages on compatible Git hosts. The container has no bundled team
template. The repository/version below is illustrative:

```yaml
template:
  use: 'git.example.com/team/puppet-template@v1.0.0'
```

CI priority: **CLI --ci → module .sync.yml ci.use → selected template.yml ci**.
The template's CI is pinned to the same resolved commit SHA as its files and audit
policy. Provider priority remains CLI --provider → ci.provider → module origin.
Use --template for an invocation-specific template override. Host config.json
contains image/docker/socket only. The offline settings query reads module YAML
without tokens, socket, template fetching or evaluation; the actual operation
loads the selected package and resolves its default CI.

The template manifest declares provider-specific entrypoints:

```yaml
ci:
  gitlab: ci/puppet-module.yml
  github: .github/workflows/puppet-module.yml
```

GitLab modules use a project include on the same server, or an anonymous HTTPS
remote include for public GitHub templates, pinned to the selected SHA. GitHub
modules use the reusable workflow on the same GitHub server. Other cross-server
combinations fail explicitly; select compatible CI with --ci or ci.use.
Missing template CI for the selected provider fails unless CI is explicitly supplied.

The selected template owns contracts, generated files and audit scripts. A team
may require executable-entity specs and 100% resource coverage through its policy. Contract
switches are accepted only when that template enables allow_contract_overrides.
There is no implicit template, no bundled team selector and no team policy in the
hosting adapters. External template fetches currently support anonymous HTTPS only.

Commit .sync.yml and .sync.lock.json with the module. Review sync conflicts before
retrying with approved --files paths. The inventory records resolved revision and
generated hashes; unchanged retired files can be removed, modified files require
review, unowned domain files are preserved. Templates contain trusted ERB/scripts.
A version tag can move; a full SHA is immutable. Read `docs templates` for the schema.

## Executable and release artifacts

Install the platform binary as pmtool. Release/build artifacts carry the selected version and platform, with SHA256SUMS. The source repository remains puppet-module-tooling-launcher.
Host settings remain at ~/.config/puppet-module-tooling/config.json. Container
images, runtime protocol and module .sync.yml names are unchanged.

## Versioned builds and releases

```sh
make build VERSION=1.2.3
```

X.Y.Z and vX.Y.Z normalize to vX.Y.Z. Omission builds dev. Release versions use
three numeric components without leading zeros; branches/path-like values are
rejected before compilation. This version is embedded in pmtool version and AI
exports. It is independent of the selected container image or module version.

Outputs for v1.2.3 are under dist/v1.2.3/: pmtool-v1.2.3-linux-amd64,
pmtool-v1.2.3-linux-arm64, pmtool-v1.2.3-darwin-amd64,
pmtool-v1.2.3-darwin-arm64 and SHA256SUMS. Dev builds use dist/dev/pmtool-dev-PLATFORM.
Install the chosen platform file as pmtool.

On branch pushes, GitHub Actions builds/tests dev artifacts. A vX.Y.Z tag builds
that exact version, then a separate job creates a GitHub Release with all four
binaries and SHA256SUMS. Only the release job has contents: write. Invalid tag
versions fail the build. Publication requires successful tests/build and an existing
tag; an existing Release is not overwritten automatically. Local make build never
creates tags, commits, pushes or releases.

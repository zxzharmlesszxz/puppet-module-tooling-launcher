# pmtool

[English](README.md)

Версійний CLI для роботи з Puppet-модулями в GitLab і GitHub.
Команди, workflow, перевірки, Git/API-запобіжники, підказки та документація для ШІ
належать pmtool. Окремий Docker image містить лише інструменти й залежності.
Контракти, baseline та правила аудиту належать шаблону команди.

## Використання

Встановіть бінарник Linux/macOS amd64/arm64 як pmtool. Для типового executor
потрібні Docker CLI і локальний daemon. Із уже налаштованого модуля,
де .sync.yml вибирає опублікований шаблон:

```sh
pmtool config init
pmtool check
```

Конфіг: ~/.config/puppet-module-tooling/config.json. Він містить image і Docker
executable, без токенів і параметрів модулів. Типовий image:
ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d.
config.image/--image задає середовище ERB/audit/template-test. VoxBox validators
і команди jig використовують окремі pins: --voxbox-image/VOXBOX_IMAGE.
Вибір модуля зберігається у .sync.yml:

```yaml
template:
  use: 'git.example.com/team/module-template@v1.0.0'
ci:
  provider: gitlab
```

Шаблон: --template → template.use, без неявного шаблону.
CI: --ci → ci.use → CI шаблону на тому самому resolved SHA.
Provider: --provider → ci.provider → визначення з origin.
Для першого підключення наявного модуля виконайте [bootstrap](docs/MODULE-ONBOARDING.md#3-bootstrap-гілка),
перш ніж запускати check проти ще не синхронізованого baseline.

Модульні Git/API-команди вимагають назву репозиторію `puppet-<module>`.
Lifecycle звіряє назву модуля з metadata. MR-команди також працюють із
валідним checkout шаблону.

Ідентичність репозиторію визначає єдиний локальний `remote.origin.url`.
Правила автентифікації runner лишаються конфігурацією Git-транспорту.
URL у конфігурації не може містити пароль, query чи fragment; кілька origin URL
відхиляються. Деталі — в [інструкції](docs/manual.uk.md#ідентичність-репозиторію-в-ci).

## Створення шаблону та читання CI-логів

```sh
pmtool template-init --name team_template --output ./new-template --provider both
pmtool template-test --module ./new-template
pmtool pipeline-jobs --ref main
pmtool pipeline-logs --ref main --failed --tail 100
pmtool pipeline-logs --pipeline 123 --job 456 --tail 0 --output ./job.log
```

`template-init` локально створює новий каталог із manifest schema 1, defaults,
ERB-прикладом, документацією, тестами та CI GitLab/GitHub. Без --origin Docker, Git
і мережа для генерації не потрібні. --origin URL опційно ініціалізує Git/main і додає
remote без коммітів чи пушів; без параметра Git не потрібний. Hosted project
створюється окремо. Наявні каталоги та файли не перезаписуються.
Це стартовий пакет валідації: контракти команди й публікацію Forge додають явно.
CI закріплює стабільну версію генератора; для dev-бінаря задайте
`--launcher-version vX.Y.Z`. Окремо створіть remote-репозиторій шаблону,
підключіть його через --origin при генерації або вручну перед публікацією. Деталі:
[підключення та публікація](docs/TEMPLATES.md#connect-and-publish-the-repository).

`pipeline-logs` читає логи через API поточного провайдера без змін Git.
За замовчуванням виводить останні 100 рядків кожного job; `--failed` залишає
невдалі jobs, `--tail 0` показує повний лог у межах ліміту. ID видно в
`pipeline-jobs`. `--output` створює лише новий файл із правами 0600.
Відомі токени приховуються; перед передаванням логів перевіряйте інші секрети.
При падінні CI у `mr`/`release-tag` хвости невдалих jobs показуються автоматично,
а первинна помилка та захист merge/tag зберігаються.

## Документація та ШІ

Усі ці команди локальні, без Docker, image pull, токенів і доступу до модуля:

```sh
pmtool help
pmtool doctor
pmtool docs launcher
pmtool docs onboarding --section list
pmtool ai export --format prompt
pmtool completion bash
pmtool completion zsh
```

AI export описує встановлену версію launcher. --output PATH створює лише новий
файл; наявні файли й symlink зберігаються. Скілл автоматично не встановлюється.
Докладніше: [інструкція](docs/manual.uk.md), [English manual](docs/manual.md).

## Межі виконання

Git/API та релізні команди виконуються в Go через хостовий Git і API провайдера.
Статус рев’ю GitHub читається через GraphQL. Ці команди працюють без Docker, Ruby SDK
та image-конфігу. Завантаження шаблону, sync, загальний audit і планування теж
виконуються в Go.
Контейнер виконує лише JSON/ERB worker та Puppet/RSpec-перевірки шаблону.
Оновлення workflow потребує перевипуску pmtool, а не перебудови image.

VoxBox запускає хостовий pmtool. Docker socket у контейнери не передається.
Токени шаблону/provider та автор комміту надаються лише потрібному кроку.
Validator-кроки їх не отримують.

Для CI, де потрібні інструменти вже встановлені в job image:

```sh
pmtool --executor native voxbox --stage unit
pmtool --executor native audit
```

Native використовує той самий план, без Docker. Це явний режим із доступом
до файлової системи й environment користувача job, а не sandbox. Host Git/API
команди, включно з читанням, і forge-publish запускайте без --executor native.
Повний check використовує VoxBox. Native module-build запускає встановлений jig.

## Структура

```text
cmd/pmtool/          точка входу
internal/pmtool/     CLI, конфіг, виконання, Go-тести
internal/hostflow/   Git/API, релізи та Go-регресії
internal/templatekit/ Go: шаблони, sync, ownership, загальний audit
internal/scaffold/   нейтральний starter і генерація CI
internal/workflow/   каталог команд
docs/               документація, вбудована в pmtool
test/               перевірки Make-таргетів source-релізів
```

## Розробка та релізи

```sh
make format
make build VERSION=v3.1.0
```

Build включає Go/Ruby перевірки, security, збірку чотирьох бінарників,
LICENSE/SHA256SUMS і smoke. Runtime — закріплений upstream VoxBox з OpenVox 8;
власний runtime-image не збирається. pmtool-checks містить лише Go/dev-інструменти.
Source-реліз керується Make-таргетами через Git та GitHub CLI. Із чистого main:

```sh
make release-prepare VERSION=v3.1.0
```

Після змін коду й записів під `## Unreleased`:

```sh
make check
make release-submit VERSION=v3.1.0 MESSAGE='Describe the implementation'
make mr VERSION=v3.1.0
```

Submit створює комміт коду, окремий `chore(release): prepare v3.1.0` зі зміною
лише заголовка changelog і пушить гілку без тегів. `mr` створює або використовує
наявний PR, чекає CI точного SHA, зливає його, синхронізує main та завантажує і
перевіряє CI-артефакти. Якщо GitHub вимагає approval для релізного PR launcher,
після рев’ю повторіть `make mr`. VERSION-файла немає. Виправлення до незмердженого релізу
можна додавати з тією самою версією; повторний пуш зберігає наявні комміти.

```sh
make pipeline-jobs
make pipeline-jobs REF=main WATCH=5
make pipeline-logs REF=main TAIL=100
```

REF за замовчуванням — поточна гілка; CI прив’язаний до SHA цього локального ref.
RUN_ID має йому відповідати. JOB задає ID job саме цього пайплайна;
FAILED_ONLY=0 включає логи успішних jobs. Перегляд CI не змінює Git refs.

Перевірте завантажений бінарник із `dist/ci/<SHA>/` зі своїм шаблоном і модулем.
Після цього публікуємо:

```sh
make push-release VERSION=v3.1.0
```

Після публікації Makefile показує preview і видаляє змерджену релізну гілку
локально й на GitHub. Для повтору використовуйте `make release-cleanup VERSION=v3.1.0`.
Анотований тег і GitHub Release зберігаються.

Перевірки артефактів, preflight, точного main, незмінного анотованого тега та
публікації через draft збережено. `make ci-artifacts` повторює завантаження без
пушу; `make push-main` лишається для вже підготовленого чистого main.
Необов’язкові release-prepare-noop і release-submit-noop нічого не змінюють.
Build/check/clean нічого не публікують. Після clean потрібно знову перевірити
CI-артефакти перед тегом. Деталі — в [інструкції](docs/manual.uk.md#збірка-launcher).

## Ліцензія

MIT License. Copyright (c) 2026 Denys Razumov. [LICENSE](LICENSE).

## Перевірка стандартного VoxBox

make voxbox-check запускає Go-регресії шаблонів із реальним ERB та строгий RuboCop Ruby-assets, потім security-сканування
закріпленого релізного upstream-образу з OpenVox 8. Знайдені вразливості дають
`[WARN]` і код 0; помилки завантаження образу чи сканера лишаються помилками.
Скан запускається явно, а не при кожній команді pmtool. Перевірки нашого
Go-коду й вихідних файлів залишаються обов’язковими. Власний image не збирається. Для тестів шаблону
зберігаються поточний VoxBox bundle і lockfile. Stock VoxBox є типовим runtime.

Каркас і пакування: module-new/module-convert/module-build через jig.
Публікація: forge-publish у Go з FORGE_URL/FORGE_PUBLISH_TOKEN; release gates лишаються в CI.
Деталі — [інструкція](docs/manual.uk.md#каркас-модуля-конвертація-та-пакування).

## Перехід на версію 3

Версія 3.0.0 переносить усі залежності до стандартного `.fixtures.yml` →
`forge_modules`. Єдина пара `FORGE_URL`/`FORGE_TOKEN` задає авторизований
Forge-проксі, включно з кешованими публічними модулями. Штатний installer
отримує тимчасовий приватний Puppet-конфіг лише перед тестами; перед RSpec
конфіг видаляється. Формат `test_fixtures` із `.sync.yml` прибрано.
Див. [Forge fixtures та перехід](docs/FIXTURES.md).

У версії 2 прибрано команди/aliases PDK і його image selectors. Спочатку опублікуйте launcher
та оновлений командний шаблон CI; потім переходьте до sync/check модулів.
Go module/imports використовують /v3; ім’я бінарника pmtool і release URL незмінні.
Деталі — [перехід на 2.0](docs/manual.uk.md#перехід-на-20).


Приватні залежності: [стандартні Forge fixtures](docs/FIXTURES.md).


[Команди, обов’язкові входи та опційні перевизначення](docs/COMMAND-INPUTS.md).


Вимоги до токенів: [scopes і ролі для розробника та CI](docs/TOKENS.md), також
доступні через `pmtool docs tokens`. `pmtool help` — короткий usage, `pmtool docs` —
повний посібник.

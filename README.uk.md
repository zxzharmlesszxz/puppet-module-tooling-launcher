# pmtool

[English](README.md)

Версійний CLI для роботи з Puppet-модулями в GitLab і GitHub.
Команди, workflow, перевірки, Git/API-запобіжники, підказки та документація для ШІ
належать pmtool. Окремий Docker image містить лише інструменти й залежності.
Контракти, baseline та правила аудиту належать шаблону команди.

## Використання

Встановіть бінарник Linux/macOS amd64/arm64 як pmtool. Для типового executor
потрібні Docker CLI і локальний daemon. Із папки модуля:

```sh
pmtool config init
pmtool check
```

Конфіг: ~/.config/puppet-module-tooling/config.json. Він містить image і Docker
executable, без токенів і параметрів модулів. Типовий image:
ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d.
Шаблон, CI та provider задаються в .sync.yml; пріоритет CLI → .sync.yml → provider
з origin. CI за замовчуванням береться з тієї самої версії шаблону.

## Документація та ШІ

Усі ці команди локальні, без Docker, image pull, токенів і доступу до модуля:

```sh
pmtool help
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

Git/API та релізні команди виконуються в Go через хостовий Git і REST API
provider. Docker, Ruby SDK та image-конфіг для них не потрібні. Завантаження шаблону, sync, загальний audit і планування теж виконуються в Go.
Контейнер виконує лише JSON/ERB worker та Puppet/RSpec-перевірки шаблону.
Оновлення workflow потребує перевипуску pmtool, а не перебудови image.

PDK/VoxBox запускає хостовий pmtool. Docker socket у контейнери не передається.
Токени шаблону/provider та автор комміту надаються лише потрібному кроку.
Validator-кроки їх не отримують.

Для CI, де потрібні інструменти вже встановлені в job image:

```sh
pmtool --executor native pdk --stage validate
pmtool --executor native voxbox --stage unit
pmtool --executor native audit
```

Native використовує той самий план, без Docker. Це явний режим із доступом
до файлової системи й environment користувача job, а не sandbox. Git/API-мутації
в ньому заборонені. Для повного check потрібні обидва toolchain.

## Структура

```text
cmd/pmtool/          точка входу
internal/pmtool/     CLI, конфіг, виконання, Go-тести
internal/hostflow/   Git/API, релізи та Go-регресії
internal/templatekit/ Go: шаблони, sync, ownership, загальний audit
internal/workflow/   каталог команд
docs/               документація, вбудована в pmtool
test/               перевірки Make-таргетів source-релізів
```

## Розробка та релізи

```sh
make format
make build VERSION=v1.0.0
```

Build включає Go/Ruby перевірки, security, збірку чотирьох бінарників,
LICENSE/SHA256SUMS і smoke. Runtime — закріплений upstream VoxBox з OpenVox 8;
власний runtime-image не збирається. pmtool-checks містить лише Go/dev-інструменти.
Спочатку коммітимо код із changelog під `## Unreleased`. Далі окремий підготовчий
комміт: фіналізуємо заголовок як `## Release 1.0.0` і коммітимо лише release-зміни
з повідомленням `chore(release): prepare v1.0.0`. VERSION-файла немає;
версія бінарника й далі береться з параметра збірки/Git-тега.

Після обох локальних коммітів публікуємо підготовлений чистий main:

```sh
make push-main
```

Таргет пушить лише main, чекає CI точного SHA, завантажує його артефакти й
перевіряє checksums, LICENSE та запуск нативного бінарника. CI-версія —
`dev-<повний SHA комміту>`, файли зберігаються в `dist/ci/<SHA>/`.
GitHub Release на цьому етапі не створюється. Перевірте завантажений бінарник
зі своїм шаблоном і модулем. `make ci-artifacts` повторює завантаження/перевірку без пушу.

Після цієї перевірки:

```sh
make push-release VERSION=v1.0.0
```

Таргет вимагає перевірені CI-артефакти поточного чистого main, виконує preflight
і створює/пушить stable-тег. Сам main тут не пушиться. Після завантаження всіх
релізних файлів draft GitHub Release стає публічним. Окремого `rc.N` prerelease
немає. Build/check/clean нічого не публікують.

## Ліцензія

MIT License. Copyright (c) 2026 Denys Razumov. [LICENSE](LICENSE).

## Перевірка стандартного VoxBox

make voxbox-check запускає Go-регресії шаблонів із реальним ERB та строгий RuboCop Ruby-assets, потім security-сканування
закріпленого релізного upstream-образу з OpenVox 8. Знайдені вразливості дають
[WARN] і код 0; помилки завантаження образу чи сканера лишаються помилками.
Скан запускається явно, а не при кожній команді pmtool. Перевірки нашого
Go-коду й вихідних файлів залишаються обов’язковими. Власний image не збирається. Для тестів шаблону
зберігаються поточний VoxBox bundle і lockfile. Stock VoxBox є типовим runtime.

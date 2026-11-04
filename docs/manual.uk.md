# Інструкція pmtool

[English](manual.md)

## Архітектура

Launcher містить команди, плани виконання, перевірки, Git/API-запобіжники,
підказки та документацію для ШІ. Інструментальний image містить лише executables
і залежності. Шаблон визначає baseline, контракти й вимоги команди.
Git/API та релізи виконуються в Go через хостовий Git і REST API, без Docker чи Ruby SDK.
Шаблони, sync, загальний audit і планування також працюють у Go. Ruby лишається
для JSON/ERB worker та Puppet/RSpec-перевірок шаблону.
Це перенесення власності коду, а не переписування всього коду на Go.

## Локальні команди

```sh
pmtool help
pmtool docs launcher
pmtool docs usage
pmtool docs onboarding --section list
pmtool manifest
pmtool ai export --format prompt
pmtool completion bash
pmtool completion zsh
```

Вони не потребують Docker, image, модуля чи credentials і не створюють config.
AI export із --output PATH відмовляється замінювати наявний файл або symlink.
Скілл не встановлюється автоматично; інструкції не є дозволом на публікацію.

## Конфігурація та параметри

Host-конфіг: ~/.config/puppet-module-tooling/config.json. Каталог 0700, файл 0600.
config init зберігає наявний файл. Конфіг містить image і Docker executable,
без токенів та налаштувань проєкту. Старе поле socket / --socket приймається
для наявних конфігів, але socket ніколи не монтується в контейнер.

```json
{
  "image": "ghcr.io/voxpupuli/voxbox:10.1.0-openvox8@sha256:01bb8ce5e24d5b111934660f75e2786b4d7c7adcaa7518a564856d1d3041d10d",
  "docker": "docker"
}
```

Глобальні параметри перед командою: --image, --config, --docker,
--executor docker|native, --color auto|always|never, --dry-run, --help, --version.
Параметри операції після команди: --module, --branch, --ref, --title, --watch,
--file, --files, --version, --ci, --provider, --template, --pdk-image,
--voxbox-image, --stage. Для документації: --section.

.sync.yml містить template.use і optional ci.use/ci.provider. CLI має пріоритет;
якщо provider не заданий, він визначається з origin. CI за замовчуванням береться
з тієї самої версії шаблону. Глобального inventory/fleet немає.

## Перевірки та реліз модуля

```text
release-prepare → sync → edit → check → release-submit → mr → check → release-tag
```

release-prepare --version X.Y.Z синхронізує чистий main і створює релізну гілку.
Оновіть metadata, changelog, код, тести й README. check послідовно виконує audit,
PDK validation/unit, VoxBox validation/specs і release-checks та зупиняється
на першій помилці зі справжнім exit code.

release-submit коммітить і пушить. Якщо push упав, комміт зберігається;
після виправлення використовуйте release-push. mr створює/знаходить MR, чекає
CI точного SHA, мерджить і синхронізує main. Після збою синхронізації повторюється
main-sync, а не мердж. Після комбінованого mr одразу check.

release-tag чекає CI точного main SHA, створює/пушить annotated tag і перевіряє
його publication pipeline та forge-publish. Зміна refs, API-помилка або
missing/failed/canceled/skipped/manual CI зупиняють реліз. Після збою очікування
публікації тег зберігається: дивіться pipeline-jobs --ref vX.Y.Z --watch 5.
Пуш тега ще не доводить публікацію; перевірте Forge й canary.

*-noop — опційні preview, а не обов'язкові додаткові кроки. Для bootstrap:
policy-noop → review → policy → bootstrap-prepare --branch bootstrap/PURPOSE,
за потреби PDK conversion, потім migrate → sync → edit → check → bootstrap-commit
→ bootstrap-push → mr. Наявні baseline-конфлікти потребують перегляду й --files.

Після перевіреної публікації та canary: cleanup-noop --branch release/X.Y.Z,
перегляд, потім окремий cleanup. Теги й Forge-артефакти не видаляються.

## Доступи й native CI

PDK/VoxBox запускаються pmtool через хостовий Docker. Жоден із цих контейнерів
не отримує Docker socket. Токени, credentials шаблону й автор комміту
надаються лише потрібному кроку; у validator-кроки вони не передаються.

```sh
pmtool --executor native pdk --stage validate
pmtool --executor native pdk --stage unit
pmtool --executor native voxbox --stage validate
pmtool --executor native voxbox --stage unit
pmtool --executor native audit
pmtool --executor native release-checks
pmtool --executor native template-test --module /template
```

Native виконує той самий план установленими інструментами job без Docker.
Це явний режим із прямим доступом до файлової системи та environment job,
не sandbox. Git/API-мутації в ньому заборонені. Для вибраних кроків job image
має містити відповідний toolchain; для check — обидва.

## Збірка launcher

```sh
make format
make build VERSION=v1.0.0
```

Повна збірка перевіряє Go, реальний ERB і RuboCop Ruby-assets, security, чотири платформи,
LICENSE, SHA256SUMS та smoke. Код розкладений у cmd/internal/docs.
Для source-релізу спочатку коммітимо код, залишивши changelog під `## Unreleased`.
Окремим коммітом фіналізуємо заголовок як `## Release 1.0.0` із повідомленням
`chore(release): prepare v1.0.0`. Цей комміт містить лише release-зміни;
VERSION-файл не додається.

Після обох локальних коммітів запускаємо CI для підготовленого чистого main:

```sh
make push-main
```

Пушиться лише main. Таргет чекає CI точного SHA, завантажує його артефакти
`pmtool-dev-<повний SHA>` і перевіряє checksums, LICENSE, джерело/run та запуск
нативного бінарника. Файли лежать у `dist/ci/<SHA>/`; перевірте цей бінарник зі
своїм шаблоном і модулем. `make ci-artifacts` повторює завантаження без пушу.
На цьому етапі GitHub Release/prerelease не створюється.

Після перевірки кандидата:

```sh
make push-release VERSION=v1.0.0
```

Потрібні перевірені артефакти поточного чистого main, синхронізованого із origin.
Таргет виконує preflight та створює/пушить stable-тег; main тут не пушиться.
Прямі tag-таргети перевіряють ті самі артефакти. Tag CI звіряє main і його успішний
commit CI; draft GitHub Release стає публічним після успішного завантаження файлів.
Окремого `rc.N` немає. Після збою зберігайте тег, перевірте/перезапустіть workflow,
тоді використовуйте push-release-tag тієї самої версії. Build/check/clean нічого не публікують.

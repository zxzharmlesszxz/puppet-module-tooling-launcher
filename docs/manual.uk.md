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

Host-конфіг: ~/.config/puppet-module-tooling/config.json. Нові каталоги мають 0700,
нові файли — 0600; права наявних не змінюються. config init зберігає наявний файл.
config show створює відсутній конфіг; config path лише друкує шлях. Docker workflow
може створити конфіг і під час --dry-run. Конфіг містить image і Docker executable,
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
--voxbox-image, --stage. Для pipeline-logs також: --pipeline, --job, --failed,
--tail, --output; для template-init: --name, --output, --launcher-version та
окремий вибір provider. Для документації: --section.

.sync.yml містить template.use і optional ci.use/ci.provider. CLI має пріоритет;
якщо provider не заданий, він визначається з origin. CI за замовчуванням береться
з тієї самої версії шаблону. Глобального inventory/fleet немає.

## Вибір images і сумісні команди

--image та поле config.image вибирають середовище ERB, template audit і template-test.
Вони не замінюють образи PDK/VoxBox. Для PDK пріоритет --pdk-image → PDK_IMAGE →
вбудований pin; для VoxBox/release-checks — --voxbox-image → VOXBOX_IMAGE → pin.
Native використовує вже встановлені executables/bundles. Генератор CI бере SDK pins
із бінарника, незалежно від host config; --launcher-version задає версію завантаження.
TOOLING_COLOR задає auto/always/never, якщо --color залишено auto; непорожній NO_COLOR вимикає
колір launcher навіть із --color always. TERM=dumb вимикає автоматичний колір;
перенаправлена документація зберігається як Markdown. COLUMNS задає ширину довідки.

Застарілий --ci-project підтримується лише в GitLab і вибирає
/ci/puppet-module.yml із фіксованим ref v0.6.0. Використовуйте повний --ci.

Сумісні aliases: bootstrap-local → sync; bootstrap-local-noop → sync-noop;
audit-local → audit; pdk-local → pdk; pdk-convert-local → pdk-convert;
pdk-convert-local-noop → pdk-convert-noop. Aliases додають лише поведінку canonical command: bootstrap-local — це sync,
а pdk-convert-local виконує PDK conversion; noop-варіант робить її preview.
bootstrap-commit-noop та release-commit-noop не змінюють index/HEAD;
release-tag-noop може чекати main CI, але не створює/пушить тег. Preview conversion
може створювати звіт, а --dry-run у Docker workflow — host config.

manifest показує каталог можливостей; read_only не гарантує відсутність remote
мутацій: mr-create, наприклад, створює MR/PR. Дозвіл користувача й runtime guards
потрібні незалежно від цього поля. У Docker executor audit helper має read-only
module mount, а PDK/VoxBox validators — writable mount для звітів, caches та build
artifacts. Native checks працюють зі звичайними правами користувача CI.

## Назва репозиторію та Git transport

Git/API-команди вимагають origin basename `puppet-<module>`; lifecycle перевіряє,
що metadata owner-module називає той самий модуль. Вибір template не змінює це
обмеження. SSH/SCP origin приймається для визначення проєкту, але remote Git
pmtool формує як HTTP(S) URL із provider token. Наявні Git transport rewrites
далі можуть змінити транспорт; це не додає окрему SSH-authentication функцію
launcher. SSH-ключі не монтуються в tools containers. Template fetch — лише HTTPS.
Для загальної довідки використовуйте pmtool --help або pmtool docs. Command-local
--help є в host Git/API-команд і template-init; validation/template-test його не
реалізують. template-test приймає лише порожні args або --module PATH окремо.

## Ідентичність репозиторію в CI

pmtool читає єдиний локальний `remote.origin.url` через Git config. Це значення
використовують визначення provider, операції із шаблоном і перевірки релізного
workflow. Правила `url.*.insteadOf`, зокрема з runner-файла через `include.path`,
лишаються налаштуваннями транспорту. Читання зберігає конфігурацію Git.

Пароль, query, fragment, кілька URL і переноси рядка в origin відхиляються.
Якщо runner записав credentials безпосередньо в origin, CI шаблону має задати
чистий URL проєкту перед запуском pmtool. Токени лишаються в призначених для них
змінних автентифікації. Без credentials Git-завантаження шаблону завершується
помилкою без інтерактивного запиту, залежності від `/bin/false` чи запуску
користувацьких askpass-програм.

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
pipeline-jobs приймає --ref або --branch, типово поточну гілку і --watch 0;
діапазон watch — 0..86400 секунд. При watch=0 активний pipeline може повернути 0;
це не означає успіх CI. Завершений невдалий pipeline повертає помилку.
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
не sandbox. Host Git/API-команди, включно зі status та pipeline reads, у цьому
режимі заборонені; локальні help/settings/template-init обробляються раніше. Для вибраних кроків job image
має містити відповідний toolchain; для check — обидва.

## Збірка launcher

```sh
make format
make build VERSION=v1.1.0
```

Повна збірка перевіряє Go, реальний ERB і RuboCop Ruby-assets, security, чотири платформи,
LICENSE, SHA256SUMS та smoke. Код розкладений у cmd/internal/docs.
Для source-релізу використовуємо окремий GitHub workflow через Make:

```sh
make release-prepare VERSION=v1.2.3
make check
make release-submit VERSION=v1.2.3 MESSAGE='Describe the implementation'
make mr VERSION=v1.2.3
```

Prepare створює гілку із синхронізованого чистого main. Submit коммітить код,
окремо фіналізує Unreleased як `chore(release): prepare v1.2.3` і пушить гілку.
MR чекає CI точного SHA, зливає PR, синхронізує main та перевіряє його артефакти.
Перевірте завантажений бінарник зі своїм шаблоном і модулем перед публікацією:

```sh
make push-release VERSION=v1.2.3
```

Перегляд CI:

```sh
make pipeline-jobs REF=main WATCH=5
make pipeline-logs REF=main TAIL=100
```

Без REF вибирається поточна гілка; пайплайн прив’язаний до SHA локального ref.
RUN_ID має відповідати йому. JOB задає job ID саме цього run, FAILED_ONLY=0
показує також успішні jobs. Помилки API/логів лишаються помилками.

Окремі main-sync, release-commit, release-finalize та release-push доступні для
відновлення після помилки. Prepare/submit/push/tag noops — необов’язкові окремі
перевірки без мутацій. Комміти після невдалого пушу зберігаються; теги не
пересуваються. MR не видаляє гілку й не публікує реліз. Перевірки CI-артефактів,
preflight та draft до завершення uploads збережено. VERSION-файла немає.

## Генерація шаблону та логи pipeline

```sh
pmtool template-init --name team_template --output ./new-template --provider both
pmtool template-test --module ./new-template
pmtool pipeline-logs --ref main --failed --tail 100
pmtool pipeline-logs --pipeline 123 --job 456 --tail 0 --output ./job.log
```

Генератор працює локально без Docker, Git і мережі. Каталог призначення має бути
новим, батьківський — наявним без symlink. Провайдер: gitlab, github або both
(типово both). CI закріплює стабільну версію генератора; dev-бінарю потрібний
`--launcher-version vX.Y.Z`. Стартовий шаблон керує лише `.editorconfig`, додає
тести, опис schema 1 та CI валідації. Контракти, поріг покриття й публікацію Forge
команда додає окремо. Нові каталоги/файли мають 0700/0600. Preview друкує план
без перевірки доступності destination; write error може лишити неповний новий
каталог. Перегляньте його або виберіть новий шлях. Наявний каталог не використовується
повторно. Генератор нічого не комітить і не пушить.

Логи читаються через API провайдера поточного модуля. Без `--pipeline` береться
останній push pipeline для `--ref` (типово поточна гілка). Якщо ref є наявною
серверною гілкою, вибір прив’язується до її поточного SHA; для тега цієї прив’язки
до branch head немає. `--pipeline` дозволяє вибрати старіший запуск; заданий `--ref` мусить
збігатися. `--job` — числовий ID з цього pipeline, який видно у `pipeline-jobs`.
Типово вибираються всі jobs, `--failed` — тільки failed. `--tail` типово 100,
0 — повний лог, максимум 100000 рядків; ліміти 16 MiB/job та 32 MiB загалом.

`--output` створює тільки новий файл із правами 0600 відносно поточної папки
виклику, незалежно від `--module`. Наявні файли й symlink не замінюються.
Перед записом мають успішно завантажитися всі вибрані логи. У файл записується обраний tail; для повного bounded log задайте --tail 0.
Відомі токени й термінальні керівні послідовності прибираються; інші секрети перевіряйте вручну.
GitHub signed URL завантажується через HTTPS без передавання токена та cookies.

При падінні CI у `mr`/`release-tag` автоматично виводяться останні 100 рядків
невдалих jobs. Помилка отримання логів дає попередження й не підміняє початкову
CI-помилку. Merge/tag не дозволяється, вже запушений тег зберігається. Успіх
`pipeline-logs` означає лише прочитаний лог. Повні деталі — в
[англійському manual](manual.md#pipeline-logs).

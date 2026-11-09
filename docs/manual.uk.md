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
--file, --files, --version, --ci, --provider, --template, --voxbox-image, --stage. Для pipeline-logs також: --pipeline, --job, --failed,
--tail, --output; для template-init: --name, --output, --launcher-version, --origin та
окремий вибір provider. --name також задає назву для module-new. Для документації: --section.

--files — один рядок відносних шляхів через пробіли, без shell-парсингу. Лапки
навколо аргументу не додають підтримку пробілів усередині назви. Явний commit
selection приймає лише ASCII літери/цифри, _, ., / та -, без початкового дефіса
або traversal; він мусить охоплювати всі змінені файли. Для назв із пробілами чи
Unicode використовуйте автоматичний selection без --files. У sync цей параметр
погоджує лише переглянуті заміни/видалення baseline.

.sync.yml містить template.use і optional ci.use/ci.provider. CLI має пріоритет;
якщо provider не заданий, він визначається з origin. CI за замовчуванням береться
з тієї самої версії шаблону. Глобального inventory/fleet немає.

## Вибір images і сумісні команди

--image та поле config.image вибирають середовище ERB, template audit і template-test.
Вони не замінюють образ перевірок і jig: --voxbox-image → VOXBOX_IMAGE → вбудований pin.
Native використовує вже встановлені executables/bundles. Генератор CI бере SDK pins
із бінарника, незалежно від host config; --launcher-version задає версію завантаження.
TOOLING_COLOR задає auto/always/never, якщо --color залишено auto; непорожній NO_COLOR вимикає
колір launcher навіть із --color always. TERM=dumb вимикає автоматичний колір;
перенаправлена документація зберігається як Markdown. COLUMNS задає ширину довідки.

Застарілий --ci-project підтримується лише в GitLab і вибирає
/ci/puppet-module.yml із фіксованим ref v0.6.0. Використовуйте повний --ci.

Сумісні aliases: bootstrap-local → sync; bootstrap-local-noop → sync-noop;
audit-local → audit. Aliases виконують лише відповідну canonical command.
bootstrap-commit-noop та release-commit-noop не змінюють index/HEAD;
release-tag-noop може чекати main CI, але не створює/пушить тег. module-convert-noop передає jig convert --dry-run; --dry-run у Docker workflow може створювати host config.

manifest показує каталог можливостей; read_only не гарантує відсутність remote
мутацій: mr-create, наприклад, створює MR/PR. Поле token стосується
module-provider authentication; forge-publish окремо потребує свій environment token. Дозвіл користувача й runtime guards
потрібні незалежно від цього поля. У Docker executor audit helper має read-only
module mount, а VoxBox validators — writable mount для звітів, caches та build
artifacts. Native checks працюють зі звичайними правами користувача CI.

## Назва репозиторію та Git transport

Git/API-команди вимагають origin basename `puppet-<module>`; lifecycle перевіряє,
що metadata owner-module називає той самий модуль. Вибір template не змінює це
обмеження. SSH/SCP origin приймається для визначення проєкту, але remote Git
pmtool формує як HTTP(S) URL із provider token. Наявні Git transport rewrites
далі можуть змінити транспорт; це не додає окрему SSH-authentication функцію
launcher. SSH-ключі не монтуються в tools containers. Template fetch — лише HTTPS.
Для загальної довідки використовуйте pmtool --help або pmtool docs. Command-local
--help є в host Git/API-команд і template-init; validation/template-test/jig
та forge-publish його не реалізують. template-test приймає лише порожні args або --module PATH окремо.

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
VoxBox validation/specs і release-checks та зупиняється
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
це не означає успіх CI. Завершений невдалий pipeline повертає помилку. Навіть
зелений watcher перевіряє статус pipeline, а не release-tag publication guards
чи наявність forge-publish. Під час recovery окремо перевірте publishing job та Forge.
Пуш тега ще не доводить публікацію; перевірте Forge й canary.

*-noop — опційні preview, а не обов'язкові додаткові кроки. Для bootstrap:
policy-noop → review → policy → bootstrap-prepare --branch bootstrap/PURPOSE,
за потреби module conversion, потім migrate → sync → edit → check → bootstrap-commit
→ bootstrap-push → mr. Наявні baseline-конфлікти потребують перегляду й --files.

Після перевіреної публікації та canary: cleanup-noop --branch release/X.Y.Z,
перегляд, потім окремий cleanup. Теги й Forge-артефакти не видаляються.

## Доступи й native CI

VoxBox запускаються pmtool через хостовий Docker. Жоден із цих контейнерів
не отримує Docker socket. Токени, credentials шаблону й автор комміту
надаються лише потрібному кроку; у validator-кроки вони не передаються.

```sh
pmtool --executor native voxbox --stage validate
pmtool --executor native voxbox --stage unit
pmtool --executor native audit
pmtool --executor native release-checks
pmtool --executor native template-test --module /template
pmtool --executor native module-build
```

Native виконує той самий план установленими інструментами job без Docker.
Це явний режим із прямим доступом до файлової системи та environment job,
не sandbox. Host Git/API-команди, включно зі status та pipeline reads, у цьому
режимі заборонені; локальні help/settings/template-init обробляються раніше. Для вибраних кроків job image
має містити потрібні інструменти: VoxBox для перевірок, jig для module-build.
Генерація й конвертація використовують Docker. forge-publish — окрема Go-команда
на хості, яка також відхиляє --executor native.

## Збірка launcher

```sh
make format
make build VERSION=v2.0.0
```

Повна збірка перевіряє Go, реальний ERB і RuboCop Ruby-assets, security, чотири платформи,
LICENSE, SHA256SUMS та smoke. Код розкладений у cmd/internal/docs. Тести шаблону
мають окремо перевіряти згенерований Ruby під підтримуваними стилями модулів:
лінт вихідних Ruby-assets не перевіряє результат ERB.
Для source-релізу використовуємо окремий GitHub workflow через Make:

```sh
make release-prepare VERSION=v2.0.0
make check
make release-submit VERSION=v2.0.0 MESSAGE='Describe the implementation'
make mr VERSION=v2.0.0
```

Prepare створює гілку із синхронізованого чистого main. Submit коммітить код,
окремо фіналізує Unreleased як `chore(release): prepare v2.0.0` і пушить гілку.
MR чекає CI точного SHA, зливає PR, синхронізує main та перевіряє його артефакти.
Перевірте завантажений бінарник зі своїм шаблоном і модулем перед публікацією:

```sh
make push-release VERSION=v2.0.0
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

Генератор працює локально без Docker і мережі; Git потрібний лише з --origin URL. Каталог призначення має бути
новим, батьківський — наявним без symlink. Провайдер: gitlab, github або both
(типово both). CI закріплює стабільну версію генератора; dev-бінарю потрібний
`--launcher-version vX.Y.Z`. Стартовий шаблон керує лише `.editorconfig`, додає
тести, опис schema 1 та CI валідації. Контракти, поріг покриття й публікацію Forge
команда додає окремо. Нові каталоги/файли мають 0700/0600. Preview друкує план
без перевірки доступності destination; write error може лишити неповний новий
каталог. Перегляньте його або виберіть новий шлях. Наявний каталог не використовується
повторно. Генератор нічого не комітить і не пушить.

NAME починається з малої ASCII літери й далі містить лише малі ASCII літери, цифри,
підкреслення або дефіси; module-new дефісів не дозволяє. --launcher-version перевіряє
лише синтаксис версії, а не доступність релізу чи сумісність. Обирайте опублікований
launcher, який підтримує згенерований CI.

Генератор не створює hosted project. Створіть окремий репозиторій шаблону на
GitLab/GitHub і передайте --origin URL: команда ініціалізує Git/main без комміту
та додасть remote. Без параметра локальний Git не створюється — підключіть його
вручну. --provider лишається незалежним, типово both. Приймаються HTTP(S), SSH та
SCP-style URL без credentials/query/fragment, з namespace і repository. SSH тут
лише зберігається як remote; template fetch лишається HTTPS. Наявність remote й
доступ до нього не перевіряються. Preview показує Origin без запуску Git; apply
перевіряє Git до запису файлів. При збої Git після генерації файли й частковий .git
зберігаються для розбору; не повторюйте генерацію в цей самий каталог. Після review/tests зробіть commit і push main,
дочекайтеся CI, потім опублікуйте version tag. Лише тоді задавайте template.use
у модулях. Деталі: [підключення remote](TEMPLATES.md#connect-and-publish-the-repository).

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
Термінальні керівні послідовності прибираються. Приховуються поточні значення
GITLAB_TOKEN, GITHUB_TOKEN, GH_TOKEN, CI_JOB_TOKEN, TOOLING_TEMPLATE_TOKEN та
FORGE_PUBLISH_TOKEN і FORGE_TOKEN, а також обраний Git credential. Інші секрети перевіряйте вручну.
GitHub signed URL завантажується через HTTPS без передавання токена та cookies.

При падінні CI у `mr`/`release-tag` автоматично виводяться останні 100 рядків
невдалих jobs. Помилка отримання логів дає попередження й не підміняє початкову
CI-помилку. Merge/tag не дозволяється, вже запушений тег зберігається. Успіх
`pipeline-logs` означає лише прочитаний лог. Повні деталі — в
[англійському manual](manual.md#pipeline-logs).

## Каркас модуля, конвертація та пакування

Новий модуль створюйте в наявній батьківській папці:

```sh
pmtool module-new --module /absolute/modules --name sample
```

Для наявного legacy-модуля конвертація є альтернативою до першого sync:

```sh
pmtool module-convert-noop
pmtool module-convert
```

Пакування запускайте з папки самого модуля:

```sh
pmtool module-build
```

module-new створює новий підкаталог у --module (типово поточна папка) через jig.
Наявні файли, каталоги й symlinks не замінюються. NAME починається з малої ASCII
літери й далі містить лише малі ASCII літери, цифри або підкреслення; дефіси заборонені.
Metadata та OS/releases потрібно заповнити.
module-convert призначений лише для початкової адаптації до першого sync: він замінює
Gemfile, Rakefile і spec/spec_helper.rb та прибирає Gemfile.lock. Перегляньте custom зміни
заздалегідь: закріплений VoxBox містить jig 2.5.0 без backup під час конвертації.
У jig 2.5.1 додано .orig backups, але старий digest образу автоматично не оновлюється.
Конвертація та її preview відхиляють наявний .sync.lock.json і вимагають каталог
manifests. Модулі з керованим baseline оновлюються лише через sync. Jig-каркас та team template.use
мають різні формати й окреме призначення. module-build запускає jig build; правила
пакування задає jig.toml, .pdkignore не використовується. Native CI підтримує module-build.

## Публікація архіву на Forge

forge-publish завантажує готовий архів через Go без Docker, provider tokens та image
config. FORGE_URL — HTTPS base URL без credentials, query або fragment; endpoint —
BASE_URL/v3/releases. FORGE_PUBLISH_TOKEN береться лише з environment: задайте сам
токен без Bearer, цей префікс pmtool додає в HTTP-заголовок.

Очікується звичайний непорожній pkg/OWNER-MODULE-X.Y.Z.tar.gz до 256 MiB, без symlinks
у шляху; після розпакування також не більше 256 MiB. Перевіряється весь gzip stream,
включно з checksum/footer. Metadata модуля має стабільну X.Y.Z версію. Архів повинен містити рівно один
відповідний звичайний OWNER-MODULE-X.Y.Z/metadata.json до 1 MiB із тим самим name/version.
Успіх — HTTP 201. Redirect, duplicate version та інші помилки зупиняють публікацію,
автоматичних повторів немає; timeout запиту — дві хвилини. --dry-run вимагає
FORGE_URL і валідний архів, але не токен; він лише перевіряє архів і показує endpoint.
Обидва режими відхиляють --executor native — uploader і так працює на хості.
CI зберігає protected tag, exact-main та release-check gates перед публікацією.

## Повнота OS-матриці

Повнота OS-матриці — політика шаблону, а не універсальна вимога launcher. Team helper
може вимагати непорожні OS/releases у metadata та факти для кожної пари: відсутня
платформа, зокрема Debian 13, тоді зупиняє suite замість тихого пропуску. Це доповнює
вимоги обраного шаблону до executable specs, pending/skipped suites і resource coverage.
Наявність фактів не доводить виконання всіх сценаріїв: реальні resource/parameter assertions
мають використовувати повну on_supported_os матрицю. Типове середовище — OpenVox 8;
OpenVox 9 потребує окремо перевіреного environment lane.

Validation-крок VoxBox виконує всі три Rake tasks: validate, lint та rubocop.
Це синтаксис, Puppet lint та Ruby lint; --stage validate не скорочує цей набір.

## Перехід на 2.0

Версія 2.0.0 прибирає команди/aliases PDK та --pdk-image/PDK_IMAGE.
Використовуйте voxbox або повний check; для початкового onboarding — module-convert
і optional module-convert-noop. Наявний managed baseline оновлюється через sync.
Validation виконує validate, lint і rubocop; unit та release checks збережено.

Спочатку публікується pmtool v2, потім команда випускає нову незмінну версію
шаблону/CI з v2 без PDK jobs. Далі модулі змінюють template.use на робочій гілці,
виконують sync/check та проходять branch CI перед звичайним релізом.
Не переміщуйте старі теги й не вибирайте неопублікований шаблон.

Поточний Go module/import path має суфікс /v3; executable, назва репозиторію, release URL і
шлях config.json лишаються чинними. Go toolchain — 1.27.3, runtime — закріплений
VoxBox з OpenVox 8. Це не перехід на OpenVox 9 і не автоматичне оновлення jig.
Повна [таблиця заміни команд](manual.md#upgrading-to-20).


## Приватні тестові залежності

У `.fixtures.yml` → `forge_modules` задаються Forge fixtures. З
`fixtures.resolve_dependencies: true` достатньо вказати прямі fixtures:
OpenVox підтягне їхні залежності з метаданих. Єдиний проксі задає `FORGE_URL`,
читання авторизує `FORGE_TOKEN`. Приватний конфіг видаляється перед RSpec.
Публікація використовує окремий `FORGE_PUBLISH_TOKEN`. Див. [Forge fixtures](FIXTURES.md).


## Входи команд та авторизація шаблону

[Таблиця входів команд](COMMAND-INPUTS.md) відокремлює обов'язкові параметри від
автоматичних значень і необов'язкових override. Авторизація шаблону: явний
`TOOLING_TEMPLATE_TOKEN` з відповідним host → Git credential helper → anonymous.
За налаштованого Git helper `TOOLING_TEMPLATE_*` не потрібні. У CI лишається
явне передавання дозволеного job token. Helper читається тільки для template
fetch на хості; SDK та audit-скрипти credentials не отримують. Можливість
поки не входить до опублікованої pmtool 2.0.0.


## Вимоги до токенів і перевірка входів

[Повна матриця токенів розробника та CI](TOKENS.md) доступна через
`pmtool docs tokens`: scopes, ролі, allowlist для CI_JOB_TOKEN, читання шаблону,
приватні fixtures та окреме право публікації Forge. Preflight показує назви
відсутніх обов'язкових параметрів і завершується до виконання дій; опційні
перевизначення не вимагаються. Серверні права перевіряються самим сервером.
`pmtool help` — короткий usage, `pmtool docs` — повний посібник;
`pmtool docs --section list` — його зміст. Усі ці запити локальні, без токенів.


## Перехід на версію 3

Схему test_fixtures у .sync.yml прибрано: перенесіть модулі та версії у
.fixtures.yml forge_modules. FORGE_TOKEN_HOST не використовується; потрібні
FORGE_URL/FORGE_TOKEN для авторизованого проксі. Без обох змінних універсальний
launcher залишає стандартний публічний Forge, одна змінна без іншої зупиняє
preflight. Neon CI вимагає обидві для свого unit job. Go module/import paths —
/v3, назва бінарника та URL релізів незмінні. Старі теги не пересуваємо.

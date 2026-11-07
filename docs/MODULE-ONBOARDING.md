# Підключення та реліз Puppet-модуля

Template: --template → .sync.yml template.use, без неявного шаблону.
CI: --ci → .sync.yml ci.use → CI обраного шаблону на тому самому SHA.
Provider: --provider → .sync.yml ci.provider → визначення з origin.
Модульний .sync.yml містить template.use, optional ci.use/ci.provider; config.json — image/docker; socket є сумісним застарілим полем і не монтується.
Токени лише через env. Деталі: docs ci.
Налаштування читає launcher. За замовчуванням CI береться з тієї самої версії
шаблону; `ci.use` або `--ci` потрібні лише для явного override.

Workflow і перевірки належать pmtool; image містить лише інструменти й залежності. Host Makefile і
командна робоча копія не потрібні. Go-launcher автоматично підбирає параметри виконання/env. Один запуск працює з одним фізичним clone; project береться з його origin. Виконуйте команди по одній; на помилці зупиніться.
Для змісту: `docs onboarding --section list`; для одного розділу: `docs onboarding --section 'Підготовка нового релізу'`. У терміналі текст форматований, перенаправлення stdout зберігає Markdown.
Команди та межі виконання описані в [USAGE.md](USAGE.md), параметри — у [manual](manual.md).

## Приватні template-репозиторії

Для приватного template pmtool спочатку використовує явний
`TOOLING_TEMPLATE_TOKEN` з відповідним `TOOLING_TEMPLATE_TOKEN_HOST`, потім
збережені credentials Git helper для точного HTTPS URL, а за їх відсутності —
анонімний HTTPS. Для звичайного `pmtool sync` із налаштованим Git helper змінні
шаблону не потрібні. `TOOLING_TEMPLATE_USERNAME` — лише опційний username для
явного token override; типово `oauth2`.

Lookup виконується на хості, без інтерактивного askpass, з урахуванням глобальної
конфігурації Git. Він не запускає helper із локальної конфігурації модуля й не
виконує store/erase. Невідповідний host явного токена — помилка без fallback.
`GITLAB_TOKEN`/`GITHUB_TOKEN` модуля не використовуються як заміна template token.
CI може явно передати дозволений `CI_JOB_TOKEN` через template override.

Credentials застосовуються лише до template fetch потрібного repository та не
передаються в ERB/audit/SDK. Доступ до CI includes налаштовується окремо.
Повний опис: [private templates](TEMPLATES.md#private-template-repositories).
Див. також [команди та потрібні входи](COMMAND-INPUTS.md).
Автоматичний helper поки є в робочій гілці launcher, не в опублікованій 2.0.0.

## Перед початком роботи з ШІ

`pmtool ai export --format prompt` друкує інструкції для нового чату.
Для інструкцій у репі використайте `ai export --output ./AGENTS.md`; для переносимого
skill — `ai export --format skill --output ./puppet-module-workflow/SKILL.md`.
Наявний файл не перезаписується: спочатку експортуйте в інше місце й погодьте з
наявними правилами репи. Експорт не встановлює skill і не дає дозволу на push/реліз.
Інструкції й перелік команд відповідають встановленому pmtool; після його оновлення
потрібно переглянути й оновити експорт. Вбудовані тексти оновлюються перезбіркою pmtool. Конфіг модуля та секрети в нього не включаються.

## Створення нового шаблону (1.1.0)

Для вже опублікованого шаблону достатньо вибрати `template.use` у `.sync.yml`.
Якщо команда ще не має шаблону, створіть новий пакет окремо від каталогу модуля:

```sh
pmtool template-init --name team_template --output ../team-template --provider both
pmtool template-test --module ../team-template
```

Генерація локальна: без Docker, мережі та host config; Git потрібний лише з --origin. Каталог має бути новим,
батьківський — наявним без symlink. Провайдер — gitlab, github або both (типово both).
Для template-test потрібний tools image або явний native SDK. Dev-бінарнику генератора
потрібний `--launcher-version vX.Y.Z`, що вказує на опублікований стабільний launcher.

Створіть окремий порожній проєкт шаблону на GitLab/GitHub і передайте його URL
через --origin під час генерації:

```sh
pmtool template-init --name team_template --output ../team-template --provider gitlab --origin https://git.example.com/team/puppet-template.git
```

Це альтернатива першому виклику генератора, а не повторний запуск у той самий
каталог. Команда ініціалізує Git із main без комміту й додає origin, але не створює
hosted project і не перевіряє його доступність. Без --origin Git не потрібний;
у такому разі підключіть remote вручну після генерації:

```sh
cd ../team-template
git init --initial-branch=main
git remote add origin https://git.example.com/team/puppet-template.git
git remote -v
```

Це origin самого шаблону, не модуля чи launcher. Для вже ініціалізованого Git
перевірте наявний origin замість повторного init/add. Після review та template-test
зробіть commit і push main з upstream, дочекайтеся зеленого CI, потім опублікуйте
annotated version tag. Лише після цього вибирайте його в template.use модуля.
Це окремі Git/hosting-операції: module release-команди pmtool не публікують шаблон.

Стартовий пакет містить schema 1, defaults/ERB, опис формату, тести та CI валідації.
Він керує лише `.editorconfig`; контракти, specs для сутностей, вимірювання 100%
resource coverage й Forge publication додає команда. Не вважайте їх увімкненими
самою генерацією. Перевірте пакет, опублікуйте незмінний Git-тег і лише після цього
вибирайте його в модулі. Генератор нічого не комітить і не пушить. Деталі:
[template authoring](TEMPLATES.md#create-a-template).

## Запуск через launcher

Із каталогу модуля всі наступні runtime-команди можна запускати через Go-бінарник:

```sh
pmtool config init
pmtool policy-noop
pmtool audit
pmtool release-prepare --version X.Y.Z
```

Виконуйте лише відповідний поточному кроку виклик. Launcher виконує Git/API у Go,
а інструментальні перевірки — в офіційних VoxBox без Docker socket і provider tokens.
Параметри операції передавайте після команди launcher-а.
За потреби іншого clone додайте `--module /absolute/clone`. Не копіюйте Docker flags
в аргументи launcher-а. Його конфіг/інсталяція: `pmtool docs launcher`.
Усі операції модуля виконує pmtool; VoxBox надає лише інструменти.

## 1. Підготовка

Замініть placeholders: погоджений image за SHA/digest, фізичний шлях звичайного clone,
версію майбутнього релізу. Не використовуйте zsh-змінні
`modules`, `path` або `status`. Працюйте з фізичним каталогом звичайного clone, без вкладених Docker-контейнерів.

```sh
pmtool config init
pmtool status
```

Налаштуйте Docker/registry access, selected provider token та права на потрібні module projects.
GitLab/GitHub project та API host визначаються з origin цього clone. Git/API
workflow вимагає назву репозиторію `puppet-<module>` і відповідну metadata. SSH/SCP
origin використовується як identity; pmtool будує HTTP(S) remote URL із provider
token, а Git transport rewrites лишаються чинними. Немає namespace
config, inventory або окремого cache. Можна одночасно працювати з модулями різних команд.
Для нестандартного API URL передайте `GITLAB_BASE_URL` лише цьому запуску; host має
відповідати origin. CI задається через `--ci 'repo/path@version'` або модульний `.sync.yml ci`.
GitHub/Enterprise та permissions: [CI providers](CI-PROVIDERS.md), `docs ci`.
Launcher сам обирає провайдера і токен; шаблон визначається з модульного `.sync.yml`.
Ідентичність читається з єдиного локального `remote.origin.url`, без transport rewrites.
Runner authentication зберігається; origin із паролем, query/fragment або кількома URL
відхиляється. Якщо runner записав credentials у сам URL, CI спершу нормалізує його.
Відсутні credentials шаблону дають неінтерактивну помилку; `/bin/false` не потрібний.

```sh
export GITLAB_TOKEN='...'
export GIT_AUTHOR_NAME="$(git config --get user.name)"
export GIT_AUTHOR_EMAIL="$(git config --get user.email)"
```

Не додавайте token у YAML, Git або URL. Host global Git config не монтується;
передавайте author тільки командам, які створюють commit/tag. Перевірте origin,
поточну гілку та локальні незакомічені зміни до prepare.

## 2. Політики module project

Спочатку preview, потім застосування політик:

```sh
pmtool policy-noop
```

```sh
pmtool policy
```

Очікується default `main`, заборона прямого push у main та захист release tags.
Не створюйте main довільно від bootstrap-змін. `policy-delete-master` — окрема
погоджена дія після міграції. Шлях CI entrypoint задає manifest обраного шаблону
або явний CI override; перевірте доступи та runner tags саме цього CI.

## 3. Bootstrap-гілка

Для чистого clone створіть bootstrap-гілку через tooling:

```sh
pmtool bootstrap-prepare --branch bootstrap/shared-contracts
```

Це fetch/sync main та створення гілки, без metadata, commit/push, MR або tests.
Prepare не повторюють на вже створеній гілці. Dirty clone або стороння ancestry
потребують розбору; tooling не виконує reset/stash.

## 4. Генерація або початкова конвертація через jig

Генерація нового модуля — окремий початок роботи, а не крок усередині вже
підготовленого clone з кроку 3. Запускайте її з батьківської папки майбутнього модуля:

```sh
pmtool module-new --name sample
```

Після генерації перейдіть у новий модуль і підготуйте його Git origin/main перед
bootstrap. Для вже наявного legacy-clone генерацію пропустіть; без керованого
baseline за потреби перегляньте й застосуйте конвертацію:

```sh
pmtool module-convert-noop
pmtool module-convert
```

Конвертація переписує Gemfile, Rakefile і spec/spec_helper.rb та прибирає Gemfile.lock.
Перегляньте custom налаштування до застосування; у закріпленому образі jig 2.5.0
не створює резервних копій. jig 2.5.1 додав .orig backups, але це не оновлює старий image pin.
Не конвертуйте baseline, яким уже керує pmtool: для нього використовуйте sync.
Задайте metadata, namespace, author, license, supported OS, dependencies та fixtures явно.
Збережіть .sync.yml і .rubocop.yml: вони використовуються нашим workflow.
Потім виконайте migrate і sync. Міграція очищає старі template metadata та fixture symlinks;
вона не запускає старі інструменти.

## 5. Detachment та baseline

Після перевірки metadata/fixtures приберіть legacy template metadata:

```sh
pmtool migrate
```

Після migrate одразу виконайте sync у локальному clone. Sync-noop доступний
як опційний readonly preview:

```sh
pmtool sync
```

При відмінних існуючих файлах tooling відмовляє. Перегляньте diff, перенесіть
потрібні RSpec customizations у `spec/spec_helper_local.rb` і повторіть sync
з `--files '.gitignore spec/spec_helper.rb'` лише для погоджених замін. Це список
шляхів через пробіли, без shell-парсингу: назву з пробілом так передати не можна.
Окремий sync-noop з цим allowlist можна виконати за потреби. Наявні metadata, fixtures та domain code baseline не генерує.
Не видаляйте керовані файли вручну для обходу конфліктів.

## 6. Доменні контракти

Основні класи зазвичай розділяють repo → install → config → service. Defaults живуть
у Hiera модуля; доменні параметри посилаються на згенеровані typed contracts.
Не додавайте `lookup()` у кожен параметр замість automatic parameter lookup.
Явно задавайте merge policy для nested Hash/Array; перевіряйте, що списки не отримують
зайві default елементи при deep merge. Package install не повинен випереджати
створення ключа, repository та apt cache update. Перевірте managed/unmanaged repo,
зміну signing key на першому проході, hold/unhold і поведінку сервісу при видаленні пакета.

Package та Service structs передаються в ресурси явними полями; наявність alias
сама по собі не означає використання. Filesource має доходити до доменних параметрів,
repository definition — реально оголошуватись, якщо вони входять до обраного шаблону.
Вибір контрактів і правил аудиту належить `template.yml` шаблону; у модулі задайте
`template.use` у `.sync.yml` перед sync. Дивіться [формат шаблону](TEMPLATES.md).

File content/source мають бути взаємовиключними; валідуйте імена перед додаванням
до filesystem path. Не керуйте пакетними файлами/каталогами без доменної потреби.
Права, `show_diff` для секретів, notify/restart і поведінку unmanaged файлів перевіряйте
окремо. AST audit не доводить data flow або runtime поведінку.

### HTTPS signing keys і architecture

У шаблонах із цим repository-контрактом HTTP(S) key_source використовує відкладений SHA256 від
agent-side `<module>::repository_key_checksum`, а не Last-Modified. Перевірте
зміну джерела на старіший за датою ключ, redirects і повторний незмінний apply.
Відсутність checksum headers у Pulp/GCS не повинна залишати попередній vendor key.
Функція та її spec генеруються в namespace модуля і належать repository group.

`architecture` — опційний рядок у repository settings. Для репозиторію лише з
`Architectures: all` використовуйте `architecture: all`; без поля зберігаються APT
defaults. Усі інші package/repo fields лишаються сумісними. Перевірте suite/component
в опублікованому Release: upstream flat layout і layout Pulp mirror можуть відрізнятися.

## 7. Audit, validation та unit у VoxBox

```sh
pmtool check
```

Audit уже є першим кроком check. Окремий pmtool audit потрібний лише для діагностики.

Audit перевіряє керований baseline, ownership та CI. Перевірки metadata/changelog,
tracked ignored files, contract usage і specs для сутностей задає audit policy шаблону.
Профілі з inventory перевіряють класи, defined types, functions, type aliases, Ruby
facts/types/providers/helpers, tasks і plans. Якщо policy вимагає changelog, потрібний
її заголовок для поточної metadata version, наприклад `## Release VERSION`. Generated lock,
fixtures і bin, які вже tracked та ignored, потрібно зняти з tracking після review.
VoxBox запускає validation і unit; на помилці виправте код і повторіть той самий command.
Тести мають перевіряти поведінку, а не лише existence файлів чи aliases.

Для кожної нової сутності додайте відповідний spec; загальні вимоги описані в
[Domain audit and tests](USAGE.md#domain-audit-and-tests).
Наприклад, `sample::install` потребує `spec/classes/install_spec.rb`, а custom provider
`lib/puppet/provider/service/cli.rb` — `spec/unit/provider/service/cli_spec.rb`.
Якщо template checker відхиляє empty/pending-only specs, зберігайте цю вимогу;
shared examples можуть бути дозволені його policy.

Якщо обраний профіль вимагає **100% resource coverage**, перевірте enforcement
у VoxBox і збережіть його вимоги до pending/skipped examples.
Для `Untouched resources` додайте змістовні resource matchers з параметрами та ordering;
окремо перевірте негативні, absent і unmanaged сценарії. Не знижуйте поріг, не додавайте
coverage filters і не замінюйте assertions на завжди позитивні. Цей показник стосується
ресурсів у протестованих каталогах, а не всіх можливих входів чи Ruby line/branch coverage.

Після зміни `template.use` на робочій гілці виконайте sync (sync-noop опційний),
перегляньте helpers/specs, які обрав саме цей профіль, і запустіть check. Зміна image
сама по собі не змінює baseline й не потребує sync, якщо audit не виявив drift.
Поки audit і VoxBox не зелені за обраною policy, не переходьте до commit/release.

## 8. Bootstrap commit та push

Після зеленого check одразу створіть локальний bootstrap-комміт:

```sh
pmtool bootstrap-commit
```

Bootstrap-commit створює локальний комміт погоджених module changes без push.
Bootstrap-commit-noop доступний як опційний перегляд складу комміту та diff. `--files` опційний; сторонні зміни не додавайте.
Після успішного комміту одразу виконайте remote push:

```sh
pmtool bootstrap-push
```

`bootstrap-push-noop` можна використати за потреби для діагностики remote push;
це опційний крок, який не гарантує проходження server hooks. Push не створює MR і не
запускає локальні тести. Не підміняйте bootstrap/release lifecycle ручним Git, коли
потрібна дія вже має команду pmtool.

## 9. MR та merge

```sh
pmtool mr --branch bootstrap/shared-contracts    --title '[UPD] Bootstrap shared module contracts'
```

mr створює або знаходить один opened MR цієї гілки в main, чекає CI його SHA
кожні 5 секунд, мерджить лише той SHA та запускає guarded main-sync.
Після успіху clone вже на актуальному main. Socket і commit author не потрібні.

Зміна head, failed/canceled/skipped/manual, missing pipeline або API error
зупиняють merge. Якщо CI ще не встановлений, виправте bootstrap/include; не
створюйте main із тієї ж гілки для приховування порожнього/конфліктного MR.
Невдалий merge не запускає sync. Якщо merge пройшов, але local sync впав,
виправте локальний стан і повторіть лише main-sync; MR уже merged. Немає reset/stash.

При failed CI автоматично виводяться останні 100 рядків невдалих jobs; помилка
завантаження логів не маскує CI-помилку й не дозволяє merge.

Окремі mr-create, mr-merge, pipeline-jobs і pipeline-logs залишаються для діагностики або
навмисно розділеного workflow. Окремий watcher у стандартному флоу не потрібний.

## 10. Після bootstrap merge

Після mr одразу виконайте check. Main-sync окремо потрібний лише після standalone
mr-merge або для recovery після невдалого local sync.
Перший крок check — audit — уже перевіряє baseline цього clone. Окремі sync-noop
і sync після merge не потрібні, якщо baseline збігається.
Якщо audit виявив baseline drift, створіть окрему робочу гілку та виконайте sync.
У разі конфлікту перегляньте diff і повторіть sync з погодженими --files. Потім check, commit/push та MR. Не застосовуйте
непереглянуті зміни прямо на main. Check не виконує sync baseline і не перемикає
гілку, але VoxBox та тести можуть створювати звіти, кеші, fixtures й build
artifacts у робочій копії.

Повна перевірка цього ж clone:

```sh
pmtool check
```

## 11. Підготовка нового релізу

Для вже підключеного модуля починайте тут. Після перевірки чистоти local clone:

```sh
pmtool release-prepare --version X.Y.Z
```

Замініть X.Y.Z погодженою новою версією, вищою за remote main metadata. Редагуйте
domain code, metadata version, changelog та міграційні приклади на release-гілці.
Для першого bootstrap-релізу, де потрібна metadata вже merged, не створюйте штучний
version bump: після mr main уже синхронізовано — виконайте check та release-tag.
Окремий main-sync потрібний лише після standalone merge або для recovery.

Запустіть check. Після успіху — release-submit: локальний комміт і push гілки.
Файли обираються автоматично, `--files` необов’язковий.

```sh
pmtool release-submit
```

Далі mr створює/знаходить MR, чекає CI, мерджить і синхронізує main.
Окремі release-commit/release-push та їхні noop лишаються доступними за потреби.
Якщо push після комміту впав, повторіть лише release-push; локальний комміт збережено.

## 12. Main, tag та publication

Після mr main уже синхронізовано — одразу check, як у кроці 10.
Окрема main-sync потрібна лише після standalone mr-merge або для recovery:

```sh
pmtool main-sync
```

Main-sync відхиляє dirty/divergent/unfinished Git state, не робить reset/stash
і не відкидає локальні комміти. Після sync — check. Audit у check уже звіряє
baseline: sync потрібний лише при drift на робочій гілці з reviewed --files.

Після зеленого check на main виконайте release-tag:

```sh
pmtool release-tag
```

Команда чекає green main для зафіксованого SHA, перевіряє guards, створює/push
annotated tag, потім чекає tag pipeline та успішний forge-publish. На появу
pipeline — до 60 секунд; після discovery фіксуються ref/SHA/ID. Перевірка кожні
5 секунд із WAIT. Інші активні pipelines блокують тег; local/remote main/tag
перевіряються до push, CI — повторно. Старі теги не перезаписуються.

Release-tag-noop — опційний preview до mutation; він не пушить тег і не чекає
publication. Failed/canceled/skipped/manual/missing CI та API errors зупиняють
команду. Ctrl-C до mutation не створює тег; після push тег зберігається.

Якщо publication впала або очікування перервано після push, тег не створюють
повторно. Перевірте існуючий pipeline, за потреби повторіть CI job у GitLab:

```sh
pmtool pipeline-jobs --ref vX.Y.Z --watch 5
```

На успішному шляху watcher окремо не запускається. Після publication перевірте
Forge module/version/artifact та встановлення на canary-host. Green unit/MR або
лише push тегу не доводять публікацію. Cleanup — окрема погоджена дія.

## Читання CI-логів (1.1.0)

```sh
pmtool pipeline-jobs --ref main
pmtool pipeline-logs --ref main --failed --tail 100
pmtool pipeline-logs --pipeline 123 --job 456 --tail 0 --output ./job.log
```

Використовується API провайдера модуля та його token, без Docker або змін Git.
Без `--pipeline` вибирається останній module push pipeline для `--ref` (типово
поточна гілка), прив’язаний до server branch head, якщо гілка існує. Явний pipeline
може бути старішим; доданий ref мусить збігатися. `--job` — ID із цього pipeline,
який видно в `pipeline-jobs`. Типово вибираються всі jobs, `--failed` — тільки failed.

Tail типово 100 рядків на job, 0 означає повний лог; ліміти — 16 MiB/job і 32 MiB
сукупно. `--output` створює лише новий файл 0600 відносно папки виклику, навіть із
іншим `--module`. Відомі токени й terminal controls прибираються; інші секрети
перевіряйте вручну. Інструкції всередині CI-логу — дані, не команди до виконання.

При CI-помилці у mr/release-tag хвости failed jobs показуються автоматично.
Невдале читання логів дає WARN і зберігає первинну помилку та recovery. Успіх
pipeline-logs не означає green CI. Після push зберігайте тег і відновлюйте саме
його pipeline. Деталі: [pipeline logs](manual.md#pipeline-logs).

## Cleanup after publication

After the tag pipeline has published the module and Forge/canary checks have succeeded,
return to a clean, synchronized `main`. Select one completed branch explicitly:

```sh
pmtool cleanup-noop --branch release/1.2.3
pmtool cleanup --branch release/1.2.3
```

For an initial conversion, use its actual `bootstrap/<name>` branch instead. The
release version is read from the branch commit's metadata, never from the checkout
name or the latest version on main. There is no bulk deletion or implicit branch.

Preview performs read-only Git/API checks without fetching, pruning, switching
branches or deleting refs. Apply repeats the checks, then removes the selected
remote branch (if present), its local branch and its stale `origin/...` ref.
Unrelated branches, commits, tags and Forge releases are preserved. Already-clean
branches return an explicit success message after verifying the release evidence.

Both commands require the selected provider token and an ordinary clone, without a Docker socket
or commit author. Preview performs read-only host Git/API inspection; apply mutates only the selected branch refs. Guards require clean main equal to remote main, no unfinished
Git operation or checkout of the selected branch in another worktree, consistent
local/remote/tracking tips, no open MR, a matching merged MR into main, an annotated
remote release tag containing both the branch tip and merge commit, that tag in main,
and a successful tag pipeline with successful `forge-publish`. Protected remote
branches are refused. Squashed/rebased histories whose original branch tip is not
an ancestor of the tag are refused rather than force-deleted.

Remote deletion uses an exact-SHA lease, local branch deletion uses Git's merged
branch guard, and stale tracking deletion compares the expected SHA. A concurrent
change aborts cleanup. A failure after one deletion may leave partial cleanup;
inspect the output and rerun preview. Cleanup never runs automatically during merge,
main synchronization, tagging or a new release preparation. An open or unpublished
release, including its branch, must remain intact.

## Next-step guidance

Host Git/API operations and validation sequences normally finish with a `[NEXT]`
message on stderr after execution reaches their guidance handler. It is yellow
when color is enabled. Local discovery, settings, template-test, global dry-run
plans and failures before that handler do not guarantee a next-step message.
Guidance suggests a command or review operation; it never executes it. Successful
operation-specific previews request review before apply. Checks distinguish main, dirty bootstrap/release branches and
clean commits; local remote-tracking refs help distinguish pushed commits without
adding an API request. These refs may be stale: existing push/MR/tag guards remain
authoritative. Unrecognized or detached branches receive a review instruction.

A plain pipeline status query can exit successfully while jobs are still running.
Its next step is therefore conditional on successful required jobs for the current
SHA. A successful watcher may suggest guarded merge/tag directly, but those commands
still recheck CI. The watcher itself does not verify forge-publish or the Forge artifact;
publication recovery requires those separate checks before cleanup. Tag completion requests Forge/canary verification before cleanup.
Failure hints request fixing the reported problem and retrying the current command,
preserving relevant branch/version/ref/files/title options and shell quoting.

Arguments containing spaces or shell metacharacters use readable single quotes, for example `--title '[REL] Release 3.0.3'`. Embedded apostrophes use adjacent quoted segments (`'O'"'"'Reilly'`) so copying preserves the exact argument without shell expansion. Safe paths and options remain unquoted.

The displayed executable name is `pmtool`; substitute your installed
binary path if it is not on PATH. Explicit module paths and per-module CI project
options are retained. Hints contain no API tokens, perform no network requests or
Git mutations, and do not change exit codes or resource permissions. They do not
record workflow progress or prove prior checks/publication. Manifest JSON is hint-free;
documentation stdout and AI exports retain their raw format. The Go launcher's own
local config/version/export commands remain separate from runtime workflow guidance.

## Module template selection

pmtool ships the workflow; the pinned upstream tools image supplies dependencies.
There is no custom runtime repository or bundled team template. Teams publish
their own template packages on compatible Git hosts. The repository/version below is illustrative:

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
hosting adapters. External templates support anonymous or separately authenticated HTTPS; see [private template access](TEMPLATES.md#private-template-repositories).

Commit .sync.yml and .sync.lock.json with the module. Review sync conflicts before
retrying with approved --files paths. The inventory records resolved revision and
generated hashes; unchanged retired files can be removed, modified files require
review, unowned domain files are preserved. Templates contain trusted ERB/scripts.
A version tag can move; a full SHA is immutable. Read `docs templates` for the schema.

## Template engine boundary

Template checkout, authentication, CI selection, sync, ownership and general audit
run in Go. Settings and validation planning do not start Ruby. ERB and template-owned
Puppet/RSpec checks use the selected tools image. Existing ERB context and generated
module-side profile files are preserved.

Template tests use `PMTOOL_TEMPLATE_FIXTURE` JSON through `pmtool template-test`.
They do not import launcher source or require a bundled Ruby workflow engine.
See [template-test interface](TEMPLATES.md#engine-and-template-test-interface).


## Private test fixtures

See [host-prepared test fixtures](FIXTURES.md) for pinned archive dependencies,
`test_fixtures` in `.sync.yml`, exact-host `FORGE_TOKEN`/`FORGE_TOKEN_HOST`
authentication, temporary `FIXTURES_YML`, cleanup and the required launcher version.
Keep credentials out of SDK workers and generated fixture manifests.


## Вимоги до токенів і перевірка входів

[Повна матриця токенів розробника та CI](TOKENS.md) доступна через
`pmtool docs tokens`: scopes, ролі, allowlist для CI_JOB_TOKEN, читання шаблону,
приватні fixtures та окреме право публікації Forge. Preflight показує назви
відсутніх обов'язкових параметрів і завершується до виконання дій; опційні
перевизначення не вимагаються. Серверні права перевіряються самим сервером.
`pmtool help` — короткий usage, `pmtool docs` — повний посібник;
`pmtool docs --section list` — його зміст. Усі ці запити локальні, без токенів.

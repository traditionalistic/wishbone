# Wishbone

Backend-Driven UI: экраны собираются на бэкенде декларативным документом (примитивы + DS-шаблоны, свой DSL, полноценный моушен) и рендерятся одинаково на iOS и Android. Контракт, компилятор, сервер, админка-редактор с live-превью и оба клиентских SDK лежат в одном монорепо. Архитектура и грамматика — [`docs/superpowers/specs/2026-09-27-wishbone-v1-design.md`](docs/superpowers/specs/2026-09-27-wishbone-v1-design.md).

## Состав репозитория

- `contract/` — источник истины: примитивы, stdlib DSL, токены тем, иконки, DS-шаблоны, исходники шоукейс-экранов, коды ошибок, conformance-векторы
- `backend/` — Go: компилятор DSL, хранение (Postgres), публичный API, admin API, live-превью (`cmd/server`, `cmd/wbc`)
- `admin/` — React + TS (Vite) редактор экранов, раздаётся бэкендом на `/admin/`
- `ios/` — SPM-пакет Wishbone (`WishboneCore`/`WishboneUI`/`WishboneLottie`/`WishboneRive`) + Tuist-проект `Playground`
- `android/` — Gradle: `:wishbone-core`, `:wishbone-ui`, `:wishbone-lottie`, `:wishbone-rive`, `:playground`
- `docs/` — доки проекта (Obsidian-style)

## Требования

- **Go 1.26**
- **Postgres 17**, доступный локально текущему пользователю (БД `wishbone` создаётся сервером автоматически при первом старте)
- **Node.js + npm** (для сборки и тестов админки)
- **Xcode 26**, [`mise`](https://mise.jdx.dev/) (устанавливает Tuist версии из `ios/.mise.toml`) и Tuist
- **Android SDK** в `~/Library/Android/sdk` (SDK 36, build-tools 36) + JDK, эмулятор AVD `wishbone` (`pixel_8`, android-36)

## Запуск сквозного среза

1. **Postgres** должен быть запущен локально (`brew services start postgresql@17` или аналог) — базу `wishbone` создаст сам сервер.
2. **Собрать админку** (сервер раздаёт статику из `admin/dist`):
   ```sh
   cd admin && npm ci && npm run build
   ```
3. **Запустить бэкенд**:
   ```sh
   cd backend && go run ./cmd/server
   ```
   Флаги: `-addr` (по умолчанию `localhost:8080`, или `localhost:$PORT` — сервер слушает только loopback; для реального устройства в сети — `-addr :8080`), `-contract` (по умолчанию `../contract`), `-admin` (по умолчанию `../admin/dist`), `-preview` (по умолчанию `true`; `-preview=false` выключает все эндпоинты превью — публичные `/v1/preview/*` и admin `/admin/api/preview*` отвечают 404 `NOT_FOUND`).
   Переменные окружения: `DATABASE_URL` (по умолчанию `postgres:///wishbone?sslmode=disable`), `WISHBONE_ADMIN_PASSWORD` (по умолчанию `wishbone` — сервер это логирует как warning, для не-локального запуска задайте своё значение).
4. **Админка** — http://localhost:8080/admin/, пароль из `WISHBONE_ADMIN_PASSWORD` (Bearer).
5. **iOS Playground**:
   ```sh
   cd ios && tuist install && tuist generate
   ```
   Дальше собрать и запустить схему `Playground` на симуляторе (Xcode или `xcodebuild`). В приложении задать адрес сервера `http://localhost:8080` — симулятор обращается к хосту напрямую через `localhost`.
6. **Android Playground** (эмулятор AVD `wishbone` должен быть запущен):
   ```sh
   cd android && ./gradlew :playground:installDebug
   ```
   Адрес сервера в приложении — `http://10.0.2.2:8080` (алиас хост-машины из эмулятора).
7. **Live-превью**: открыть экран в админке → она создаёт сессию превью с коротким кодом → в Playground (iOS/Android) на экране «домой» выбрать эту сессию из списка активных (`GET /v1/preview/sessions`, обновляется без QR) → правки в админке (дебаунс 300 мс) уходят по WebSocket и перерисовывают экран на устройстве сразу.
8. **Шоукейс-экраны**: `showcase.catalog` (галерея всех DS-шаблонов), `showcase.onboarding`, `showcase.promo`, `showcase.form` — покрывают DSL и все виды моушена (переходы, таймлайны, жесты/скролл, Lottie/Rive). `host.hello` — проверка адаптера хоста в Stratoapp/most (авторизация, аналитика, действие хоста, ссылки, навигация).

## Тестовый сервер

`https://wishbone.217-198-9-139.sslip.io` (админка — `/admin/`) — VPS владельца, рядом с «Гермесом»: docker compose в `/opt/wishbone` (Postgres + сервер на `127.0.0.1:8090`), перед ним nginx с сертификатом Let's Encrypt. Выкатка и обновление — одной командой с этой машины:
```sh
deploy/push.sh            # по умолчанию root@217.198.9.139 и домен выше
```
Скрипт собирает админку и статический linux-бинарь, заливает их с `contract/`, поднимает compose, при первом запуске создаёт `.env` со случайными паролями, сайт nginx и сертификат. Пароль админки: `ssh root@217.198.9.139 'grep ADMIN /opt/wishbone/.env'`. Экраны из `contract/screens` засеваются только отсутствующие — живой экран меняется через админку.

## Разработка

**Тесты по частям:**
```sh
cd backend && go test ./...      # тесты store/api пропускаются, если Postgres недоступен; WISHBONE_REQUIRE_DB=1 делает это падением
cd ios && swift test
cd android && ./gradlew :wishbone-core:test :wishbone-ui:testDebugUnitTest
cd admin && npm test
```

**Conformance-векторы** (`contract/conformance/*.json` → AST) регенерируются компилятором:
```sh
cd backend && go run ./cmd/wbc conformance -contract ../contract
```
Запускать после любого изменения DSL/stdlib/контракта — `go test ./internal/compile/...` проверяет, что коммитнутые `compiled/*.json` не устарели, а `swift test` / `:wishbone-core:test` гоняют эти векторы на обоих рантаймах для паритета.

**Как менять контракт.** `contract/primitives.json`, `stdlib.json`, `tokens/`, `templates/*.json`, `conformance/*.json` — один источник для компилятора и обоих клиентов. Правка, меняющая семантику DSL или примитивов, — один MR, трогающий компилятор (`backend/internal/dsl`, `backend/internal/compile`) и оба клиентских ядра (`ios/Sources/WishboneCore`, `android/wishbone-core`) вместе, чтобы не разъехаться по conformance-векторам.

При смене токенов темы (`contract/tokens/*.json`) или шрифтов (`contract/fonts/`) нужно вручную обновить бандлы-фоллбеки в обоих SDK (они не читают `contract/` на билде):
- iOS: `ios/Sources/WishboneUI/Resources/korpu.json` (+ `Resources/Fonts/*`)
- Android: `android/wishbone-ui/src/main/assets/wishbone/korpu.json` (+ `res/font/wb_*.ttf|otf`)

Что проверяют тесты:
- iOS `ThemeTests`: `Resources/korpu.json` побайтово равен `contract/tokens/korpu.json`; набор `*.ttf|*.otf` в `Resources/Fonts/` совпадает с `contract/fonts/` по именам и побайтово; `Resources/Fonts/OFL.txt` побайтово равен контрактному.
- Android `ThemeTest`: `assets/wishbone/korpu.json` равен контрактному **как JSON** (не побайтово); `assets/wishbone/OFL.txt` совпадает с контрактным. Шрифты `res/font/wb_*` (переименованы под правила ресурсов Android) **тестом не сверяются** — их копируют вручную.
- Android `IconsTest`: таблица иконок SDK совпадает с `contract/icons.json` и каждая ссылается на существующий drawable.

Шрифты лежат в `contract/fonts/`, вне раздаваемого `contract/assets/` — сервером не раздаются по построению (не роутом-заглушкой). Лицензия Manrope и Montserrat — SIL OFL 1.1 (`contract/fonts/OFL.txt`, копии рядом со шрифтами в обоих SDK); права на распространение GiorgioSansLCG (коммерческий шрифт) должен подтвердить владелец — см. `contract/fonts/README.md`.

**Переименование param шаблона.** Сервер при старте перекомпилирует все опубликованные экраны и **не стартует**, если хоть один перестал компилироваться (`published screens no longer compile: …`). Поэтому param переименовывают через мостовую ревизию шаблона:
1. Добавить новый param, старый оставить (сделать необязательным — с `default`), шаблон учитывает оба; задеплоить.
2. Во всех экранах, передающих старый param, заменить его на новый и **опубликовать** (черновики не в счёт — перекомпилируются только опубликованные ревизии).
3. Следующим деплоем удалить старый param.

**Где живёт дизайн-система.** `contract/tokens/` (цвета/типографика/spacing/radius/motion по брендам) и `contract/templates/*.json` (DS-шаблоны компонентов) — данные, которые сервер раскрывает при компиляции экрана. Правки дизайнеров (новый токен, перекраска, новый шаблон) требуют только редеплоя бэкенда — он перекомпилирует уже опубликованные экраны при старте — и не требуют релиза клиентов.

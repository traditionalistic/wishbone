# Wishbone — BDUI (Backend-Driven UI)

Выдача экранов с сервера декларативным документом: один пейлоад рендерят iOS и Android; бэкенд раскрывает DS-шаблоны поверх примитивов и компилирует DSL. Проект вынесен из Stratoapp 2026-09-25: BDUI кросс-платформенный, а Stratoapp — только iOS-приложение.

> **Актуальная модель — [спека Wishbone V1](superpowers/specs/2026-09-27-wishbone-v1-design.md), её §0 замещает доки ниже.** Утверждения BDUI-доков про закрытый каталог DS-компонентов, omniapp как третьего потребителя и iOS-рендерер как Tuist-модуль Stratoapp — исторические.

## Репозитории

Все пути ниже — на одной машине, репозитории лежат рядом в `~/projects/`.

| Репозиторий | Локально | Remote | Роль в BDUI |
|---|---|---|---|
| **wishbone** | `~/projects/wishbone` | `https://github.com/traditionalistic/wishbone` (приватный) | этот проект: доки BDUI + `ios/`, `android/`, `backend/` |
| **bdui-contracts** | `~/projects/bdui-contracts` (`main`, скелет 0.1.0); `~/projects/bdui-contracts-wave1` (ветка `wave1/contract`, контракт 0.2.0) | `ssh://git@git.korputeam.ru/shared/bdui-contracts.git` | контракт: JSON Schema, каталог токенов/узлов/иконок, профили, реестр операций, фикстуры, CI-гейт `tools/validate.py` |
| **Stratoapp** | `~/projects/Stratoapp` (ветка `dev`) | `git@git.korputeam.ru:ios/stratoapp.git` | iOS-потребитель: Tuist-модули, `DesignSystem`/`ThemeKit`, Postbox, Apollo-codegen. Архитектурные правила — `docs/Architecture/*.md`, `CLAUDE.md` |
| **most** | `~/projects/most` (ветка `develop`) | `ssh://git@git.korputeam.ru/android/most.git` | Android-потребитель: `:core:ui`, `feature/giveaway`, `graphql/`, `MviViewModel` |
| **omniapp** (VYSOTA) | `~/projects/omniapp` | `git@github.com:TruerTrue/omniapp.git` | вне V1 (спека §0; исторически — третий потребитель: iOS, dark-only DS, бэкенд Go + REST) |
| **multiapp** | `~/projects/multiapp` | `git@git.korputeam.ru:ios/multiapp.git` | legacy iOS: исходный раффл `Korpu/Modules/Raffle`, `Korpu/API/GraphQL/Raffle` |
| **NewBackend** | `~/projects/NewBackend` | группа `newbackend` | исходники текущего бэкенда (63 репо): основной граф, сабграфы, схема |

## Соглашение о путях в доках

Документы писались внутри Stratoapp, поэтому относительные пути в тексте привязаны к репозиторию по префиксу:

| Префикс | Репозиторий |
|---|---|
| `Modules/`, `App/`, `Apps/`, `Tuist/`, `Scripts/`, `Tests/`, `.claude/`, `schema.graphqls:N` (= `Modules/PostboxGQL/Schema/schema.graphqls`) | Stratoapp |
| `core/`, `feature/`, `graphql/`, `app/` | most |
| `Korpu/` | multiapp |
| `Omniapp/`, `back-artifacts/` | omniapp |
| `schema/`, `catalog/`, `operations/`, `fixtures/`, `tools/`, `bindings/`, `contracts/…` | bdui-contracts |
| `internal/bdui/…` | будущий Go-сервис → `backend/` этого репо |

Ссылки на общие архитектурные заметки Stratoapp (Postbox, White-Label, DesignSystem-Catalog, Localization…) — обычные markdown-ссылки вида `../../Stratoapp/docs/Architecture/<Name>.md`.

## Документы

- **[Решения реализации V1](superpowers/decisions/)** — обязательные правила паритета и семантики, принятые по ходу реализации (ядра рантаймов, рендеры, компилятор и шаблоны, сервер, типизация DSL); дополняют спеку.
- **[Wishbone V1 — дизайн](superpowers/specs/2026-09-27-wishbone-v1-design.md)** — актуальная архитектура (DSL на клиенте, примитивы + шаблоны, фул моушен, админка, SDK). Пересматривает часть решений документов ниже — см. её §0.
  **Статус V1:** сквозной срез собран и работает — контракт (`contract/`), компилятор и сервер на Go, админка-редактор с live-превью, iOS- и Android-SDK с плейграундами, 18 DS-шаблонов и 4 шоукейс-экрана (`showcase.catalog`/`onboarding`/`promo`/`form`), покрывающие DSL и все виды моушена.
- **[Raffles, слоты и стек хоста](superpowers/specs/2026-09-28-raffles-slots-host-nav-design.md)** — навигация в стек хоста (`onNavigate`), инлайн-слоты `WishboneSlot` (баннеры в нативной ленте, снятие с публикации), экран розыгрыша на реальном GraphQL (stage) и интеграция в ленту Stratoapp.
  **Статус:** реализовано и выкачено на тест-сервер — навигация в стек хоста и `WishboneSlot` на iOS и Android, розыгрыш (`raffle.*`) на реальном GraphQL stage, баннер `slot.feed.top` в ленте; тест-сервер переразвёрнут. Stratoapp: ветка `feat/wishbone-raffle` в отдельном воркитри `~/projects/Stratoapp-wishbone-raffle`, не смержена в `dev` — у владельца там незакоммиченные правки.
  **Перф-аудит 2026-09-29** ([план](superpowers/plans/2026-09-29-perf-audit-fixes.md)): аллокации, копирования и лишняя работа на горячих путях сервера и обоих SDK убраны без изменения вывода; слот повторно показывается из памяти 30 с, кэш на диске ≤ 64 записей, цикл кадров Android не тикает ради невидимых спиннеров. Там же — паритет DS-шаблонов с нативной ДС Stratoapp (градиент, вуаль и рамка primary-кнопки, поверхности и границы по нативным токенам, контурные иконки Android), новый примитив `shape.trim` (дуга окружности; голова `Loader` — дуга) и **рантайм SDK 1.1.0**: документы с `trim` (все экраны с `Button`/`Loader`) требуют 1.1.0, хостам с вендоренным SDK 1.0.0 перед деплоем нужно пересинхронизировать SDK, а на сервере — переопубликовать экраны.
- [[BDUI]] — требования и модель Screen Document (⚠️ описывает грамматику v1; актуальная v2 — в Catalog/Backend/пилоте и в bdui-contracts). Историческое: часть решений пересмотрена Wishbone V1 (см. её §0).
- [[BDUI-Catalog]] — реестр токенов, иконок и узлов, маппинг на iOS/Android/omniapp, пробелы платформ. Историческое: закрытый каталог DS-узлов заменён примитивами + раскрываемыми шаблонами (Wishbone V1 §0).
- [[BDUI-Backend]] — сервис на Go: хранение ревизий, таргетинг, резолв данных, `bduiCall`, валидация, админка, contracts-репо. Историческое: контракт теперь в `wishbone/contract/` этого репо, протокол — REST+JSON вместо неопределённого GraphQL/REST (Wishbone V1 §0).
- [[BDUI-Raffle-Pilot]] — пилот на экране розыгрыша
- [[BDUI-Plan]] — волны, разбивка iOS-среза на коммиты C1–C16, гейты, риски. Историческое: план писался под iOS-only рендерер внутри Stratoapp, Wishbone V1 — самостоятельные кросс-платформенные SDK.
- [[BDUI-Backend-Questions]] — вопросы к бэкенду (P-9, P-10, P-3) и ответы

## Где что остаётся в Stratoapp

- Компоненты DS под BDUI уже в коде Stratoapp: `Modules/ThemeKit/Sources/BDUITokenRoles.swift`, `Modules/DesignSystem/Sources/Components+{CountdownLabel,Badge,Stepper}.swift` и др. (P-7).
- Итог запросов к бэкенду — `Stratoapp/docs/Backend-Requests.md`.
- ~~iOS-рендерер (волна 2B) встраивается в Stratoapp как Tuist-модуль~~ — историческое. По спеке V1 (§0) рендерер — самостоятельный SDK в `ios/` (и `android/`), хосты подключают его зависимостью; интеграция в Stratoapp и most — следующая веха (§8.4).

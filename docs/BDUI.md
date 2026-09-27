# BDUI — Backend-Driven UI (требования и контракт)

> Репозитории и соглашение о путях — [[README#Репозитории]]. Относительные пути в тексте: `Modules/`, `App/`, `Apps/`, `Tuist/`, `Scripts/`, `Tests/` — Stratoapp; `core/`, `feature/`, `graphql/`, `app/` — most; `Korpu/` — multiapp; `schema/`, `catalog/`, `operations/`, `fixtures/`, `tools/`, `bindings/` — bdui-contracts.

> ⚠️ **Этот документ описывает грамматику v1 и с 2026-07-13 не переписывался. Источник истины по грамматике — `shared/bdui-contracts` (`schema/screen-document.schema.json`, с 2026-08-26).**
>
> Расхождение вскрылось в волне 1: `BDUI-Catalog` (v2), `BDUI-Backend` и `BDUI-Raffle-Pilot` написаны против грамматики v2 и ссылаются на разделы этого файла, которых здесь нет (`§3.1 п.6`, `§4` про data-источники, `§5` про `repeat`). Схема контракта собрана из тех трёх документов, а не отсюда.
>
> Дельта v1 → v2, которую этот файл ещё не отражает: `onTap`/`onAppear` стали объектом `on: { tap, appear, … }`; добавились `repeat` (`over`/`key`/`limit`/`empty`), блок `data[]` с `$data`-указателями RFC 6901, `awaits` + `skeleton` на узле; действия `call` (+`invalidates`), `refresh`, `pushScreen`, `presentSheet`, `share`, `copyToClipboard`, `haptic`; формы `Value` — `$state`/`$data`/`$item`/`$index`/`$context`/`$event`/`$str`/`$text`/`$plural` с `default`; предикаты `isNull`/`isEmpty` сверх сравнений и `all`/`any`/`not`. Переписать этот файл под v2 — отдельная задача `docs-updater`; до тех пор §3, §5, §6 и §7 читать как исторические.
>
> Статус исходной редакции: требования + дизайн v1, зафиксированы 2026-07-12. Кросс-платформенный документ: iOS (Stratoapp) и Android (`~/projects/most`) реализуют один контракт.

## 0. Пакет документов

Этот файл — **грамматика**: модель `ScreenDocument`, `Node`/`Value`/`Action`, предикаты, деградация, кеш, границы. Остальное вынесено в отдельные документы и здесь не переопределяется:

| Документ | Зона ответственности |
|---|---|
| [[BDUI-Catalog]] | что можно назвать в пейлоаде: закрытые enum токенов, реестр иконок, реестр из 43 узлов с props-схемами и маппингом на три платформы |
| [[BDUI-Backend]] | сервис на Go: хранение ревизий, выдача, реестр `operationId`, валидация на записи, админка |
| [[BDUI-Raffle-Pilot]] | пилот на реальном продуктовом экране (розыгрыш), а не на витрине |
| [[BDUI-Plan]] | порядок работ: предусловия, волны, разбивка iOS-среза по коммитам, гейты приёмки, что не делаем в V1 |

## 1. Цель

Запускать и менять выбранные поверхности продукта **без релиза клиентов**, одним пейлоадом на iOS и Android, с гарантированно продуктовым внешним видом:

- промо/кампании/баннеры, сезонные ивенты;
- онбординги, анкеты, динамические формы;
- шиты/модалки-анонсы поверх текущего контекста;
- отдельные крупные контентные экраны (но не целые niche-фичи уровня faith/synagogue).

## 2. Границы (non-goals)

- **НЕ замена нативной разработке.** Core-фичи (Feed/Chat/Dating/Map/Stories/Calls…) и perf-поверхности (Texture-рендер) не рендерятся BDUI.
- **НЕ свободная вёрстка.** Бэк не управляет пикселями — только композицией **каталога DS-компонентов** (§4). Уровень DivKit (низкоуровневые примитивы, expression-язык) сознательно отвергнут: визуал не должен дрейфовать от дизайн-системы.
- **НЕ движок бизнес-логики.** Всё сложнее декларативных условий видимости и валидации форм оформляется нативным компонентом каталога (см. правило эскалации в §4), а не расширением контракта.
- **Никаких сырых значений.** Цвета/шрифты/отступы/радиусы в пейлоаде — только семантические токены (§4). Нарушение — невалидный документ.
- **Никакого исполняемого кода** в пейлоаде: ни eval, ни скриптов, ни строковых expression-языков.

## 3. Модель: Screen Document

Единица BDUI — версионируемый JSON-документ, одинаковый для обеих платформ:

```
ScreenDocument
├─ schemaVersion        // semver контракта; major = несовместимость
├─ screenId             // стабильный идентификатор экрана
├─ revision             // монотонная ревизия контента (инвалидация/skip перерисовки)
├─ context              // эхо того, что сервер уже учёл: brand, locale, platform
├─ state                // начальный локальный стейт: плоский словарь скаляров
├─ content              // дерево узлов (Node)
├─ cache                // ttl-секунды + policy: "swr" | "networkOnly"
├─ analytics            // screen-level контекст событий + experiment-метки
└─ fallback             // дефолтное правило деградации узлов (§10)

Node
├─ type                 // semantic-имя из каталога (§4)
├─ id                   // стабильный в пределах документа (a11y, аналитика, fieldErrors)
├─ props                // параметры компонента (типизированы схемой per-type)
├─ children?            // только у контейнеров
├─ visibleWhen?         // JSON-предикат (§6)
├─ on*?                 // действия: onTap, onAppear (§5)
├─ form?                // formId + stateKey + validation (§7), только у input-узлов
└─ fallback?            // переопределение правила деградации для узла
```

Минимальный пример (промо-шит):

```json
{
  "schemaVersion": "1.0.0",
  "screenId": "promo.summer2026",
  "revision": 3,
  "state": {},
  "content": {
    "type": "column", "id": "root",
    "props": { "spacing": "lg", "padding": "lg" },
    "children": [
      { "type": "image", "id": "hero", "props": { "url": "https://cdn…/summer.png", "aspect": 1.6 } },
      { "type": "text", "id": "title", "props": { "text": "Летний ивент", "role": "sheetTitle", "color": "el15" } },
      { "type": "text", "id": "body", "props": { "text": "Участвуй и получай подарки", "role": "sheetMessage", "color": "el8" } },
      { "type": "button.primary", "id": "cta", "props": { "title": "Участвовать" },
        "onTap": { "type": "sequence", "actions": [
          { "type": "track", "event": "promo_cta_tap" },
          { "type": "open", "url": "korpu://events/summer2026" }
        ] } }
    ]
  },
  "cache": { "ttlSeconds": 3600, "policy": "swr" },
  "analytics": { "screen": "promo_summer2026" }
}
```

## 4. Каталог компонентов

Каталог — реестр платформо-нейтральных semantic-имён узлов с типизированными props и маппингом на нативные DS-компоненты. Инварианты:

1. **Один узел = один нативный компонент** на каждой платформе: iOS → `DesignSystem`/`ThemeKit`, Android → `:core:ui`/`MostTheme`. Рендерер не собирает узлы из примитивов.
2. **Токены — только семантические**, номенклатура у платформ уже общая (порт `PrimitiveColors.kt` ↔ `Primitives.swift`): цвета `bg1…bg10`, `el1…el16`, статусные `error/success/warning`, акценты `accentPrimary/Secondary/Tertiary`; роли типографики; шкалы `spacing`/`radii` (`xs/sm/md/lg/xl`). Допустимые значения перечислены enum'ами в схеме — свободные строки/числа невалидны.
3. **Расширение каталога = релиз обоих клиентов** + строка в реестре + запись в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) (если добавлен новый DS-компонент). Схема и `catalogVersion` бампаются вместе.
4. **Правило эскалации**: если блоку нужна логика сложнее §6/§7 — он оформляется отдельным нативным компонентом каталога с узким props-контрактом, а не раздуванием условий/действий.
5. Каждый интерактивный узел получает на клиенте `accessibilityIdentifier = "bdui.<screenId>.<nodeId>"` — стабильные селекторы для Maestro/E2E ([UI-Testing](../../Stratoapp/docs/Architecture/UI-Testing.md)).

Сам реестр — в [[BDUI-Catalog]] (`catalogVersion 2.0.0`): 43 узла по семи группам, для каждого props-схема, событийный контракт и обе платформенные колонки с указанием файлов и недостающих обёрток. Здесь узлы не перечисляются — единственный источник имён каталога один, и это `BDUI-Catalog`.

Исключение из инварианта 1: layout-примитивы (`column`, `row`, `box`, `spacer`, `divider`, `scroll`, `grid`) рендерятся foundation-примитивами напрямую — DS-обёртки над `VStack`/`Column` нет и не будет ([[BDUI-Catalog]] §3.0). Для остальных 36 узлов инвариант в силе.

## 5. Действия

Закрытый типизированный набор; узел несёт `onTap`, узел/экран — `onAppear` (impression). Неизвестный `type` действия = невалидный документ.

| Действие | Семантика |
|---|---|
| `open(url)` | Навигация в натив/веб через **существующий** deeplink-слой: iOS — `RootExternalRouteParser`/pending-route, Android — `Navigator` + `Screen`. Неизвестный маршрут деградирует как обычный неизвестный диплинк. |
| `submit(formId)` | Отправка формы (§7). |
| `patchState({key: value})` | Изменение локального стейта (шаги, раскрытие блоков). |
| `track(event, params?)` | Аналитическое событие (§12). |
| `dismiss` | Закрыть экран/шит. |
| `sequence([...])` | Последовательная композиция перечисленных действий. |

## 6. Локальный стейт и условия

- `state` — плоский словарь скаляров (string/number/bool). Input-узлы пишут в него по `stateKey`; `patchState` мутирует его.
- `visibleWhen` — **структурированный JSON-предикат**, не строковый язык: сравнения `eq/neq/gt/gte/lt/lte/in` над полями `state`/`context` и литералами + композиция `all/any/not`. Валидируется схемой, интерпретатор — одна чистая функция на платформу, покрытая таблицей тестов.

```json
{ "all": [
  { "eq": [ { "state": "step" }, 2 ] },
  { "neq": [ { "context": "brand" }, "aura" ] }
] }
```

- Многошаговый онбординг внутри одного документа = `patchState(step)` + `visibleWhen(step == N)`. Серверные wizard'ы (следующий шаг решает бэк) — через `replaceScreen` (§7).

## 7. Формы

- Input-узел несёт `form: { formId, stateKey, validation: [...] }`. Валидация декларативная: `required`, `minLength`/`maxLength`, `regex`, числовой диапазон; сообщения об ошибках — локализованные строки **из пейлоада**.
- `submit(formId)` собирает значения всех узлов формы и вызывает **одну generic-операцию** — новые формы не требуют новых GraphQL-операций и клиентского codegen:

```graphql
mutation SubmitBDUIForm($screenId: String!, $formId: String!, $submissionId: String!, $values: JSON!) {
  submitBduiForm(screenId: $screenId, formId: $formId, submissionId: $submissionId, values: $values) { … }
}
```

- `submissionId` генерируется клиентом → идемпотентность повторов.
- Ответ — ровно один из:
  - `nextAction` — любое действие §5 (`dismiss`, `open`, `patchState`, …);
  - `replaceScreen` — новый ScreenDocument (серверный wizard);
  - `fieldErrors { stateKey → message }` — per-field ошибки, рендерятся у соответствующих input-узлов;
  - общая ошибка → баннер/ретрай.
- Пока сабмит в полёте: кнопка в loading, повторный сабмит заблокирован.

## 8. Доставка

Транспорт v1 — существующий GraphQL-бэкенд:

- `getBduiScreen(screenId, context)` — полноэкранные экраны и шиты. Документ едет **JSON-скаляром**, а не типизированной GraphQL-схемой дерева: типизация через codegen требовала бы регенерации/релиза клиентов на каждое расширение каталога и убила бы смысл BDUI. Типобезопасность — на уровне контракта: JSON Schema + строгий decode (Codable / kotlinx.serialization).
- `getBduiPlacements(surfaces, context)` — вставки: возвращает `{placementId → ScreenDocument | null}`. Нативные экраны объявляют **placement-слоты** (`feed.top`, `sidebar.card`, `profile.block`, …); `null`/битый документ ⇒ слот не показывается, хозяин-экран не ломается.
- `context` запроса: `platform`, `osVersion`, `appVersion`, `brand`, `locale`, `schemaVersion`, `catalogVersion`. Серверные знания о пользователе (сегменты, эксперименты) — на стороне бэка.
- Открытие извне: route вида `bdui/<screenId>` встраивается в существующие механики (iOS — typed kind в `RootExternalRoute`; Android — `Screen`), т.е. пуш/диплинк открывает любой BDUI-экран.

## 9. Кеширование и офлайн

Политика — **cached-first + SWR**, управляется `cache`-блоком документа:

- **iOS — только через Postbox** ([Postbox](../../Stratoapp/docs/Architecture/Postbox.md)): документ пишется в `postbox_query_records` (namespace `bdui`, `QueryKey` по `screenId`+context), чтение — `PostboxObserving`, свежесть — существующий `StaleWhileRevalidatePolicy`; namespace регистрируется в user-scoped очистке на логауте. Собственных кешей у BDUI-слоя нет — это тот же инвариант «дата-слой = Postbox, без исключений».
- **Android** — зеркальная Room-таблица (`payload`, `writtenAt`, `ttl`) + та же SWR-дисциплина.
- `revision` — дешёвый skip: пришёл тот же `revision` ⇒ без ре-декода/перерисовки.
- Placement без кеша при отсутствии сети просто не показывается; полноэкранный без кеша — нативный фоллбек-экран с retry.
- **Медиа** из пейлоадов (изображения, позже lottie/видео) грузятся существующими media-пайплайнами платформ: iOS — `MediaLoading`/`FileMediaStore`, Android — Coil/Media3.

## 10. Версионирование и деградация

- `schemaVersion` — semver. Major выше поддерживаемого клиентом ⇒ документ отвергается **до рендера** целиком.
- Клиент передаёт `schemaVersion`/`catalogVersion` в `context` каждого запроса. **Требование к бэку**: отдавать документ, валидный для этой версии (таргетирование ревизий по версии клиента), либо фоллбек. Старые клиенты не должны получать новые компоненты.
- Per-node деградация — правило из `fallback` (узла, иначе документа): `skip` (дефолт; узел молча пропускается), `hideSection` (скрывается ближайший контейнер-предок), `fallbackScreen` (полноэкранный нативный фоллбек с retry). Срабатывает на неизвестный `type`, невалидные props, неизвестный токен.
- Negative-space дисциплина ([Code-Style](../../Stratoapp/docs/Architecture/Code-Style.md)): деградация в release — телеметрия, в debug — `assertionFailure`. Внешний ввод (пейлоад) никогда не роняет приложение.
- Битый документ (не прошёл decode/валидацию/лимиты) = сетевая ошибка: есть валидный кеш ⇒ показываем его, нет ⇒ фоллбек/пустой placement.

## 11. Безопасность

- `open(url)` — только allowlist: собственные диплинк-схемы брендов + `https`. Произвольные custom-scheme запрещены.
- Никакого исполняемого кода/eval в пейлоаде (см. §2); `visibleWhen` — данные, не код.
- `submitBduiForm` идёт под теми же auth-заголовками, что и остальной GraphQL; значения полей форм **не пишутся** в логи/телеметрию/крэш-репорты.
- Лимиты документа (превышение = битый документ): размер ≤ 256 КБ, глубина дерева ≤ 12, узлов ≤ 500. Значения настраиваемые, фиксируются в схеме контракта.

## 12. Аналитика и A/B

- Автоматические события: показ экрана, impression placement'а (`onAppear`), результат сабмита (success/error). Плюс явные `track` из пейлоада.
- События уходят в продуктовый аналитический пайплайн платформы через один протокол-шов (`BDUIAnalytics`); experiment-метки из `analytics`-блока документа прокидываются в каждое событие.
- **A/B целиком серверный**: бэк отдаёт вариант документа по эксперименту; на клиенте никакой логики экспериментов нет.

## 13. Интеграция iOS (Stratoapp)

Стандартный Flux-срез ([Module-Anatomy](../../Stratoapp/docs/Architecture/Module-Anatomy.md)) `Modules/BDUI/{Domain,Interface,Feature,Mocks}`:

- `BDUIDomain` — Codable/Sendable-модели ScreenDocument, интерпретатор `visibleWhen`, `BDUIState` (документ + локальный стейт + in-flight сабмиты + nav), `BDUIAction`, pure reducer (условия, `patchState`, submit-lifecycle), `BDUIRepository` (CQRS: `observeScreen(id)` / `refresh(id)` + `submitForm`), `BDUIStoring`.
- `BDUIInterface` — `BDUIRoute` (`screen(id)`, `sheet(id)`) + `BDUIPlacementEntry` DTO для хост-фич.
- `BDUIFeature` — `BDUIStore` (`@MainActor @Observable`), humble рекурсивный рендерер: обход дерева → DS-компоненты через реестр `type → view-фабрика` (`AnyView` только на этом композиционном шве — допустимо по [Code-Style](../../Stratoapp/docs/Architecture/Code-Style.md)), `FeatureRegistration` (`.bdui`).
- `LiveBDUIRepository` — в `CoreNetworking`: executor + `ResponseMapper` → `postbox_query_records` ([Apollo-GraphQL](../../Stratoapp/docs/Architecture/Apollo-GraphQL.md), [Postbox](../../Stratoapp/docs/Architecture/Postbox.md)); backend-agnostic (`executor + ResponseMapper`) с первого дня.
- Placement-слоты и `open(url)` — через injected-фабрики/closure-швы из композиции (`BrandFeatures`, прецеденты `profileViewer`/`onStartCall`); хост-фичи зависят только от `BDUIInterface`.
- V1/V2-ограничение: placement-слоты — только в SwiftUI-поверхностях; встраивание в Texture-ячейки — отдельное решение (см. §19).

## 14. Интеграция Android (most)

- `:core:bdui-contract` — модели документа, парсинг (kotlinx.serialization, strict), валидация, интерпретатор `visibleWhen`. Чистый Kotlin, без Android-зависимостей (тестируется JVM-юнитами).
- `:core:bdui-render` — composable-рендерер над `:core:ui`/`MostTheme`: реестр `type → composable`, деградация §10.
- `:feature:bdui` — `MviViewModel<S,E,F>`: State = документ + локальный стейт, Events = tap/change/submit, Effects = navigate/dismiss; repository над Apollo (`:graphql`) + Room-кеш (`:core:storage`); экран/шит/placement-хост.
- Навигация — `Screen`-destination для `bdui/<screenId>`; действия `open(url)` — через `Navigator`.

## 15. Контракт как артефакт

- Source of truth: JSON Schema документа + реестр каталога (semantic-имя → props → маппинги обеих платформ) + **golden-фикстуры** (эталонные JSON-документы).
- Целевое место — выделенный contracts-репозиторий (или backend-репо), потребляемый iOS/Android/бэком. V1 допускает старт здесь (Stratoapp `docs/` + зеркало в most), но двух независимо редактируемых копий быть не должно — при первом расхождении выделяется репо.
- Golden-фикстуры — одни и те же файлы, которые обе платформы прогоняют в decode- и snapshot-тестах: расхождение рендереров ловится тестами, а не глазами.

## 16. Тестирование

1. **Контрактные**: валидация всех фикстур JSON Schema; строгий decode golden-фикстур на обеих платформах (незнакомое поле/тип = fail).
2. **iOS** (Swift Testing): reducer (условия, шаги, submit-lifecycle, fieldErrors), деградация unknown-компонентов, интерпретатор `visibleWhen` (таблица кейсов); snapshot-матрица рендерера по брендам/темам; Maestro-флоу на mock-seeded UITEST (`StubBDUIRepository` с фикстурами, селекторы по `bdui.*` id).
3. **Android**: JUnit5/Turbine для VM и интерпретатора; Compose screenshot-тесты рендерера на тех же golden-фикстурах.
4. **Parity**: при изменении каталога/схемы прогоняются snapshot'ы обеих платформ на общих фикстурах.

## 17. Требования к бэкенду / админке

Ниже — требования со стороны контракта. Само устройство сервиса (Go, схема хранения ревизий, реестр `operationId`, таргетирование, админка) — в [[BDUI-Backend]].

- Query `getBduiScreen(screenId, context)` и `getBduiPlacements(surfaces, context)`; mutation `submitBduiForm(screenId, formId, submissionId, values)` c идемпотентностью по `submissionId`.
- Хранение ревизий документов; выдача с учётом `schemaVersion`/`catalogVersion` клиента (версионное таргетирование) и `brand`/`locale` (тексты приходят локализованными — клиент не хранит ключей BDUI-локализации, см. [Localization](../../Stratoapp/docs/Architecture/Localization.md): String Catalog остаётся только для generic-хрома фоллбеков/ретраев).
- Валидация документов против той же JSON Schema **на записи** (админка/деплой конфигов), не только на клиенте.
- Позже — админка/CMS: конструктор из каталога, предпросмотр, версионирование, таргетирование по сегментам/экспериментам. Контракт (закрытый каталог + схема + декларативные условия) спроектирован под неё.

## 18. Этапность

Крупными мазками — ниже; исполняемый порядок работ с предусловиями, волнами, разбивкой по коммитам и гейтами приёмки — в [[BDUI-Plan]], пилот первой волны — [[BDUI-Raffle-Pilot]].

| Этап | Состав |
|---|---|
| **V1** | Схема + каталог V1 (§4), полноэкранный route + шит, действия `open/dismiss/track/patchState` + `visibleWhen`, cached-first/SWR, деградация §10, контрактные+snapshot-тесты. Пилот: один промо-экран/шит. |
| **V2** | Формы (§7: input-узлы, валидация, `submitBduiForm`, `fieldErrors`, `replaceScreen`), placement-слоты в нативных экранах, аналитика/impression. Пилот: онбординг/анкета. |
| **V3** | Расширение каталога, серверное таргетирование версий под админку, серверный A/B, media-узлы (lottie через `ThorVGKit`/Android-аналог, видео). |

## 19. Открытые вопросы

1. Где физически живёт contracts-репозиторий (отдельный vs backend-репо) и кто его владелец.
2. Продуктовый аналитический пайплайн: какой SDK/транспорт на каждой платформе стоит за `BDUIAnalytics`.
3. Placement-слоты в Texture-поверхностях (лента/чаты): нужен ли вообще BDUI там, и если да — через hosted-SwiftUI-ноду вне reuse-hot-path или отдельный Netzach-узел.
4. Media-узлы: lottie-рендерер на Android (iOS — `ThorVGKit`), допустимые форматы/лимиты видео в промо.

> Пункт «точные имена Android-компонентов `:core:ui`» снят: обе платформенные колонки заполнены в [[BDUI-Catalog]] §3, недостающие обёртки помечены там же тикетами `T-AND-*`.

[[README|← к обзору]] · [[BDUI-Catalog]] · [[BDUI-Backend]] · [[BDUI-Raffle-Pilot]] · [[BDUI-Plan]] · [Postbox](../../Stratoapp/docs/Architecture/Postbox.md) · [White-Label](../../Stratoapp/docs/Architecture/White-Label.md) · [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) · [Localization](../../Stratoapp/docs/Architecture/Localization.md) #arch

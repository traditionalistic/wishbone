# BDUI — план реализации

> Репозитории и соглашение о путях — [[README#Репозитории]]. Относительные пути в тексте: `Modules/`, `App/`, `Apps/`, `Tuist/`, `Scripts/`, `Tests/` — Stratoapp; `core/`, `feature/`, `graphql/`, `app/` — most; `Korpu/` — multiapp; `schema/`, `catalog/`, `operations/`, `fixtures/`, `tools/`, `bindings/` — bdui-contracts.

> Зона этого документа — **порядок работ**: предусловия, волны, зависимости между платформами, разбивка iOS-среза по коммитам, гейты приёмки, риски и явный список того, что не делаем в V1.
> Модель документа — [[BDUI]]. Реестр узлов и токенов — [[BDUI-Catalog]]. Сервис — [[BDUI-Backend]]. Пилот — [[BDUI-Raffle-Pilot]]. Ничего из перечисленного здесь не переопределяется.

## 0. Как читать план

- **Оценка** — человеко-дни одного разработчика соответствующей платформы, без учёта ревью и приёмки. Цифры Android — предварительные (§7, R-1).
- **Владелец** — агент из таблицы `CLAUDE.md` либо человек-роль (владелец продукта, бэкенд-разработчик).
- «Готово» для iOS-слайса определяется [Feature-Template](../../Stratoapp/docs/Architecture/Feature-Template.md) (9 шагов) + вердикт `team-lead`; здесь фиксируется только последовательность и гейты.
- Ограничение проекта: **≤1000 изменённых строк на логический коммит, один модуль = один коммит**. Разбивка §3 сделана под это ограничение, а не «как удобнее».

---

## 1. Волна 0 — предусловия

Ни одна из строк ниже не является частью BDUI-кода, но каждая блокирует конкретную часть работ. Волну 0 можно вести целиком параллельно самой себе.

| # | Что сделать | Владелец | Что блокирует | Как проверить | Оценка |
|---|---|---|---|---|---|
| P-1 | **ЗАКРЫТ 2026-08-21.** Полный клон `most` получен. DNS-ловушка обойдена: `nslookup` резолвит `git.korputeam.ru`, а `getaddrinfo` — нет, поэтому в локальный конфиг репо прописано `url."ssh://git@10.60.0.12/".insteadOf "https://git.korputeam.ru/"` + `core.sshCommand` с `HostKeyAlias`; blob-фильтр снят, sparse развёрнут. На диске `feature/giveaway`, `core/common`, `core/storage`, `graphql`, `app` (1.6G). **Выяснено:** сигнатура — `MviViewModel<S : UiState, E : UiEvent, F : UiEffect>` + `abstract fun onEvent(event: E)`, то есть корневой `CLAUDE.md` прав, а `.claude/rules/mvi.md` (`<S,I,E>`/`onIntent`) устарел и подлежит правке; Room под кэш есть — `MostDatabase`, `PostEntity`, `ProfilePostDao`, `MigrationTest`, `StorageModule`. | Android-лид | — | сделано | 0 |
| P-2 | **ЗАКРЫТ 2026-08-21.** contracts-репо заведён: **`shared/bdui-contracts`** (https://git.korputeam.ru/shared/bdui-contracts), рабочая копия — `~/projects/bdui-contracts`. Разложен скелет по раскладке [[BDUI-Backend]] §15.1 (`VERSION` 0.1.0, `CHANGELOG.md`, `README.md` с правилами semver, каталоги `schema/`, `catalog/profiles/`, `operations/`, `fixtures/{golden,invalid,expected/{ios,android}}`, `bindings/`) и CI-гейт `tools/validate.py`: разбирается весь JSON, каждая golden проходит схему, каждая invalid отвергается. На пустом наборе фикстур гейт зелёный — это и есть критерий готовности. С этого момента редактируемая копия контракта одна; `docs/Architecture/BDUI*.md` остаются проектной документацией, но не источником схемы/каталога/фикстур. | владелец продукта + iOS | — | сделано: репо существует, CI зелёный на пустом наборе фикстур | 1 |
| P-3 | **ОТВЕЧЕНО 2026-08-25, но не разблокировано.** Бэкенд: «бренд определяется по доменному имени» — отдельного поля/заголовка не будет. Сегодня это не работает ни на одном окружении: хостов четыре и они общие для всех 7 брендов (`api.korpu.app`, `api3.korpu.app`, `stage.korputeam.ru`, `dev.korputeam.ru` — `Modules/PostboxCore/Sources/GraphQLEndpoint.swift`), `BrandConfig.graphQLEndpoint` дефолтится в `.stage` (`App/Sources/BrandConfig.swift:52`) и ни один `Apps/<brand>/BrandConfig+Current.swift` его не переопределяет ⇒ `Host` у Khidi и Kuper одинаков. Нужно одно из двух: (1) per-brand домены — список хостов от бэка, дальше `graphQLEndpoint` в каждом `Apps/<brand>/BrandConfig+Current.swift` и `GraphQLEndpoint` перестаёт быть четырьмя константами; (2) заголовок `brand` — одна строка в `RequestContextHeaders`, доменному определению не мешает, снимает блокер сразу. Разбор — [[BDUI-Backend-Questions]] §3. | бэкенд (домены) + iOS | таргетинг ревизий по бренду ([[BDUI-Backend]] §5 шаг 7), валидацию брендовых токенов, любой промо-документ с брендовой вариацией | запрос с Khidi-сборки и с Kuper-сборки различим на сервере | 0.5 iOS после списка хостов |
| P-4 | **Расширить `AppLanguage`** `{ru,en,az}` → `{ru,en,az,ka,tt,uz,ab}` (`Modules/Localization/Sources/AppLanguage.swift:3`). **Решено 2026-08-21: делаем только enum + резолв + заголовок `language`, строки generic-хрома пока фолбэкают на `en`.** Полных каталогов на четыре новые локали не ждём: тексты самих BDUI-документов приходят с сервера уже локализованными, клиентские строки нужны только для фоллбеков/ретраев. | `feature-writer` | локализацию BDUI для Khidi (ka), Kuper (tt), Koprik (uz), Aura (ab): бэк отдаёт готовые строки по `locale`, но клиент не умеет попросить эти локали ⇒ грузинский бренд получит русский промо-экран. Также — таблицу CLDR-форм для `$plural` | `LocalizationRuntime.selection.effective(available:)` резолвит все 7; заголовок `language` отдаёт `ka` на Khidi | 1 |
| P-5 | **РЕШЕНО 2026-08-21: канон — Android.** iOS перекрашивается: заводится `KopirTheme` = `#00ABC2` / `#44C2D7` / `#AFE5F3` (`KazakhstanCyan` c100/c700/c1000), `KoprikTheme` правится на `#00ACA0` / `#44D7D0` / `#AFF2F3` (`UzbekTeal` c100/c700/c1000). Обоснование: андроидные акценты систематичны — ровно c100/c700/c1000 именованной рампы, тогда как iOS-триплет Koprik подобран вручную, а KZ на iOS не покрашен вовсе (`Apps/kopir` = `DefaultTheme()`, cornflower `#5A85D3`); плюс `#51C5BD` уже живёт на Android как `diamondGradientStart` у `UzBrand`, то есть оттенок тот же, роль другая. Цена решения — перекраска двух живых брендов в проде. | владелец продукта → `design-system` | `accentPrimary/Secondary/Tertiary` в кросс-брендовых документах | значения совпадают в `Brands.swift` и `brands/{Kz,Uz}Brand.kt` | 0.5 iOS |
| P-6 | **Android: пять недостающих компонентов V1** — `MostText` (T-AND-2), `MostAsyncImage` (T-AND-3), `MostCard` (T-AND-6), `MostBanner` (T-AND-7), `LinkButton` (T-AND-8) — плюс три объекта токенов: `MostSpacing` (T-AND-1, семантической шкалы spacing на Android нет вообще), `BduiShapes` (2 значения; `MostShapes.lg/xl` расходятся с контрактом и не трогаются) и `BduiTypography` (T-AND-14, 18 ролей со значениями iOS — канон типографики айосный, `MostTypography` для нативных экранов не трогается). `surface.frosted` нового компонента НЕ требует: `BlurSurface` уже есть. | Android-лид | layout-узлы, `text`, `image`, `card`/`surface`, `banner`, `button.link` — то есть каркас V1-каталога | компоненты есть в `:core:ui`, `@Preview` на каждый | ≈4.5 |
| P-7 | **ЗАКРЫТ 2026-08-27.** Компоненты в `ThemeKit`/`DesignSystem`, зарегистрированы в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) (125 позиций), `tuist build DesignSystem` зелёный, `graph-lint` чистый. Сверх списка добавлен `BDUIColorRole` (те же 40 цветов закрытым подмножеством — иначе prop `color` резолвится ad hoc в рендерере). `Divider` назван `DSDivider`: имя `Divider` занято SwiftUI. Тикер `CountdownLabel` шарится фазой (`static let` старта, выровненный по целой секунде), а не объектом-тикером — объект был бы process-wide держателем состояния, запрещённым правилом «никаких синглтонов». **Снапшот-матрица 7 брендов × light/dark не снята:** тест-таргет `KorpuTests` не собирается на `dev` (стухшие `Tests/Dating*` против удалённого `readyForPhotos`). **iOS: DS-пробелы, блокирующие V1** — `BDUITypeRole` + switch на существующую `Typography` (T-iOS-1, 18 родных айосных ролей — канон типографики), `Badge` (T-iOS-3), `DSToggle` (T-iOS-4), `ProgressBar` (T-iOS-10), `CountdownLabel` с шаренным тикером (T-iOS-11), `Divider` через `theme.colors.divider` + `LayoutMetrics.hairlineWidth`. V2/V3-пробелы (T-iOS-2/5/6/7/8/9) **не блокируют**. | `design-system` | узлы `text`, `badge`, `toggle`, `progress`, `countdown`, `divider` — без них пилот раффла не рисуется | компоненты есть, строки в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md), снапшоты 7 брендов × light/dark | ≈4 |
| P-7a | **ЗАКРЫТ 2026-08-25.** `_jsonValue` рекурсивен, `.null` → `NSNull()`, мёртвые `serializedString`/`plainValue` удалены; мост `PostboxCore.JSON ↔ PostboxGQL.JSON` — `Modules/CoreNetworking/Sources/PostboxGQLJSONBridge.swift`. **Грабли для будущих правок:** key-path форма `mapValues(\._jsonValue)` НЕ компилируется (`JSONValue` = `any Sendable & Hashable`, а `[String: any Sendable & Hashable]` не Hashable) — работает только замыкание с явным `as JSONValue`, тогда вывод типов выбирает `AnyHashable`. Ровно из-за этого исходный автор и вернул строку. Проверено на реальных исходниках изолированным swiftc-харнессом (8/8): до правки `_jsonValue` объекта имел тип `String` и уезжал на провод как `"{\"kind\"...}"`, после — настоящий объект. `Tests/PostboxGQLJSONScalarTests.swift` закоммичен, но **не прогонялся**: тест-таргет `KorpuTests` на `dev` не собирается (стухшие `Tests/Dating*` против удалённого `readyForPhotos` + WIP параллельной сессии в `Tests/ChatTests.swift`). | iOS | — | сделано | 0.5 |
| P-8 | **РЕШЕНО 2026-08-21: omniapp ВХОДИТ третьим потребителем.** Рекомендация §5 «не входит в V1» отклонена владельцем. Следствия — в переписанном §5: T-OMN-1…4 переезжают из «условий входа» в состав работ, deeplink-слой omniapp становится предусловием волны 0, гейт паритета расширяется с двух платформ до трёх. | владелец продукта | профиль каталога, ролевой словарь токенов, третью дорожку волны 2 | §5 | см. §5 |
| P-9 | **ОТВЕЧЕНО 2026-08-25: отдельный сервис, НЕ субграф** — в supergraph не федерируется. Следствия: `extend type Query/Mutation` из [[BDUI-Backend]] §1.3 отменяется (у сервиса свой корневой `Query`); прецедент `appattest.graphqls` **не подходит** — он склеивается в один namespace `PostboxGQL`, то есть в схему одного эндпоинта, а SDL независимого сервиса туда мержить нельзя (коллизии имён + ложные поля основного графа). GraphQL-вариант = второй codegen-конфиг + модуль `BduiGQL` + второй `NetworkExecutor` + строка в `Scripts/graph-lint.py:23`. Раз федерации нет, **GraphQL перестал быть обязательным**: REST снимает всю эту обвязку, а Postbox нейтрален на уровне `ResponseMapper` (прецедент `RestMediaUploadTransport`). Осталось от бэка: адрес сервиса на окружение, протокол, и принимает ли он тот же access token. | бэкенд | старт волны 2A; форму транспортного слоя волны 3 | адрес + протокол + модель аутентификации зафиксированы письменно | решение (клиент: REST ≈ 0, GraphQL ≈ +1 день) |
| P-10 | **ЗАКРЫТ 2026-08-25: авторитетен `language`** — ровно тот заголовок, который клиент уже шлёт (`RequestContextHeaders.swift:166`). Транспорт менять не надо. Осталось в клиенте — P-4 (без расширения `AppLanguage` Khidi/Kuper/Koprik/Aura не могут попросить свой язык). Мелочь на бэке: докстринг `Holiday.name` (`schema.graphqls:3921`) обещает `Accept-Language`, которого клиент не шлёт — либо протух, либо праздники молча фоллбечатся. | бэкенд | — | сделано | 0 |

**Что предусловием НЕ является** (частая ошибка планирования): готовый бэк (волна 2B идёт на фикстурах без сети), `block.raffle.leaderboard` (волна 4), omniapp, V2/V3-узлы каталога, аналитический пайплайн (шов ставится, за ним может стоять логгер).

---

## 2. Волны

```
Волна 0 (предусловия) ─────────────────────────────────────────────┐
                                                                    │
Волна 1  contracts: схема + каталог V1 + фикстуры ──────────────────┤
   │                                                                │
   ├──► 2A бэк:  скелет субграфа, хранение, валидатор, render       │  ← P-9, P-2
   ├──► 2B iOS:  таргеты, домен, рендерер V1 (на фикстурах)         │  ← P-7
   └──► 2C Android: contract + render + компоненты                  │  ← P-1, P-6
                    │                                               │
                    └──► гейт паритета рендереров на golden ────────┤
                                          │                         │
Волна 3  транспорт и данные (SDL, репозиторий, Postbox, deeplink) ──┤  ← P-3
                                          │                         │
Волна 4  пилот раффла (block + серверные агрегаты + CMS) ───────────┤  ← P-4 для не-Korpu
                                          │                         │
Волна 5  placement-слоты, V2-узлы, миграция прото-BDUI ─────────────┘
```

### Волна 1 — контракт

> **ЗАКРЫТА 2026-08-26, кроме SDL.** Репозиторий `shared/bdui-contracts` доведён с пустого скелета до `VERSION 0.2.0` (`schemaVersion` 2.0.0, `catalogVersion` 2.0.0): грамматика, лимиты, два семейства кодов ошибок, 40+18+6+7 токенов, 14 иконок, 31 узел + `block.raffle.leaderboard`, профили `korpu`/`vysota`, реестр из пяти операций пилота, 2 golden и 12 invalid-фикстур. Гейт из четырёх слоёв (синтаксис → грамматика → ссылочная целостность → сверка с каталогом) зелёный, плюс `tools/selfcheck.py`, который ломает golden одиннадцатью способами и требует, чтобы гейт заметил каждый.
>
> **Не сделано намеренно, три пункта.**
> 1. `bindings/bdui.graphqls` — ждёт P-9. После ответа «отдельный сервис, не субграф» протокол (GraphQL со своим корневым `Query` или REST) не выбран; SDL несуществующего графа зафиксировал бы непринятое решение.
> 2. `fixtures/invalid` покрывает только нарушения **грамматики** — это ровно то, что умеет `tools/validate.py`. Семантически битые документы (висячий `$data`/`$state`, вложенный `repeat`, стекло в стекле, неизвестный `screenId`) грамматике не противоречат и обязаны отвергаться Go-валидатором волны 2A; положить их сюда сейчас — покрасить гейт в красный за правильное поведение.
> 3. `fixtures/expected/{ios,android}` пусты: снапшоты появляются вместе с рендерерами волны 2.
>
> **Вскрылось при сборке, требует действий вне контракта.**
> - `BDUI.md` описывает грамматику **v1** и с 2026-07-13 не переписывался, хотя `BDUI-Catalog` (v2), `BDUI-Backend` и пилот написаны против v2 и ссылаются на его несуществующие разделы. Схема собрана из этих трёх, а не из `BDUI.md`; в сам файл добавлен баннер, полный переписывание под v2 — задача `docs-updater`.
> - `BDUI-Catalog` §3.1 называл prop выравнивания `alignment`, обе golden и §3.2 у `text` — `align`; §3.2 объявлял дефолт `role` значением `body2`, которого нет среди 18 ролей §1.3. Обе правки внесены.
> - `BDUI-Backend` §7.4 объявляет args лидерборда как `{limit, offset}`, пилот §4 проектирует накопительное окно без `offset`. Конструкции взаимоисключающие; зафиксировано окно, конфликт — в `pending` реестра операций.
> - Четыре лимита (`repeat.maxLimit`, число ключей `state`, размер `args` и data-пейлоада) не заданы нигде: §10 бэкенда отправляет за ними в `BDUI` §10, а там раздел про другое. Вынесены в `pending`, а не проставлены на глаз.
> - Из 14 иконок стартового набора в `:core:ui` у Android реально лежат 7; 4 нужно поднять из feature-модулей, 3 (`ticket`, `warning`, `retry`) — отрисовать. Это вход в P-6.


| Состав | Владелец |
|---|---|
| `schema/screen-document.schema.json` (JSON Schema 2020-12, `additionalProperties: false` везде), `schema/limits.json`, `schema/errors.json` | бэкенд + iOS совместно |
| `catalog/tokens.json` — 40 цветов, 18 ролей типографики (родные айосные), 6 spacing, 7 радиусов ([[BDUI-Catalog]] §1) | `design-system` |
| `catalog/catalog.json` — 32 узла V1 с props-схемами и `sinceVersion` (включая `block.*`, [[BDUI-Catalog]] §3.8); `catalog/icons.json` — 14 иконок | `design-system` |
| `catalog/profiles/korpu.json` (+ `vysota.json` только если P-8 = «да») | владелец продукта |
| `operations/registry.json` — 5 операций пилота ([[BDUI-Backend]] §7.4; имена совпадают с документом [[BDUI-Raffle-Pilot]] §3 посимвольно) | бэкенд |
| `bindings/bdui.graphqls` — SDL §1.3 | бэкенд |
| `fixtures/golden/*.json` — `raffle.main`, `raffle.rules` из [[BDUI-Raffle-Pilot]] §3 + грамматические фикстуры (repeat с `key` и без, все формы Value, все операторы предиката, все три правила деградации, `surface.frosted` — включая невалидный вложенный случай в `fixtures/invalid/`, поскольку пилот раффла этот узел не покрывает) | iOS + Android |
| `fixtures/invalid/*.json` — документы, обязанные быть отвергнутыми, с ожидаемым кодом §8.2 | бэкенд |

**Критерий готовности:** CI contracts зелёный — схема валидна, каждая golden проходит схему, каждая `invalid` падает с ожидаемым кодом, `catalog.json` ↔ `tokens.json` ссылочно целостны, semver-diff-гейт работает.
**Параллельно:** P-6, P-7 (компоненты платформ пишутся под уже согласованный список узлов; список узлов V1 зафиксирован в [[BDUI-Catalog]] §3.8 и не ждёт кода).

### Волна 2 — три дорожки параллельно

**2A. Бэкенд (Go).** Раскладка — [[BDUI-Backend]] §14. Порядок: `contract` (кодоген из contracts, `go:embed`) → `store` + миграции (§3) → `validate` (§8, включая вычисление `catalog_version_min`) → `render` (frame + messages + ETag) → `target` (§5) → `gql` (только `getBduiScreen`, `data: []`) → `call` + идемпотентность (§7) → `resolve` (fan-out, singleflight, бюджет) → `admin` + preview.
Критерий: `getBduiScreen("raffle.rules")` отдаёт валидный документ **без источников**; публикация невалидного черновика невозможна; `go generate ./internal/bdui/contract && git diff --exit-code` чист.

**2B. iOS.** Коммиты C1–C8 из §3. Работа идёт на `StubBDUIRepository` с golden-фикстурами, сеть не нужна.
Критерий: `BDUIScreen(store:)` рисует обе golden-фикстуры на 7 брендах × light/dark; снапшоты совпадают с `expected/ios`; табличные тесты резолвера и предикатов зелёные; Maestro-флоу на a11y-контейнерах зелёный.

**2C. Android.** `:core:bdui-contract` (модели + strict `kotlinx.serialization` + интерпретаторы Value/предиката, чистый Kotlin, JUnit5) → компоненты P-6 и остальные T-AND-* → `:core:bdui-render` (реестр `type → @Composable` поверх `MostTheme`, деградация §9) → `:feature:bdui` (`MviViewModel`, репозиторий, Room-кэш).
Критерий: те же фикстуры, Compose screenshot против `expected/android`.

**Гейт волны 2 (parity):** обе платформы декодят один и тот же набор фикстур строго, рендерят их без сети, и расхождение снапшотов ловится CI, а не глазами. До этого гейта транспорт не подключается — иначе рассинхрон каталога вскроется на живых данных.

### Волна 3 — транспорт и данные

| Дорожка | Состав |
|---|---|
| iOS | C9–C12: `Modules/PostboxGQL/Schema/bdui.graphqls` (прецедент — `appattest.graphqls`, второй `schemaSearchPath`), `LiveBDUIRepository` в `CoreNetworking`, Postbox namespace `bdui` + SWR + дедуп, `getBduiData`, конверт `call`, диплинк `bdui/<screenId>`, аналитика |
| Бэк | реальные read-резолверы `raffle.*`, батчинг `GetUsersForRaffle`, `bduiCall` для `raffle.claimDailyBonus`/`raffle.setVisibility`, Redis-кэш фрейма и данных, ETag/304 |
| Android | репозиторий над Apollo 4.x + Room-кэш, та же SWR-дисциплина, очистка на логауте |

Критерий: первый кадр = один сетевой запрос; офлайн-старт показывает stale-документ; 401 проходит существующий single-flight refresh; namespace `bdui` исчезает на логауте **без правок** (`RootEffects.installScopedPersistenceNamespaces` — allowlist сохраняемого).

### Волна 4 — пилот раффла

Состав: `block.raffle.leaderboard` на обеих платформах (C14), серверные агрегаты [[BDUI-Raffle-Pilot]] §5 (билеты за задание, абсолютное место, доля прогресса, маскирование, группировка призов, статус кампании, `me.isWinner` — оживает мёртвый `GetMyPrizes`), контент кампании в CMS, локализация ru/en/az.
Критерий — 14 пунктов приёмки [[BDUI-Raffle-Pilot]] §7. Ключевые: №4 (обе платформы, один документ, совпадающий экран), №8 (единственный нативный код фичи — один блок), №10 (≤4 запроса/мин против ≈120/мин у легаси).

### Волна 5 — расширение

Placement-слоты (`getBduiPlacements`, инъецированная фабрика в `BrandFeatures`, первый хост — **`sidebar.card`**, не `feed.top`; см. §7, R-5), V2-узлы каталога по продуктовой потребности, кандидат на миграцию — `DatingFamilyPromoBottomSheet` (`schema.graphqls:6923`: `templateName` + 4 серверные строки + `recordDatingFamilyPromoBottomSheetShown` — готовый прото-BDUI).

---

## 3. iOS: разбивка по коммитам

Конвенции: название на русском и объясняет логику; `*Repository`/Postbox-проводка входит в тот же коммит, что доменная логика; comment-free Swift; после каждого коммита с новыми файлами — `Scripts/generate.sh` (новый `.swift` без `tuist generate` молча не попадает в таргет).

| # | Коммит | Состав | Тесты (`Tests/`) | ≈строк |
|---|---|---|---|---|
| C1 | Завести таргеты BDUI в Tuist-графе | `Tuist/ProjectDescriptionHelpers/Modules.swift`: 4 кейса (`bduiDomain/bduiInterface/bduiMocks/bduiFeature`), `path`, `deps`, `.coreNetworking` ← `.bduiDomain`, `.appCore` ← `.bduiInterface`, `.bduiFeature` в `resources` и `defaultActorIsolation`; `Brand.swift` — `"BDUIFeature"` в `featureModules`; `Project.swift` — 4 таргета в `KorpuTests`; заглушечные `Sources/*.swift` в каждом таргете | сборка 7 схем | ~120 |
| C2 | Модель документа BDUI и строгий декод | `BDUIDomain`: `ScreenDocument`, `Node`, `Value`, `Action`, `Predicate`, `DataSource`, `Form`, `Repeat` — `Codable & Sendable`; лимиты из контракта; правила «битый документ» ([[BDUI]] §9) | `BDUIContractTests.swift` — strict decode всех golden + все `invalid` с ожидаемым исходом | ~600 |
| C3 | Резолвер значений и интерпретатор предикатов | `BDUIValueResolver` (`$state`/`$data` RFC 6901/`$item`/`$index`/`$context`/`$text`/`$plural`, `default`, промахи типов), `BDUIPredicateEvaluator` (11 операторов), CLDR-таблица форм | `BDUIValueResolverTests.swift`, `BDUIPredicateTests.swift` — табличные: строка на форму Value и на оператор × тип операнда | ~500 |
| C4 | Стейт, редьюсер и эффекты BDUI | `BDUIState` (документ + `state` + payload источников + in-flight + `fieldErrors` + nav-path), `BDUIAction`, чистый `bduiReducer`, протоколы `BDUIStoring`/`BDUIRepository`/`BDUIAnalytics`; чеканка `idempotencyKey` в редьюсере (прецедент `ChatReducer`), pending, валидация формы, optimistic + откат | `BDUIReducerTests.swift` — patchState, шаги, submit-lifecycle, `fieldErrors`, откат, идемпотентность повтора, идентичность `repeat` с `key` и без | ~700 |
| C5 | Моки и фикстуры BDUI | `BDUIMocks`: `MockBDUIStore`, `StubBDUIRepository`, `BDUIFixtures` (golden JSON ресурсами) | используется остальными | ~250 |
| C6 | Рендерер BDUI: раскладка и контент | `BDUINodeView` + реестр `type → @MainActor @Sendable (RenderContext) -> AnyView?`; 7 layout-узлов + `text`/`image`/`icon`/`avatar`/`badge`; a11y-id `bdui.<screenId>.<nodeId>` + **обязательный** `.accessibilityElement(children: .contain)` на контейнерах; `\.mediaLoader` из окружения | `BDUIRenderLayoutTests.swift` + снапшоты 7 брендов × light/dark через `MockBDUIStore` | ~800 |
| C7 | Рендерер BDUI: поверхности, действия и состояния | `card`/`surface`/`banner`/`sheet.prompt`; `button.*`/`chip`/`chipRow`/`toggle`; `tabs`/`list.section`; `progress`/`skeleton`/`loader`/`countdown`/`emptyState` с **одним шаренным тикером**; хаптик (`.impact(.medium)` на коммит, `.selection` на переключения, `.success`/`.error` на исход) | `BDUIRenderActionsTests.swift`, снапшоты состояний | ~800 |
| C8 | Закрыть DS-пробелы под каталог V1 | Отдельный коммит модулей `DesignSystem`/`ThemeKit` (P-7): `BDUITypeRole`, `Badge`, `DSToggle`, `ProgressBar`, `CountdownLabel`, `Divider` + строки в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) | снапшоты компонентов | ~600 |
| C9 | Транспорт BDUI и кэш в Postbox | `Modules/PostboxGQL/Schema/bdui.graphqls`; `LiveBDUIRepository` в `CoreNetworking` — `executor + ResponseMapper`, namespace `bdui`, `QueryKey(namespace:operation:variables:)`, `SetQueryRecordMutation`, `observeCachedQueryRecord`, `StaleWhileRevalidatePolicy`, `PostboxInFlightDeduplicator`; JSON едет скаляром (шаблон — `LiveDailyRewardsRepository.swift`) | `LiveBDUIRepositoryTests.swift` — SWR, дедуп, stale-offline, отсутствие ре-энкода payload | ~700 |
| C10 | Подключить BDUI к composition root | `FeatureID.bdui`; `BrandFeature` + `.standard`; арм в `BrandFeatures.registrations` + приватная `bduiRepository(for:)` с UITEST-гейтом (прецедент `dailyRewardsRepository(for:)`); `RootPresentedFeature.bdui(BDUIEntry)` + `opensAsPush` + арм в `RootView.presentedFeatureView`; `SessionTeardownRegistering` (owner-scoped, снятие в `deinit`) | `BDUIRegistrationTests.swift` | ~400 |
| C11 | Диплинк `bdui/<screenId>` | 6 сайтов: `RootExternalRoute.Kind` (payload `String`), `knownSections` + арм в `RootExternalRouteParser.deepLink(section:parts:comps:)`, три исчерпывающих `switch` (`targetFeatureID`/`requiresAuthenticatedRoot`/`navigationDeduplicationKey`), `apply(route:to:)` в `RootReducer.swift:566`, `RootScreen.hasDestination(for:)` → `resolver.hasRoute(.bdui)` | `BDUIDeepLinkTests.swift` — разбор URL, дроп маршрута на бренде без BDUI, preview-токен | ~250 |
| C12 | Аналитика BDUI | `AnalyticsEvent.bdui(name:params:)` в `Analytics` + адаптер `BDUIAnalytics` + инъекция трекера в FeatureModule (**первый прецедент в проекте** — сегодня `AnalyticsTracking` получает только `AppEntry`) | `AnalyticsTests.swift` (дополнение) | ~150 |
| C13 | Placement-слоты BDUI | `BDUIPlacementEntry`, инъецированная фабрика `(BDUIPlacementEntry) -> AnyView` из `BrandFeatures` (тот же приём, что `profileViewer`/`commentsViewer`), первый хост `sidebar.card` | `BDUIPlacementTests.swift` | ~300 |
| C14 | Пилот: нативный блок таблицы лидеров | `block.raffle.leaderboard` — строки, липкая строка «я», пагинация накопительным окном, тап по строке через deeplink-allowlist; блок ходит только через `data` документа | `BDUILeaderboardBlockTests.swift` | ~600 |
| C15 | Maestro-флоу BDUI | `Modules/BDUI/UITests/maestro/`: `bdui-container-a11y.yaml` (вложенные контейнеры на golden-фикстуре), `bdui-tabs-and-call.yaml` (вкладки → `call` → pending → конверт), `bdui-deeplink.yaml`, `bdui-degradation.yaml` (неизвестный узел/токен не роняет экран) + `config.yaml` | прогон на mock-seeded UITEST | ~200 |
| C16 | Зафиксировать статус BDUI в документации | wishbone: `docs/Status/BDUI.md` (полный лог); Stratoapp: `.claude/rules/status/bdui.md` (выжимка iOS-среза + `paths` в frontmatter) и строка в карте `docs/Architecture/Implementation-Status.md` | `make docs-check` в Stratoapp | ~120 |

Порядок: C1 → C2 → C3 → C4 → C5 → (C6 ‖ C8) → C7 — это волна 2B; C9 → C10 → C11 → C12 — волна 3; C14 — волна 4; C13 — волна 5; C15 идёт следом за первым рендерящим коммитом и дополняется; C16 — после каждой волны.

---

## 4. Android-срез

Порядок и состав. **Всё требует полного клона (P-1)**: в sparse-клоне на диске только `core/ui`. `Screen.kt`, `feature/giveaway` и `graphql/` разведка читала как исходники (через GitLab API); не подтверждён `:core:common` — сигнатура `MviViewModel`, которая в документации most сама себе противоречит, — и `:core:storage` под Room-кэш.

| # | Модуль / шаг | Состав | Зависит от |
|---|---|---|---|
| A1 | `:core:ui` — токены и компоненты | `MostSpacing` (T-AND-1), `BduiShapes`, `BduiTypography` (T-AND-14), `MostText` (T-AND-2), `MostAsyncImage` (T-AND-3), `MostCard` (T-AND-6), `MostBanner` (T-AND-7), `LinkButton` (T-AND-8), `MostBadge` (T-AND-5), `Countdown` с шаренным тикером (T-AND-12), `EmptyState` (T-AND-13). `BlurSurface` под `surface.frosted` уже есть — писать не надо | волна 1 (список узлов V1) |
| A2 | `:core:bdui-contract` | модели документа, strict `kotlinx.serialization`, валидация лимитов, интерпретаторы Value и предиката; чистый Kotlin, JUnit5 + Turbine; **зеркалит C2–C3 iOS** | волна 1 |
| A3 | `:core:bdui-render` | реестр `type → @Composable` поверх `MostTheme`; алиас `elN → text{N}` (fill/text) и `elN → stroke{N}` (border) **по слоту prop**; типографика через `BduiTypography`, не `MostTheme.typography`; деградация [[BDUI]] §9; `Modifier.testTag("bdui.…")` в цепочке узла; **один `rememberLayerBackdrop()` в корне документа** — его требует `surface.frosted` ([[BDUI-Catalog]] §3.3); при отсутствии backdrop узел деградирует в обычный `surface` | A1, A2 |
| A4 | `:feature:bdui` | `MviViewModel` (state = документ + `state`, events = tap/change/submit, effects = navigate/dismiss), репозиторий над Apollo 4.x, Room-кэш в `:core:storage` (`payload`/`writtenAt`/`ttl`), очистка на логауте | A3, волна 3 |
| A5 | Навигация и вход | `Screen.Bdui(screenId)` как `@Serializable` NavKey (Navigation3), `open(url)` через `Navigator`, входы из feed/sidebar | A4 |
| A6 | `block.raffle.leaderboard` | нативный блок пилота, контракт props — [[BDUI-Raffle-Pilot]] §4 | A4 |

Открыто до полного клона: точная сигнатура базового `MviViewModel`, наличие и форма Room-DAO под generic-кэш, место `:core:bdui-*` в `settings.gradle.kts`, есть ли у Compose-компонентов параметр `testTag` (в `:core:ui` он есть у единиц — `GlassSegmentedTabItem`, `SegmentedTabsContent`, `ProfileSwitcherSegment`, `AlertData`, `DailyRewardsProgressRow`).

---

## 5. omniapp (VYSOTA) — входит в V1

**Решение владельца 2026-08-21: omniapp входит третьим потребителем BDUI.** Прежняя редакция этого раздела рекомендовала отложить его до V2 — рекомендация отклонена. Ниже — что это меняет; сами пробелы никуда не делись, они просто переехали из «условий входа» в состав работ.

| Пробел | Факт | Цена |
|---|---|---|
| Транспорт | Его продуктовый бэкенд — Go + REST, «no GraphQL» (`back-artifacts/README.md`); его GraphQL-эндпоинт — **другая схема** (`cosmo_schema.graphqls`, sport/dating/energy) | второй биндинг либо три операции на cosmo |
| Deeplink | **Слоя нет вообще**: 0 `onOpenURL`, 0 `CFBundleURLSchemes`/`applinks`, `Router` не URL-адресуем | без него `open(url)` некуда положить; набор действий сжимается до `patchState/refresh/submit/track/dismiss` (T-OMN-3, 1+ дн) |
| Токены | `DSColor` — плоские статики с литеральным hex, **dark-only**, продуктовые имена (`background/surface/elevated/textPrimary/danger`) | таблица алиасов ~30 строк (T-OMN-1); инвариант «один токен = одна light/dark пара» там ложен |
| Каталог | Покрытие V1 ≈60 %: нет `card`, `banner`, `sheet.prompt`, `list.section`, `progress`, `toggle`, `select` | T-OMN-2, ≈2 дн |
| Аналитика | SDK нет в проекте вовсе | `BDUIAnalytics` = логгер, 0.25 дн |

**Следствия для плана.**

- `catalogId` в `BduiContextInput` и колонка `bdui_screen.catalog_id` — по-прежнему волна 1, но теперь это не «дверь на будущее», а рабочий механизм выбора профиля каталога.
- Профиль `catalog/profiles/vysota.json` и **ролевой словарь токенов** — переезжают **в волну 1**. Раньше были явно исключены.
- **Deeplink-слой omniapp (T-OMN-3) становится предусловием волны 0.** Владелец определён 2026-08-21: делается на стороне этого проекта отдельным срезом до начала волны 2 — URL-адресуемый `Router` + `CFBundleURLSchemes`, полезно само по себе, а не только под BDUI. Проверено 2026-08-21: в omniapp ноль `onOpenURL`, ноль `CFBundleURLSchemes`/`applinks`, `Router` не URL-адресуем. Без него действие `open(url)` некуда положить, и набор действий на третьей платформе сжимается до `patchState/refresh/submit/track/dismiss` — то есть контракт действий перестаёт быть общим для всех потребителей.
- **Транспорт-биндинг** — предусловие волны 3: продуктовый бэкенд omniapp это Go + REST («no GraphQL», `back-artifacts/README.md`), а его GraphQL-эндпоинт живёт на другой схеме (`cosmo_schema.graphqls`). Нужен либо второй биндинг, либо три операции на cosmo.
- **Волна 2 получает третью дорожку (2D)** — рендер-модуль omniapp параллельно iOS и Android; гейт паритета расширяется с двух платформ до трёх на тех же golden-фикстурах.
- `DSColor` — плоские статики с литеральным hex и **dark-only**; инвариант «один токен = одна light/dark пара» на omniapp ложен (DRIFT-7). Это принято и в V1 не чинится: документ, свёрстанный под light, на omniapp всегда отрисуется тёмным.
- Аналитического SDK в проекте нет вовсе — за `BDUIAnalytics` там стоит логгер (0.25 дн), волны это не блокирует.

Порядок работ внутри omniapp-дорожки: транспорт-биндинг → профиль каталога → ролевые токены → deeplink-хост → рендер-модуль.

⚠️ **Цена решения, зафиксированная явно.** Покрытие каталога V1 у omniapp ≈60 % (нет `card`, `banner`, `sheet.prompt`, `list.section`, `progress`, `toggle`, `select`), то есть T-OMN-2 ≈2 дн — и это до того, как появится хоть одна строка рендерера. Суммарно omniapp добавляет к волне 0 отсутствующий deeplink-слой, а к волнам 1–3 — четыре задачи T-OMN-1…4.

---

## 6. Гейты приёмки

| Волна | Гейт | Команда / проверка |
|---|---|---|
| 0 | Полный клон most | `git -C ~/projects/most sparse-checkout disable && ls feature/giveaway core/common core/storage graphql` |
| 0 | contracts-репо живо | ✔ `shared/bdui-contracts`, `python tools/validate.py` зелёный на пустом наборе фикстур |
| 0 | Локали | `AppLanguage.allCases.count == 7`; фокусный прогон `LocalizationTests` |
| 0 | iOS DS-пробелы | `mise exec -- tuist build DesignSystem --no-binary-cache`; снапшоты компонентов |
| 1 | Контракт | схема валидна; каждая golden проходит; каждая `invalid` падает с **ожидаемым** кодом; semver-diff-гейт |
| 2A | Бэк | `go generate ./internal/bdui/contract && git diff --exit-code`; `validate` на всех фикстурах даёт те же коды; реестр handler'ов покрывает `operations/registry.json` (fail-fast на старте) |
| 2B | iOS сборка графа | `Scripts/generate.sh`; `mise exec -- tuist build BDUIFeature --no-binary-cache`; брендовая матрица 7/7 (`make build SCHEME=<brand>` для `Korpu Aura Kamurj Khidi Kopir Koprik Kuper`) |
| 2B | iOS логика | `xcodebuild test -workspace Stratoapp.xcworkspace -scheme StratoappTests -destination '<sim>' -only-testing 'KorpuTests/BDUIContractTests' -only-testing 'KorpuTests/BDUIValueResolverTests' -only-testing 'KorpuTests/BDUIPredicateTests' -only-testing 'KorpuTests/BDUIReducerTests'` — **фокусно** (см. §7, R-4) |
| 2B | Карантин вендоров | `make graph-lint` — `BDUIDomain`/`BDUIFeature` не импортируют Apollo/GRDB/`URLSession` |
| 2B | a11y-контейнеры | `maestro test Modules/BDUI/UITests/maestro/bdui-container-a11y.yaml`; предварительно — `maestro hierarchy`, дети контейнеров видны |
| 2 | Паритет рендереров | снапшоты iOS против `expected/ios`, Compose против `expected/android`, на одних и тех же golden |
| 3 | Транспорт | первый кадр — один запрос (проверка по `DurablePostboxRequestLogger`/трейсу); офлайн-старт отдаёт stale; namespace `bdui` исчезает после логаута (проверка `postbox_query_records`) |
| 3 | Диплинк | `xcrun simctl openurl booted "korpu://bdui/raffle.rules"` → экран; на бренде без BDUI маршрут дропается |
| 4 | Пилот | 14 пунктов [[BDUI-Raffle-Pilot]] §7; замер трафика: ≤4 запроса/мин против ≈120/мин легаси |
| любая | Документация | `make docs-check`; обновлены три уровня статуса (карта Stratoapp → `.claude/rules/status/bdui.md` Stratoapp → `docs/Status/BDUI.md` wishbone) |

---

## 7. Риски

| # | Риск | Факт | Митигация |
|---|---|---|---|
| R-1 | **Все Android-оценки предварительны.** | `~/projects/most` — sparse clone: на диске только `core/ui`, `docs`, `.claude`; `git.korputeam.ru` с машины не резолвится. 13 задач T-AND-* оценены по одному модулю из четырёх нужных | P-1 первым пунктом; до полного клона Android-волна не планируется по срокам, только по составу. Известный обход — VPN + `--resolve git.korputeam.ru:443:10.60.0.13` + `PRIVATE-TOKEN` (проверен разведкой) |
| R-2 | **Две редактируемые копии контракта.** Пока contracts-репо нет, истина — четыре markdown-файла; первое расхождение iOS/Android ловится глазами, а не CI | [[BDUI-Backend]] §15.4 | P-2 до начала волны 2; до тех пор любое изменение грамматики/каталога — один MR, трогающий оба клиента |
| R-3 | **Рассинхрон каталога у пользователя.** Клиент получает узел, которого не знает | `catalog_version_min` вычисляется валидатором из фактических узлов ревизии, а не пишется руками ([[BDUI-Backend]] §8.3) + шаги 4–5 таргетинга | Плюс клиентская телеметрия `bdui_node_degraded_total` с алертом — единственный ранний сигнал |
| R-4 | **Полный `StratoappTests` на `dev` не собирается.** `Tests/Wallet*`, `Tests/Dating*`, `Tests/RootExternal*`, `RootScreenRuntimeTests` устарели против исходников (Implementation-Status, слайс «Диагностические логи») | — | BDUI-тесты гоняются фокусно (`--test-targets`); «зелёный полный сьют» не ставится гейтом; при массовых правках — изолированный worktree (`git worktree add --detach`), иначе чужие незакоммиченные правки ломают прогон |
| R-5 | **`feed.top` садится на сломанную поверхность.** Лента (`FeedTextureTimelineView.swift` — вопреки имени `ScrollView { LazyVStack }`) имеет открытый баг вечного layout-цикла на iOS 26.5 (100 % CPU, тапы мертвы) | Implementation-Status, слайс Feed | Первый placement-хост — `sidebar.card`, не `feed.top`; лента подключается после починки цикла |
| R-6 | **Новый `.swift` без `tuist generate` молча не попадает в таргет** — добавленные тесты «проходят», потому что их нет | конвенция проекта | C1 идёт первым; `Scripts/generate.sh` после каждого коммита с новыми файлами; в гейте волны 2B — явный `tuist build` |
| R-7 | **Ловушка a11y-контейнера воспроизводится на каждом документе разом.** Идентификатор на контейнере без `.accessibilityElement(children: .contain)` схлопывает поддерево | в репозитории на это наступали четырежды: `root.tabbar`, `GlassMenuCard`, `root.rating`, `settings.back` | Правило зашито в рендерер (C6), а не в код-ревью; отдельный Maestro-флоу на golden-фикстуре с вложенными контейнерами — обязательный пункт приёмки |
| R-8 | **Рукописный маппер скаляра `JSON` сегодня лоссовый.** `Modules/PostboxGQL/Sources/Generated/Schema/CustomScalars/JSON.swift`: `_jsonValue` отдаёт `.object`/`.array` строкой, `.null` — литералом `"null"`; моста с `PostboxCore.JSON` нет. Пока не починен — любой вложенный объект в аргументе молча ломается | [[BDUI-Backend]] §1.2 | T-XPL-2 — предусловие волны 3 (правка ~10 строк + мост + round-trip-тест на вложенном объекте). Файл помечен `@generated`, но кодогенерация его не перезатирает — риск не в перезаписи, а в том, что правку примут за автоген и откатят: пункт чек-листа код-ревью §16 бэкенд-документа |
| R-9 | **Акценты на `kopir`/`koprik` красятся по-разному** на одном документе | DRIFT-1/2, [[BDUI-Catalog]] §1.6 | До T-XPL-1 — запрет `accent*` в кросс-брендовых документах (проверяется валидатором на записи, а не ревью); пилот — только Korpu |
| R-10 | **Брендовые локали недостижимы.** `AppLanguage = {ru,en,az}`; Khidi/Kuper/Koprik/Aura получат `ru` или `en` — то есть грузинский бренд получит русский промо-экран | `Modules/Localization/Sources/AppLanguage.swift:3` | P-4 до раскатки BDUI за пределы Korpu; пилот на Korpu ограничение не проявляет |
| R-11 | **Аналитика: шов есть, пайплайна нет.** `AnalyticsEvent` — закрытый enum из 24 кейсов без `custom`; ни один feature-модуль сегодня не получает `AnalyticsTracking` (сервис потребляет только `AppEntry`) | `Modules/Analytics/Sources/AnalyticsEvent.swift` | C12 добавляет один кейс и первый прецедент инъекции; за швом может стоять логгер — это не блокирует волны |
| R-12 | **Продукт потребует секундной живости** (последние минуты розыгрыша), а пилот даёт `refreshSeconds: 30` | подписки `raffleSnapshot` не существует ни на одной платформе | Зафиксировано как не-цель V1; если понадобится — отдельная GraphQL-подписка только на `me.ticketsCount/place` и топ-N, а не возврат к поллингу |
| R-13 | **≈17 человеко-дней DS-работы до первой строки рендерера** (14 задач Android + 11 iOS) — соблазн начать с рендерера и застрять на `toggle` | [[BDUI-Catalog]] §7 | P-6/P-7 — предусловия волны 2, а не её содержимое; блокирующее подмножество (5 Android + 6 iOS) выделено явно, V2/V3-пробелы отложены вместе со своими узлами |

---

## 8. Чего осознанно НЕ делаем в V1

| Не делаем | Почему |
|---|---|
| Узлы V2/V3: `input.text`, `input.otp`, `select`, `segmented`, `accordion`, `carousel`, `avatarStack`, `lottie`, `video`, `slider`, `stepper`, `datePicker` | пилоту не нужны; каждый тянет по DS-компоненту на трёх платформах ([[BDUI-Catalog]] §3.8) |
| `block.raffle.dailyRewards` (церемония летящих монет) | ровно тот случай, когда `block.*` стоит релиза обоих клиентов ради декоративной анимации |
| Жест раскрытия таблицы поверх картинки приза, липкая строка из `layoutInfo`, `@mention` внутри абзаца, shimmer/wobble | механика лэйаута и инлайн-ссылки вне грамматики; теряются осознанно ([[BDUI-Raffle-Pilot]] §4) |
| BDUI в Texture-поверхностях (`ChatTexture`, `SearchTexture`) | требуется `ASCellNode`-хост; placement-слоты живут только в SwiftUI/Compose |
| omniapp как потребитель | §5; вводим только `catalogId`, чтобы не закрыть дверь |
| Подписка на секундную живость раффла | R-12 |
| Pre-auth документы (namespace `bdui.public` в preserve-list) | сейчас namespace `bdui` корректно стирается на логауте без правок; отдельный публичный namespace — осознанная правка allowlist'а, а не побочный эффект |
| Remote SVG/векторные иконки по URL | реестр иконок закрыт и кодогенерится; растровые URL закрывает узел `image` |
| BDUI в core-фичах (Feed, Chat, Dating, Map, Stories, Calls, Composer) | non-goal модели: BDUI не заменяет нативную разработку |
| Веб-конструктор админки как источник истины визуала | истина — превью на устройстве ([[BDUI-Backend]] §13.3/§13.4) |
| Автоматический ретрай на `retryable: false` | клиентское правило [[BDUI-Backend]] §11 |

---

[[README|← к обзору]] · [[BDUI]] · [[BDUI-Catalog]] · [[BDUI-Backend]] · [[BDUI-Raffle-Pilot]] · [Feature-Template](../../Stratoapp/docs/Architecture/Feature-Template.md) · [UI-Testing](../../Stratoapp/docs/Architecture/UI-Testing.md) · [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) #arch

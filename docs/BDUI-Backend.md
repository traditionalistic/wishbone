# BDUI — бэкенд-сервис (Go)

> Репозитории и соглашение о путях — [[README#Репозитории]]. Относительные пути в тексте: `Modules/`, `App/`, `Apps/`, `Tuist/`, `Scripts/`, `Tests/` — Stratoapp; `core/`, `feature/`, `graphql/`, `app/` — most; `Korpu/` — multiapp; `schema/`, `catalog/`, `operations/`, `fixtures/`, `tools/`, `bindings/` — bdui-contracts.

> Спека сервиса, который отдаёт и хранит `ScreenDocument`. Грамматика документа — [[BDUI]] (не переопределяется здесь), реестр узлов и токенов — [[BDUI-Catalog]], пилот — [[BDUI-Raffle-Pilot]], порядок работ — [[BDUI-Plan]].
> Целевой стенд — Go/GitLab/ArgoCD, как у остального бэкенда. **Решено 2026-08-25 бэкендом: BDUI — отдельный сервис со своим адресом, в supergraph НЕ федерируется** (не субграф и не поля в существующей схеме). Что из этого следует для клиента — §1.1 и [[BDUI-Backend-Questions]] §1.

## 0. Инварианты сервиса

1. **Клиент никогда не называет настоящую ручку.** `operationId` резолвится по серверному allowlist-реестру (`bdui_operation`). Это единственная граница безопасности записи.
2. **Клиент никогда не получает узел, которого не знает.** Минимальная версия каталога ревизии **вычисляется валидатором на публикации** из фактически использованных узлов, не пишется руками.
3. **Валидация — на записи.** Документ, не прошедший JSON Schema + семантические проверки, не может стать `published`. Клиентская деградация — вторая линия, не первая.
4. **Первый кадр = один запрос.** `getBduiScreen` возвращает статический фрейм + инлайненные пейлоады объявленных источников.
5. **Статика и данные кэшируются раздельно.** Фрейм — долгий ETag/304, данные — свой TTL. Иначе живой раффл обнуляет любой кэш.
6. **Значения полей форм не попадают ни в логи, ни в трейсы, ни в метрики.**

## 1. Поверхность GraphQL

### 1.1 Как это попадёт на клиент

**Переписано 2026-08-25 после ответа бэкенда.** Прежняя редакция опиралась на прецедент `Modules/PostboxGQL/Schema/appattest.graphqls` (второй `schemaSearchPath`, рукописный `extend type Query/Mutation`). Для отдельного сервиса он **не годится**: все `schemaSearchPath` склеиваются в один namespace `PostboxGQL` — то есть в схему одного эндпоинта. SDL независимого сервиса туда мержить нельзя (имена типов двух графов столкнутся, а BDUI-операции будут выглядеть как поля основного графа).

Отсюда два варианта, и выбор за бэкендом:

- **GraphQL со своим корневым `Query`/`Mutation`** (не `extend`): второй `apollo-codegen-config`, отдельный сгенерённый модуль `BduiGQL`, второй `NetworkExecutor` на BDUI-URL, плюс строка в allowlist `Scripts/graph-lint.py:23` — сейчас `ApolloAPI` разрешён только `CoreNetworking`/`PostboxGQL`. Клиентская цена ≈ 1 день обвязки.
- **REST** (`GET /bdui/screen`, `POST /bdui/form`): раз федерации нет, типизированный граф ничего не даёт. Снимает второй codegen, лишний модуль и правку карантина целиком; Postbox транспортно-нейтрален на уровне `ResponseMapper`, REST-прецедент уже живёт (`RestMediaUploadTransport` для `/upload`). Клиентская цена ≈ 0.

SDL в §1.3 остаётся описанием поверхности (имена операций, входы, пейлоады) и в GraphQL-варианте переписывается с `extend type Query` на собственный `type Query`. Ни одной BDUI-операции в `schema.graphqls` нет и не появится — основной граф BDUI не отдаёт.

**Открыто:** адрес сервиса на каждое окружение, протокол и принимает ли сервис тот же access token основного графа (тогда на него вешается существующий `AuthHeaderInterceptor` + `TokenRefreshCoordinator`).

### 1.2 Формат JSON на проводе — скаляр `JSON`, рукописный маппер чиним

Все JSON-пейлоады (документ, аргументы, значения формы, патчи) едут **скаляром `JSON`**, который в схеме уже есть (`schema.graphqls:4504`). Прежняя редакция везла их строкой в обход скаляра — **отменено решением владельца**: вместо обхода чиним сам маппер.

**Что сегодня сломано.** На iOS скаляр реализован рукописным `Modules/PostboxGQL/Sources/Generated/Schema/CustomScalars/JSON.swift`. Файл помечен `@generated`, но его собственная шапка гласит: «can be edited to implement advanced custom scalar functionality. Any changes to this file will not be overwritten by future code generation execution» — то есть это именно наш редактируемый маппер, а не автоген, который затрут. Дефект в одном методе:

```swift
@_spi(Internal) public var _jsonValue: JSONValue {
  switch self {
  case .object, .array: return serializedString   // ← объект уезжает СТРОКОЙ
  ...
  case .null:           return "null"             // ← null уезжает СТРОКОЙ "null"
  }
}
```

Вложенный объект в аргументе молча превращается в строку, `null` — в литерал `"null"`. Это баг для любого отправителя вложенного JSON, не только для BDUI.

**Правка сделана 2026-08-25** (`JSON.swift`, задача T-XPL-2): `_jsonValue` рекурсивно отдаёт `JSONValue` вместо строки, мёртвые `serializedString`/`plainValue` удалены —

```swift
case let .object(value): return value.mapValues { $0._jsonValue } as JSONValue
case let .array(value):  return value.map { $0._jsonValue } as JSONValue
case .null:              return NSNull()
```

**Спеллинг обязателен именно такой.** Key-path форма (`mapValues(\._jsonValue)`) не компилируется: `JSONValue` — это `any Sendable & Hashable`, а `[String: any Sendable & Hashable]` не Hashable, поэтому в существенциал не эрейзится. С замыканием и явным `as JSONValue` вывод типов выбирает `T = AnyHashable`, и всё сходится. Ровно об эту стену и споткнулся исходный автор, вернув строку, — не повторяйте.

Плюс мост `PostboxGQL.JSON ↔ PostboxCore.JSON` в `CoreNetworking` (единственный модуль, импортирующий оба). `.int(Int)` при этом менять не надо: на iOS `Int` 64-битный, bigint-идентификаторы переживают парсинг — расхождение с `PostboxCore.JSON.int(Int64)` чисто номинальное и закрывается мостом.

**Что меняется в обмен на отказ от строки.** Пропадает экранирование (~7 % размера) и двойная сериализация на бэке. Появляется одно следствие: Apollo декодит документ в дерево `PostboxGQL.JSON`, и для записи в `CachedQueryRecord.payload: Data` его надо сериализовать заново — байты в кэше не идентичны байтам сервера (порядок ключей). На корректность это не влияет: **ETag хранится отдельным полем записи и сравнивается как есть**, а дешёвый skip перерисовки идёт по `revision`, не по байтам.

### 1.3 SDL

```graphql
extend type Query {
  getBduiScreen(input: BduiScreenInput!): BduiScreenPayload!
  getBduiPlacements(input: BduiPlacementsInput!): BduiPlacementsPayload!
  getBduiData(input: BduiDataInput!): BduiDataPayload!
}

extend type Mutation {
  bduiCall(input: BduiCallInput!): BduiCallPayload!
}

enum BduiPlatform { IOS ANDROID }

input BduiContextInput {
  platform: BduiPlatform!
  osVersion: String!          # "18.4"
  appVersion: String!         # CFBundleShortVersionString / versionName
  appBuild: String            # CFBundleVersion / versionCode
  brand: String!              # korpu|aura|kamurj|khidi|kopir|koprik|kuper|vysota
  catalogId: String!          # korpu | vysota — см. §15.3
  locale: String!             # BCP-47: ru|en|az|ka|tt|uz|ab
  timeZone: String            # IANA, для серверного форматирования дат ("Asia/Baku")
  schemaVersion: String!      # semver грамматики, которую понимает клиент
  catalogVersion: String!     # semver каталога, который понимает клиент
  previewToken: String        # см. §13.4; вне превью — null
}

input BduiScreenInput {
  context: BduiContextInput!
  screenId: String!
  params: JSON                # объект: параметры маршрута (deep link, id сущности)
  knownEtag: String           # ETag фрейма из кэша клиента; протокол свежести (§9) построен только на нём
}

type BduiScreenPayload {
  status: BduiStatus!
  document: JSON              # ScreenDocument; null при NOT_MODIFIED и любой не-OK
  etag: String
  revision: Int
  frameTtlSeconds: Int!       # свежесть фрейма даже когда document == null
  data: [BduiDataItem!]!      # инлайн резолвнутых источников — всегда, даже при NOT_MODIFIED
  experiments: [BduiExperiment!]!
  error: BduiError
}

enum BduiStatus {
  OK
  NOT_MODIFIED          # knownEtag совпал; данные всё равно свежие
  NOT_FOUND             # screenId неизвестен или архивирован
  DISABLED              # экран выключен для этого бренда/сегмента — не ошибка
  UNSUPPORTED_VERSION   # нет ревизии, совместимой со schemaVersion/catalogVersion клиента
  TEMPORARILY_UNAVAILABLE  # упал required-источник или превышен бюджет (§6.4)
}

type BduiDataItem {
  id: String!                 # DataSource.id из документа
  ok: Boolean!
  payload: JSON               # null если !ok
  ttlSeconds: Int!
  refreshSeconds: Int         # мягкий период поллинга, если объявлен биндингом
  etag: String
  error: BduiError
}

type BduiExperiment { key: String!  variant: String! }

input BduiPlacementsInput {
  context: BduiContextInput!
  surfaces: [String!]!        # ["feed.top","sidebar.card"] — максимум 16 (§10)
  knownEtags: [BduiSurfaceEtag!]
}
input BduiSurfaceEtag { surface: String!  etag: String! }

type BduiPlacementsPayload { items: [BduiPlacementItem!]! }
type BduiPlacementItem {
  surface: String!
  placementId: String         # null ⇒ слот пуст, хозяин-экран ничего не рисует
  status: BduiStatus!
  document: JSON
  etag: String
  revision: Int
  frameTtlSeconds: Int!
  data: [BduiDataItem!]!
}

input BduiDataInput {
  context: BduiContextInput!
  screenId: String!
  revision: Int!              # ревизия, под которой клиент рисует; сервер сверяет биндинг
  sources: [BduiDataRequest!]!  # максимум 12
}
input BduiDataRequest {
  id: String!                 # DataSource.id
  args: JSON                  # объект; допустимы ТОЛЬКО ключи из bdui_operation.pageable_args
  knownEtag: String
}
type BduiDataPayload { items: [BduiDataItem!]! }

input BduiCallInput {
  context: BduiContextInput!
  screenId: String!
  revision: Int!
  operationId: String!        # allowlist; резолвится в реальный handler
  args: JSON                  # объект
  formId: String              # для submit(formId) — только для аудита/валидации
  idempotencyKey: String      # UUID, генерируется клиентом
}

type BduiCallPayload {
  ok: Boolean!
  error: BduiError
  fieldErrors: [BduiFieldError!]!
  nextAction: JSON            # один Action (§5 [[BDUI]])
  replaceScreen: JSON         # новый ScreenDocument
  patch: JSON                 # {"state":{…},"data":{"<id>":<payload>}}
  invalidated: [String!]!     # dataId, которые сервер уже знает как протухшие
}

type BduiFieldError { stateKey: String!  message: String! }
type BduiError { code: String!  message: String!  retryable: Boolean! }
```

`fieldErrors` — транспортное представление карты **`stateKey → локализованное сообщение`** ([[BDUI]] §6.1): `stateKey` уникален в пределах ответа, порядок незначим. Список выбран вместо GraphQL-map только потому, что map-скаляра в схеме нет.

`invalidated` — авторитетный список протухших `dataId`; клиентский `call.invalidates` из документа — подсказка, при расхождении применяется объединение ([[BDUI]] §6.1).

`bduiCall` **обобщает** `submitBduiForm` из [[BDUI]] v1: сабмит формы — это `bduiCall` с непустым `formId`, отдельной мутации нет.

`message` в `BduiError` — **локализованная строка, готовая к показу** (прецедент: `ServiceMode.message` в `AppConfig`, `schema.graphqls:7894`, и весь error-envelope VYSOTA). Клиент не мапит коды в тексты.

## 2. Контекст запроса — что на клиенте есть, чего нет

Аргумент `context`, а не заголовки. Причина: заголовочная цепочка (`AuthenticatedInterceptorProvider.httpInterceptors`, `Modules/CoreNetworking/Sources/AuthTransport.swift:63-78`) — общая для всех операций, расширять её ради одной фичи дороже, чем прокинуть типизированный input; плюс REST-транспорт `/upload` через неё вообще не идёт.

| Поле | Источник на iOS сегодня | Готовность |
|---|---|---|
| `platform` | константа | есть |
| `osVersion`, `appVersion`, `appBuild` | зашиты только внутрь `User-Agent` (`RequestContextHeaders.swift:26-35`) | нужно вытащить в отдельные поля |
| `locale` | `RequestContextProvider.languageCode` → заголовок `language` | **есть, но обрезан (см. ниже)** |
| `brand` | **никак** — бэк не получает бренд ни в одном заголовке | **блокер** |
| `catalogId`, `schemaVersion`, `catalogVersion` | константы BDUI-модуля | появляются вместе с фичей |
| `timeZone` | `TimeZone.current.identifier` | тривиально |

### 2.1 Блокер «бренд» — ответ получен, блокер остался

**Ответ бэкенда 2026-08-25: «бренд определяется по доменному имени»**, отдельного поля/заголовка не будет.

Сегодня это не работает ни на одном окружении. Хостов всего четыре, и они общие для всех семи брендов (`Modules/PostboxCore/Sources/GraphQLEndpoint.swift`): `api.korpu.app` (`prodRussia`), `api3.korpu.app` (`prodGlobal`), `stage.korputeam.ru`, `dev.korputeam.ru`. `BrandConfig.graphQLEndpoint` дефолтится в `.stage` (`App/Sources/BrandConfig.swift:52`), ни один из семи `Apps/<brand>/BrandConfig+Current.swift` его не переопределяет ⇒ `Host` у Khidi и у Kuper одинаков, вывести из него бренд нельзя. Различить бренды сегодня можно только по bundle id внутри `User-Agent` и по `telemetryRegionCode`.

Чтобы ответ стал рабочим, нужно одно из двух:

1. **Per-brand домены.** От бэка — список хостов (7 брендов × окружения) и подтверждение, что за каждым тот же граф. От клиента — `graphQLEndpoint` в каждом `Apps/<brand>/BrandConfig+Current.swift`, а `GraphQLEndpoint` перестаёт быть набором четырёх статических констант. ≈0.5 дня после получения списка.
2. **Заголовок `brand`** — одна строка в `RequestContextHeaderInterceptor`, значение = slug из `Tuist/ProjectDescriptionHelpers/Brand.swift`. Доменному определению не мешает и снимает блокер немедленно, без ожидания инфраструктуры.

Пока нет ни того, ни другого, BDUI не может ни таргетировать ревизии по бренду (§5 шаг 7), ни валидировать токены брендовой палитры (§8.2).

### 2.2 Блокер «локаль»

`Modules/Localization/Sources/AppLanguage.swift:3` — `enum AppLanguage: String { case ru, en, az }`. Это **весь** набор. `languageCode` резолвится через `AppLanguage.allCases`, `BrandConfig.languages` фильтрует `CFBundleLocalizations` через тот же enum. Значит для Khidi (`ka`), Kuper (`tt`), Koprik (`uz`), Aura (`ab`) представимой локали **нет** — они получат `ru` или `en`.

Раз бэкенд отдаёт готовые локализованные строки ([[BDUI]] §17), это блокер именно BDUI, а не косметика: грузинский бренд получит русский промо-экран.

**Миграция, порядок:**
1. Расширить `AppLanguage` до `{ru, en, az, ka, tt, uz, ab}` (одна строка + каталоги строк для generic-хрома фоллбеков).
2. Бэк принимает `locale` как BCP-47 и хранит переводы по этому же ключу (`bdui_message.locale`).
3. Фоллбек локали на сервере: `locale` → базовый язык бренда → `ru`. Отсутствие перевода ключа = ошибка публикации (§8), а не рантайм-фоллбек.

**Заголовок подтверждён 2026-08-25:** сервер читает `language` — ровно тот, что клиент уже шлёт (`RequestContextHeaders.swift:166`). Транспорт менять не надо, блокер сводится к пункту 1 выше (расширение `AppLanguage`). Остаётся локальная мелочь на стороне бэка: докстринг `Holiday.name` (`schema.graphqls:3921`) — единственное место схемы, где обещан `Accept-Language`, которого клиент не шлёт вовсе; либо докстринг протух, либо праздники молча фоллбечатся вместо локализации.

## 3. Модель хранения (PostgreSQL)

Разделение, на котором держится §9: **фрейм** (структура + токены + действия) и **строки** живут в ревизии, **данные** не хранятся вообще — резолвятся на лету.

`{"$str":"key"}` — **серверная внутренняя форма хранения**, а не часть грамматики ([[BDUI]] §3.1): `render/` подставляет строку из `bdui_message` до отдачи, наружу `$str` не выходит ни при каких условиях, включая превью на устройстве (§13.4). Документ с `$str` на проводе клиент считает битым.

```sql
create table bdui_screen (
  screen_id   text primary key,                       -- 'promo.summer2026', 'raffle.rules'
  catalog_id  text not null default 'korpu',
  kind        text not null check (kind in ('screen','sheet','placement')),
  title       text not null,                          -- админский заголовок, не UI
  owner       text not null,
  is_enabled  boolean not null default true,
  created_at  timestamptz not null default now(),
  archived_at timestamptz
);

create table bdui_revision (
  screen_id           text    not null references bdui_screen(screen_id),
  revision            integer not null,               -- монотонная в пределах screen_id
  status              text    not null check (status in ('draft','review','published','archived')),
  is_default          boolean not null default false,
  frame               jsonb   not null,               -- ScreenDocument со ссылками {"$str":"key"}
  frame_hash          bytea   not null,               -- sha256 канонизированного frame
  schema_version      text    not null,               -- semver грамматики
  catalog_version_min text    not null,               -- ВЫЧИСЛЯЕТСЯ валидатором (§8.3)
  node_count          integer not null,
  max_depth           integer not null,
  byte_size           integer not null,
  source_ids          text[]  not null default '{}',  -- объявленные DataSource.id
  operation_ids       text[]  not null default '{}',  -- все operationId из action.call
  created_by          text    not null,
  created_at          timestamptz not null default now(),
  published_at        timestamptz,
  archived_at         timestamptz,
  notes               text,
  primary key (screen_id, revision)
);
create unique index bdui_revision_default
  on bdui_revision (screen_id) where status = 'published' and is_default;
create index bdui_revision_published
  on bdui_revision (screen_id, catalog_version_min) where status = 'published';

create table bdui_message (
  screen_id text    not null,
  revision  integer not null,
  key       text    not null,                          -- ключ из {"$str":"key"}
  locale    text    not null,                          -- BCP-47
  value     jsonb   not null,                          -- "строка" | {"one":…,"few":…,"many":…,"other":…}
  primary key (screen_id, revision, key, locale),
  foreign key (screen_id, revision) references bdui_revision(screen_id, revision) on delete cascade
);

create table bdui_placement (
  placement_id text primary key,
  surface      text    not null,                       -- 'feed.top', 'sidebar.card', 'profile.block'
  screen_id    text    not null references bdui_screen(screen_id),
  priority     integer not null default 100,           -- меньше = раньше
  is_enabled   boolean not null default true
);
create index bdui_placement_surface on bdui_placement (surface, priority) where is_enabled;

create table bdui_experiment (
  experiment_id text primary key,
  key           text not null unique,                  -- уходит в analytics.experiments
  status        text not null check (status in ('draft','running','stopped')),
  salt          text not null,                         -- фиксируется при создании, НЕ меняется
  variants      jsonb not null,                        -- [{"variant":"a","bps":5000}, …] сумма = 10000
  created_at    timestamptz not null default now()
);

create table bdui_segment (
  segment_id text primary key,
  key        text not null unique,
  predicate  jsonb not null,                           -- та же грамматика предикатов, над bdui_user_attributes.attrs
  updated_at timestamptz not null default now()
);

create table bdui_user_attributes (
  user_id    bigint primary key,
  attrs      jsonb not null,                           -- {"level":12,"isNew":false,"hasDating":true,"tickets":340}
  updated_at timestamptz not null default now()
);
create index bdui_user_attributes_gin on bdui_user_attributes using gin (attrs jsonb_path_ops);

create table bdui_targeting_rule (
  rule_id       bigserial primary key,
  screen_id     text    not null references bdui_screen(screen_id),
  revision      integer,                               -- null ⇒ дефолтная published-ревизия
  priority      integer not null,                      -- меньше = раньше
  predicate     jsonb   not null,                      -- над context + segment + user attrs
  experiment_id text    references bdui_experiment(experiment_id),
  variant       text,
  rollout_bps   integer not null default 10000 check (rollout_bps between 0 and 10000),
  starts_at     timestamptz,
  ends_at       timestamptz,
  is_enabled    boolean not null default true,
  foreign key (screen_id, revision) references bdui_revision(screen_id, revision)
);
create index bdui_targeting_rule_lookup on bdui_targeting_rule (screen_id, priority) where is_enabled;

create table bdui_operation (
  operation_id  text primary key,                      -- 'raffle.claimDailyBonus'
  kind          text not null check (kind in ('read','write')),
  handler       text not null,                         -- имя в реестре Go, проверяется на старте
  args_schema   jsonb not null,                        -- JSON Schema аргументов
  scopes        text[] not null default '{}',
  requires_auth boolean not null default true,
  rate_limit    text,                                  -- '10/1m', '1/1s'
  cache_ttl_s   integer,                               -- только read
  refresh_s     integer,                               -- только read; уходит в BduiDataItem.refreshSeconds
  pageable_args text[] not null default '{}',          -- какие args клиенту можно переопределять
  invalidates   text[] not null default '{}',          -- какие read-operationId протухают после write
  is_enabled    boolean not null default true
);

create table bdui_idempotency (
  user_id         bigint  not null,
  operation_id    text    not null,
  idempotency_key text    not null,
  request_hash    bytea   not null,
  status          text    not null check (status in ('in_flight','done')),
  response        jsonb,
  created_at      timestamptz not null default now(),
  expires_at      timestamptz not null,
  primary key (user_id, operation_id, idempotency_key)
);
create index bdui_idempotency_gc on bdui_idempotency (expires_at);

create table bdui_audit (
  audit_id   bigserial primary key,
  actor      text not null,
  action     text not null,                            -- 'revision.publish', 'rule.update', …
  entity     text not null,
  before     jsonb,
  after      jsonb,
  at         timestamptz not null default now()
);
create index bdui_audit_entity on bdui_audit (entity, at desc);
```

**Чего здесь намеренно нет:** таблицы `bdui_form_submission` с полями формы. Значения сабмита пишет доменный handler в свою таблицу; BDUI хранит только `bdui_idempotency` (ключ + хэш запроса + ответ). Иначе персональные данные растекаются по инфраструктурной таблице, которую читает вся админка.

## 4. Ревизии и публикация

```
draft ──(submit)──► review ──(approve+publish)──► published ──(archive)──► archived
  ▲                    │                              │
  └───(reject)─────────┘                              └──(publish другой ревизии как default)
```

| Правило | Как обеспечено |
|---|---|
| `revision` монотонна | `insert … select coalesce(max(revision),0)+1 … for update` на строке `bdui_screen` |
| Опубликованных ревизий может быть несколько | таргетинг выбирает по версии клиента (§5); `is_default` — ровно одна (partial unique index) |
| Публикация атомарна | одна транзакция: `update … set status='published'`, снятие старого `is_default`, вставка в `bdui_audit` |
| Откат | **не** меняет статус старой ревизии, а **копирует** её в новую (`revision+1`) и публикует; история линейна, ETag'и старых ревизий не переиспользуются |
| Черновик не отдаётся | `getBduiScreen` смотрит только `published`, кроме превью-токена (§13.4) |
| Публикация без валидации невозможна | §8 гоняется в той же транзакции, `catalog_version_min`/`node_count`/`max_depth`/`byte_size` записываются её результатом |
| Архивация | `archived_at`, `is_default` снимается; запросы на архивную ревизию ⇒ `NOT_FOUND` |

## 5. Таргетинг: выбор ревизии

Порядок — жёсткий, первый прошедший выигрывает. Все шаги детерминированы (никаких `random`, никакого «времени последнего показа» в горячем пути).

| # | Шаг | Не прошёл ⇒ |
|---|---|---|
| 1 | `bdui_screen.is_enabled and archived_at is null` | `NOT_FOUND` |
| 2 | `bdui_screen.catalog_id == context.catalogId` | `NOT_FOUND` |
| 3 | Кандидаты = `published`-ревизии экрана | `NOT_FOUND` |
| 4 | `major(revision.schema_version) <= major(context.schemaVersion)` | отброшена |
| 5 | `semver(revision.catalog_version_min) <= semver(context.catalogVersion)` | отброшена |
| 6 | Правила `bdui_targeting_rule` по `priority asc`: временное окно `starts_at/ends_at` | правило пропущено |
| 7 | `predicate` над `{context.*, segments, user.attrs}` | правило пропущено |
| 8 | Эксперимент: `bucket(salt, subject) → variant`, совпадает с `rule.variant` | правило пропущено |
| 9 | Раскатка: `bucket(rule_id, subject) < rollout_bps` | правило пропущено |
| 10 | Ни одно правило не подошло ⇒ `is_default`-ревизия, если она прошла 4–5 | `UNSUPPORTED_VERSION` |

**Детерминизм бакета:**

```go
func bucket(salt string, subject string) int { // 0..9999
    h := crc32.Checksum([]byte(salt+":"+subject), crc32.MakeTable(crc32.Castagnoli))
    return int(h % 10000)
}
```

`subject` = `userId` для авторизованного, иначе стабильный `deviceId` (на iOS — `X-Device-Key-ID` из App Attest, где он есть, иначе IDFV). `salt` фиксируется при создании эксперимента и **не меняется** — иначе пользователи перепрыгивают между вариантами.

Шаги 4–5 — это и есть гарантия «клиент не получит незнакомый узел»: `catalog_version_min` вычислена из фактических узлов ревизии (§8.3), поэтому старому клиенту физически не может достаться документ с новым `block.*`.

**Сегменты** резолвятся не в SQL-предикате правила, а заранее: `bdui_user_attributes.attrs` тянется одним `select` по `user_id` (или из Redis, TTL 60 s), сегменты вычисляются в Go тем же интерпретатором предикатов, что и `visibleWhen` — одна реализация, одна таблица тестов.

## 6. Резолв data-источников

### 6.1 Реестр

`bdui_operation.kind='read'` → `handler` → зарегистрированный в Go резолвер:

```go
type ReadResolver interface {
    Resolve(ctx context.Context, req ReadRequest) (json.RawMessage, ReadMeta, error)
}
type ReadRequest struct {
    OperationID string
    Args        map[string]any // уже провалидированы по args_schema
    Ctx         RequestContext // brand, locale, timeZone, userID, platform
}
type ReadMeta struct { ETag string; TTL, Refresh time.Duration }
```

Реестр наполняется на старте (`registry.MustRegister("raffle.leaderboard", raffle.Leaderboard{})`); если в БД есть `handler`, которого нет в реестре, процесс **не стартует** (fail-fast, не рантайм-500).

### 6.2 Fan-out

```
errgroup.WithContext(ctx)  →  на каждый DataSource:
  ctx с per-source deadline = min(binding.timeout, остаток бюджета документа)
  singleflight по ключу (operationId, canonicalArgs, userID)
  redis GET  → hit ⇒ отдать;  miss ⇒ resolver → redis SET ttl=cache_ttl_s
```

| Параметр | Значение | Обоснование |
|---|---|---|
| Бюджет документа | 1200 ms | первый кадр не должен быть медленнее нативного |
| Таймаут источника по умолчанию | 500 ms | 2 последовательных ретрая не влезают — их и нет |
| Параллельность | все источники сразу, ≤ 12 (§10) | легаси-раффл делал ~15 последовательно-параллельных round trip'ов; здесь один |
| Ретраи | нет | ретраит клиент по `refreshSeconds`/pull-to-refresh |

### 6.3 Батчинг

Источники, у которых `handler` совпадает и `args` различаются только по `pageable_args`, склеиваются в один вызов доменного сервиса, если резолвер реализует `BatchReadResolver`. Обязательно для раффла: `GetUsersForRaffle(userIds:[Int!]!)` иначе превращается в N+1 (в Android-клиенте это ровно так и есть — `getTicketHistory` делает `GetUserQuery` на каждого).

### 6.4 Деградация

| Ситуация | Ответ |
|---|---|
| `required: true` источник упал/таймаут | `status: TEMPORARILY_UNAVAILABLE`, `document: null` — клиент показывает кэш, иначе нативный фоллбек |
| `required: false` упал | `status: OK`, `BduiDataItem{ok:false, error}` — узлы на этом источнике деградируют по `fallback` |
| Бюджет документа исчерпан | как `required` упал, если недорезолвлен хоть один `required`; иначе OK с частичными данными |
| Источник вернул JSON больше лимита | `ok:false`, code `BDUI_DATA_TOO_LARGE`, метрика |

### 6.5 Что резолвер обязан посчитать сам

Грамматика [[BDUI]] запрещает арифметику и условные выражения. Всё, что легаси считал на устройстве, — работа резолвера. Для раффла это (источники — `legacy-raffle.md` §5, `android-giveaway.md` §5):

- `ticketsEarned = count × RaffleTaskRule.ticketsCount` — сервер никогда не отдавал произведение, оба клиента множили сами;
- `place = offset + index + 1` — абсолютный номер строки лидерборда;
- максимум прогресс-бара (`max(firstPlacePoints, myTickets)`);
- дедуп промо-правил (`isPromo` + `taskType`), дедуп картинок призов;
- маскирование скрытых участников (`userId == null` ⇒ `"**********"`), фоллбек имени `Пользователь #N`;
- формат дат/обратного отсчёта — строкой, в `context.timeZone`;
- плюрализация: резолвер отдаёт `{"$plural":{"count":…,"forms":{…}}}`, формы берутся из `bdui_message`.

## 7. Реестр операций записи (`bduiCall`)

### 7.1 Пайплайн

```
1. bdui_operation[operationId] существует, is_enabled, kind='write'      → BDUI_OPERATION_UNKNOWN
2. operationId ∈ bdui_revision(screenId, revision).operation_ids         → BDUI_OPERATION_NOT_BOUND
3. requires_auth ⇒ есть userID; scopes ⊆ права пользователя             → BDUI_FORBIDDEN
4. args валидны по args_schema                                          → BDUI_ARGS_INVALID / fieldErrors
5. rate limit (Redis token bucket, ключ user+operationId)               → BDUI_RATE_LIMITED (retryable)
6. идемпотентность: reserve                                             → см. 7.2
7. handler.Execute(ctx, args) → конверт
8. idempotency commit / rollback
9. invalidated = bdui_operation.invalidates ∩ revision.source_ids
```

Шаг 2 — важный: даже валидный `operationId` нельзя дёрнуть с чужого экрана. Экран объявляет свой список операций на публикации.

### 7.2 Идемпотентность

Двухфазная, зеркалит `PostboxPushReceiptDeduplicator` (`Modules/AppCore/Sources/Push/PushReceiptDeduplicator.swift:24`) — reserve → commit только на успехе → rollback на ошибке.

```sql
insert into bdui_idempotency (user_id, operation_id, idempotency_key, request_hash, status, expires_at)
values ($1,$2,$3,$4,'in_flight', now() + interval '24 hours')
on conflict (user_id, operation_id, idempotency_key) do nothing
returning 1;
```

- вставка прошла ⇒ выполняем handler; успех ⇒ `status='done', response=<конверт>`; ошибка ⇒ `delete` (повтор разрешён);
- конфликт + `status='done'` + `request_hash` совпал ⇒ **отдаём сохранённый конверт**, handler не вызывается;
- конфликт + `status='done'` + хэш другой ⇒ `BDUI_IDEMPOTENCY_MISMATCH`, non-retryable;
- конфликт + `status='in_flight'` ⇒ `BDUI_CALL_IN_FLIGHT`, **retryable**;
- `idempotencyKey` не передан и `kind='write'` ⇒ `BDUI_IDEMPOTENCY_REQUIRED`. Клиент обязан минтить UUID в редьюсере вместе с in-flight состоянием (прецедент: `ChatReducer` минтит `idempotencyKey: localID`, `Modules/Chat/Domain/Sources/Reducer/ChatReducer.swift:361`).
- GC: `delete from bdui_idempotency where expires_at < now()` по крону, батчами.

### 7.3 Что кладём в конверт

| Поле | Когда |
|---|---|
| `patch.state` | оптимистичный/подтверждённый локальный стейт (тумблер видимости в раффле: `{"hidden": true}`) |
| `patch.data` | сервер уже знает новое значение источника и экономит round-trip (после `raffle.claimDailyBonus` — свежий агрегат `raffle.overview`) |
| `invalidated` | источники протухли, но пересчитывать дорого — клиент дёрнет `getBduiData` |
| `nextAction` | `dismiss`, `open`, `pushScreen` — одноходовые сценарии |
| `replaceScreen` | серверный визард (следующий шаг анкеты) |
| `fieldErrors` | ошибки полей формы, ключ = `stateKey` узла |

Правило: **`patch` предпочтительнее `invalidated`, `invalidated` предпочтительнее `replaceScreen`.** `replaceScreen` перерисовывает весь экран и стирает локальный стейт — оправдан только для визардов.

### 7.4 Стартовый реестр пилота

Имена — канон пилота ([[BDUI-Raffle-Pilot]] §3); реестр и документ пилота обязаны совпадать посимвольно, иначе `call` падает на шаге 1 §7.1.

| operationId | kind | handler | args | cache_ttl_s / refresh_s | rate | invalidates |
|---|---|---|---|---|---|---|
| `raffle.overview` | read | `raffle.Overview` | — | 30 / 30 | — | — |
| `raffle.leaderboard` | read | `raffle.Leaderboard` | `{limit,offset}` (pageable) | 30 / 30 | — | — |
| `raffle.rules` | read | `raffle.Rules` | — | 900 / — | — | — |
| `raffle.claimDailyBonus` | write | `raffle.ClaimDaily` | `{}` | — | `5/1m` | `raffle.overview` |
| `raffle.setVisibility` | write | `raffle.SetVisibility` | `{visible:bool}` | — | `10/1m` | `raffle.leaderboard`, `raffle.overview` |

`raffle.overview` — **один** агрегат вместо ~10 отдельных запросов, которые Android-клиент делает каждые 5 s (`REALTIME_REFRESH_INTERVAL_MS = 5_000L`): в него входят снапшот пользователя, задания, ежедневный бонус и призы (прежние `raffle.snapshot`/`raffle.tasks`/`raffle.prizes` отдельными операциями не заводятся — это его внутренние источники). `cache_ttl_s = 30`, `refresh_s = 30` даёт ≤ 4 запроса/мин на пользователя — критерий приёмки пилота ([[BDUI-Raffle-Pilot]] §7 п.10). Секундная живость последних минут розыгрыша — не поллинг, а отдельная подписка `raffleSnapshot`, которой сегодня не существует ни на одной платформе (пилот §5 п.5).

## 8. Валидация на записи

Гоняется в транзакции публикации; те же проверки доступны в админке как «проверить черновик».

### 8.1 JSON Schema

`contracts/schema/screen-document.schema.json` (JSON Schema 2020-12), библиотека `santhosh-tekuri/jsonschema/v6`. Схема компилируется один раз на старте; `additionalProperties: false` **везде** — незнакомое поле = ошибка, а не молчание.

### 8.2 Семантические проверки

| # | Проверка | Код |
|---|---|---|
| 1 | `node.type` есть в реестре каталога для `catalog_id` | `BDUI_UNKNOWN_NODE` |
| 2 | `props` узла валидны по props-схеме этого типа | `BDUI_PROPS_INVALID` |
| 3 | Каждый токен (цвет/типографика/spacing/радиус) есть в `contracts/catalog/tokens.json` | `BDUI_UNKNOWN_TOKEN` |
| 4 | `{"$data":"sourceId/ptr"}` ссылается на объявленный в `data[]` источник | `BDUI_DANGLING_DATA_REF` |
| 5 | RFC 6901-указатель синтаксически валиден | `BDUI_BAD_POINTER` |
| 6 | `{"$state":"k"}` — ключ есть в `state` или пишется каким-то `patchState`/формой | `BDUI_DANGLING_STATE_REF` |
| 7 | `{"$item"}`/`{"$index"}` — только внутри `repeat` | `BDUI_ITEM_REF_OUTSIDE_REPEAT` |
| 7a | `{"$event"}` — только внутри `on.change` узла, который его несёт ([[BDUI]] §3.1) | `BDUI_EVENT_REF_OUTSIDE_CHANGE` |
| 8 | Вложенность `repeat` ≤ 1 | `BDUI_REPEAT_NESTING` |
| 9 | `action.call.operationId` есть в `bdui_operation`, `is_enabled`, `kind='write'` | `BDUI_OPERATION_UNKNOWN` |
| 10 | `action.open.url`: **литеральный** URL — схема из allowlist (`https` + брендовые схемы) на записи; `open.url`-Value (`$data`/`$item`) на записи непроверяем, поэтому тот же allowlist применяет резолвер источника на отдаче **и** клиент перед открытием | `BDUI_URL_NOT_ALLOWED` |
| 11 | `pushScreen/presentSheet` ссылается на существующий `screen_id` того же `catalog_id` | `BDUI_DANGLING_SCREEN_REF` |
| 12 | `node.id` уникален в пределах документа | `BDUI_DUPLICATE_NODE_ID` |
| 13 | Нет циклов (дерево, а не граф — проверяется при разборе `children`) | `BDUI_CYCLE` |
| 14 | Лимиты §10 не превышены | `BDUI_LIMIT_EXCEEDED` |
| 15 | Каждый `{"$str":"key"}` имеет перевод на **все** локали брендов, которым экран может быть выдан | `BDUI_MISSING_TRANSLATION` |
| 16 | `$plural.forms` содержит все CLDR-категории, обязательные для этих локалей (ru: one/few/many/other) | `BDUI_MISSING_PLURAL_FORM` |
| 17 | `submit(formId)` — существует форма с таким id и хотя бы одним input-узлом | `BDUI_DANGLING_FORM_REF` |
| 18 | `surface.frosted` не вложен в `surface.frosted` ни на какой глубине ([[BDUI-Catalog]] §3.3) | `BDUI_FROSTED_NESTING` |
| 18 | `visibleWhen`-предикат типово согласован (сравнение числа со строкой — ошибка) | `BDUI_PREDICATE_TYPE` |

### 8.3 Вычисление `catalog_version_min`

```
catalog_version_min = max( catalog.nodes[t].sinceVersion  для каждого использованного t )
                      ∪ max( catalog.tokens[k].sinceVersion для каждого использованного k )
                      ∪ max( catalog.props[t][p].sinceVersion … )
```

Автор ревизии **не может** указать её руками. Это инвариант №2 сервиса: старый клиент не получает новый узел, потому что число посчитано из содержимого, а не заявлено.

## 9. Кэш и отдача

Два уровня, потому что фрейм меняется раз в неделю, а данные раффла — раз в 5 секунд.

| Уровень | Ключ | TTL | Инвалидация |
|---|---|---|---|
| Фрейм (рендер `frame` + строки локали) | `(screen_id, revision, locale, brand)` | до публикации новой ревизии | публикация/архивация |
| Данные | `(operation_id, canonicalArgs, userID?)` | `bdui_operation.cache_ttl_s` | `invalidates` после `bduiCall` |
| Резолв таргетинга | `(screen_id, hash(контекст-релевантных полей), subject)` | 60 s | публикация правил |

**ETag.** `etag = base64url(sha256(rendered_frame_bytes))`. Клиент шлёт `knownEtag`; совпал ⇒ `status: NOT_MODIFIED, document: null`, но `data` **всё равно резолвится и отдаётся** — иначе 304 бесполезен для живого экрана. `revision` возвращается всегда: это дешёвый skip перерисовки на клиенте.

**HTTP.** GraphQL идёт POST'ом, поэтому HTTP-кэш не работает; `Cache-Control` ставится только на CDN-путь.

**CDN.** Через CDN можно отдавать **только** документ, который не зависит от пользователя: нет `required` источников с `userID`, нет сегментов/экспериментов в выбравшем правиле, `requires_auth=false`. Такой ответ помечается `Cache-Control: public, s-maxage=<frameTtl>, stale-while-revalidate=600` и ключуется `(screenId, brand, locale, schemaVersion, catalogVersion, platform)`. Всё остальное — `Cache-Control: private, no-store`. Практически под CDN попадают промо-фреймы и правила раффла; лидерборд — никогда.

**Клиентский кэш — [[BDUI]] §11**, здесь не переопределяется.

## 10. Лимиты и защита

**Лимиты документа** (размер, глубина, число узлов, `repeat.limit`, число источников, ключей `state`, размер `args` и data-пейлоада) — **значения см. [[BDUI]] §10**, здесь они не переопределяются. Сервер проверяет их на публикации, а размер и пейлоады — ещё и на отдаче/резолве.

Лимиты самого транспорта и защита — только здесь:

| Лимит | Значение | Проверяется |
|---|---|---|
| `surfaces` в `getBduiPlacements` | 16 | на запросе |
| `sources` в `getBduiData` | 12 | на запросе |
| Rate limit `getBduiScreen` | 60/min на пользователя | Redis token bucket |
| Rate limit `getBduiData` | 120/min на пользователя | там же (`refresh_s = 30` ⇒ 2/min на источник) |
| Rate limit `bduiCall` | из `bdui_operation.rate_limit`, дефолт 30/min | там же |
| Глубина рекурсии парсера | hard cap 32, паника невозможна | `validate` |

Единственный источник числовых лимитов — `contracts/schema/limits.json`: оттуда они попадают и в бэк (кодоген), и в оба клиента, а таблица [[BDUI]] §10 — его человеко-читаемое зеркало. Изменение лимита = bump minor схемы.

## 11. Коды ошибок

`BduiError.code` — стабильный machine-readable, `message` — локализованная строка для пользователя (пустая для кодов, которые пользователю не показывают).

| Код | HTTP-аналог | retryable | Что делает клиент |
|---|---|---|---|
| `BDUI_SCREEN_NOT_FOUND` | 404 | нет | нативный фоллбек / пустой placement |
| `BDUI_SCREEN_DISABLED` | 200 | нет | ничего не рисует (не ошибка) |
| `BDUI_UNSUPPORTED_VERSION` | 200 | нет | нативный фоллбек, телеметрия «клиент устарел» |
| `BDUI_TEMPORARILY_UNAVAILABLE` | 503 | **да** | показать кэш; нет кэша — фоллбек с retry |
| `BDUI_DATA_SOURCE_FAILED` | 200 | **да** | деградация узлов по `fallback`, retry по `refreshSeconds` |
| `BDUI_DATA_TOO_LARGE` | 200 | нет | как `DATA_SOURCE_FAILED`, но без retry |
| `BDUI_REVISION_MISMATCH` | 409 | **да** | перезапросить `getBduiScreen` (документ обновился под руками) |
| `BDUI_OPERATION_UNKNOWN` | 400 | нет | `assertionFailure` в debug, тихая деградация в release |
| `BDUI_OPERATION_NOT_BOUND` | 403 | нет | то же |
| `BDUI_ARGS_INVALID` | 400 | нет | показать `fieldErrors`, иначе общий баннер |
| `BDUI_FORBIDDEN` | 403 | нет | общий баннер |
| `BDUI_UNAUTHENTICATED` | 401 | **да** (после refresh) | существующий single-flight refresh, один ретрай |
| `BDUI_RATE_LIMITED` | 429 | **да** | backoff, значение из `Retry-After`-аналога в `message` |
| `BDUI_IDEMPOTENCY_REQUIRED` | 400 | нет | баг клиента (write без ключа; в документе плейсхолдеры вроде `"auto"` запрещены — [[BDUI]] §6.2) |
| `BDUI_IDEMPOTENCY_MISMATCH` | 409 | нет | баг клиента, не ретраить |
| `BDUI_CALL_IN_FLIGHT` | 409 | **да** | подождать, не дублировать кнопку |
| `BDUI_INTERNAL` | 500 | **да** | общий баннер + retry |

Клиентское правило одно: **`retryable: false` никогда не ретраится автоматически**, даже по pull-to-refresh для того же запроса.

## 12. Наблюдаемость

**Метрики (Prometheus).** Лейблы везде: `screen_id`, `brand`, `platform`, `catalog_version`; на `bduiCall` дополнительно `operation_id`.

| Метрика | Тип | Зачем |
|---|---|---|
| `bdui_screen_request_duration_seconds` | histogram | SLO первого кадра, p95 < 800 ms |
| `bdui_screen_status_total{status}` | counter | доля `UNSUPPORTED_VERSION` = сколько клиентов отстало |
| `bdui_data_resolve_duration_seconds{operation_id}` | histogram | какой источник тормозит документ |
| `bdui_data_resolve_failures_total{operation_id,required}` | counter | деградации |
| `bdui_budget_exhausted_total` | counter | бюджет 1200 ms не влезает |
| `bdui_cache_hit_ratio{layer}` | gauge | фрейм / данные / таргетинг |
| `bdui_etag_not_modified_total` | counter | реальная польза 304 |
| `bdui_call_total{operation_id,result}` | counter | конверсия сабмитов |
| `bdui_idempotency_replay_total` | counter | сколько дублей гасим |
| `bdui_validation_failures_total{code}` | counter | качество админки |
| `bdui_node_degraded_total{node_type,reason}` | counter | **репортит клиент** отдельным `track`; главный сигнал рассинхрона каталога |
| `bdui_publish_total{result}` | counter | частота публикаций |

**Логи.** Структурные (slog), поля: `request_id`, `screen_id`, `revision`, `operation_id`, `user_id`, `brand`, `locale`, `duration_ms`, `status`.

**Логировать НЕЛЬЗЯ:** значения полей форм (`args` write-операций, `values`, `patch.state`), содержимое `bdui_user_attributes`, `Authorization`, `X-Device-Assertion`. В логи идёт `sha256(args)` и список **ключей** без значений. Это же правило действует для Sentry-контекста и для трейс-атрибутов.

**Трейсинг (OTel).** Спан на запрос → дочерний спан на резолв каждого источника (`bdui.data.resolve`, атрибуты `operation_id`, `cache_hit`, `batched`) → спан на handler `bduiCall`. Trace-id прокидывается в ответ (`extensions.traceId`), чтобы баг-репорт с клиента был находим.

## 13. Админка / CMS

### 13.1 API

Отдельная поверхность (`cmd/bdui-admin`), **не** публичный субграф: другой origin, другая авторизация (SSO сотрудников), другой rate limit.

```
GET    /admin/screens                       список + фильтры
POST   /admin/screens                       создать экран
GET    /admin/screens/{id}/revisions        история
POST   /admin/screens/{id}/revisions        создать draft (можно из существующей ревизии — «клон»)
PUT    /admin/screens/{id}/revisions/{n}    правка draft
POST   /admin/screens/{id}/revisions/{n}/validate   §8 без записи
POST   /admin/screens/{id}/revisions/{n}/review     draft → review
POST   /admin/screens/{id}/revisions/{n}/publish    review → published (+ is_default)
POST   /admin/screens/{id}/rollback         копия указанной ревизии в новую + publish
POST   /admin/screens/{id}/revisions/{n}/archive
GET/PUT /admin/screens/{id}/revisions/{n}/messages/{locale}   переводы
CRUD   /admin/placements, /admin/rules, /admin/experiments, /admin/segments
GET    /admin/operations                    реестр (read-only из БД+кода)
POST   /admin/preview                       выдать preview-токен
GET    /admin/audit                         журнал
GET    /admin/catalog                       текущий catalog.json + tokens.json (для конструктора)
```

### 13.2 Роли

| Роль | Права |
|---|---|
| `viewer` | чтение всего, кроме `bdui_user_attributes` |
| `translator` | правка `bdui_message` в `draft`/`review` |
| `editor` | CRUD draft, validate, preview |
| `reviewer` | draft → review → отклонить |
| `publisher` | publish, rollback, archive, правка правил таргетинга и раскатки |
| `admin` | эксперименты, сегменты, `bdui_operation`, роли |

Публикация требует `reviewer ≠ author` (четыре глаза). Всё пишется в `bdui_audit`.

### 13.3 Конструктор

Админка строит формы из `contracts/catalog/catalog.json`: узел → его props-схема → контролы; токены — выпадающие списки из `tokens.json` (свободный ввод цвета/размера невозможен по построению). Предпросмотр дерева — веб-рендер по тому же каталогу, но он **не является** источником истины: истина — превью на устройстве.

### 13.4 Превью на устройстве

1. Редактор в админке жмёт «Превью» → `POST /admin/preview {screenId, revision, deviceId?}` → короткоживущий токен (TTL 15 мин, одноразовый привязкой к `user_id` или к отсканированному QR с `deviceId`).
2. Токен уходит в приложение диплинком `korpu://bdui/<screenId>?preview=<token>` (диплинк-слой на iOS — `RootExternalRouteParser`, `Modules/AppCore/Sources/State/RootExternalRoute.swift`).
3. Клиент кладёт токен в `BduiContextInput.previewToken`.
4. Сервер: токен валиден и не истёк ⇒ отдаёт **указанную в токене ревизию** независимо от статуса и таргетинга; `frameTtlSeconds = 0`, `Cache-Control: no-store`, ответ помечен `X-BDUI-Preview: 1`, CDN исключён.
5. Превью-выдача пишется в аудит и не попадает в продуктовые метрики (лейбл `preview=true` отсекается в рекординге).

## 14. Раскладка Go

```
cmd/
  bdui-api/          публичный субграф (getBduiScreen/Placements/Data, bduiCall)
  bdui-admin/        админ-API
  bdui-migrate/      миграции + сид реестра операций из contracts
internal/bdui/
  contract/          сгенерированные из contracts-репо типы: узлы, токены, лимиты, коды ошибок
  model/             доменные структуры БД (sqlc-генерат + обвязка)
  store/             pgx/v5 репозитории: screens, revisions, messages, rules, idempotency, audit
  validate/          JSON Schema + §8.2 семантика + §8.3 catalog_version_min
  render/            frame + messages(locale) + params → готовый JSON документа; ETag
  target/            §5: отбор ревизии, предикаты, сегменты, бакеты экспериментов
  predicate/         ОДИН интерпретатор предикатов (visibleWhen, targeting, segments)
  resolve/           реестр read-резолверов, fan-out, singleflight, батчинг, бюджет
  call/              реестр write-операций, идемпотентность, rate limit, конверт
  cache/             Redis: фреймы, данные, таргетинг; ключи и инвалидация
  admin/             хендлеры админки, роли, preview-токены
  telemetry/         метрики, логгер с редактированием секретов, OTel
  gql/               gqlgen resolvers — тонкий слой, только маппинг DTO
```

| Задача | Библиотека | Почему она |
|---|---|---|
| GraphQL-субграф | `99designs/gqlgen` (+ `federation`) | schema-first, тот же supergraph |
| JSON Schema 2020-12 | `santhosh-tekuri/jsonschema/v6` | единственный полноценный 2020-12 в Go, компилируется один раз |
| Postgres | `jackc/pgx/v5` + `sqlc` | без ORM, запросы видно глазами |
| Миграции | `pressly/goose` | простые SQL-файлы, ArgoCD-friendly |
| Redis | `redis/go-redis/v9` | кэш + token bucket |
| Fan-out | `golang.org/x/sync/errgroup` + `singleflight` | stdlib-адъюнкт, ничего своего |
| Semver | `Masterminds/semver/v3` | сравнение диапазонов в §5 |
| Наблюдаемость | `prometheus/client_golang`, `otel`, `log/slog` | как в остальном стеке |

**Кодоген из contracts-репо.** `go generate ./internal/bdui/contract` читает `contracts/catalog/catalog.json`, `tokens.json`, `operations/registry.json`, `schema/limits.json` и эмитит:
- `nodes_gen.go` — `map[NodeType]NodeSpec{SinceVersion, PropsSchemaRef}`;
- `tokens_gen.go` — множества допустимых токенов + `SinceVersion`;
- `limits_gen.go` — константы §10;
- `errors_gen.go` — коды §11 и их `retryable`.

Схемы (`*.schema.json`) **встраиваются** через `go:embed`, а не читаются с диска — иначе прод разъедется с контрактом. Версия контракта пишется в `/health` и в лейбл метрик, чтобы было видно, какой бэк какой каталог понимает.

Сид `bdui_operation` из `operations/registry.json` — миграцией (`bdui-migrate`), не руками в проде.

## 15. contracts-репо

### 15.1 Раскладка

```
contracts/
  VERSION                       # semver контракта в целом
  CHANGELOG.md
  schema/
    screen-document.schema.json # грамматика [[BDUI]]: Node, Value, Action, предикаты
    limits.json                 # §10
    errors.json                 # §11
  catalog/
    catalog.json                # узлы: type → {sinceVersion, props: {…}, platforms: {ios, android}}
    tokens.json                 # цвета/типографика/spacing/радиусы как закрытые enum
    profiles/korpu.json         # какие узлы/токены входят в catalogId=korpu
    profiles/vysota.json        # то же для omniapp (см. [[BDUI-Catalog]])
  operations/
    registry.json               # operationId → {kind, argsSchema, scopes, rateLimit, pageableArgs, invalidates}
  fixtures/
    golden/*.json               # эталонные ScreenDocument
    expected/ios/*.png          # снапшоты рендерера
    expected/android/*.png
    invalid/*.json              # документы, которые ОБЯЗАНЫ быть отвергнуты (+ ожидаемый код §8.2)
  bindings/
    bdui.graphqls               # SDL §1.3 — источник и для клиентов, и для бэка
```

### 15.2 Semver

| Изменение | Bump |
|---|---|
| Новый узел / новый props-ключ (optional) / новый токен | **minor** каталога |
| Новый обязательный props-ключ, удаление узла/токена, смена типа props | **major** каталога |
| Новый оператор предиката, новое действие, новый вид `Value` | **minor** схемы |
| Смена формы `Node`/`ScreenDocument`, удаление действия | **major** схемы |
| Смена лимита | **minor** схемы (клиенты читают лимиты из контракта) |
| Новая `operationId` | **patch** — реестр операций версионируется отдельно от каталога |

`schemaVersion` и `catalogVersion` — **разные** версии и бампаются независимо: расширение каталога не должно ломать грамматику.

Обе — **строго semver** (`MAJOR.MINOR.PATCH`); календарные версии (`2026.09.0`) запрещены и отвергаются валидатором на записи. Текущее значение каталога — `2.0.0` ([[BDUI-Catalog]]).

Major схемы = документ отвергается клиентом до рендера ([[BDUI]] §10), поэтому major планируется как событие, а не как побочный эффект.

### 15.3 `catalogId`

`korpu` (Stratoapp + most) и `vysota` (omniapp) имеют разные токены и разный состав узлов: omniapp — dark-only, `DSColor.background/textPrimary/danger` вместо `bg1/el16/error`, нет `card`/`banner`/`list.section`, нет диплинк-слоя вообще. Один документ на оба продукта не поедет. Поэтому: **схема одна, профили каталога разные**, `catalogId` — обязательное поле контекста и колонка `bdui_screen.catalog_id`; шаг 2 таргетинга (§5) отсекает чужой профиль. Детали профилей — [[BDUI-Catalog]].

⚠️ `platform` **не различает Stratoapp и omniapp** — оба `IOS`, третьего значения в `BduiPlatform` нет и не будет (omniapp — iOS-приложение). Единственный различитель продуктов — `catalogId`, поэтому шаг 2 таргетинга обязателен, а не опционален: без него документ `korpu` уедет в `vysota` и не отрисуется по токенам.

### 15.4 CI

| Потребитель | Что гоняет |
|---|---|
| contracts | схема валидна; каждая golden-фикстура проходит схему; каждая `invalid/*.json` падает с **ожидаемым** кодом; `catalog.json` ↔ `tokens.json` ссылочная целостность; semver-diff-гейт (major требует ручного подтверждения в MR) |
| бэк | `go generate` + `git diff --exit-code` (генерат не дрейфует); `validate` на всех фикстурах даёт те же коды; реестр handler'ов покрывает `operations/registry.json` |
| iOS | strict decode всех golden-фикстур (незнакомое поле = fail); snapshot-матрица рендерера по брендам/темам против `expected/ios`; таблица тестов интерпретатора предикатов |
| Android | kotlinx strict decode тех же фикстур; Compose screenshot против `expected/android` |

Расширение каталога = один MR в contracts (bump minor + фикстура + оба `expected/`) → зелёный CI всех трёх → релизы клиентов → только после этого узел разрешён к использованию в админке (она читает `catalog.json` из задеплоенной версии бэка).

Пока contracts-репо не выделен, источник — `docs/BDUI*.md` (репозиторий wishbone), но **двух редактируемых копий быть не должно**: первое расхождение iOS/Android = повод выделить репо немедленно.

## 16. Чек-лист код-ревью сервиса

Архитектура
- [ ] `operationId` из запроса **никогда** не используется как имя ручки/SQL/HTTP-пути — только как ключ в `bdui_operation`.
- [ ] Шаг «operationId объявлен в ревизии экрана» (§7.1 п.2) реально есть — не только allowlist.
- [ ] `catalog_version_min` вычисляется валидатором; в API админки нет способа задать её руками.
- [ ] Один интерпретатор предикатов на `visibleWhen`, таргетинг и сегменты — не три копии.
- [ ] Реестр handler'ов проверяется на старте; отсутствующий handler = fail-fast, не 500 в рантайме.

Данные и корректность
- [ ] Все резолверы отдают **готовые к показу** значения: никакой арифметики/условий не остаётся клиенту (§6.5).
- [ ] Плюрализация отдаётся формами CLDR, а не одной строкой.
- [ ] Даты/таймеры форматируются в `context.timeZone`, а не в UTC и не в TZ сервера.
- [ ] JSON едет скаляром `JSON`; нигде не осталось строкового обхода (§1.2).
- [ ] Рукописный `JSON.swift` починен (`_jsonValue` рекурсивен, `.null` → `NSNull()`), мост `PostboxGQL.JSON ↔ PostboxCore.JSON` покрыт round-trip-тестом на вложенном объекте (T-XPL-2).
- [ ] Int64-идентификаторы не теряют точность на всём пути: сервер → скаляр → домен → кэш.

Транзакции и идемпотентность
- [ ] Публикация ревизии, снятие `is_default` и аудит — одна транзакция.
- [ ] Откат создаёт новую ревизию, а не воскрешает старую.
- [ ] `bdui_idempotency` двухфазная: `in_flight` → `done`/удаление; несовпадение `request_hash` = ошибка, а не тихий повтор.
- [ ] Write-операция без `idempotencyKey` отвергается.
- [ ] GC идемпотентности и аудита есть и не блокирует таблицу.

Производительность
- [ ] Бюджет документа и per-source таймаут реально применяются (`ctx` с deadline, а не «мы посчитали и надеемся»).
- [ ] `singleflight` на резолве; батчинг там, где легаси делал N+1 (`GetUsersForRaffle`).
- [ ] Индексы §3 существуют; `explain` на выборке правил таргетинга не даёт seq scan.
- [ ] Кэш фрейма и кэш данных разделены; инвалидация фрейма не сносит данные и наоборот.
- [ ] ETag считается по канонизированным байтам (стабильный порядок ключей), иначе 304 не срабатывает никогда.

Безопасность и приватность
- [ ] `open(url)` allowlist проверяется **на записи для литералов, на резолве для Value, на клиенте всегда**.
- [ ] Значения форм и `bdui_user_attributes` отсутствуют в логах, метриках, трейсах и Sentry.
- [ ] Админ-API отдельным процессом/origin, роли проверяются на каждом хендлере, `reviewer ≠ author` на публикации.
- [ ] Preview-токен: TTL ≤ 15 мин, привязан к субъекту, одноразовый, `no-store`, исключён из CDN и из продуктовых метрик.
- [ ] Rate limit на `getBduiScreen`/`getBduiData`/`bduiCall` включён по умолчанию, а не «настроим потом».
- [ ] CDN-путь включается только для документов без пользовательских источников, сегментов и экспериментов.

Эксплуатация
- [ ] `salt` эксперимента неизменяем после создания (нет UPDATE в админ-API).
- [ ] Версия контракта видна в `/health` и в лейблах метрик.
- [ ] `bdui_node_degraded_total` с клиентов доходит и на него есть алерт — это единственный ранний сигнал рассинхрона каталога.
- [ ] Есть runbook: «клиент массово получает `UNSUPPORTED_VERSION`» и «required-источник лёг».

## 17. Открытые вопросы

1. ~~**Куда физически ставится субграф.**~~ **Закрыт 2026-08-25: отдельный сервис, не субграф** — в supergraph не федерируется. Осталось: адрес на окружение, протокол (GraphQL со своим корневым `Query` или REST — §1.1) и принимает ли сервис тот же access token.
2. ~~**`Accept-Language` vs `language`.**~~ **Закрыт 2026-08-25: `language`** — тот, что клиент уже шлёт. Починка докстринга `Holiday.name` остаётся мелкой задачей бэка.
2a. **`brand` по доменному имени** (§2.1) — ответ получен, но все 7 брендов ходят на 4 общих хоста. Нужен список per-brand доменов либо решение всё-таки принимать заголовок `brand`.
3. **Кто владелец contracts-репо** и в чьём GitLab-namespace он живёт (§15).
4. **Источник `bdui_user_attributes`** — материализация из основной БД (чем и как часто), или онлайн-запрос в профильный сервис с TTL. От этого зависит, можно ли таргетировать по «уровню» и «числу билетов» без деградации p95.
5. **`deviceId` для анонимного бакета.** На iOS есть `X-Device-Key-ID` (App Attest), но только в `devNew` (`DeviceAttestationPolicy.backendSupportedEnvironments`). Для остальных окружений нужен стабильный анонимный идентификатор, иначе раскатка по процентам для неавторизованных недетерминирована.
6. **Судьба `DatingFamilyPromoBottomSheet`** (`schema.graphqls:6923`, `templateName` + 4 серверных строки + `recordDatingFamilyPromoBottomSheetShown`) — это готовый прото-BDUI. Мигрируем его в BDUI первым после раффла или оставляем как есть?

[[README|← к обзору]] · [[BDUI]] · [[BDUI-Catalog]] · [[BDUI-Raffle-Pilot]] · [[BDUI-Plan]] · [Postbox](../../Stratoapp/docs/Architecture/Postbox.md) · [Localization](../../Stratoapp/docs/Architecture/Localization.md) #arch

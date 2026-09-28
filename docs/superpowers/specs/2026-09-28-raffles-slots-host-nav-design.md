# Raffles, слоты и стек хоста — дизайн

> Статус: **утверждён 2026-09-28**. Дополняет [спеку V1](2026-09-27-wishbone-v1-design.md): снимает с «не делаем в V1» (§8.3) placement-слоты в минимальной форме, добавляет навигацию в стек хоста (§6.3) и первый экран на реальном бэкенде.

## 0. Цель и объём

1. **Стек хоста.** Экран Wishbone можно пушнуть в навигацию хоста (NavigationStack, редьюсер, Navigator), и `navigate` внутри продолжает тот же стек — без вложенного стека SDK, с нативным «назад».
2. **Слот.** Хост встраивает в свой нативный экран (ленту) инлайн-вьюху `WishboneSlot`; сервер отдаёт в неё документ или «ничего», и тогда слот нулевой высоты. Баннер добавляется и убирается публикацией в админке, без релиза; условие «розыгрыш идёт» — `when` по данным в документе.
3. **Raffles.** Порт экрана розыгрыша Stratoapp (`Modules/Raffle`) на Wishbone с данными из GraphQL NewBackend (stage) через операции `raffle.*` сервера Wishbone; баннер розыгрыша в слоте ленты.
4. **Stratoapp.** Слот в ленте (debug/TestFlight), Wishbone-экраны открываются push-оверлеем, шаринг рефералки.

Вне объёма: Android-хост most (ветка `feat/wishbone-host` не собирается без Nexus), ротация/таргетинг/расписание баннеров, состояния «розыгрыш завершён / выигрыш» (в API их нет), локализация кроме ru.

## 1. Навигация в стек хоста

### 1.1 API

iOS:
```swift
public struct WishboneRoute: Hashable, Identifiable, Sendable {
  public let id: UUID            // каждый пуш — отдельная запись стека хоста
  public let screen: String
  public let params: [String: Value]
  public let transition: String? // "push" | "fade" | nil — подсказка хосту
}
WishboneScreen(wishbone, id:, params:, onExit:, onNavigate: ((WishboneRoute) -> Void)? = nil)
WishboneScreen(wishbone, route: WishboneRoute, onExit:, onNavigate:)
```
Android — зеркально: `data class WishboneRoute(screen, params, transition)` + параметр `onNavigate: ((WishboneRoute) -> Unit)? = null` у `@Composable WishboneScreen`.

### 1.2 Семантика (обе платформы)

Режим включается наличием `onNavigate`:

| DSL | без `onNavigate` (как сейчас) | с `onNavigate` |
|---|---|---|
| `navigate(id, params, t)` | пуш во внутренний стек SDK | `onNavigate(route)`; хост пушит `WishboneScreen(route:)` в свой стек |
| `back()` | pop внутреннего стека, в корне — `onExit` | `onExit` (хост делает pop) |
| `present(id, params)` | шит SDK со своим стеком | так же — модалка остаётся за SDK |
| `dismiss()` | закрыть шит, иначе как `back()` | закрыть шит, иначе `onExit` |

- iOS: с `onNavigate` `StackHost` не оборачивает экран в `NavigationStack` (иначе стек в стеке хоста). Системный навбар по-прежнему скрыт (экраны рисуют свои шапки); `SwipeBackEnabler` работает и на навконтроллере хоста (`viewControllers.count > 1`).
- Внутри шита, открытого `present`, стек всегда свой (модалка — отдельный контекст): `navigate` в шите пушит внутри шита.
- Playground iOS и Android переходят на этот режим: экраны пушатся в стек самого плейграунда (iOS сейчас — вложенный `NavigationStack`).

## 2. Слот

### 2.1 Модель

Слот — **обычный экран** с id по соглашению `slot.<место>` (`slot.feed.top`). Отдельной сущности «плейсмент» нет: несколько баннеров, карусель, условия показа — авторство внутри документа (`when`, `repeat`, `data`). Место в нативном экране выбирает хост.

### 2.2 API

```swift
WishboneSlot(wishbone, id: "slot.feed.top", params: [:], onNavigate: { route in … })
```
Android: `@Composable fun WishboneSlot(wishbone, id, params = emptyMap(), onNavigate: (WishboneRoute) -> Unit, modifier = Modifier)`. `onNavigate` обязателен — слоту некуда пушить самому.

### 2.3 Семантика

- Инлайн, без стека; ширина — предложенная хостом, высота — по контенту корня. `env.safeTop/safeBottom` = 0.
- **Пустой слот** (ничего не рисуется, высота 0; ни спиннера, ни фоллбека):
  - загрузка без кэша;
  - `SCREEN_NOT_FOUND` (не опубликован или снят) — дополнительно **стирает кэш слота**, иначе снятый баннер жил бы в кэше;
  - `DATA_REQUIRED_FAILED` и прочие ошибки без кэша (с кэшем — показывается кэш, как у экрана);
  - корень документа с `when == false`.
- `navigate` → `onNavigate`; `present` → шит SDK поверх; `back`/`dismiss` — no-op.
- Загрузка — при появлении вьюхи (кэш сразу, затем сеть, как у экрана). Принудительное обновление — сменой identity вьюхи у хоста.

### 2.4 Сервер и админка

- Миграция `002`: `screens.unpublished_at timestamptz NULL`.
- `POST /admin/api/screens/{id}/unpublish` → 204; `publish` и `rollback` сбрасывают флаг; запись в `audit`.
- `GET /v1/screens/{id}` для снятого экрана → 404 `SCREEN_NOT_FOUND`. `/v1/data` и `/v1/call` по номеру ревизии работают как раньше (открытые экраны доживают).
- `GET /admin/api/screens`: у снятого `publishedRev: null`.
- Админка: кнопка «Снять с публикации» рядом с «Опубликовать».

## 3. Raffles на реальном бэкенде

### 3.1 Апстрим

- `WISHBONE_GRAPHQL_URL` — GraphQL NewBackend; тест-сервер — `https://stage.korputeam.ru/graphql` (проверено: достижим с VPS, сабграф raffle требует токен). Не задан → операции `raffle.*` отвечают `UPSTREAM_FAILED`.
- Заголовок `Authorization` запроса к Wishbone (`Req.User`) прокидывается в GraphQL как есть; `Accept-Language` — тоже.
- `WISHBONE_GRAPHQL_FALLBACK_TOKEN` (необязательный): для **read**-операций запросов без `Authorization` (превью в админке, плейграунд) подставляется `Bearer <token>` тестового stage-аккаунта. Write без токена → `UNAUTHENTICATED`.
- Ошибки: `extensions.code == "UNAUTHENTICATED"` → `UNAUTHENTICATED`; прочие GraphQL-ошибки, таймаут (10 с), не-200 → `UPSTREAM_FAILED` (новый код в `errors.json`, retryable; приходит как доменная ошибка `{ok: false}` источника или `call`). Токен и тела ответов не логируются.
- Образ сервера `FROM scratch` получает CA-бандл (иначе TLS до stage не пройдёт): `push.sh` кладёт системный бандл в контекст сборки, `Dockerfile` копирует его в `/etc/ssl/certs/ca-certificates.crt`.
- Один апстрим: токены dev/georgiaStage/featureStand на stage не валидны → экраны показывают ошибку, слот пуст. Потолок; при необходимости — allowlist «контур → URL» по заголовку хоста.

### 3.2 Операции (`contract/operations.json`, `backend/internal/ops/raffle.go`)

Время — мс UTC (ISO из GraphQL переводит сервер). Сборка повторяет `RaffleOverviewAssembler` / `RaffleResponseMapper` Stratoapp.

| Операция | kind | GraphQL | Результат |
|---|---|---|---|
| `raffle.overview` `{}` | read | `GetRaffleScreenData(limit 20, offset 0)` + `getRaffleTime` (best effort) + `getUsersForRaffle` + `getUser{myRefCode}` | см. ниже |
| `raffle.promo` `{}` | read | `getRaffleTime` + `getMyProgressRaffle` + `getRaffleTaskRules` + `getRaffleUsersInfo(limit 1)` | `{active: bool, endsAt: number?, place: number?, tasksDone: number, tasksTotal: number}` |
| `raffle.leaderboard` `{offset, limit}` | read | `GetRaffleLeaderboard` + `getUsersForRaffle` | `{total, myPosition: number?, items: list<{key: string, position, name, tickets, isMe: bool}>}` |
| `raffle.tickets` `{offset, limit}` | read | `GetMyRaffleTickets` | `{total, items: list<{id: string, createdAt: number}>}` |
| `raffle.invites` `{}` | read | `GetMyReferralInvites(limit 100)` + `getUsersForRaffle` | `{total, maxTicketsPerInvite, items: list<{name, tickets}>}` |
| `raffle.claimDaily` `{}` | write | `claimDailyReward` | `{streakDay}` |
| `raffle.toggleHidden` `{}` | write | `toggleMyVisibilityInRaffle` | `{hidden: bool}` |

`raffle.overview`:
```
{ tickets: number, place: number?, participants: number, hidden: bool,
  startsAt: number?, endsAt: number?,
  daily: {streakDay: number, claimedToday: bool, claimAmount: number?,
          days: list<{day: number, amount: number, state: "claimed" | "available" | "waiting"}>},
  mainPrize: {title: string, image: string?}?,
  prizes: list<{rank: number, title: string, tickets: number, image: string?}>,
  tasks: list<{id: string, name: string, description: string, icon: string?, tickets: number,
               cooldownUntil: number?, progress: list<{label: string, value: string, ticket: bool}>}>,
  rules: list<{name: string, description: string, icon: string?, tickets: number}>,
  tasksDone: number, tasksTotal: number,
  share: string? }
```
Правила сборки (как в Stratoapp): задания — дедуп правил по `(isPromo, taskType, name)`, без promo, по убыванию `priority`, склейка с прогрессом по `taskType`, строки прогресса (шаги/часы/«Выполнено: a/b»/«Получено билетов:»), cooldown — только будущий `nextAvailableAt`; призы — первая группа `raffleId`, сортировка по билетам ↓ и имени, дубли картинок убираются (кроме «coin»); дейли — `claimed` до текущего дня, текущий — `available`/`claimed`, дальше `waiting`; имена в таблице — `displayName ?? "**********"`, текущий пользователь — `"Я " + (hidden ? "**********" : имя)`; `share` = `"📱 Скачай приложение: <myRefCode>\n\n<текст raffle.share.body>"` (null без кода); `tasksDone/Total` — по не-promo типам. `raffle.promo.active` = `start ≤ now < end`.

Потолки: строки только ru; `@`-упоминания в названиях заданий — обычный текст.

### 3.3 Экраны (`contract/screens/`, сидятся в БД)

- **`raffle.main`** — шапка (назад, «Розыгрыш», «i» → `present("raffle.rules")`); `SegmentedTabs` «Задания / Призы / Таблица» (стейт `tab`); карточка «Мои билеты» (билеты, место, «Подробнее» → `navigate("raffle.tickets")`), кроме таба «Таблица»; главный приз (картинка, «Итоги розыгрыша проведем …»).
  - «Задания»: дейли-рельс (горизонтальный скролл дней) + «Забрать N» → `await call raffle.claimDaily()` → `refresh(overview)`; «Как выиграть главный приз?» → rules; карточка заданий с отсчётом до `endsAt` (`env.now`) и строками заданий (иконка, название, «+N билетов», описание, строки прогресса, «Доступно через …»); «Мои приглашения» → `navigate("raffle.invites")`; реферальная карточка «Получи 30 билетов за каждого друга» → `host("share", {text: share})`; «Показывать ФИО в таблице» — `Toggle`, оптимистично + `call raffle.toggleHidden()`, откат при ошибке.
  - «Призы»: `grid` в 2 колонки (картинка, ранг, бейдж «от N», название).
  - «Таблица»: `data board = raffle.leaderboard({offset: 0, limit: 20})`, строки `#место имя — N`, своя подсвечена; догрузка — `appear` футера → `await call raffle.leaderboard({offset: rows.count, limit: 20})` и добавление в стейт.
  - Ошибка `overview` (значение `null`) → «Не удалось загрузить розыгрыш» + «Повторить» (`refresh`).
- **`raffle.tickets`** (push) — сводка (билеты, место) + список `#i id` с догрузкой по `appear`.
- **`raffle.invites`** (push) — «Мои приглашения»: число, строки «имя — N билетов», пустое/ошибка.
- **`raffle.rules`** (present) — «Как получить билеты»: правила (иконка, название, «+N», описание) + «Пригласить друзей» → share.
- **`slot.feed.top`** — `data promo = raffle.promo() required`; корень `when: promo.active && env.now < (promo.endsAt ?? 0)` (кэшированный баннер гаснет по окончании сам); карточка: артворк подарка, заголовок, бейдж места, тикающий отсчёт до `endsAt`, «Задания N/M»; тап → `navigate("raffle.main")`.

Артворк (подарок, монета, лавры, кубок) — PNG из ассетов Stratoapp в `contract/assets/raffle/`. Цвета и типографика — токены темы: принудительной тёмной схемы и палитры `raffleColors` нет.

Не переносим (как в пилоте [[BDUI-Raffle-Pilot]]): drag-expand таблицы поверх приза, закреплённый таб-бар, shimmer/wobble, анимированный полосатый прогресс-бар (заменён `ProgressBar`).

### 3.4 Шаринг

`host("share", {text})` — действие хоста: Stratoapp — системный share sheet; плейграунды — лог и тост (как прочие действия).

## 4. Stratoapp (локальная ветка, без пуша)

- Синк SDK: `Scripts/sync-wishbone.sh`.
- **Лента:** новый `FeedRenderItem` со слотом `slot.feed.top` — только когда Wishbone доступен (debug/TF), после нативного raffle-промо, в скоупе главной ленты. Вьюха слота поставляется хуком из App-таргета (лента не импортирует WishboneUI), как `FeedRafflePromoHooks`. Пустой слот — нулевая высота строки.
- **Открытие экранов:** `onNavigate` слота и диплинк `bdui/<id>` открывают Wishbone-экран push-оверлеем (`ManualSlidingOverlay`, как нативный розыгрыш) вместо текущего шита; внутри — стек SDK (у оверлеев Stratoapp нет NavigationStack хоста); `onExit` закрывает оверлей.
- **`WishboneAppHost.action("share")`** → системный share sheet.

## 5. Проверка

- Go: `raffle.*` против фейкового GraphQL-апстрима (`httptest`, фикстуры ответов stage) — сборка overview, маскирование, призы, дейли, промо, маппинг ошибок, fallback-токен только для read; unpublish (API + 404 + сброс публикацией); компиляция новых экранов (существующий golden/seed-тест).
- SDK: форвардинг `navigate`/`back` в режиме хоста (iOS и Android, модель Navigator); правило «пустой слот» (чистая функция фазы → видимость).
- Ручная: плейграунды iOS/Android против тест-сервера со stage — `raffle.*` экраны, фейковая лента со слотом, пуш в стек хоста; скриншоты обеих платформ. Stratoapp: сборка, лента со слотом на симуляторе, открытие розыгрыша из баннера.
- Для реальных данных нужен stage-токен тестового аккаунта (fallback на сервере); без него проверка — фейковый апстрим + Stratoapp с залогиненным stage-пользователем.

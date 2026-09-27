# BDUI пилот — розыгрыш (Raffle)

> Репозитории и соглашение о путях — [[README#Репозитории]]. Относительные пути в тексте: `Modules/`, `App/`, `Apps/`, `Tuist/`, `Scripts/`, `Tests/` — Stratoapp; `core/`, `feature/`, `graphql/`, `app/` — most; `Korpu/` — multiapp; `schema/`, `catalog/`, `operations/`, `fixtures/`, `tools/`, `bindings/` — bdui-contracts.

Пилот проверяет модель [[BDUI]] на реальном продуктовом экране, а не на витрине. Источник — legacy-модуль `Raffle` в `~/projects/multiapp` (Korpu) и его андроид-двойник `feature/giveaway` в `~/projects/most@develop`. Каталог узлов — [[BDUI-Catalog]], серверная часть — [[BDUI-Backend]], порядок работ — [[BDUI-Plan]].

Почему именно раффл: экран был захардкожен в двух приложениях, отличается по брендам, разыгран один раз и в Stratoapp **не переносился** — то есть цена ошибки нулевая, а поверхность максимальная (списки, пагинация, таймеры, мутации, приватность, локализация, промо-таргетинг).

---

## 1. Что делал legacy-раффл

### 1.1 Экраны и входы

| Экран | iOS (`Korpu/Modules/Raffle`) | Android (`feature/giveaway`) |
|---|---|---|
| Корень, 3 вкладки | `Views/RaffleScreen.swift`, `ViewModels/RaffleViewModel.swift`, route `Route.raffle` | `Screen.Giveaway(initialTab)`, `GiveawayScreen.kt`, вкладки `TASKS/GIFTS/TABLE` |
| «Мои билеты» | `Views/Tickets/RaffleTicketsScreen.swift`, `Route.raffleTickets` | `Screen.GiveawayMyTickets` |
| «История начислений» | `Views/AccrualHistory/AccrualHistoryScreen.swift`, `Route.raffleAccrualHistory` | `Screen.GiveawayTicketHistory` — **зарегистрирован, но недостижим** |
| «Мои приглашения» | нет (счётчик внутри корня) | `Screen.GiveawayMyInvitations` |
| Шторка «Как начисляются билеты?» | `Views/Components/RaffleTicketAccrualBottomSheetView.swift`, 3 точки входа | `GiveawayPointsRulesSheet`, 3 точки входа |
| Промо-баннеры | `FeedPresenter.swift:5501`, `AccountSidebarViewModel.didTapRafflePrizes()` | `feature/feed/.../GiveawayBanner.kt` (4 варианта) |

Весь iOS-модуль принудительно `.preferredColorScheme(.dark)` со своей приватной палитрой `Config/RaffleColors.swift`; андроид — с захардкоженными `0xFF141414`/`0xFF232323`. Ни один не использует брендовую тему.

### 1.2 Вкладки и блоки корневого экрана

- Общая шапка: `RaffleTicketProgressCard` (билеты, место, полоса прогресса), `RaffleDailyRewardsCard` (7 дней стрика), `RaffleMainPrizeCard` (главный приз + дата итогов).
- `.tasks`: `RaffleConditionsCard` → `RaffleTasksSectionView` (заголовок с таймером, строки заданий, кулдаун, прогресс-подпункты, пустое состояние) → `RaffleInvitationsCard` → `RaffleReferralCard` → `RaffleSettingsCard` (тумблер «показывать имя»).
- `.prizes`: заголовок с датой + двухколоночная сетка `RafflePrizeGridCard` (бейдж «от N»). `RaffleSuperPrizeView` — мёртвый код.
- `.table`: `RaffleLeaderboardExpandContainer` (жест раскрытия поверх картинки приза, `expandProgress` 0…1, арбитраж внутреннего/внешнего скролла) с `RaffleLeaderboardView` (строки, липкая строка «я», кубок с вобблом).

### 1.3 Состояния

Есть: `isInitialLoading`, `errorState == .failedToLoad` (полноэкранная ошибка + retry), `isEmpty` (нет заданий, призов и участников), инлайн-ошибка `.failedToClaimDailyReward`, флаги `isClaimingDailyReward`, `isUpdatingFullNameVisibility`, `isLoadingMoreParticipants`, `hasMoreParticipants`.

Нет: состояний **«не начался» / «завершён» / «я выиграл»**. Срок розыгрыша существует только как текст `raffleResultDateFormat` («Итоги розыгрыша проведем %@») и строка обратного отсчёта `%dдн:%dч:%dм`. `GetMyRafflePrizes` (что я выиграл) объявлен в `.graphql` и **не вызывается ни на одной платформе**.

### 1.4 GraphQL (`Korpu/API/GraphQL/Raffle/RaffleOperations.graphql`, сервис `Korpu/Services/RaffleService.swift`)

| Операция | Что отдаёт |
|---|---|
| `GetRaffleScreenData($leaderboardLimit,$leaderboardOffset)` | агрегат: `dailyBonusRewards{streakDay,amount}`, `currentDailyBonus{streakDay,claimedToday}`, `getMyProgressRaffle{ticketsCount,hidden,items{taskType,name,count,subDescription,priority,progress,target,completed,nextAvailableAt,ticketsEarned}}`, `getPrizes{mainPrize,prizes}`, `getRaffleUsersInfo{total,currentUserPosition,items{userId,ticketsCount}}`, `getRaffleTaskRules{taskType,name,ticketsCount,description,iconUrl,iconUrlSmall,priority,target,isPromo}` |
| `GetRaffleLeaderboard($limit,$offset)` | страница таблицы |
| `GetUsersForRaffle($userIds:[Int!]!)` | `id,firstname,lastname` — имена для таблицы и истории |
| `GetMyRaffleTickets($limit,$offset)` | `total, items{ticketId,createdAt}` |
| `GetMyEarningsHistory($limit,$offset)` | `total, items{userId, items{taskType,name,count}}` |
| `GetMyRafflePrizes` | **не используется** |
| `ClaimDailyReward` (mutation) | `streakDay` |
| `ToggleMyVisibilityInRaffle` (mutation) | `Boolean` = новое **isHidden** (инвертировано относительно UI) |
| вне `.graphql`, инлайн в `RaffleService.swift` | `getRaffleTime{start,end}` (best-effort, ошибка ⇒ nil), `getMyReferralInvites(limit:1,offset:0){total}` |

Все запросы `cachePolicy: .fetchIgnoringCacheData`, локального кэша нет; `.onAppear` перезапрашивает весь агрегат. Андроид добивает это **пятисекундным поллингом** (`REALTIME_REFRESH_INTERVAL_MS = 5_000`), ≈9–10 запросов на цикл, первый кадр ≈15 round-trip.

### 1.5 Бизнес-правила (всё это сегодня считает клиент)

| Правило | Где |
|---|---|
| Билеты за задание = `progress.count × rule.ticketsCount` | iOS `RaffleMapper` (история начислений), Android `GiveawayRepositoryImpl` (HOUR_IN_APP и история) |
| Джойн заданий: `taskRules.filter{!isPromo}.sorted(priority desc)` ⋈ `progress` по `taskType`; дубли суммируются; правило-дедуп по ключу `"isPromo|taskType|name"` | `RaffleMapper` |
| Промо-правила (`isPromo`) показываются **только** в шторке | обе платформы |
| Кулдаун: `nextAvailableAt` ISO8601 → показывать, только если `> now`; тогда строка прогресса заменяется на «Получено билетов: N» | `RaffleMapper` |
| Полоса прогресса = `min(myTickets / max(положительный ticketsCount на текущей странице таблицы), 1)`; если максимума нет — `1` при `currentUserPosition == 1`, иначе `0` | `RaffleMapper.makeProgress` |
| Абсолютное место = `offset + index + 1`; дедуп по id; `hasMore = combined < total && page.count >= 20` | обе платформы |
| Маскирование скрытых: имя → `"**********"`, аватар выброшен, себе префикс `"(Я)"`; `avatarPath` всегда `nil` ⇒ аватары в таблице **никогда не грузятся** | `RaffleMapper` |
| Ежедневный бонус: клейм доступен ⇔ `!claimedToday && rewards.contains(streakDay == currentDailyBonus.streakDay)`; ячейка `<` → `.claimed`, `==` → по `claimedToday`, `>` → `.waiting` | `RaffleViewModel:158-197` |
| После клейма: локальная отметка → рефетч агрегата → **повторное применение локальной отметки**, чтобы обойти read-after-writeлаг бэка | `RaffleViewModel:158-197` |
| Призы: группировка по `raffleId`, сортировка `ticketsCount desc, name asc`, бейдж `"от N"` (RU-литерал), дедуп повторяющихся `imageUrl` кроме имён по `/coin/i` | `RaffleMapper` |
| Главный приз: RU-литералы `"Квартира в "` + `"Sea Breeze"` прямо в маппере | `RaffleMapper` |
| Приватность: `hidden` ⇒ `showsFullNameInLeaderboard = !hidden`; тумблер оптимистичный с откатом, ответ инвертирован | `RaffleViewModel` |
| Реферал: текст шаринга собирается на клиенте из `session.user.myRefCode` через `ReferralLinkGenerator` | iOS; на андроиде `refLink` в модуле приглашений захардкожен `""` ⇒ кнопка мертва |
| `@mention` в тексте условий → хардкод-карта `korpu → 7798`, `shevgi → 2` | `RaffleMentionText` |
| Форматтеры прибиты к `ru_RU` и `dd.MM.yyyy` | `Config/RaffleStyle.swift` |

### 1.6 Бренды и локализация

`TargetLayout.Raffle.isEnabled`: Korpu `true`, Khidi `true`, Kamurj `false`; `showsRafflePromoSection`: только Korpu. Поверх — рантайм-флаг `raffle_section` из `fetchAppConfig().featureFlags` с проверкой платформы. Сам route не гейтится — гейтятся только входы.

Строки: один каталог `Account.xcstrings`, **79 ключей** `account.raffle*`, языки `az,en,hy,ru,tt,uz`, где **hy/tt/uz — нетронутые русские копии**. Брендовых переопределений строк нет (0 ключей в `BrandOverrides.xcstrings`/`KhidiStrings.xcstrings`). Мёртвых ключей — 6.

---

## 2. Разбор блоков: что рендерится декларативно

Легенда: **Д** — декларативный узел каталога, **Б** — нативный `block.*`, **С** — требует серверного агрегата (см. §5).

| Блок legacy | Узлы каталога | Источники | Действия | Вердикт |
|---|---|---|---|---|
| Навбар (заголовок + «i») | `row`, `text`, `button.icon` | — | `presentSheet("raffle.rules")` | Д |
| Вкладки | `tabs` | — | `patchState({tab})` | Д |
| Карточка прогресса | `card`, `text` (`$plural`), `progress`, `button.link` | `raffle` | `pushScreen("raffle.tickets")` | Д + **С** (доля прогресса, место) |
| Обратный отсчёт до итогов | `countdown` | `raffle` | `on.expire → refresh(["raffle"])` | Д |
| Карточка главного приза | `card`, `image`, `text`, `countdown` | `raffle` | — | Д (теряются shimmer/wobble) |
| Полоса ежедневного бонуса | `scroll`+`row` + `repeat` по 7 дням, `icon`, `button.primary` | `raffle` | `call("raffle.claimDailyBonus", invalidates:["raffle"])` | Д (теряется церемония монет) |
| Список заданий | `repeat`, `card`, `avatar`, `text`, `badge`, `progress`, `countdown`, `visibleWhen` | `raffle` | `open(deeplink)` для follow-заданий | Д + **С** (джойн правил, произведение билетов, промо-дедуп) |
| Карточка условий | `card`, `text`, `button.link` | `raffle` | `presentSheet("raffle.rules")` | Д |
| Карточка приглашений | `card`, `text` (`$plural`), `button.link` | `raffle` | `pushScreen("raffle.invites")` | Д |
| Реферальная карточка | `card`, `text`, `button.primary` | `raffle` | `share({text,url})` + `copyToClipboard` | Д + **С** (текст и ссылка приходят готовыми) |
| Тумблер «показывать имя» | `toggle` | `raffle` | `on.change → call("raffle.setVisibility", args:{visible:$state.showFullName})` | Д (откат — через `patch.state` в конверте ответа) |
| Вкладка призов | `grid` + `repeat`, `card`, `image`, `badge` | `raffle` | — | Д + **С** (группировка, сортировка, дедуп картинок) |
| Таблица лидеров | **`block.raffle.leaderboard`** | `board` | тап по строке → `row.deeplink` | **Б** |
| Шторка «как начисляются билеты» | отдельный документ `raffle.rules`: `column`+`repeat`, `avatar`, `text`, `badge`, `button.primary` | `rules` | `open(invite deeplink)` | Д |
| Пустое / ошибка / ретрай | `banner`, `text`, `button.primary` | — | `refresh(["raffle"])` | Д |
| Жест раскрытия таблицы поверх картинки приза | — | — | — | **не переносится** (§4) |
| `@mention` внутри текста условий | — | — | — | **не переносится** (§4) |

Итого: **один** нативный блок на весь экран. Всё остальное — каталог. Это и есть проверка тезиса «богатый каталог»: чем богаче каталог, тем меньше релизных зависимостей.

Соответствие узлов реальным компонентам платформ и список того, чего в дизайн-системах ещё нет, ведётся **в одном месте** — [[BDUI-Catalog]] §3 (маппинг) и §7 (таблицы пробелов `T-iOS-*`/`T-AND-*`/`T-OMN-*`). Пилот своего инвентаря компонентов не держит: из V1-узлов ему нужны `text`, `image`, `avatar`, `icon`, `badge`, `card`, `banner`, `button.*`, `tabs`, `toggle`, `progress`, `countdown`, `skeleton`, `grid` и layout-примитивы — блокирующее подмножество пробелов под них перечислено в [[BDUI-Plan]] P-6/P-7.

---

## 3. ScreenDocument пилота

Кодировка действий и предикатов принята такой: действие — объект с полем `type`; предикат — объект `{"<оператор>": [операнды]}`. Ниже — документ целиком, ровно то, что вернёт `getBduiScreen(screenId:"raffle.main")` для `brand=korpu, locale=ru, platform=ios`.

Инварианты, которые документ соблюдает буквально: **в пилоте нет ни одного токена-Value** — все `color`/`role`/`spacing`/`padding`/`radius`/`icon`/`style`/`size` заданы литералами из [[BDUI-Catalog]] §1 ([[BDUI]] §3.1 п.6); состояние ячеек выражается **данными** (`$item /state`) и `visibleWhen`, а не приезжающим с бэка именем токена. Идентификаторы доступности prop'ами не задаются — они выводятся из `id` узла (`bdui.raffle.main.<nodeId>`, [[BDUI]] §3). Данных в самом документе нет: `data[]` — только объявления, пейлоады приезжают отдельной картой ([[BDUI]] §4).

```json
{
  "schemaVersion": "2.0.0",
  "catalogVersion": "2.0.0",
  "screenId": "raffle.main",
  "revision": 1487,
  "context": { "brand": "korpu", "locale": "ru", "platform": "ios", "appVersion": "1.42.0", "catalogId": "korpu" },
  "cache": { "ttlSeconds": 30, "policy": "swr" },
  "analytics": { "screen": "raffle", "experiments": ["raffle_autumn_2026"] },
  "fallback": "hideSection",

  "state": { "tab": "tasks", "boardLimit": 20, "showFullName": true },

  "data": [
    { "id": "raffle", "operationId": "raffle.overview", "args": {},
      "cache": { "ttlSeconds": 30, "refreshSeconds": 30 }, "required": true },
    { "id": "board", "operationId": "raffle.leaderboard", "args": { "limit": { "$state": "boardLimit" } },
      "cache": { "ttlSeconds": 30, "refreshSeconds": 30 }, "required": false }
  ],

  "content": {
    "type": "column", "id": "root", "props": { "spacing": "md", "padding": "lg" },
    "children": [

      { "type": "row", "id": "navbar", "props": { "spacing": "sm", "align": "center" }, "children": [
        { "type": "text", "id": "navbar.title",
          "props": { "text": { "$data": "raffle/title" }, "role": "title", "color": "el16" } },
        { "type": "spacer", "id": "navbar.spacer" },
        { "type": "button.icon", "id": "navbar.info",
          "props": { "icon": "info", "accessibilityLabel": { "$data": "raffle/labels/rulesA11y" } },
          "on": { "tap": { "type": "presentSheet", "screenId": "raffle.rules" } } }
      ]},

      { "type": "column", "id": "state.scheduled", "props": { "spacing": "sm" },
        "visibleWhen": { "eq": [ { "$data": "raffle/status" }, "scheduled" ] },
        "children": [
          { "type": "banner", "id": "state.scheduled.banner",
            "props": { "message": { "$data": "raffle/notice/scheduledMessage" },
                       "style": "info", "icon": "clock", "dismissible": false } },
          { "type": "countdown", "id": "state.scheduled.timer",
            "props": { "until": { "$data": "raffle/startsAt" }, "format": "dhms",
                       "label": { "$data": "raffle/labels/startsLabel" } },
            "on": { "expire": { "type": "refresh", "sources": ["raffle"] } } }
        ]},

      { "type": "column", "id": "state.won", "props": { "spacing": "sm" },
        "visibleWhen": { "all": [
          { "eq": [ { "$data": "raffle/status" }, "finished" ] },
          { "eq": [ { "$data": "raffle/me/isWinner" }, true ] } ] },
        "children": [
          { "type": "banner", "id": "state.won.banner",
            "props": { "message": { "$data": "raffle/notice/wonMessage" },
                       "style": "success", "icon": "gift", "dismissible": false } },
          { "type": "button.primary", "id": "state.won.cta",
            "props": { "title": { "$data": "raffle/notice/wonCta" } },
            "on": { "tap": { "type": "open", "url": { "$data": "raffle/me/prizeDeeplink" } } } }
        ]},

      { "type": "banner", "id": "state.finished",
        "visibleWhen": { "all": [
          { "eq": [ { "$data": "raffle/status" }, "finished" ] },
          { "neq": [ { "$data": "raffle/me/isWinner" }, true ] } ] },
        "props": { "message": { "$data": "raffle/notice/finishedMessage" },
                   "style": "info", "dismissible": false } },

      { "type": "tabs", "id": "tabs",
        "props": { "selected": { "$state": "tab" },
                   "items": [
                     { "id": "tasks",  "title": { "$data": "raffle/labels/tabTasks" } },
                     { "id": "prizes", "title": { "$data": "raffle/labels/tabPrizes" } },
                     { "id": "table",  "title": { "$data": "raffle/labels/tabTable" } } ] },
        "on": { "change": { "type": "sequence", "actions": [
                  { "type": "patchState", "values": { "tab": { "$event": true } } },
                  { "type": "track", "event": "raffle_tab_selected", "params": { "tab": { "$state": "tab" } } } ] } } },

      { "type": "card", "id": "progress",
        "visibleWhen": { "neq": [ { "$state": "tab" }, "table" ] },
        "props": { "padding": "lg", "radius": "card", "background": "bg1Elevated" },
        "children": [
          { "type": "text", "id": "progress.tickets",
            "props": { "role": "largeTitle", "color": "el16",
              "text": { "$plural": { "count": { "$data": "raffle/me/ticketsCount" },
                "forms": { "one": "{count} билет", "few": "{count} билета", "many": "{count} билетов", "other": "{count} билета" } } } } },
          { "type": "text", "id": "progress.place",
            "visibleWhen": { "not": { "isNull": { "$data": "raffle/me/place" } } },
            "props": { "role": "bodySmall", "color": "el9",
              "text": { "$text": { "template": { "$data": "raffle/labels/placeTemplate" },
                                   "args": { "place": { "$data": "raffle/me/place" } } } } } },
          { "type": "text", "id": "progress.noPlace",
            "visibleWhen": { "isNull": { "$data": "raffle/me/place" } },
            "props": { "role": "bodySmall", "color": "el9", "text": { "$data": "raffle/labels/noPlace" } } },
          { "type": "progress", "id": "progress.bar",
            "props": { "value": { "$data": "raffle/me/progressFraction" }, "style": "linear",
                       "label": { "$data": "raffle/me/progressCaption" } } },
          { "type": "button.link", "id": "progress.tickets.link",
            "props": { "title": { "$data": "raffle/labels/myTickets" } },
            "on": { "tap": { "type": "pushScreen", "screenId": "raffle.tickets" } } }
        ]},

      { "type": "card", "id": "mainPrize",
        "visibleWhen": { "neq": [ { "$state": "tab" }, "table" ] },
        "props": { "padding": "lg", "radius": "card", "background": "bg1Elevated" },
        "children": [
          { "type": "image", "id": "mainPrize.image",
            "props": { "url": { "$data": "raffle/mainPrize/imageUrl", "default": null },
                       "aspect": 1.6, "radius": "md",
                       "accessibilityLabel": { "$data": "raffle/mainPrize/title" } } },
          { "type": "text", "id": "mainPrize.title",
            "props": { "role": "title", "color": "el16", "text": { "$data": "raffle/mainPrize/title" } } },
          { "type": "text", "id": "mainPrize.subtitle",
            "props": { "role": "bodySmall", "color": "el9", "text": { "$data": "raffle/mainPrize/subtitle" } } },
          { "type": "countdown", "id": "mainPrize.countdown",
            "visibleWhen": { "eq": [ { "$data": "raffle/status" }, "running" ] },
            "props": { "until": { "$data": "raffle/endsAt" }, "format": "dhms",
                       "label": { "$data": "raffle/labels/endsLabel" } },
            "on": { "expire": { "type": "refresh", "sources": ["raffle"] } } }
        ]},

      { "type": "column", "id": "tab.tasks", "props": { "spacing": "md" },
        "visibleWhen": { "all": [ { "eq": [ { "$state": "tab" }, "tasks" ] },
                                  { "neq": [ { "$data": "raffle/status" }, "scheduled" ] } ] },
        "children": [

          { "type": "card", "id": "daily", "props": { "padding": "md", "radius": "card" }, "children": [
            { "type": "text", "id": "daily.title",
              "props": { "role": "bodyStrong", "color": "el16", "text": { "$data": "raffle/daily/title" } } },
            { "type": "scroll", "id": "daily.rail", "props": { "axis": "horizontal" }, "children": [
              { "type": "row", "id": "daily.rail.row", "props": { "spacing": "xs" }, "children": [
                { "type": "card", "id": "daily.cell", "props": { "padding": "sm", "radius": "md" },
                  "repeat": { "over": { "$data": "raffle/daily/days" }, "key": "/id", "limit": 10 },
                  "children": [
                    { "type": "text", "id": "daily.cell.day",
                      "props": { "role": "caption", "color": "el9", "text": { "$item": "/dayLabel" } } },
                    { "type": "text", "id": "daily.cell.amount",
                      "props": { "role": "bodyStrong", "color": "el16", "text": { "$item": "/amountLabel" } } },
                    { "type": "icon", "id": "daily.cell.state",
                      "visibleWhen": { "eq": [ { "$item": "/state" }, "claimed" ] },
                      "props": { "icon": "check", "color": "success", "size": "sm" } }
                  ]}
              ]}
            ]},
            { "type": "button.primary", "id": "daily.claim",
              "visibleWhen": { "eq": [ { "$data": "raffle/daily/claimAvailable" }, true ] },
              "props": { "title": { "$data": "raffle/daily/claimTitle" } },
              "on": { "tap": { "type": "call", "operationId": "raffle.claimDailyBonus",
                               "invalidates": ["raffle"] } } }
          ]},

          { "type": "column", "id": "tasks", "props": { "spacing": "sm" }, "children": [
            { "type": "text", "id": "tasks.title",
              "props": { "role": "bodyStrong", "color": "el16", "text": { "$data": "raffle/tasks/title" } } },
            { "type": "card", "id": "tasks.row", "props": { "padding": "md", "radius": "md" },
              "repeat": { "over": { "$data": "raffle/tasks/items" }, "key": "/id", "limit": 40,
                "empty": { "type": "text", "id": "tasks.empty",
                           "props": { "role": "bodySmall", "color": "el9",
                                      "text": { "$data": "raffle/tasks/emptyText" } } } },
              "on": { "tap": { "type": "open", "url": { "$item": "/deeplink", "default": null } } },
              "children": [
                { "type": "row", "id": "tasks.row.head", "props": { "spacing": "sm", "align": "center" }, "children": [
                  { "type": "avatar", "id": "tasks.row.icon",
                    "props": { "url": { "$item": "/iconUrl", "default": null }, "size": "lg", "radius": "sm" } },
                  { "type": "column", "id": "tasks.row.text", "props": { "spacing": "xs" }, "children": [
                    { "type": "text", "id": "tasks.row.title",
                      "props": { "role": "body", "color": "el16", "text": { "$item": "/title" } } },
                    { "type": "text", "id": "tasks.row.subtitle",
                      "visibleWhen": { "not": { "isEmpty": { "$item": "/subtitle" } } },
                      "props": { "role": "caption", "color": "el9", "text": { "$item": "/subtitle" } } }
                  ]},
                  { "type": "spacer", "id": "tasks.row.spacer" },
                  { "type": "badge", "id": "tasks.row.tickets",
                    "props": { "title": { "$item": "/ticketsLabel" }, "style": "accent" } }
                ]},
                { "type": "progress", "id": "tasks.row.progress",
                  "visibleWhen": { "not": { "isNull": { "$item": "/progressFraction" } } },
                  "props": { "value": { "$item": "/progressFraction" }, "style": "linear",
                             "label": { "$item": "/progressCaption" } } },
                { "type": "countdown", "id": "tasks.row.cooldown",
                  "visibleWhen": { "not": { "isNull": { "$item": "/cooldownUntil" } } },
                  "props": { "until": { "$item": "/cooldownUntil" }, "format": "hms",
                             "label": { "$data": "raffle/labels/cooldownLabel" } },
                  "on": { "expire": { "type": "refresh", "sources": ["raffle"] } } }
              ]}
          ]},

          { "type": "card", "id": "conditions", "props": { "padding": "md", "radius": "card" }, "children": [
            { "type": "text", "id": "conditions.title",
              "props": { "role": "bodyStrong", "color": "el16", "text": { "$data": "raffle/conditions/title" } } },
            { "type": "text", "id": "conditions.subtitle",
              "props": { "role": "bodySmall", "color": "el9", "text": { "$data": "raffle/conditions/subtitle" } } },
            { "type": "button.link", "id": "conditions.cta",
              "props": { "title": { "$data": "raffle/conditions/cta" } },
              "on": { "tap": { "type": "presentSheet", "screenId": "raffle.rules" } } }
          ]},

          { "type": "card", "id": "referral", "props": { "padding": "md", "radius": "card" }, "children": [
            { "type": "text", "id": "referral.title",
              "props": { "role": "bodyStrong", "color": "el16", "text": { "$data": "raffle/referral/title" } } },
            { "type": "text", "id": "referral.invited",
              "props": { "role": "bodySmall", "color": "el9",
                "text": { "$plural": { "count": { "$data": "raffle/referral/invitedCount" },
                  "forms": { "one": "{count} друг уже с вами", "few": "{count} друга уже с вами",
                             "many": "{count} друзей уже с вами", "other": "{count} друга уже с вами" } } } } },
            { "type": "row", "id": "referral.actions", "props": { "spacing": "sm" }, "children": [
              { "type": "button.primary", "id": "referral.share",
                "props": { "title": { "$data": "raffle/referral/shareCta" } },
                "on": { "tap": { "type": "share",
                                 "text": { "$data": "raffle/referral/shareText" },
                                 "url": { "$data": "raffle/referral/link" } } } },
              { "type": "button.link", "id": "referral.copy",
                "props": { "title": { "$data": "raffle/referral/copyCta" } },
                "on": { "tap": { "type": "sequence", "actions": [
                          { "type": "copyToClipboard", "value": { "$data": "raffle/referral/link" } },
                          { "type": "haptic", "kind": "success" } ] } } }
            ]}
          ]},

          { "type": "card", "id": "settings", "props": { "padding": "md", "radius": "card" }, "children": [
            { "type": "toggle", "id": "settings.fullName",
              "props": { "label": { "$data": "raffle/settings/fullNameTitle" } },
              "form": { "formId": "raffle.settings", "stateKey": "showFullName", "validation": [] },
              "on": { "change": { "type": "call", "operationId": "raffle.setVisibility",
                                  "args": { "visible": { "$state": "showFullName" } },
                                  "invalidates": ["board"] } } }
          ]}
        ]},

      { "type": "column", "id": "tab.prizes", "props": { "spacing": "sm" },
        "visibleWhen": { "eq": [ { "$state": "tab" }, "prizes" ] },
        "children": [
          { "type": "text", "id": "prizes.title",
            "props": { "role": "title", "color": "el16", "text": { "$data": "raffle/prizes/title" } } },
          { "type": "text", "id": "prizes.date",
            "props": { "role": "bodySmall", "color": "el9", "text": { "$data": "raffle/prizes/dateText" } } },
          { "type": "grid", "id": "prizes.grid", "props": { "columns": 2, "spacing": "sm" }, "children": [
            { "type": "card", "id": "prizes.cell", "props": { "padding": "sm", "radius": "md" },
              "repeat": { "over": { "$data": "raffle/prizes/items" }, "key": "/id", "limit": 60,
                "empty": { "type": "text", "id": "prizes.empty",
                           "props": { "role": "bodySmall", "color": "el9", "text": { "$data": "raffle/prizes/emptyText" } } } },
              "children": [
                { "type": "image", "id": "prizes.cell.image",
                  "visibleWhen": { "not": { "isNull": { "$item": "/imageUrl" } } },
                  "props": { "url": { "$item": "/imageUrl" }, "aspect": 1, "radius": "sm" } },
                { "type": "text", "id": "prizes.cell.title",
                  "props": { "role": "body", "color": "el16", "text": { "$item": "/name" } } },
                { "type": "badge", "id": "prizes.cell.badge",
                  "props": { "title": { "$item": "/thresholdLabel" }, "style": "accent" } }
              ]}
          ]}
        ]},

      { "type": "block.raffle.leaderboard", "id": "tab.table",
        "visibleWhen": { "eq": [ { "$state": "tab" }, "table" ] },
        "props": {
          "rows": { "$data": "board/items" },
          "total": { "$data": "board/total" },
          "self": { "$data": "board/self", "default": null },
          "hasMore": { "$data": "board/hasMore" },
          "nextLimit": { "$data": "board/nextLimit", "default": null },
          "labels": { "$data": "board/labels" }
        },
        "on": { "appear": { "type": "track", "event": "raffle_table_shown" },
                "refresh": { "type": "refresh", "sources": ["board"] } },
        "awaits": ["board"],
        "skeleton": { "type": "skeleton", "id": "tab.table.skeleton",
                      "props": { "shape": "rect", "height": 320 } },
        "fallback": "skip" },

      { "type": "button.link", "id": "table.more",
        "visibleWhen": { "all": [ { "eq": [ { "$state": "tab" }, "table" ] },
                                  { "eq": [ { "$data": "board/hasMore" }, true ] } ] },
        "props": { "title": { "$data": "board/labels/loadMore" } },
        "on": { "tap": { "type": "patchState", "values": { "boardLimit": { "$data": "board/nextLimit" } } } } }
    ]}
}
```

Второй документ — шторка правил, `getBduiScreen(screenId:"raffle.rules")`, целиком декларативный:

```json
{
  "schemaVersion": "2.0.0", "catalogVersion": "2.0.0",
  "screenId": "raffle.rules", "revision": 1487,
  "context": { "brand": "korpu", "locale": "ru", "platform": "ios", "appVersion": "1.42.0", "catalogId": "korpu" },
  "cache": { "ttlSeconds": 900, "policy": "swr" },
  "analytics": { "screen": "raffle_rules" },
  "state": {},
  "data": [ { "id": "rules", "operationId": "raffle.rules", "args": {},
              "cache": { "ttlSeconds": 900 }, "required": true } ],
  "content": {
    "type": "column", "id": "root", "props": { "spacing": "md", "padding": "lg" },
    "children": [
      { "type": "text", "id": "title",
        "props": { "role": "title", "color": "el16", "text": { "$data": "rules/title" } } },
      { "type": "column", "id": "items", "props": { "spacing": "sm" }, "children": [
        { "type": "row", "id": "items.row", "props": { "spacing": "sm", "align": "center" },
          "repeat": { "over": { "$data": "rules/items" }, "key": "/id", "limit": 30 },
          "children": [
            { "type": "avatar", "id": "items.row.icon",
              "props": { "url": { "$item": "/iconUrl", "default": null }, "size": "md", "radius": "sm" } },
            { "type": "column", "id": "items.row.text", "props": { "spacing": "xs" }, "children": [
              { "type": "text", "id": "items.row.title",
                "props": { "role": "body", "color": "el16", "text": { "$item": "/title" } } },
              { "type": "text", "id": "items.row.desc",
                "props": { "role": "caption", "color": "el9", "text": { "$item": "/description" } } }
            ]},
            { "type": "spacer", "id": "items.row.spacer" },
            { "type": "badge", "id": "items.row.tickets",
              "props": { "title": { "$item": "/ticketsLabel" }, "style": "accent" } }
          ]}
      ]},
      { "type": "button.primary", "id": "invite",
        "props": { "title": { "$data": "rules/inviteCta" } },
        "on": { "tap": { "type": "share", "text": { "$data": "rules/shareText" }, "url": { "$data": "rules/link" } } } }
    ]}
}
```

Что этот JSON доказывает по пунктам модели:
- **`repeat`** — задания, дни бонуса, призы, строки шторки; вложенность ≤ 1 соблюдена (сетка призов и полоса дней — плоские `repeat`, внутри только статические дети). `repeat` висит на **ячейке**, а не на `grid`/`row`: иначе размножился бы сам контейнер ([[BDUI-Catalog]] §3.1).
- **`countdown`** — три инстанса (старт, итоги, кулдаун задания), все с `on.expire → refresh`; это единственный способ обойтись без арифметики над временем.
- **`$plural`** — билеты и приглашённые друзья; формы приходят локализованными с бэка.
- **`$text`** — «Ваше место: {place}», шаблон локализован сервером.
- **`visibleWhen`** — четыре состояния кампании (`scheduled/running/finished/won`) и три вкладки на одном документе, без второго запроса при переключении.
- **`call` + `invalidates`** — клейм бонуса и приватность; названия реальных ручек клиент не знает.
- **`patchState`** — вкладка (значение поднимает `{"$event": true}` из `on.change`) и `boardLimit`; смена `boardLimit` меняет `args` источника `board` и вызывает точечный `getBduiData` ([[BDUI]] §4, четвёртый триггер рефреша).
- **`share`/`copyToClipboard`/`haptic`/`track`** — платформенные действия без нативного кода фичи.

---

## 4. Что из legacy НЕ выражается моделью

| Legacy-поведение | Почему не выражается | Решение в пилоте |
|---|---|---|
| Пагинация с **дозагрузкой** таблицы (`offset += 20`, append) | `patchState` не умеет арифметику, а смена `args` источника **заменяет** payload, а не дописывает | Сервер отдаёт **накопительное окно**: `limit=N` возвращает строки 1…N и `nextLimit`. Тогда `patchState({boardLimit: $data board/nextLimit})` — легальный declarative-append. Внутри `block.raffle.leaderboard` бесконечный скролл остаётся нативным |
| Жест раскрытия таблицы поверх картинки приза (`expandProgress` 0…1, арбитраж вложенных скроллов) | это механика лэйаута, а не узел | **Не переносится.** Таблица — обычная секция внутри блока |
| Липкая строка «я», зависящая от `layoutInfo.visibleItemsInfo` | видимость выводится из рантайм-лэйаута | внутри `block.raffle.leaderboard` |
| Церемония «летящих монет» при клейме (гео из `onGloballyPositioned`, хаптик на монету, мутация **после** анимации) | нет узла с координатной анимацией и нет способа отложить `call` до конца анимации | В пилоте — `call` сразу + `haptic`. Если владелец захочет церемонию — `block.raffle.dailyRewards` (контракт ниже) |
| `@mention` внутри абзаца → переход в профиль | `$text` не даёт инлайновых ссылок; в legacy это к тому же хардкод-карта `korpu→7798` | Заменяется явными строками с `open(deeplink)`; хардкод-карта умирает вместе с клиентским маппером |
| Мёртвый `RaffleSuperPrizeView` / `getMyPrizes` | не используется | `getMyPrizes` **оживает** как источник состояния «я выиграл» (`raffle/me/isWinner`, `prizeDeeplink`) — этого состояния в legacy не было вообще |
| Shimmer главного приза, wobble кубка, анимированные полосы прогресса | декоративная анимация вне грамматики | Теряются осознанно. Загрузку закрывает пара `awaits: ["board"]` + `node.skeleton` ([[BDUI]] §4); `fallback` — это про деградацию, а не про загрузку, и у блока он `skip` |
| Оптимистичный тумблер приватности с откатом | `call` асинхронный | `toggle` меняет `state` локально сразу (это и есть оптимистичность), сервер на ошибке возвращает `patch.state = {showFullName: <прежнее>}` — откат выражается конвертом ответа, новых механизмов не нужно |

### Контракты `block.*`

**`block.raffle.leaderboard`** (единственный обязательный):

| prop | тип | смысл |
|---|---|---|
| `rows` | массив `{id, place, name, ticketsLabel, avatarUrl?, isMasked, isSelf, deeplink?}` | строки уже с абсолютным местом и маскированными именами |
| `total` | number | всего участников |
| `self` | `{place, name, ticketsLabel, deeplink?}` \| null | липкая строка «я», если её нет в загруженном окне |
| `hasMore` | bool | |
| `nextLimit` | number \| null | следующее значение накопительного окна |
| `labels` | `{header, emptyText, loadMore, maskedName, selfMarker}` | всё локализовано сервером |

События: `on.refresh` — pull-to-refresh; `on.appear`. Тап по строке открывает `row.deeplink` через платформенный deeplink-слой (allowlist, как у `open`); блок **не** принимает произвольные действия и не умеет `call`.

**`block.raffle.dailyRewards`** (опционально, только если церемония обязательна): props `{days:[{dayLabel, amountLabel, state}], claimAvailable, claimTitle, claimOperationId}`; блок сам вызывает `call(claimOperationId)` после завершения анимации. Пока не заводим — это ровно тот случай, когда `block.*` стоит релиза обоих клиентов ради декоративной анимации.

---

## 5. Требования к бэкенду, вытекающие из пилота

В пейлоаде нет вычислений, поэтому всё перечисленное **обязано** считаться на сервере. Слева — где это считает клиент сегодня.

| Агрегат | Сегодня | Требование |
|---|---|---|
| Билеты за задание | `count × rule.ticketsCount` (iOS `RaffleMapper`, Android `GiveawayRepositoryImpl`) | `tasks.items[].ticketsLabel` (готовая локализованная строка) + `ticketsCount` числом |
| Джойн заданий с правилами, сортировка по `priority`, дедуп дублей и промо | клиент | сервер отдаёт готовый упорядоченный `tasks.items` |
| Абсолютное место | `offset + index + 1` | `rows[].place`, `me.place`, `board.self.place` |
| Доля прогресса | `min(my / max(page), 1)` — заведомо неверно, зависит от загруженной страницы | `me.progressFraction` ∈ [0,1] считается от реального лидера, `me.progressCaption` — строка |
| Маскирование скрытых участников | клиент подставляет `"**********"` | `rows[].name` уже замаскировано, `isMasked: true`, аватар не отдаётся |
| Метка «(Я)» | префикс на клиенте | `rows[].isSelf` + `labels.selfMarker` |
| Множественные формы | `String(format:)` с одним вариантом | либо `$plural.forms` на локаль (как в пилоте), либо готовая строка |
| Группировка призов по `raffleId`, сортировка, дедуп `imageUrl` (кроме `/coin/i`) | клиент | `prizes.items` в готовом порядке, `imageUrl: null` там, где картинка дублируется |
| «от N» на бейдже приза | RU-литерал в маппере | `thresholdLabel` |
| Название главного приза «Квартира в Sea Breeze» | RU-литералы в маппере | `mainPrize.title/subtitle` из CMS |
| Даты и таймеры | `dd.MM.yyyy`, `ru_RU`, `%dдн:%dч:%dм` | ISO-8601 инстанты (`startsAt`, `endsAt`, `cooldownUntil`) + локализованные шаблоны (`countdownTemplate`, `cooldownTemplate`) |
| Статус кампании | не существует | `status: scheduled|running|finished` |
| «Я выиграл» | не существует; `getMyPrizes` не вызывается | `me.isWinner`, `me.prizeDeeplink` |
| Реферальная ссылка и текст шаринга | собираются из `session.user.myRefCode` (на андроиде вообще `""` ⇒ мёртвая кнопка) | `referral.link`, `referral.shareText`, `referral.invitedCount` |
| `hidden` → `visible` | клиент инвертирует, мутация возвращает `isHidden` | ручка `raffle.setVisibility(visible: Bool)`, `state.showFullName` сервер-сидированный |
| Read-after-write лаг после клейма | клиент повторно применяет локальную отметку поверх рефетча (`RaffleViewModel:158-197`) | `call` возвращает `patch.data.raffle` со свежим агрегатом в том же round-trip — костыль исчезает |

### Чем заменяется 5-секундный поллинг андроида

Сегодня: `REALTIME_REFRESH_INTERVAL_MS = 5_000`, ≈9–10 запросов на цикл ⇒ **≈120 запросов/мин на пользователя**, первый кадр ≈15 round-trip.

В пилоте:
1. Первый кадр — **один** запрос `getBduiScreen`; оба источника (`raffle`, `board`) сервер резолвит у себя и инлайнит.
2. Живость — `data[].cache.refreshSeconds: 30` на обоих источниках ⇒ **4 запроса/мин** (`getBduiData`, точечно), падение в **30 раз**; SWR отдаёт кэш мгновенно и обновляет фоном.
3. Мутации не ждут таймера: `call` несёт `invalidates` и возвращает `patch.data`, так что клейм и тумблер обновляют экран синхронно.
4. Пользовательский `refresh` — pull-to-refresh на блоке и `on.expire` у `countdown`.
5. Если продукту нужна секундная живость (последние минуты розыгрыша) — это отдельная GraphQL-подписка `raffleSnapshot`, отдающая **только** `me.ticketsCount/place` и топ-N; её сегодня не существует ни на одной платформе, и в v1 пилота она не нужна.

**Закрыто в модели** (было двумя открытыми вопросами пилота):

1. Источник, чьи `args` ссылаются на `$state`, переопрашивается при изменении ключа — это четвёртый триггер рефреша [[BDUI]] §4: дебаунс 300 мс, во время переопроса показываются прежние данные (не skeleton), при `required: true` — прежние данные плюс индикатор. Пагинация `boardLimit` строится ровно на нём.
2. `idempotencyKey` в документе **не пишется** (клиент чеканит UUID в редьюсере — прецедент `ChatMessageDraft.idempotencyKey`, `Modules/Chat/Domain/Sources/Reducer/ChatReducer.swift`); литерал допустим только для операций «ровно один раз за экран», плейсхолдеры вроде `"auto"` запрещены ([[BDUI]] §6.2). Оба `call` пилота поле опускают.

---

## 6. Локализация

Что есть сейчас: 79 ключей `account.raffle*` в одном каталоге `Account.xcstrings`, языки `az,en,hy,ru,tt,uz`, из них **hy/tt/uz — русские копии**; плюс RU-литералы прямо в мапперах (`"от N"`, `"Квартира в "`, `"Sea Breeze"`, `"**********"`, `"(Я)"`, `"Пользователь #%d"`, форматы дат) и `plurals giveaway_tickets_count` на андроиде.

Что даёт BDUI: **строки уезжают на бэк**. Клиент не несёт ни одного raffle-ключа, локализация становится контентом в CMS, а не релизом. Литералы из мапперов исчезают вместе с мапперами (§5). Множественные формы — `$plural` с формами от сервера (ru: `one/few/many/other`, en: `one/other`, az: `other`).

Что при этом ломается и как чинится:

1. **Клиент умеет ровно три локали.** `Modules/Localization/Sources/AppLanguage.swift` — `enum AppLanguage { ru, en, az }`, и заголовок `language` может быть только одним из трёх. Бренды Khidi (ka), Kuper (tt), Koprik (uz), Aura (ab) **не имеют представимой локали** и получат `ru`/`en`. Это ограничение клиента, а не BDUI: пока `AppLanguage` не расширен, брендовые языки недостижимы и через бэк. Пилот берёт только Korpu (`ru/en/az`) — там ограничение не проявляется.
2. **Сервер должен видеть локаль явно.** Сегодня клиент шлёт заголовок `language` (нижний регистр). Единственное место схемы, где обещан `Accept-Language`, — докстринг `Holiday.name` (`schema.graphqls:3921`); `searchCategoryItems` (`:329`) документирует как раз заголовок `Language`, который клиент и шлёт. То есть расхождение локальное (праздники), а не системное. Для BDUI локаль всё равно обязана быть **аргументом операции**, а не заголовком: `getBduiScreen(screenId:, context:{brand, locale, platform, appVersion})` — это же требование покрывает `platform`/`appVersion`/`brand`, которых в заголовках нет (они спрятаны внутри `User-Agent`).
3. **Фолбэк локали.** Если для запрошенной локали нет перевода строки — сервер отдаёт дефолт кампании и помечает документ; клиент ничего не делает (у него нет ключей и нет фолбэка). Правило фолбэка — серверное, см. [[BDUI-Backend]].
4. **Нетронутые hy/tt/uz копии** перестают быть проблемой релиза: они становятся пустыми полями перевода в CMS, видимыми редактору.
5. **Брендовые различия текста** (в legacy их не было — 0 переопределений) выражаются таргетингом документа по `brand`, а не форком строк.

---

## 7. Приёмка пилота

Пилот считается доказанным, если выполнено всё:

**Контракт**
1. Оба документа (`raffle.main`, `raffle.rules`) валидируются JSON Schema из contracts-репо; golden-фикстуры лежат там же и гоняются в CI обеих платформ.
2. Ни один `props` не содержит хекса, шрифта или числового отступа — только токены из [[BDUI-Catalog]] (проверяется схемой, не ревью).
3. Любой узел неизвестного типа деградирует по `fallback` и не роняет экран (тест: подсунуть `block.raffle.unknown`).

**Клиент**
4. iOS и Android рендерят **один и тот же** документ и дают визуально совпадающий экран на `brand=korpu, locale=ru`, светлая и тёмная тема.
5. Четыре состояния кампании (`scheduled/running/finished/won`) переключаются **сменой данных**, без релиза и без второго запроса.
6. Переключение вкладок — `patchState`, ноль сетевых запросов.
7. `raffle.claimDailyBonus` и `raffle.setVisibility` проходят через generic `call`; в клиентском коде **нет** ни одного упоминания реальных GraphQL-операций раффла.
8. Единственный нативный код фичи — `block.raffle.leaderboard`; в модуле нет ни одной строки продуктового текста и ни одного вычисления из §5.
9. Идентификатор каждого интерактивного узла выведен из его `id` (`bdui.raffle.main.<nodeId>`, экземпляры `repeat` — `…<nodeId>.<key|index>`) и виден Maestro (контейнеры несут `.accessibilityElement(children: .contain)`); флоу «вкладки → клейм → тумблер → шторка правил» зелёный на mock-seeded UITEST-сборке, фикстуры — те же golden-документы.

**Данные и сеть**
10. Первый кадр — один сетевой запрос; фоновая живость — ≤ 4 запроса/мин (замер против legacy ≈120/мин).
11. Документ и оба источника читаются из Postbox namespace `bdui`, без собственного кэша в фиче; после логаута повторный запуск не показывает stale-документ. Механика — [[BDUI]] §11.
12. Офлайн-старт показывает stale-документ, а не ошибку.

**Продукт**
13. Раффл включается/выключается для бренда таргетингом документа, а не `TargetLayout`-флагом и не пересборкой.
14. Правка текста, порядка призов или дедлайна выкатывается ревизией документа без релиза; откат — переключением `revision`.

Если пункты 4, 8 и 10 выполнены — модель v2 доказана на продуктовом экране: одна сборка, один блок нативного кода, тридцатикратное падение трафика.

---

[[BDUI|← модель]] · [[BDUI-Catalog]] · [[BDUI-Backend]] · [[BDUI-Plan]] · [[README|← к обзору]] #arch

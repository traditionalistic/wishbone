# BDUI Catalog — реестр токенов и узлов

> Репозитории и соглашение о путях — [[README#Репозитории]]. Относительные пути в тексте: `Modules/`, `App/`, `Apps/`, `Tuist/`, `Scripts/`, `Tests/` — Stratoapp; `core/`, `feature/`, `graphql/`, `app/` — most; `Korpu/` — multiapp; `schema/`, `catalog/`, `operations/`, `fixtures/`, `tools/`, `bindings/` — bdui-contracts.

> Статус: v2, каталог `catalogVersion 2.0.0`. Зона ответственности этого документа — **что можно назвать в пейлоаде**: закрытые enum токенов, закрытый реестр иконок, реестр узлов с props-схемами и маппингом на три платформы. Грамматика документа (`Node`/`Value`/`Action`/предикаты/деградация/кеш) — в [[BDUI]]. Устройство бэкенда, хранение, реестр `operationId` — в [[BDUI-Backend]]. Здесь нет ни того, ни другого.

Каталог — **источник истины для валидации на записи**: значение, которого нет в перечислениях ниже, делает документ невалидным ещё в админке, а не на клиенте. Физически реестр живёт в contracts-репо (`catalog/nodes/*.schema.json`, `catalog/tokens.json`, `catalog/icons.json`); этот документ — его человеко-читаемое зеркало и место, где фиксируются решения.

Сокращения путей:

| Платформа | Корень | В документе |
|---|---|---|
| iOS (Stratoapp) | `Modules/DesignSystem/Sources/` | `DS/` |
| iOS токены | `Modules/ThemeKit/Sources/Theme.swift` | `Theme.swift:<строка>` |
| Android (most) | `core/ui/src/main/java/com/most/core/ui/` | `$UI/` |
| omniapp (VYSOTA) | `Omniapp/DesignSystem/` | `Omni/` |

⚠️ `/Users/ivan/projects/most` — sparse partial clone: на диске только `core/ui/`, `docs/`, `.claude/`. Всё, что помечено **`[не выкачано]`**, взято из докуметации most и требует подтверждения на полном клоне.

---

## 1. Словарь токенов

### 1.1 Принцип: токен-props — только литерал

Любой prop, чьё значение — токен (`color`, `role`, `spacing`, `padding`, `radius`, `icon`, `style`, `size`), принимает **только литерал из перечисления**. Ссылки `$state`/`$data`/`$item` в токен-props запрещены.

Причина не стилистическая: валидация на записи ([[BDUI-Backend]]) умеет проверить только литерал. Токен, вычисляемый в рантайме, превращает опечатку в бэкенде из ошибки записи в деградацию узла у пользователя. Второе следствие — визуал не может дрейфовать от дизайн-системы через данные.

Динамика внешнего вида делается двумя способами, оба уже в грамматике:
- компонент сам маппит своё состояние на токены (`chip.selected`, `input.text.state`, `button.*.enabled`) — payload передаёт **состояние**, не цвет;
- две ветки узлов + `visibleWhen`.

### 1.2 Цвет — `elN` один enum, слот решает платформа

**Решение (принимаю рекомендацию (b) владельца).** Enum цвета в контракте — iOS-нотация: `bg*`, `el1…el16`, статусы, акценты. Android-рендерер выбирает `text{N}` или `stroke{N}` **по слоту prop**, а не по имени токена.

Обоснование: `token-parity.md` §«element scale» — 16/16 значений hex-идентичны в light и dark, расщепление на `text*`/`stroke*` (`$UI/brand/BaseBrand.kt` L112-154) чисто номинальное, обе семьи биндятся к одному `ElementsLight/Dark.elN`. Слот известен рендереру в момент отрисовки (`props.color` → fill/text, `props.borderColor` → stroke), поэтому алиас — 2 функции на Android и 0 строк на iOS. Обратный вариант (расщепить enum контракта на `text*`/`stroke*`) стоил бы миграции всех iOS call-сайтов ради нулевой выгоды.

| Группа | Легальные значения | Кол-во | iOS | Android | omniapp |
|---|---|---|---|---|---|
| Фон | `bgInput`, `bg1`, `bg1Elevated`, `bg2`…`bg10` | 12 | `theme.colors.<same>` (`Theme.swift:241`) | `MostTheme.colors.<same>` (`$UI/theme/MostColorScheme.kt`) | таблица алиасов (§1.7) |
| Контент/обводка | `el1`…`el16` | 16 | `theme.colors.elN` | fill/text → `colors.text{N}`, border → `colors.stroke{N}` | таблица алиасов |
| Статус | `error`, `errorPressed`, `errorDisabled`, `success`, `successPressed`, `successDisabled`, `warning`, `warningPressed`, `warningDisabled` | 9 | `theme.colors.<same>` | fill/text → `textError*`, border → `strokeError*` | `danger` (только error-ветка) |
| Акцент | `accentPrimary`, `accentSecondary`, `accentTertiary` | 3 | `theme.colors.<same>` | `colors.<same>` | `DSColor.primary` (+2 не имеют пары) |
| **Итого V1** | | **40** | | | |

Не входят в enum и невалидны в пейлоаде: `onAccent`, `accentGradient*`, `BrandPalette c100…c1100`, `navigationFrostButtonTint`, все ~200 feature-scoped полей `SemanticColors` (`comments*`, `story*`, `sidebar*`, `rating*`, `level*`, `composer*`, `camera*`, `editor*`, `map*`, `dating*`, `glass*`, `media*`) и все ~90 feature-полей `MostColorScheme` (`vipGold`, `goldButton*`, `chatBubbleIncoming*`, `spotlightCard*`, вложенные `sidebar`/`rating`/`levelsTierDetails`). Это открытые растущие множества — они не могут быть закрытым enum контракта. Нужен фичевый цвет — это признак того, что нужен `block.*` (§4), а не новый токен.

⚠️ **Акценты легальны по имени, но не по значению.** `token-parity.md` §«Brand roster»: iOS `kopir` (KZ) и `aura` отдают `DefaultTheme()` (cornflower `#5A85D3`), Android `KzBrand` — cyan `#00ABC2`; iOS `koprik` (UZ) `#51C5BD` против Android `UzBrand` `#00ACA0`. Один и тот же документ на KZ/UZ красился по-разному. **Решено 2026-08-21: канон — Android**, iOS перекрашивается; см. §1.6 (DRIFT-1/2).

### 1.3 Типографика — роли родные айосные, платформы подстраиваются

**Решение владельца: канон — iOS.** Enum роли — подмножество уже отгруженной `Typography` (`Modules/ThemeKit/Sources/Theme.swift:1689`); iOS-значения авторитетны, Android приводит свои к ним («ведро поменяет если надо»). Прежняя редакция брала за основу андроидную лесенку `MostTypography` — **отменено**.

Обоснование:
- Значения iOS уже прошли Figma-паритет и снапшот-матрицу 7 брендов × light/dark. Объявив каноном чужую лесенку, мы бы перекрашивали проверенный продукт под неотревьюенные числа.
- iOS-платформа несёт 7 брендов против 4 андроидных и все брендовые шрифтовые слоты (`KorpuFont`), то есть является более широким ограничением. Сужать надо к ней, а не от неё.
- Ценой становится одна таблица на Android вместо одной на iOS — обмен нейтральный по объёму работ и выигрышный по риску.

Из ~50 полей `Typography` в контракт берутся только продуктово-нейтральные роли общего хрома. Фиче-специфичные (`giftHandle`, `mapPromoTitle`, `levelTaskNumeral`, `ratingPodiumName`, `sidebar*`, `editor*`, `registration*`, `wordmark`, `profileStat`) в enum **не входят** — это открытое растущее множество, ему место в `block.*`.

| Легальные значения `role` (18) | Тип на iOS |
|---|---|
| `display`, `greeting`, `largeTitle`, `title`, `headingSmall` | `Font` |
| `body`, `bodyMedium`, `bodyStrong`, `bodySmall`, `bodySmallStrong` | `Font` |
| `caption`, `captionMedium`, `captionStrong` | `Font` |
| `displayAction`, `displayActionCompact` | `Font` |
| `sheetTitle`, `sheetMessage`, `sheetAction` | `TypeToken` (несёт lineHeight) |

| Платформа | Реализация | Статус |
|---|---|---|
| iOS | `BDUITypeRole` → `switch` по существующим полям `Typography` | **есть**, работы ≈0 (только enum и исчерпывающий switch) |
| Android | `BduiTypography` — таблица из 18 `TextStyle`, значения копируются с iOS | **нужно написать** (§7, T-AND-14) |
| omniapp | `DSTypography` → таблица алиасов, приблизительно | **нужно написать**, паритет не гарантируется (dark-only, другие семейства: Suisse Intl / GiorgioSansLCG) |

⚠️ **lineHeight определён не у всех ролей.** 15 из 18 iOS-ролей — `Font`, они несут только семейство/кегль/вес; lineHeight задают лишь три `TypeToken`-роли (`sheetTitle`/`sheetMessage`/`sheetAction`). Для остальных Android копирует кегль и вес, а межстрочный интервал берёт свой дефолтный — **это задокументированное расхождение line box, а не баг рендерера**. Если продукту понадобится точный паритет line box, лечится переводом нужных ролей на `TypeToken` на iOS (роль за ролью, по мере надобности), а не сменой канона.

⚠️ Шрифтовой дрейф семейств, который канон не чинит: Manrope **Light** — только Android; Manrope **ExtraBold** — только iOS; Montserrat **ExtraBold/Black** — только iOS. Роли, чей вес не зарегистрирован на платформе, отдают ближайший зарегистрированный — задокументированная деградация.

### 1.4 Spacing — шкалу владеет контракт, платформы добавляют резолвер

**Решение: оспариваю рекомендацию (c) владельца в этой части.** Буквальное «пустить только совпадающие ступени» даёт **ноль** легальных значений spacing (`token-parity.md` §«Spacing scale»: iOS `xs/sm/md/lg/xl`, Android — семантической шкалы нет вообще, только сырая dp-лесенка `MostSizes.x1…x80` в `$UI/tokens/Sizes.kt`, используемая ровно в одном файле). Без spacing нельзя отрендерить ни один контейнер — каталог перестаёт существовать.

Дешевле обратное: **шкалу определяет контракт**, платформы дают узкий BDUI-резолвер. Значения берутся из уже отгруженной iOS-шкалы (`Spacing`, `Theme.swift:1866`), т.е. iOS-стороне работы 0, Android дописывает ~8 строк, ничего не переименовывая и не мигрируя.

| Значение | pt/dp | iOS | Android | omniapp |
|---|---|---|---|---|
| `xs` | 4 | `theme.spacing.xs` | `MostSpacing.xs` **(новый объект)** | `DSSpacing.micro` (4) |
| `sm` | 8 | `theme.spacing.sm` | `MostSpacing.sm` | `DSSpacing.xxs` (8) |
| `md` | 12 | `theme.spacing.md` | `MostSpacing.md` | `DSSpacing.xs` (12) |
| `lg` | 16 | `theme.spacing.lg` | `MostSpacing.lg` | `DSSpacing.md` (16) |
| `xl` | 24 | `theme.spacing.xl` | `MostSpacing.xl` | `DSSpacing.xxl` (24) |
| `none` | 0 | `0` | `0.dp` | `0` |

`MostSpacing` — новый `object` в `$UI/tokens/Spacing.kt`, 6 значений. Это **не** миграция существующих экранов most: их хардкод `.dp` остаётся как есть, объект нужен только рендереру BDUI (§7, T-AND-1).

### 1.5 Радиусы — то же решение

Совпадают только `sm` и `md`; `lg` и `xl` — value-drift (iOS 20/32 против Android 16/24). Как и со spacing: числа объявляет контракт, платформы дают BDUI-резолвер, ничего не переименовывая.

| Значение | pt/dp | iOS | Android | omniapp |
|---|---|---|---|---|
| `sm` | 8 | `theme.radii.sm` | `MostShapes.sm` — **MATCH** | `DSRadius.sm` (8) |
| `md` | 12 | `theme.radii.md` | `MostShapes.md` — **MATCH** | `DSRadius.md` (12) |
| `lg` | 20 | `theme.radii.lg` | `BduiShapes.lg` **(новый, 20)**; `MostShapes.lg` = 16 не трогаем | `DSRadius.bar` (20) |
| `xl` | 32 | `theme.radii.xl` | `BduiShapes.xl` **(новый, 32)**; `MostShapes.xl` = 24 не трогаем | ≈ `DSRadius.container` (26), дрейф |
| `card` | 32 | `theme.radii.card` | `BduiShapes.xl` | ≈ `DSRadius.container` |
| `pill` | капсула | `Capsule()` | `MostShapes.full` (`CircleShape`) — **MATCH** | `DSRadius.pill` (999) |
| `none` | 0 | `0` | `MostShapes.none` | `0` |

`BduiShapes` — новый `object` в `$UI/theme/BduiShapes.kt`, 2 значения. Осознанно **не** правим `MostShapes.lg/xl` — это тронуло бы весь most ради BDUI.

### 1.6 DRIFT — что расходится сейчас

| # | Что | iOS | Android | omniapp | Последствие | Действие |
|---|---|---|---|---|---|---|
| DRIFT-1 | Акценты KZ | `kopir` = cornflower `#5A85D3` (бренд не покрашен вовсе — `DefaultTheme()`) | `KzBrand` = `KazakhstanCyan` c100/c700/c1000 = `#00ABC2` / `#44C2D7` / `#AFE5F3` | — | `accentPrimary` красился по-разному на одном документе | **РЕШЕНО 2026-08-21: канон — Android.** Заводится `KopirTheme` с андроидным триплетом; запрет на `accent*` снят после перекраски |
| DRIFT-2 | Акценты UZ | `koprik` `#51C5BD`/`#B9D9D6`/`#D6E5E4` (подобран вручную) | `UzBrand` = `UzbekTeal` c100/c700/c1000 = `#00ACA0`/`#44D7D0`/`#AFF2F3` | — | то же, все 3 акцента | **РЕШЕНО 2026-08-21: канон — Android.** `KoprikTheme` правится под `UzbekTeal`. Заметка: `#51C5BD` уже живёт на Android как `diamondGradientStart` у `UzBrand` — оттенок тот же, роль другая |
| DRIFT-3 | `captionMedium` | Manrope 12 medium | Manrope 10 Medium / lh14 | — | одно имя, разный кегль | закрывается §1.3: канон — iOS (Manrope 12), Android копирует его в `BduiTypography` (T-AND-14); собственный `MostTypography.captionMedium` для нативных экранов не меняется |
| DRIFT-4 | `radii.lg` / `radii.xl` | 20 / 32 | 16 / 24 | 20 / ≈26 | углы разной крутизны | закрывается §1.5 (`BduiShapes`) |
| DRIFT-5 | lineHeight | `Font`-роли не несут | каждая роль несёт | не несёт | разный line box | закрывается §1.3 (iOS отдаёт `TypeToken`) |
| DRIFT-6 | `BrandPalette c100…c1100` | одна `.cornflower` на все бренды (`Theme.swift:86`, дефолт протокол-расширения, ни один бренд не переопределяет) | 3 рампы: `CornflowerBlue`, `KazakhstanCyan`, `UzbekTeal` | нет | палитра не может быть в enum | **не входит** в контракт |
| DRIFT-7 | Тема | light+dark | light+dark | **dark-only**, хардкод hex | документ, сверстанный под light, на omniapp всегда dark | принято: инвариант «один токен = одна light/dark пара» на omniapp ложен |
| DRIFT-8 | Бренд-роспись | 7 брендов | 4 (`AZ/JEWISH/KZ/UZ`), `JewishBrand : BrandTokens by AzBrand` (плейсхолдер) | 1 (`ru.vysota`) | `context.brand` — не общий enum; `KorpuBrand` (iOS) вообще перечисляет только `khidi/kamurj/kuper` | `context.brand` — свободная строка бренд-slug, не enum; см. [[BDUI-Backend]] |

### 1.7 omniapp — таблица алиасов, а не переименование

`omniapp-fit.md` §8: `DSColor` — плоские статики с литеральным hex, dark-only, имена продуктовые (`background/surface/elevated/primary/textPrimary/textSecondary/danger/stroke/fieldFill`). Переименовывать его в `bg1…el16` — это правка каждого DS-компонента omniapp ради нулевой продуктовой выгоды. Вместо этого — таблица ~30 строк в модуле `BDUIRender` omniapp:

| Токен контракта | omniapp |
|---|---|
| `bg1` | `DSColor.background` |
| `bg1Elevated` | `DSColor.elevated` |
| `bg2` | `DSColor.surface` |
| `el16`, `el15` | `DSColor.textPrimary` |
| `el9`, `el10` | `DSColor.textSecondary` |
| `el3` | `DSColor.stroke` |
| `error*` | `DSColor.danger` |
| `accentPrimary` | `DSColor.primary` |
| `accentSecondary/Tertiary` | пары нет → деградация `skip` |
| остальные `bgN`/`elN` | ближайшая ступень по яркости, таблица фиксирована в коде |

Тот же принцип у `catalogId`: документ, свёрстанный под `korpu`, не отдаётся `vysota` — таргетирование по `catalogId` описано в [[BDUI-Backend]].

---

> **Правки волны 1 (2026-08-26), внесены по факту сборки контракта.**
> - `alignment` → **`align`**: обе golden-фикстуры и §3.2 у `text` используют `align`, два имени под одну роль на соседних узлах не выживают.
> - дефолт `role` был `body2` — такой роли нет среди 18 легальных §1.3. Теперь **`body`**.
> - Стартовый набор иконок сверен с полным клоном `most`: в `:core:ui` реально лежат 7 из 14 (`ic_close`, `ic_back`, `ic_chevron_right`, `ic_info_circle`, `ic_share`, `ic_copy`, `ic_gift`); `ic_chevron_down`, `ic_check`, `ic_plus`, `ic_clock` существуют только внутри feature-модулей и требуют переноса в `:core:ui`; `ticket`, `warning`, `retry` не существуют вовсе и требуют отрисовки. Разрыв зафиксирован блоком `pending` в `catalog/icons.json`. Пример §2 называет android-ресурс `ic_info`, фактический — `ic_info_circle`, и ставит `"omni": null` у `ticket`, хотя `ticket` — валидный SF Symbol, а omniapp живёт на SF Symbols (214 файлов с `systemName:`); в контракте у него символ.

## 2. Реестр иконок

JSON не умеет ни `@DrawableRes`, ни `SF Symbol`. iOS активно живёт на SF Symbols (379 вызовов `Image(systemName:)` в `Modules/`, против 151 `Image("…")` из ассетов), Android — только на drawable-ресурсах, SF Symbols там нет физически. `Modules/DesignSystem/Resources/Assets.xcassets` содержит 7 фиче-групп и один `familyTreeIcon` — общего иконочного набора у iOS DS нет.

**Решение: закрытый реестр `catalog/icons.json` в contracts-репо, codegen в enum на каждой платформе.**

```json
{ "version": "2.0.0", "icons": [
  { "name": "close",       "ios": "xmark",             "android": "ic_close",       "omni": "xmark" },
  { "name": "chevronRight","ios": "chevron.right",     "android": "ic_chevron_right","omni": "chevron.right" },
  { "name": "info",        "ios": "info.circle",       "android": "ic_info",        "omni": "info.circle" },
  { "name": "ticket",      "ios": "ticket",            "android": "ic_ticket",      "omni": null }
]}
```

Правила:
- `props.icon` принимает **только `name` из реестра**, только литералом. Свободная строка (в т.ч. валидный SF Symbol) — невалидный документ.
- Значение `null` на платформе = иконки нет ⇒ узел деградирует по своему `fallback` (по умолчанию `skip`). Отсутствие иконки никогда не роняет экран.
- Пополнение реестра иконкой, у которой уже есть ресурс на всех платформах, **не требует релиза** (реестр читается как данные) — но требует бампа `catalogVersion` (patch) и прогона codegen. Иконка, требующая нового drawable на Android, — требует релиза Android.
- Codegen: `BDUIIcon` (Swift enum + `var image: Image`), `BduiIcon` (Kotlin enum + `@DrawableRes val res: Int`). Генератор и CI-гейт — в [[BDUI-Backend]].
- **Remote-иконки (SVG/URL) — вне V1/V2.** Узел `image` покрывает растровые URL-картинки; векторные удалённые иконки — V3 вместе с remote-lottie.

Стартовый набор V1 — иконки, реально нужные пилоту раффла и generic-хрому: `close`, `back`, `chevronRight`, `chevronDown`, `info`, `check`, `plus`, `share`, `copy`, `ticket`, `gift`, `clock`, `warning`, `retry`. 14 позиций.

---

## 3. Реестр узлов

Легенда колонки **Value**: `V` — prop принимает любой `Value` (литерал или `$state`/`$data`/`$item`/`$context`/`$text`/`$plural`); `L` — **только литерал** (все токен-props, см. §1.1).
Легенда `on`: перечислены события, легальные для узла. Событие вне списка — невалидный документ; сам набор событий закрыт в [[BDUI]] §3.
Колонка **omni**: `+` есть компонент, `~` есть приблизительный, `−` нет.

**Общий prop всех узлов:** `accessibilityLabel` — string, принимает `Value`, необязателен. Идентификатор узла (`bdui.<screenId>.<nodeId>`) выводится из структуры и prop'ом **не задаётся** (§6). Для `text` метка по умолчанию — собственный текст узла.

### 3.0 Оговорка к инварианту «один узел = один DS-компонент»

[[BDUI]] §3 требует, чтобы узел рендерился одним нативным DS-компонентом. Для **layout-примитивов** (`column`, `row`, `box`, `spacer`, `divider`, `scroll`, `grid`) это правило неприменимо и осознанно снято: ни на одной из трёх платформ нет и не будет DS-обёртки над `VStack`/`Column`. Они рендерятся foundation-примитивами напрямую, с параметрами **только из шкал §1.4/§1.5**. Для всех остальных 36 узлов инвариант в силе.

### 3.1 Layout — 7 узлов

| Узел | Назначение | iOS | Android | omni | V |
|---|---|---|---|---|---|
| `column` | вертикальный стек | `VStack` | `Column` | + | V1 |
| `row` | горизонтальный стек | `HStack` | `Row` | + | V1 |
| `box` | наложение слоёв | `ZStack` | `Box` | + | V1 |
| `spacer` | распорка | `Spacer` | `Spacer` | + | V1 |
| `divider` | разделитель | `Rectangle` + `theme.colors.divider`, `LayoutMetrics.hairlineWidth` (`Theme.swift:1873`) | `HorizontalDivider` **(нужна BDUI-обёртка)** | − | V1 |
| `scroll` | скролл-контейнер | `ScrollView` | `Column`+`verticalScroll` / `LazyColumn` | + | V1 |
| `grid` | сетка N колонок | `LazyVGrid` | `LazyVerticalGrid` | + | V1 |

| prop | тип | Value | обяз. | дефолт | применимо к |
|---|---|---|---|---|---|
| `spacing` | enum §1.4 | L | нет | `none` | column,row,scroll,grid |
| `padding` | enum §1.4 \| `{top,leading,bottom,trailing}` | L | нет | `none` | все |
| `align` | `start\|center\|end\|stretch` | L | нет | `start` | column,row,box |
| `background` | enum цвета §1.2 | L | нет | — | column,row,box,scroll,grid |
| `radius` | enum §1.5 | L | нет | `none` | column,row,box |
| `border` | `{color, width: 1\|2}` | L | нет | — | column,row,box |
| `weight` | number ≥0 | L | нет | — | spacer (внутри row/column) |
| `height`/`width` | number pt/dp | L | нет | — | spacer, divider |
| `columns` | int 2…4 | L | **да** | — | grid |
| `axis` | `vertical\|horizontal` | L | нет | `vertical` | scroll |

`on`: `appear`, `disappear`, `tap` (кроме `spacer`, `divider`). `repeat` разрешён на `column`/`row`/`grid` (вложенность ≤ 1, [[BDUI]] §5). ⚠️ `repeat` на контейнере размножает **сам контейнер**: чтобы размножить ячейки внутри сетки/ряда, `repeat` вешается на дочерний узел, а не на `grid`/`row`.

⚠️ `scroll` внутри `scroll` той же оси — невалидный документ (валидация на записи): вложенные скроллы одной оси не разрешимы ни в SwiftUI, ни в Compose без арбитража жестов.

### 3.2 Контент — 8 узлов

| Узел | iOS | Файл | Android | omni | V |
|---|---|---|---|---|---|
| `text` | `Text` + `Typography` по `BDUITypeRole` | `Theme.swift:1689` | `MostText` + `BduiTypography` (нет ни обёртки, ни таблицы — **T-AND-2**, **T-AND-14**) | + | V1 |
| `image` | `RemoteImageView` | `DS/Components+RemoteImage.swift:132` (удобный init `Placeholder == EmptyView`) | Coil `AsyncImage`, **нет DS-обёртки (T-AND-3)** | ~ | V1 |
| `icon` | `Image(systemName:)` через `BDUIIcon` | §2 | `Icon(painterResource(BduiIcon.res))` | + | V1 |
| `avatar` | `CommentsAuthorAvatar` | `DS/Components+CommentsAuthorAvatar.swift:15` | `UserAvatar` | `$UI/components/user/UserAvatar.kt` | − | V1 |
| `avatarStack` | — **(T-iOS-2)** | — | — **(T-AND-4)** | − | V2 |
| `badge` | — **(T-iOS-3)**; `ProfileTagNameBadge` непригоден (импортирует `SharedModels`) | `DS/Components+ProfileTagNameBadge.swift:21` | — **(T-AND-5)** | − | V1 |
| `lottie` | `LottieView` (**только bundled**, ThorVGKit) | `DS/Components+Lottie.swift:10` | — | − | V3 |
| `video` | `MetalVideoPlayerView` | `DS/Components+MetalVideo.swift:171` | Media3 | − | V3 |

| prop | тип | Value | обяз. | дефолт | узлы |
|---|---|---|---|---|---|
| `text` | string | **V** | да | — | text |
| `role` | enum §1.3 | L | нет | `body` | text |
| `color` | enum §1.2 | L | нет | `el16` | text, icon, badge |
| `align` | `start\|center\|end` | L | нет | `start` | text |
| `maxLines` | int 1…20 | L | нет | — | text |
| `url` | string (https) | **V** | да | — | image, avatar, lottie, video |
| `aspect` | number 0.2…5 | L | нет | `1` | image, video |
| `contentMode` | `fit\|fill` | L | нет | `fill` | image |
| `radius` | enum §1.5 | L | нет | `none` | image, avatar |
| `placeholder` | enum цвета §1.2 | L | нет | `bg2` | image, avatar |
| `icon` | enum §2 | L | да | — | icon |
| `size` | `sm(16)\|md(24)\|lg(32)` | L | нет | `md` | icon, avatar, badge |
| `initials` | string ≤2 симв. | **V** | нет | — | avatar |
| `title` | string | **V** | да | — | badge |
| `style` | `neutral\|accent\|success\|warning\|error` | L | нет | `neutral` | badge |
| `loop` | bool | L | нет | `true` | lottie, video |

`on`: `tap`, `longPress`, `appear`, `disappear`. У `video`/`lottie` дополнительно `expire` (конец воспроизведения).

⚠️ `image`/`avatar` на iOS требуют инъекции `\.mediaLoader` из composition root — дефолт `NoopMediaLoading()` (`DS/Components+MediaLoader.swift`) молча рисует пустоту. Проводка — обязательный пункт приёмки рендерера ([[BDUI]] §13).

### 3.3 Поверхности — 5 узлов

| Узел | Назначение | iOS | Android | omni | V |
|---|---|---|---|---|---|
| `card` | карточка-контейнер | `CardSurface` (`DS/Components.swift:68`, `@ViewBuilder`-контейнер — подходит) | — **(T-AND-6)**; `SpotlightCard` не годится (зашитый levels-артворк) | − | V1 |
| `surface` | плоская подложка/секция | `RoundedRectangle` + токены | `Surface` | ~ | V1 |
| `surface.frosted` | **матовая** стеклянная подложка | `FrostGlassBackground` (`DS/Components+Frost.swift:37`) + `FrostBlurStyle` (`:129`) | `BlurSurface` (`$UI/components/common/BlurSurface.kt:18`) | − **(T-OMN-2)** | V1 |
| `banner` | компактная вставка в placement-слот | `OperationNoticeBanner` (`DS/Components+InAppNotice.swift:130`) | — **(T-AND-7)**; `AlertCard` — транзиентный тост, не inline-вставка | ~ `AlertBanner` (Energy-специфичный) | V1 |
| `sheet.prompt` | подтверждение в шите | `BottomSheetPrompt` (`DS/Components+BottomSheet.swift:1031`) — точное совпадение, включая `primaryAccessibilityIdentifier` | `AppAlertDialog` / `BottomSheetPrimary` (`$UI/components/dialog/`, `$UI/components/sheet/`) | ~ | V1 |

| prop | тип | Value | обяз. | дефолт | узлы |
|---|---|---|---|---|---|
| `background` | enum §1.2 | L | нет | `bg1Elevated` | card, surface |
| `radius` | enum §1.5 | L | нет | `card` | card, surface, surface.frosted |
| `padding` | enum §1.4 | L | нет | `lg` | card, surface, surface.frosted |
| `border` | `{color,width}` | L | нет | — | card, surface |
| `elevated` | bool | L | нет | `false` | card |
| `blur` | `soft\|regular\|deep` | L | нет | `regular` | surface.frosted |
| `tint` | enum §1.2 | L | нет | — | surface.frosted |
| `message` | string | **V** | да | — | banner |
| `style` | `info\|success\|warning\|error` | L | нет | `info` | banner |
| `icon` | enum §2 | L | нет | — | banner |
| `dismissible` | bool | L | нет | `true` | banner |
| `title`, `message` | string | **V** | да | — | sheet.prompt |
| `primaryTitle` | string | **V** | да | — | sheet.prompt |
| `secondaryTitle` | string | **V** | нет | — | sheet.prompt |

`on`: `tap`, `appear`, `disappear`; у `banner` дополнительно `dismiss`; у `sheet.prompt` — `primaryTap`, `secondaryTap` вместо `tap`.
`children`: `card`, `surface` — контейнеры; `banner`, `sheet.prompt` — листья.

### 3.4 Действия — 6 узлов

| Узел | iOS | Файл | Android | omni | V |
|---|---|---|---|---|---|
| `button.primary` | `MainButton(style: .primary)` | `DS/Components+MainButton.swift:17` | `PrimaryButton` | `$UI/components/button/PrimaryButton.kt` | + | V1 |
| `button.secondary` | `MainButton(style: .secondary)` | там же | `PrimaryVariantButton(style: .Variant)` | `$UI/components/button/PrimaryVariantButton.kt` | + | V1 |
| `button.link` | `LinkButton` | `DS/Components+Brand.swift:40` | — **(T-AND-8)** | ~ `SecondaryButton` | V1 |
| `button.icon` | `CircularIconButton` | `DS/Components+CircularIconButton.swift:30` | `PlainHeaderIconButton` | `$UI/components/navigation/PlainHeaderIconButton.kt` | + `CircleGlassIconButton` | V1 |
| `chip` | `FilterChipRow`+`FilterChipModel` на 1 элемент | `DS/Components+FilterChip.swift:14,58` | `WhiteTintPressedButton` | `$UI/components/button/WhiteTintPressedButton.kt` | − | V1 |
| `chipRow` | горизонтальный ряд чипов | `FilterChipRow` | там же | ряд `WhiteTintPressedButton` в `Row`+scroll | − | V1 |

| prop | тип | Value | обяз. | дефолт | узлы |
|---|---|---|---|---|---|
| `title` | string | **V** | да (кроме `button.icon`) | — | все |
| `enabled` | bool | **V** | нет | `true` | все |
| `loading` | bool | **V** | нет | `false` | button.* |
| `size` | `standard\|compact` | L | нет | `standard` | button.primary/secondary (→ `MainButtonSize.standard`/`.compactDisplay`) |
| `icon` | enum §2 | L | да у `button.icon`, нет у прочих | — | все |
| `destructive` | bool | L | нет | `false` | button.primary/secondary |
| `selected` | bool | **V** | нет | `false` | chip |
| `items` | массив `{id,title,selected}` | **V** | да | — | chipRow |

`on`: `tap` (**обязателен** — кнопка без `on.tap` невалидна), `longPress`, `appear`.

**Pending-состояние.** Если `on.tap` — это `action.call`, узел `button.*` сам уходит в loading на время полёта и блокирует повторный тап; `props.loading` при этом игнорируется. Это поведение рендерера, не пейлоада.

⚠️ `MainButtonStyle.datingVIP` **не выставляется в контракт**: gold-инвариант из [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) («Gold разрешён только для Dating VIP») не может проверяться на стороне бэкенда.

⚠️ Стекло в каталоге есть, но **только матовое** — узел `surface.frosted` (§3.3). Решение владельца; прежняя редакция исключала стекло целиком — отменено.

**Почему именно матовое, а не liquid glass.** Матовая поверхность — это «размытие фона + полупрозрачная заливка + тень», и ровно это делают обе платформы уже сейчас: iOS `FrostGlassBackground`, Android `BlurSurface` (`drawPlainBackdrop` + `blur` + 60 % `bg1`). Liquid glass — другой класс: линза, рефракция, falling light, vibrancy (`GlassSurface(shouldUseLens/refractionAmount/shouldUseVibrancy…)`, `LiquidGlassVariant` на iOS). Он физически зависит от того, **что** под ним лежит и **на какой глубине**, а дерево из пейлоада эту композицию не контролирует. Поэтому в каталог не входят: `GlassPillButton`, `CircleGlassIconButton`, `GlassSegmentedTabs`, `GlassPopupMenu`, `LiquidSlider`, `SearchInput` и любые `LiquidGlassVariant`-поверхности — им место в `block.*`, где натив владеет композицией.

Три правила, без которых `surface.frosted` ломается:

1. **Вложение запрещено.** `surface.frosted` не может содержать `surface.frosted` ни на какой глубине — правило most (`.claude/rules/compose-patterns.md`) запрещает стекло в стекле, а на iOS вложенный blur даёт двойное затемнение. Проверяется валидатором **на записи** ([[BDUI-Backend]] §8.2), а не рендерером.
2. **Backdrop — один на документ.** Android-рендерер поднимает единственный `rememberLayerBackdrop()` в корне документа и передаёт его каждому `surface.frosted`. Пейлоад про backdrop не знает и управлять им не может.
3. **Матовость деградирует, а не падает.** Если платформа не может дать blur (снижен quality scale, Reduce Transparency, отсутствует backdrop), узел рисуется как обычный `surface` с тем же `background`/`radius`/`padding`. Это штатная деградация, телеметрия по §12 [[BDUI]], не `BDUI_UNKNOWN_NODE`.

### 3.5 Формы — 8 узлов

Все form-узлы несут `form: { formId, stateKey, validation }` ([[BDUI]] §7) и пишут значение в `state[stateKey]`.

| Узел | iOS | Файл | Android | omni | V |
|---|---|---|---|---|---|
| `input.text` | `AuthInputField` | `DS/Components+Auth.swift:42` | `InputPrimary` | `$UI/components/input/InputPrimary.kt` | + `AuthEmailField` | V2 |
| `input.otp` | `OTPCodeField` | `DS/Components+OTP.swift:10` | `CodeField` | `$UI/components/input/CodeField.kt` | + `OTPCodeField` | V2 |
| `toggle` | — **(T-iOS-4)**; в DS нет публичного `Toggle`-компонента | — | `SwitchPrimary` | `$UI/components/toggle/SwitchPrimary.kt` | − | **V1** (нужен пилоту: «показывать имя в таблице») |
| `select` | — **(T-iOS-5)** | — | `WheelPicker` / `RadioPrimary` | `$UI/components/picker/`, `radio/` | − | V2 |
| `slider` | — **(T-iOS-6)** | — | `LiquidSlider` (нужен `Backdrop` ⇒ **T-AND-9**) | − | V3 |
| `stepper` | — **(T-iOS-7)** | — | — **(T-AND-10)** | − | V3 |
| `datePicker` | — **(T-iOS-8)** | — | `BirthdayWheelPicker` / `MostCalendar` | `$UI/components/picker/`, `calendar/` | − | V3 |
| `segmented` | `GlassSegmentedTabs` | `DS/Components+GlassSegmentedTabs.swift:26` (полностью generic по `Tab`) | `SegmentedTabs` (нужен `Backdrop`) | `$UI/components/tabs/SegmentedTabs.kt` | ~ | V2 |

| prop | тип | Value | обяз. | дефолт | узлы |
|---|---|---|---|---|---|
| `placeholder` | string | **V** | нет | — | input.text |
| `keyboard` | `text\|email\|phone\|number` | L | нет | `text` | input.text |
| `secure` | bool | L | нет | `false` | input.text |
| `length` | int 4…8 | L | нет | `4` | input.otp |
| `label` | string | **V** | нет | — | toggle, select, slider, stepper, datePicker |
| `options` | массив `{value,title}` | **V** | да | — | select, segmented |
| `min`/`max`/`step` | number | L | да | — | slider, stepper |
| `format` | `date\|dateTime` | L | нет | `date` | datePicker |

`on`: `change` (значение изменилось), `appear`, `disappear`. `tap` на form-узлах невалиден — интеракция задаётся компонентом.

**Ошибки поля** приходят в конверте ответа `call/submit` как `fieldErrors: { stateKey → message }` и рендерятся самим form-узлом; отдельного узла ошибки в каталоге нет.

⚠️ iOS-компоненты форм — `Binding`-based (Tier C по `ios-uikit.md` §3): рендерер обязан владеть стейтом и мостить `state[stateKey]` ↔ `Binding`. Это уже заложено в `BDUIStore` ([[BDUI]] §13), но на приёмке проверяется отдельно.

### 3.6 Навигация и раскрытие — 4 узла

| Узел | Назначение | iOS | Android | omni | V |
|---|---|---|---|---|---|
| `tabs` | переключение секций внутри документа | `GlassSegmentedTabs` (`DS/Components+GlassSegmentedTabs.swift:26`) | `SegmentedTabs` / `GlassFilterTabs` (`Backdrop`) | ~ `PageControl` | V1 |
| `accordion` | раскрывающийся блок | — **(T-iOS-9)** | — **(T-AND-11)** | − | V2 |
| `carousel` | горизонтальная лента карточек | `ScrollView(.horizontal)` + `LazyHStack` | `LazyRow` | + | V2 |
| `list.section` | секция строк с заголовком | `GlassMenuCard`+`GlassMenuRow` (`DS/Components+GlassMenu.swift:4,37`) | `MenuListItem` в `Column` (`$UI/components/menu/MenuListItem.kt`) | − | V1 |

| prop | тип | Value | обяз. | дефолт | узлы |
|---|---|---|---|---|---|
| `items` | массив `{id,title,badge?}` | **V** | да | — | tabs |
| `selected` | string (id) | **V** | да | — | tabs |
| `title` | string | **V** | нет | — | accordion, list.section |
| `expanded` | bool | **V** | нет | `false` | accordion |
| `rows` | массив `{id,title,icon?,trailing?,destructive?}` | **V** | да | — | list.section |
| `itemWidth` | number pt/dp | L | нет | — | carousel |

`on`: `tabs` → `change`; `accordion` → `change`, `tap`; `list.section` → `rowTap` (несёт `$item` строки); `carousel` → `appear`, `disappear`.

`tabs` **не** переключает содержимое сам: канон — `on.change → patchState({tab: …})` + `visibleWhen` на ветках. Так каталог не обзаводится собственной моделью навигации.

### 3.7 Состояние — 5 узлов

| Узел | Назначение | iOS | Android | omni | V |
|---|---|---|---|---|---|
| `progress` | линейный/кольцевой прогресс | — **(T-iOS-10)**; в DS нет прогресс-бара | `Loader(progress:)` / `RadialProgressIndicator` (`$UI/components/common/`) | + `ProgressRing` | V1 |
| `skeleton` | плейсхолдер загрузки | `SkeletonView`/`SkeletonLine`/`SkeletonCircle` (`DS/Components+Shimmer.swift:6,81,64`) | `SkeletonLine`/`SkeletonCircle`/`SkeletonImage` (`$UI/components/shimmer/Shimmer.kt`) | − | V1 |
| `loader` | индетерминированный спиннер | `ProgressView()` | `Loader` (`$UI/components/common/Loader.kt`) | + | V1 |
| `countdown` | обратный отсчёт до момента | — **(T-iOS-11)** | — **(T-AND-12)** | − | V1 |
| `emptyState` | пустое/ошибочное состояние + CTA | `EmptyStatePlaceholder` (`DS/Components+EmptyState.swift:4`) | — **(T-AND-13)** | − | V1 |

| prop | тип | Value | обяз. | дефолт | узлы |
|---|---|---|---|---|---|
| `value` | number 0…1 | **V** | да | — | progress |
| `style` | `linear\|ring` | L | нет | `linear` | progress |
| `label` | string | **V** | нет | — | progress, countdown |
| `shape` | `line\|circle\|rect` | L | нет | `line` | skeleton |
| `width`/`height` | number | L | нет | — | skeleton |
| `until` | ISO-8601 UTC | **V** | да | — | countdown |
| `format` | `dhms\|hms\|ms` | L | нет | `hms` | countdown |
| `icon` | enum §2 | L | нет | — | emptyState |
| `title` | string | **V** | да | — | emptyState |
| `subtitle` | string | **V** | нет | — | emptyState |

`on`: `countdown` → `expire` (обязателен, иначе истёкший отсчёт молча замирает), `appear`; остальные → `appear`, `disappear`.

⚠️ `countdown` — единственный узел с собственным таймером. Он **обязан** тикать через один шаренный тикер рендерера, а не заводить таймер на узел: несколько `countdown` в одном документе — норма (пилот раффла: таймер шапки + таймер каждой задачи). Это требование к рендереру, а не к пейлоаду.

### 3.8 Сводка версий

| Версия | Узлов | Состав |
|---|---|---|
| **V1** | 32 | все layout (7), `text`/`image`/`icon`/`avatar`/`badge` (5), все поверхности (5, включая `surface.frosted`), все действия (6), `toggle`, `tabs`, `list.section`, все состояния (5), `block.*` (пилоту раффла нужен `block.raffle.leaderboard` уже в V1 — [[BDUI-Raffle-Pilot]] §2) |
| **V2** | 7 | `avatarStack`, `input.text`, `input.otp`, `select`, `segmented`, `accordion`, `carousel` |
| **V3** | 5 | `lottie`, `video`, `slider`, `stepper`, `datePicker` |
| **Итого** | **44** | 43 каталожных + `block.*` |

---

## 4. `block.<name>` — эскалация в натив

Единственный тип узла, расширение которого стоит релиза **обоих** клиентов. Это осознанная цена: `block.*` — то место, куда уезжает всё, что не выражается §3.

Правила:
- Имя — `block.<feature>.<name>`, kebab/dot-case, из реестра `catalog/blocks/*.schema.json`.
- Props — **узкий типизированный контракт** конкретного блока, свой JSON Schema. Никаких «сырых» деревьев внутри props.
- Блок сам ходит за данными **только через `data`-источники документа** (`$data`-ссылки в props) — своих сетевых вызовов у него нет; иначе он перестаёт быть узлом каталога и становится обычной фичей.
- Блок не имеет `children`. Композиция — снаружи.
- Неизвестное имя блока деградирует по `fallback` узла (по умолчанию `skip`), как любой неизвестный `type`.
- Регистрация блока = строка в реестре + запись в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) (если появился новый DS-компонент) + бамп `catalogVersion` **minor** + релиз обоих клиентов.

Заявленные пилотом раффла блоки (детали — в [[BDUI-Raffle-Pilot]]):

| Блок | Почему не выражается каталогом |
|---|---|
| `block.raffle.leaderboard` | пагинация, абсолютная нумерация мест, липкий футер текущего пользователя, маскирование имён, жест expand-over-car с арбитражем внутреннего/внешнего скролла. **Входит в V1** (единственный обязательный блок пилота); контракт props — [[BDUI-Raffle-Pilot]] §4 |
| `block.raffle.ticketProgress` | анимированный полосатый прогресс с thermal-гейтом и tap-through |
| `block.raffle.tasks` | живой countdown в каждой строке + tappable `@mention` в attributed-тексте + вложенные списки прогресса |
| `block.raffle.dailyRewards` | горизонтальный рельс с per-cell анимацией завершения и loading/hidden состояниями кнопки клейма |

---

## 5. Правила расширения каталога

| Изменение | Релиз клиентов | Бамп | Что ещё |
|---|---|---|---|
| Новая иконка, ресурс есть на всех платформах | **нет** | `catalogVersion` patch | codegen `BDUIIcon`/`BduiIcon`, golden-фикстура |
| Новая иконка, нужен новый drawable/asset | да (той платформы) | patch | тот же codegen |
| Новый **опциональный** prop у существующего узла | **нет** (старые клиенты игнорируют неизвестный prop) | minor | обновить props-схему + фикстуру |
| Новый **обязательный** prop | да | **major** | старые клиенты не смогут отрендерить узел |
| Новый узел каталога | да | minor | props-схема, маппинг всех 3 платформ, golden-фикстура, snapshot-тест, строка в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) если завёлся DS-компонент |
| Новый `block.*` | да | minor | то же + JSON Schema блока |
| Новое значение в enum токена | да | minor | таблицы §1 на всех платформах + DRIFT-проверка |
| Удаление узла/prop/токена | да | **major** | документы старых ревизий должны остаться валидными ⇒ практически всегда deprecate, а не удаление |

**Строгий decode.** Обе платформы декодят строго: незнакомое **поле** внутри известного узла — не ошибка (forward-compat для опциональных props), незнакомый **`type` узла** — деградация по `fallback`, незнакомое значение **enum-токена** — деградация по `fallback`. Незнакомое поле на уровне `ScreenDocument` — битый документ. Разграничение зафиксировано golden-фикстурами.

**Путь в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md).** Новый узел, потребовавший нового DS-компонента, регистрируется там по обычному reuse-first протоколу: компонент заводит агент `design-system`, добавляет `#Preview`, вносит строку в реестр. BDUI-каталог **не** дублирует эту строку — он ссылается на имя компонента. Обратное тоже верно: компонент, существующий в `DesignSystem`, не становится узлом BDUI автоматически — узел заводится только явной строкой здесь.

---

## 6. a11y и селекторы

Каждый узел получает стабильный идентификатор `bdui.<screenId>.<nodeId>`; узлы, материализованные `repeat`, — `bdui.<screenId>.<nodeId>.<key|index>` (`key` из `repeat.key`, иначе индекс; формат — [[BDUI]] §5, зафиксирован golden-фикстурой).

| Платформа | Механизм |
|---|---|
| iOS | `.accessibilityIdentifier("bdui.…")`. У `MainButton`, `CircularIconButton`, `SquareIconButton`, `BottomSheetPrompt`, `EmptyStatePlaceholder`, `GlassMenuRow`, `FilterChipRow` параметр `accessibilityIdentifier`/`identifierPrefix` уже есть — рендерер передаёт его внутрь, а не вешает снаружи. |
| Android | `Modifier.testTag("bdui.…")` в модифай-цепочке узла. Большинство компонентов `:core:ui` параметра `testTag` не имеют, но рендерер владеет цепочкой ⇒ правок компонентов не требуется. Частичные прецеденты параметра: `GlassSegmentedTabItem`, `SegmentedTabsContent`, `ProfileSwitcherSegment`, `AlertData`, `DailyRewardsProgressRow`. |
| omniapp | `.accessibilityIdentifier`, идентично iOS. |

🔴 **Ловушка контейнера (iOS).** Идентификатор на контейнерном узле (`column`, `row`, `box`, `card`, `list.section`, `tabs`) **обязан** ставиться вместе с `.accessibilityElement(children: .contain)`. Без этого поддерево схлопывается в один элемент и дети исчезают из дерева доступности — Maestro перестаёт их видеть. В этом репозитории на грабли уже наступали четырежды: `root.tabbar`, `GlassMenuCard`, `root.rating`, `settings.back`. Для BDUI цена ошибки выше обычного: контейнеры генерируются из пейлоада, значит баг воспроизводится на **каждом** документе разом. Правило проверяется одним Maestro-флоу на golden-фикстуре с вложенными контейнерами.

Прочее:
- `text` → `accessibilityLabel` = собственный текст; декоративные `image`/`icon` без `on` → `accessibilityHidden(true)`.
- Все интерактивные узлы уважают `LayoutMetrics.minimumTouchTarget = 44` (`Theme.swift:1873`); iOS-ловушка `Image` + `.frame` без `.contentShape` — зона тапа только по глифу.
- Хаптик обязателен на основные действия ([Code-Style](../../Stratoapp/docs/Architecture/Code-Style.md) §SwiftUI): `button.*` c `on.tap` → `.impact(.medium)`, `chip`/`tabs`/`toggle` → `.selection`, результат `call` → `.success`/`.error`. Триггерит рендерер (render-side), не reducer.
- Dynamic Type / Reduce Motion — обязанность нативных компонентов; `skeleton` на iOS уже Reduce-Motion-aware.

---

## 7. Пробелы — что надо написать

Оценка в человеко-днях одного разработчика платформы. Владелец задачи — соответствующий агент/лид платформы.

### Android (`:core:ui`) — 14 задач, ≈10 дн

| # | Что | Почему | Оценка | Блокирует |
|---|---|---|---|---|
| T-AND-1 | `object MostSpacing` (`$UI/tokens/Spacing.kt`, 6 значений) | семантической шкалы spacing на Android нет вообще | 0.25 | все layout-узлы |
| T-AND-2 | `MostText(text, role, color, align, maxLines)` | нет DS-обёртки текста; иначе рендерер сам зовёт M3 `Text` и ломает инвариант §3.0 | 0.5 | `text` |
| T-AND-3 | `MostAsyncImage(url, aspect, contentMode, radius, placeholder)` над Coil | generic remote-image компонента нет, Coil `AsyncImage` используется ad hoc | 0.5 | `image` |
| T-AND-4 | `AvatarStack` | нет | 0.5 | `avatarStack` (V2) |
| T-AND-5 | `MostBadge(title, style, size)` | нет | 0.5 | `badge` |
| T-AND-6 | `MostCard` (плоская карточка) | `SpotlightCard` несёт зашитый levels-артворк, glass-поверхности требуют `Backdrop` | 0.5 | `card`, `surface` |
| T-AND-7 | `MostBanner(message, style, icon, dismissible)` | `AlertCard` — транзиентный тост из `AlertManager`, не inline-вставка | 1 | `banner` |
| T-AND-8 | `LinkButton` | текст-кнопки нет | 0.25 | `button.link` |
| T-AND-9 | `Slider` без `Backdrop` | `LiquidSlider` требует host-supplied `Backdrop` | 1 | `slider` (V3) |
| T-AND-10 | `Stepper` | нет | 0.5 | `stepper` (V3) |
| T-AND-11 | `Accordion` | нет | 0.75 | `accordion` (V2) |
| T-AND-12 | `Countdown(until, format, label)` с шаренным тикером | нет | 1 | `countdown` |
| T-AND-13 | `EmptyState(icon, title, subtitle)` | нет generic-компонента | 0.5 | `emptyState` |
| T-AND-14 | `object BduiTypography` — 18 `TextStyle`, кегль/вес копируются с iOS (§1.3) | канон типографики — iOS; `MostTypography` остаётся для нативных экранов и не трогается | 1 | `text`, все текстовые props |
| — | `object BduiShapes` (2 значения, §1.5) | `MostShapes.lg/xl` расходятся с контрактом, трогать их нельзя | 0.25 | все узлы с `radius` |

⚠️ Все оценки Android **предварительные**: на диске sparse-клона только `core/ui`. Из исходников (выкачаны через GitLab API, отчёт разведки `recon/android-giveaway.md`) подтверждены `app/.../navigation/Screen.kt`, `feature/giveaway` и `graphql/`. **Не подтверждён только `:core:common`** — сигнатура `MviViewModel` и противоречие `<S,E,F>`+`onEvent` (корневой `CLAUDE.md`) против `<S,I,E>`+`onIntent` (`.claude/rules/mvi.md`); `:core:storage` под Room-кэш тоже не проверен.

### iOS (`Modules/DesignSystem`, `Modules/ThemeKit`) — 11 задач, ≈7,25 дн

| # | Что | Почему | Оценка | Блокирует |
|---|---|---|---|---|
| ~~T-iOS-1~~ **ЗАКРЫТ 2026-08-27** | `enum BDUITypeRole` + исчерпывающий `switch` на существующие поля `Typography` (18 ролей, §1.3) | контракту нужно закрытое подмножество, а `Typography` — открытое множество из ~50 полей | 0.25 | `text`, все текстовые props |
| ~~T-iOS-2~~ **ЗАКРЫТ 2026-08-27** | `AvatarStack(urls, size, overlap)` | нет | 0.5 | `avatarStack` (V2) |
| ~~T-iOS-3~~ **ЗАКРЫТ 2026-08-27** | `Badge(title, style, size)` | `ProfileTagNameBadge` импортирует `SharedModels` ⇒ не generic | 0.5 | `badge` |
| ~~T-iOS-4~~ **ЗАКРЫТ 2026-08-27** | `DSToggle(isOn, label)` | публичного `Toggle`-компонента в DS нет | 0.5 | `toggle` |
| ~~T-iOS-5~~ **ЗАКРЫТ 2026-08-27** | `DSSelect(options, selection, label)` | нет | 0.75 | `select` (V2) |
| ~~T-iOS-6~~ **ЗАКРЫТ 2026-08-27** | `DSSlider(value, min, max, step)` | нет | 0.5 | `slider` (V3) |
| ~~T-iOS-7~~ **ЗАКРЫТ 2026-08-27** | `DSStepper(value, min, max, step)` | нет | 0.5 | `stepper` (V3) |
| ~~T-iOS-8~~ **ЗАКРЫТ 2026-08-27** | `DSDatePicker(date, format)` | нет generic (`registrationWheel` — фиче-специфичен) | 0.75 | `datePicker` (V3) |
| ~~T-iOS-9~~ **ЗАКРЫТ 2026-08-27** | `Accordion(title, expanded, content)` | нет | 0.75 | `accordion` (V2) |
| ~~T-iOS-10~~ **ЗАКРЫТ 2026-08-27** | `ProgressBar(value, style)` — linear + ring | прогресс-бара в DS нет вовсе | 0.75 | `progress` |
| ~~T-iOS-11~~ **ЗАКРЫТ 2026-08-27** | `CountdownLabel(until, format)` с шаренным тикером | нет | 0.75 | `countdown` |
| ~~—~~ **ЗАКРЫТ 2026-08-27 как `DSDivider`** | `Divider` через `theme.colors.divider` + `LayoutMetrics.hairlineWidth` | в DS `Divider(` не встречается ни разу | 0.25 | `divider` |

Попутно (не блокирует BDUI, но всплыло при инвентаризации, `ios-uikit.md` §4): в [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) числится фантомный `BrandRefreshIndicator` (ни файла, ни символа в репозитории), 9 реальных публичных компонентов в реестре отсутствуют (`FrostIconButton`, `SelectionIndicator`, `NavGlassButton`, `NavGlassTitlePill`, `GiftArtworkView`, `GiftsPromoSheet`, `DatingBridgeMessageField`, `DatingBridgeActionButton`, `FlowLayout`), и `radii.sheet` задокументирован как 24 при фактических 32 (`Theme.swift:1978`). Владелец — `design-system`/`docs-updater`, отдельной задачей.

> **Блокирующее подмножество iOS закрыто 2026-08-27** (P-7): `BDUITypeRole`+`BDUIColorRole` (`ThemeKit/BDUITokenRoles.swift`, сгенерированы из `catalog/tokens.json`), `Badge`, `DSToggle`, `ProgressBar`, `CountdownLabel`, `DSDivider` — в реестре [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md). Сверх списка добавлен `BDUIColorRole`: у 40 цветовых токенов ровно та же проблема закрытого подмножества, что у типографики, и без него рендерер V1 резолвил бы prop `color` ad hoc. V2/V3-пробелы T-iOS-2/5/6/7/8/9 закрыты тем же днём вторым заходом (`AvatarStack`, `DSSelect`, `DSSlider`, `DSStepper`, `DSDatePicker`, `Accordion`) — **весь список iOS-пробелов пуст**. Внешность шести написана без макета, выведена из соседних компонентов и общих токенов; что именно решено по умолчанию и что нужно подтвердить дизайну — в `docs/DesignSystem/Design-Requests.md`. Снапшот-матрица 7 брендов × light/dark **не снята**: тест-таргет `KorpuTests` не собирается на `dev` (стухшие `Tests/Dating*`).

### omniapp — 4 задачи, ≈4 дн (только если продукт подключается)

| # | Что | Оценка |
|---|---|---|
| T-OMN-1 | Таблица алиасов токенов (§1.7) + `DSTypography`-маппинг 18 ролей | 1 |
| T-OMN-2 | `CardSurface`, `Banner`, `EmptyState`, `Badge`, `Toggle` (generic-обёртки; сейчас всё фиче-связанное) | 2 |
| T-OMN-3 | Deeplink-слой (его нет вообще: 0 `onOpenURL`, 0 `CFBundleURLSchemes`/`applinks`) — без него `open(url)` не имеет хоста | 1+ |
| T-OMN-4 | `BDUIAnalytics` с логирующей реализацией — аналитического SDK в проекте нет | 0.25 |

### Кросс-платформенные

| # | Что | Владелец |
|---|---|---|
| T-XPL-1 | Свести акценты KZ/UZ (DRIFT-1, DRIFT-2) — либо iOS `kopir`/`koprik` приводятся к Android-значениям, либо наоборот | владелец продукта |
| T-XPL-2 | Довезти недостающие начертания: Manrope Light (iOS), Manrope ExtraBold (Android), Montserrat ExtraBold/Black (Android) | обе платформы |
| T-XPL-3 | `catalog/icons.json` + codegen `BDUIIcon`/`BduiIcon` + CI-гейт диффа | contracts-репо, см. [[BDUI-Backend]] |

---

## 8. Открытые вопросы

1. ~~**DRIFT-1/DRIFT-2 (акценты KZ/UZ)** — какое значение канон?~~ **ЗАКРЫТО 2026-08-21: канон — Android.** iOS перекрашивается (`KopirTheme` заводится, `KoprikTheme` правится); запрет на `accent*` для `kopir`/`koprik` действует до момента перекраски, дальше снимается.
2. ~~**`JewishBrand`** на Android — плейсхолдер `by AzBrand`; iOS-пары у него нет вообще.~~ **ЗАКРЫТО 2026-08-21: бренд будет и на iOS.** `context.brand = "jewish"` легален на обеих платформах. Следствие вне BDUI: в таблицу брендов iOS добавляется восьмая строка (тема + нишевой модуль синагоги, см. [White-Label](../../Stratoapp/docs/Architecture/White-Label.md) про `nicheModules`) — это отдельный срез, BDUI его не ждёт, но и не должен закладываться на «семь брендов». До появления iOS-пары документы для `jewish` фактически отдаются только Android.
3. **`captionMedium`** — после порта (§1.3) на iOS будет два разных значения под похожими именами: `theme.typography.captionMedium` (Manrope 12, для нативных экранов) и BDUI-роль `captionMedium` (Manrope 10). Приемлемо ли, или BDUI-роль переименовать в реестре?
4. **`block.*` и данные** — правило §4 «блок ходит только через `data` документа» надо подтвердить на леадерборде раффла: там пагинация с догрузкой. Умеет ли `getBduiData` отдавать следующую страницу в тот же `dataId`, или блоку нужен собственный `action.call` с `invalidates`? Решение — за [[BDUI-Backend]].
5. **`grid` + `repeat`** — материализованные ячейки считаются в лимит узлов документа (500 по [[BDUI]] §10). Сетка призов раффла × N призов может упереться. Нужен ли отдельный лимит на `repeat.limit` в grid?
6. ~~**Полный клон most**~~ **ЗАКРЫТО 2026-08-21.** Клон получен целиком. В `settings.gradle.kts` 41 модуль плоской конвенции `:core:*` / `:feature:*` (`:app`, `:core:ui/common/storage/network/media/debug/steps/webrtc/firebase/testing`, `:graphql`, 20+ `:feature:*`) — `:core:bdui-contract`, `:core:bdui-render` и `:feature:bdui` ложатся в неё без исключений. Room под кэш есть (`MostDatabase`, `PostEntity`, `ProfilePostDao`, `MigrationTest`). Оценки 13 задач §7 по-прежнему сделаны по `core/ui` и требуют пересчёта теперь, когда виден весь граф.
7. ~~**omniapp как третий потребитель**~~ **ЗАКРЫТО 2026-08-21: входит в V1** (решение владельца, вопреки рекомендации [[BDUI-Plan]] §5). Профиль `catalog/profiles/vysota.json` и ролевой словарь токенов делаются в волне 1; покрытие каталога V1 у omniapp ≈60 %, инвариант «один токен = одна light/dark пара» там ложен (DRIFT-7).
8. **Иконки `null` на платформе** — деградация `skip` у `button.icon` означает исчезнувшую кнопку. Не нужен ли отдельный режим «рендерить без иконки»?

[[README|← к обзору]] · [[BDUI]] · [[BDUI-Backend]] · [[BDUI-Raffle-Pilot]] · [[BDUI-Plan]] · [DesignSystem-Catalog](../../Stratoapp/docs/Architecture/DesignSystem-Catalog.md) · [White-Label](../../Stratoapp/docs/Architecture/White-Label.md) #arch

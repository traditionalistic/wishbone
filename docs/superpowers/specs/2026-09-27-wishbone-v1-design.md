# Wishbone V1 — дизайн

> Статус: **утверждён 2026-09-27**. Пересматривает часть решений [[BDUI]], [[BDUI-Plan]], [[BDUI-Catalog]], [[BDUI-Backend]] — см. §0.
> Раскладка монорепо — §1. Проводной формат (контракт между бэком и клиентами) — §3–§5, он же нормативная часть `contract/`.

## 0. Что изменилось относительно доков BDUI

| Было (BDUI v2, bdui-contracts 0.2.0) | Стало (Wishbone V1) |
|---|---|
| «НЕ движок бизнес-логики», никаких expression-языков | собственный типизированный **DSL**, исполняется на клиенте; компилируется на сервере в AST |
| закрытый каталог DS-узлов (`button.primary`, `card`…) | **примитивы** (`stack`, `text`, `shape`…) + **шаблоны** DS, раскрываемые сервером |
| анимации — не-цель V1, lottie в V3 | **фул моушен** в V1: переходы по стейту, таймлайны, жесты/скролл, Lottie/Rive |
| веб-конструктор — не источник истины, не в V1 | **админка-редактор** + hot-reload превью на реальном устройстве |
| iOS-рендерер — Tuist-модуль внутри Stratoapp | самостоятельные **SDK** в `ios/` и `android/`; хосты подключают зависимостью |
| контракт — отдельный репо `bdui-contracts` | `wishbone/contract/`, монорепо; из 0.2.0 взяты токены, иконки, коды ошибок |
| omniapp — третий потребитель V1 | вне V1 (P-8 пересмотрен) |
| GraphQL/REST — не выбран (P-9) | **REST + JSON**; превью — WebSocket |

Сохраняются инварианты: валидация на записи (теперь с проверкой типов DSL), «старый клиент не получает незнакомое» (`runtimeMin` вычисляется компилятором), allowlist операций для записи, идемпотентность `call`, первый кадр = один запрос, значения форм не логируются.

**Критерий готовности V1** — сквозной срез: экран собирается в админке → бэк компилирует, хранит, публикует → playground на iOS и Android рисует одинаково; три шоукейс-экрана (§9) покрывают DSL и все четыре вида моушена. Интеграция в Stratoapp и most — следующая веха (§8.4).

## 1. Состав и раскладка

```
wishbone/
├─ contract/            источник истины
│  ├─ primitives.json   реестр примитивов: props, типы, события, sinceVersion
│  ├─ stdlib.json       сигнатуры встроенных функций DSL
│  ├─ tokens/           темы по брендам: цвета light/dark, типографика, spacing, радиусы, моушен
│  ├─ icons.json        реестр иконок: name → SF Symbol / Android drawable
│  ├─ templates/        DS-шаблоны (*.json в формате исходника экрана, §3.1)
│  ├─ screens/          исходники шоукейс-экранов (сидятся в БД при первом старте)
│  ├─ errors.json       коды ошибок
│  └─ conformance/      векторы паритета: *.json (исходник + ожидание) и compiled/*.json (AST)
├─ backend/             Go: компилятор DSL, хранение, выдача, call, admin API, preview-хаб
├─ admin/               React + TS (Vite), раздаётся бэкендом
├─ ios/                 SPM-пакет Wishbone + Tuist-проект Playground
├─ android/             Gradle: :wishbone-core, :wishbone-ui, :wishbone-lottie, :wishbone-rive, :playground
└─ docs/
```

Потоки:
- **Правка:** админка → `POST /admin/api/preview/{session}/document` (исходник) → компилятор → диагностика в админку + документ по WebSocket во все устройства сессии → перерисовка.
- **Публикация:** `POST /admin/api/screens/{id}/publish` → компиляция → новая ревизия (иммутабельна) с `etag` и `runtimeMin`.
- **Выдача:** `GET /v1/screens/{id}` → AST + инлайн-данные; `POST /v1/call` — операции; `GET /v1/theme` — тема.

Граница SDK/хоста (§6.3): хост отдаёт auth-заголовки, `openURL`, аналитику, бренд и схему, `nativeActions`, `nativeViews`; SDK отдаёт `WishboneScreen(id, params)`. Темы и шаблоны — данные с сервера: смена DS дизайнерами = правка `contract/tokens` и `contract/templates` + деплой бэка, **без релиза клиентов**.

## 2. Модель документа

### 2.1 Примитивы V1

| Примитив | Назначение | Специфичные props | События |
|---|---|---|---|
| `stack` | стек | `axis`: `"v"\|"h"\|"z"` (дефолт `"v"`), `spacing`: number, `align`: см. ниже | — |
| `scroll` | скролл | `axis`: `"v"\|"h"`, `spacing`, `align`, `paging`: bool, `indicators`: bool, `lazy`: bool | — |
| `grid` | сетка | `columns`: number (≥1), `spacing`, `rowSpacing` | — |
| `spacer` | распорка | `min`: number | — |
| `text` | текст | `text`: string, `style`: textStyle, `color`: color, `textAlign`: `"start"\|"center"\|"end"`, `maxLines`: number | — |
| `image` | растровая картинка | `url`: string, `fit`: `"fill"\|"fit"`, `placeholder`: color | — |
| `icon` | иконка реестра | `name`: icon, `size`: number (24), `color`: color | — |
| `shape` | фигура | `kind`: `"rect"\|"circle"\|"capsule"`, `fill`: paint, `stroke`: `{color, width}` | — |
| `textField` | ввод | `value`: string, `placeholder`: string, `keyboard`: `"text"\|"email"\|"number"\|"phone"`, `secure`: bool, `style`, `color` | `change`, `submit` (`event.value`) |
| `lottie` | Lottie по URL | `url`, `loop`: bool, `playing`: bool, `progress`: number? (0…1, если задан — управляет кадром), `speed`: number | `end` |
| `rive` | Rive по URL | `url`, `stateMachine`: string, `inputs`: record | `event` (`event.name`) |
| `native` | вьюха хоста | `name`: string, `props`: record | — |

`align`: для `axis:"v"` — `"start"|"center"|"end"` по горизонтали; для `"h"` — по вертикали; для `"z"` — `"topStart"|"top"|"topEnd"|"start"|"center"|"end"|"bottomStart"|"bottom"|"bottomEnd"`. Дефолт — `"center"` у `z`, `"start"` у остальных.

**Общие props (модификаторы)** — у любого узла, применяются в каноническом порядке, автор порядок не задаёт:

1. `width`/`height`: number | `"fill"` | `"wrap"` (дефолт `"wrap"`); `minWidth`, `maxWidth`, `minHeight`, `maxHeight`; `aspect` (w/h)
2. `padding`: number | `{top, bottom, start, end, h, v}` (все необязательны)
3. `background`: paint; `material`: `"thin"|"regular"|"thick"` (размытие фона)
4. `corner`: number (капсула — `@radius.pill` = 9999, рендер клампит радиус к половине стороны); `border`: `{color, width}`; `clip`: bool
5. `shadow`: `{color, radius, x, y}`
6. `opacity`, `offsetX`, `offsetY`, `scale`, `rotation` (градусы) — визуальные, на лэйаут не влияют; `zIndex`

**Общие поля узла:** `id` (уникален в документе, обязателен), `when` (bool; ложь ⇒ узла нет в дереве, с transition), `repeat` (`{over, as, index, key}`), `on` (события → операторы), `animate`, `transition`, `matchId`, `drag` (§4), `a11y` (`{label, role: "button"|"header"|"image"|"none", hidden}`).

События любого узла: `tap`, `longPress`, `appear`, `disappear`; у `drag`-узла — `dragEnd`. `self.pressed` доступен узлу с `on.tap`.

### 2.2 Значения и токены

Значение prop — литерал, токен или выражение DSL. Токены: `@color.<name>`, `@type.<name>` (textStyle), `@space.<name>` (number), `@radius.<name>` (number; `pill` = 9999), `@motion.<name>` (motion), `@icon.<name>` (icon).

- **Цвета и типографика — только токенами.** У типов `color`/`textStyle` нет литералов; `paint` = `color` | `gradient([colors], angleDeg)`; `alpha(color, a)` допустим.
- Геометрия — числа или токены spacing/radius.
- Токены резолвит **клиент** по текущей теме (бренд × light/dark) — переключение схемы без перезапроса.

### 2.3 Шаблоны

Шаблон — документ того же формата, что экран (§3.1), с объявлениями `param` и `event`. Инстанс шаблона — узел с `type`, начинающимся с заглавной буквы (`"type": "Button"`). Сервер **раскрывает** шаблоны при компиляции, клиент видит только примитивы.

- `param name: Type = default` → подставляется в выражения инстанса (внутри шаблона — обычное имя).
- `event tap` + `emit tap` в операторах шаблона → на месте `emit` инлайнятся операторы из `on.tap` инстанса. Событие может нести payload: `event change(value: string)` + `emit change(expr)` → в обработчике инстанса `event.value` (payload — запись из объявленных полей; при инлайне выражение payload привязывается к гигиенично переименованной локальной).
- Узлы с литеральным `when: false` (после подстановки параметров) вырезаются компилятором вместе с поддеревом и таймлайн-треками на них.
- `state` внутри шаблона — **локальный стейт экземпляра**: имена на проводе квалифицированы шаблоном (`state on` в `Toggle` → `["i", "Toggle.on"]`), корень инстанса несёт `inst.state`; клиент хранит его на материализованном экземпляре (включая экземпляры внутри `repeat`) и резолвит имя, поднимаясь по цепочке вложенных экземпляров до объявившего его.
- Локальные имена шаблона (repeat-переменные, параметры лямбд и обработчиков, `let`) при раскрытии переименовываются (гигиена); `let` шаблона подставляется в места использования, `fn` поднимаются в документ как `<Шаблон>.<fn>`, обработчики шаблона инлайнятся.
- `slot` — узел-заглушка `{"type": "slot"}` в дереве шаблона, заменяется `children` инстанса.
- Общие props (модификаторы §2.1) и общие поля узла (`when`, `repeat`, `animate`, `transition`, `matchId`, `drag`, `a11y`) инстанса переносятся на корень раскрытого шаблона и перекрывают его значения; события инстанса, объявленные шаблоном через `event`, инлайнятся в `emit`, остальные общие события (`appear`, `disappear`, …) вешаются на корень.
- Идентификаторы внутренних узлов: `<instanceId>/<innerId>`; таймлайны шаблона — `<instanceId>/<name>`.
- Библиотека шаблонов — `contract/templates/*.json`, грузится бэком на старте; перед сменой библиотеки бэк перекомпилирует все опубликованные экраны и отказывается стартовать, если хоть один не собирается.

## 3. DSL

### 3.1 Исходник экрана (формат хранения)

```json
{
  "id": "showcase.onboarding",
  "title": "Онбординг",
  "script": "state page = 0\nlet last = page == 2\nhandler next() {\n  animate(@motion.smooth) { page = min(page + 1, 2) }\n}",
  "timelines": [ { "name": "intro", "tracks": [ { "target": "hero", "prop": "opacity", "keys": [ {"t": 0, "v": 0}, {"t": 400, "v": 1, "ease": "easeOut"} ] } ] } ],
  "root": { "type": "stack", "id": "root", "props": { "spacing": "@space.lg", "padding": 16 }, "on": { "appear": "play(intro)" }, "children": [] }
}
```

Значения `props` в исходнике — JSON. Строка, начинающаяся с `=`, — выражение (`"=\"Привет, ${me.name}\""`); строка с `@` — токен; остальное — литерал (строка `"\\="`/`"\\@"` экранирует первый символ). Значения `on.*`, `when`, `repeat.over/key`, `matchId`, `a11y.label`, значения `animate` — всегда DSL (без `=`). `repeat` — `{"over": "offers", "as": "item", "index": "i", "key": "item.id"}` (`as`/`index` по умолчанию `item`/`index`). `transition` — строки `{"in": "fade + slide(bottom) + scale(0.9)", "out": "fade"}`. `drag` — литеральный JSON как на проводе. Таймлайн: `{name, repeat?: 1, autoreverse?: false, autoplay?: false, tracks: [{target, prop, stagger?, keys}]}`; длительность = максимальное `t` ключей (stagger удлиняет её на рантайме). Сахар `textField.bind: "email"` ≡ `value: =email` + `on.change: email = event.value`.

### 3.2 Лексика

Идентификаторы `[A-Za-z_][A-Za-z0-9_]*`; числа `12`, `1.5`, с единицами `300ms` (=300), `1.2s` (=1200) — длительности всегда в мс; строки `"…"` с `\" \\ \n \t \$ \u{1F600}` (прочие escape — ошибка компиляции) и интерполяцией `${expr}`; токены `@group.name`; комментарии `// …`. Ключевые слова: `state let fn handler data param if else true false null await animate emit`; `event` — контекстное (объявление в шаблоне / payload внутри `on.*`).

### 3.3 Объявления (верхний уровень `script`)

```
state count: number = 0                  // стейт экрана (в шаблоне — экземпляра)
let total = items.map(i => i.price).sum() // computed, реактивно
fn tax(x: number): number = x * 0.2      // чистая функция; рекурсия (прямая/косвенная) запрещена
handler add(i: number) { … }             // операторы
data me = demo.profile()                 // источник данных; `required` в конце — экран не отдаётся без него
data offer = demo.offer({id: id}) required
param id: string = ""                    // параметр экрана/шаблона
event tap                                // событие шаблона
```

Аргументы `data` могут ссылаться только на литералы и `param` (сервер подставляет их сам). Все имена верхнего уровня (`state`, `let`, `fn`, `handler`, `data`, `param`) — в одном пространстве имён и читаются без префикса: `me.name`, `offers.count`.

### 3.4 Типы

`bool`, `number` (float64), `string`, `color`, `paint` (⊃ color), `textStyle`, `motion`, `icon`, `list<T>`, записи `{a: T, b?: U}` (необязательное поле читается как `U?`), `map<T>` (строковые ключи; литерал записи присваиваем `map<T>`, если все поля присваиваемы `T`), опционал `T?`, объединения (`"a" | "b"`, `number | "fill"`), `null`; `any` — только в сигнатурах контракта. Уточнения семантики (null-propagation методов, перегрузки, индексы, иммутабельность значений, видимость объявлений) — нормативно в `contract/stdlib.json` (`notes`, `language`). Вывод типов локальный; аннотации обязательны у `param`, `fn`-параметров, пустых литералов (`state xs: list<number> = []`).

Правила: арифметика — только number; `+` — number+number или string+string; сравнения `< <= > >=` — number/string; `== !=` — одинаковые типы или с `null`; `&& || !` — только bool (истинности нет); `c ? a : b` — c bool; `a ?? b` — a: `T?`, b: `T` → `T`. Доступ `.field` к опционалу даёт опционал (null-safe без `?.`); `list[i]` → `T?`. Присваивание — только в стейт (экрана или экземпляра), по пути `x.a[i].b`.

### 3.5 Выражения

Приоритет (снизу вверх): `?:` → `??` → `||` → `&&` → `== !=` → `< <= > >=` → `+ -` → `* / %` → унарные `! -` → постфикс (вызов, `.`, `[]`). Литералы: числа, строки, `true/false/null`, списки `[…]`, записи `{a: 1}`. Лямбды `x => e`, `(a, b) => e` — только аргументами stdlib. Метод-синтаксис — сахар: `xs.map(f)` ≡ `map(xs, f)`.

Специальные имена: `env.now` (мс UTC, тикает раз в секунду, когда читается), `env.platform` (`"ios"|"android"`), `env.locale`, `env.brand`, `env.colorScheme`, `env.reduceMotion`, `env.screenWidth`, `env.screenHeight`, `env.safeTop`, `env.safeBottom`; драйверы `scroll.<id>.x|y`, `drag.<id>.x|y`; `self.pressed`; `event.*` внутри обработчика события (на проводе — локальная `["l", "event"]`, рантайм кладёт payload в локальные под именем `event`); `item`/`index` (или имена из `repeat.as/index`) внутри `repeat`.

### 3.6 Stdlib (нормативно — `contract/stdlib.json`)

- **Числа:** `min max abs round floor ceil clamp(x,lo,hi) pow sqrt sin cos interpolate(x, [in…], [out…])` (кусочно-линейно, с клампом).
- **Строки:** `length upper lower trim contains startsWith endsWith split replace matches(re) substring(a, b?) isEmpty`, `num(s) → number?`, `str(x) → string`.
- **Списки:** `count isEmpty first last map filter find some every reduce(init, f) sum join append removeAt set(i, v) slice indexOf contains sortBy reverse`, `range(n)`.
- **Записи:** `r.with({k: v})` — копия с заменой полей.
- **Формат:** `formatNumber(x, decimals)`, `formatDuration(ms, "hms"|"ms"|"dhms")` → `"01:02:03"`, `formatDate(ms, "date"|"time"|"dateTime")` → `dd.MM.yyyy`, `HH:mm` в локальной зоне, `plural(n, {one, few, many, other})` по CLDR для `env.locale` (ru — one/few/many/other; прочие V1 — one/other).
- **Цвет/моушен:** `gradient([c…], angle)`, `alpha(c, a)`, `spring(response, damping)`, `ease(duration)`, `linear(duration)`, `curve(duration, x1, y1, x2, y2)`.

Приведение числа к строке (интерполяция, `str`): целое по модулю < 1e15 — без дробной части; иначе до 6 знаков после точки без хвостовых нулей. Точка — всегда `.`.

### 3.7 Операторы (тело `handler`, значения `on.*`)

```
x = expr                  // x — стейт; путь x.a[i]
let y = expr
if c { … } else if d { … } else { … }
animate(@motion.snappy) { … }     // транзакция: изменения внутри анимируются этим моушеном
play(intro)  stop(intro)  await play(intro)
let r = await call demo.subscribe({email: email})   // r: {ok, value?, error?: {code, message}, fieldErrors?: {k: msg}}
call demo.track({…})                                 // без ожидания
navigate("screen.id", {id: "1"}, "push"|"fade")  back()  present("screen.id", {…})  dismiss()
open("https://…")  haptic("light"|"medium"|"heavy"|"selection"|"success"|"warning"|"error")
track("event", {…})  host("action", {…})  wait(300ms)  refresh(me)
emit tap                  // только в шаблоне
next()                    // вызов другого handler (дожидается)
```

Исполнение асинхронное, на главном потоке; каждое событие — отдельный прогон; запись в стейт видна сразу. Циклов нет ⇒ исполнение конечно; бюджет — 10 000 шагов вычисления на прогон (превышение = ошибка прогона, телеметрия, экран живёт дальше).

### 3.8 Проводной AST (нормативно — этот раздел + `contract/README.md`)

**Выражения** — JSON-массивы:

| Форма | Смысл |
|---|---|
| `["v", json]` | литерал (любой JSON) |
| `["s", name]` / `["i", name]` / `["c", name]` / `["d", name]` / `["l", name]` / `["p", name]` | стейт экрана / экземпляра / computed / data / локальная (let, параметр, лямбда, repeat) / параметр экрана |
| `["e", name]` | env |
| `["t", group, name]` | токен |
| `["dr", "scroll"\|"drag", id, "x"\|"y"]` | драйвер |
| `["self", "pressed"]` | состояние узла |
| `["get", obj, field]` / `["at", obj, key]` | доступ (null-safe); `at` — индекс списка (целое в диапазоне) или ключ `map` |
| `["op", op, a, b]` | `+ - * / % == != < <= > >= && \|\| ??` (`&& \|\| ??` — ленивые) |
| `["un", "!"\|"-", a]` | унарные |
| `["if", c, a, b]` | тернарный |
| `["f", name, [args]]` | stdlib |
| `["u", name, [args]]` | пользовательская `fn` |
| `["lam", [params], body]` | лямбда |
| `["list", [e…]]` / `["rec", {k: e}]` | конструкторы |
| `["str", [e…]]` | интерполяция: конкатенация `str(e)` |

**Операторы:**

| Форма | Смысл |
|---|---|
| `["set", "s"\|"i", name, [pathExpr…], expr]` | присваивание (элемент пути: string → поле, number → индекс) |
| `["let", name, expr]` | локальная |
| `["if", cond, [then…], [else…]]` | ветвление |
| `["anim", motionExpr, [body…]]` | транзакция анимации |
| `["call", opId, argsExpr, resultName\|null, await: bool]` | операция бэка |
| `["do", effect, [args…], await: bool]` | эффект: `navigate back present dismiss open haptic track host wait refresh play stop` |
| `["run", handler, [args…]]` | вызов handler |

**Узел:**

```json
{ "t": "stack", "id": "root",
  "p": { "spacing": ["t", "space", "lg"], "opacity": ["v", 1] },
  "c": [ … ],
  "when": ["s", "visible"],
  "rep": { "over": ["d", "offers"], "as": "item", "index": "index", "key": ["get", ["l", "item"], "id"] },
  "on": { "tap": [ ["run", "next", []] ] },
  "anim": { "scale": ["t", "motion", "snappy"] },
  "tr": { "in": [ {"k": "fade"}, {"k": "slide", "edge": "bottom"} ], "out": [ {"k": "fade"} ] },
  "match": ["v", "hero"],
  "drag": { "axis": "x", "snap": [0, 300], "bounds": [-400, 400] },
  "inst": { "state": { "on": ["v", false] } },
  "a11y": { "label": ["v", "Далее"], "role": "button" } }
```

**Документ:**

```json
{ "format": 1, "runtimeMin": "1.0.0", "screen": "showcase.promo", "revision": 4,
  "params": { "id": ["v", ""] },
  "state": [ ["page", ["v", 0]] ],
  "computed": { "last": ["op", "==", ["s", "page"], ["v", 2]] },
  "fns": { "tax": { "params": ["x"], "body": ["op", "*", ["l", "x"], ["v", 0.2]] } },
  "handlers": { "next": { "params": [], "body": [ … ] } },
  "data": [ { "name": "me", "op": "demo.profile", "args": {"id": {"$param": "id"}}, "required": false } ],
  "timelines": { "intro": { "duration": 400, "repeat": 1, "autoreverse": false, "autoplay": false,
      "tracks": [ { "target": "hero", "prop": "opacity", "stagger": 0,
                    "keys": [ {"t": 0, "v": 0}, {"t": 400, "v": 1, "ease": [0, 0, 0.58, 1]} ] } ] } },
  "root": { … } }
```

`state` — упорядоченный список (инициализаторы видят предыдущие). Неизвестная форма выражения/оператора/примитива на клиенте: узел не рисуется (skip), прогон обработчика прерывается; в debug — assert, в release — телеметрия. Документ, не прошедший декод, = сетевая ошибка (показывается кэш, иначе фоллбек с retry).

## 4. Моушен

### 4.1 Переходы по стейту
- **Неявные:** `animate: {"<prop>"|"*": motion}` на узле — изменение значения prop анимируется этим моушеном. iOS — scoped `.animation(_:body:)`; Android — `animate*AsState(spec)`.
- **Транзакция:** `animate(m) { … }` в обработчике — все визуальные изменения, вызванные присваиваниями внутри, анимируются `m`. iOS — `withAnimation`; Android — стор выставляет «спек текущей транзакции», props берут `anim[prop] ?: txSpec ?: snap`.
- **Появление/исчезновение:** `transition: {in, out}` для узлов с `when`; виды: `fade`, `slide(edge)`, `scale(from)`, `offset(x, y)`, комбинируются списком. iOS — `.transition(.asymmetric)`; Android — `AnimatedVisibility(enter, exit)`.
- **Лэйаут и общий элемент:** изменение размеров внутри анимированной транзакции анимируется (iOS — нативно; Android — `animateContentSize` на узлах с `animate` по `width/height/"*"`). `matchId` — общий элемент между ветками `when` одного экрана: iOS `matchedGeometryEffect`, Android `SharedTransitionLayout` + `sharedBounds`.
- **Экраны:** `navigate(…, "push"|"fade")`, `present` — модальный шит. SDK владеет своим стеком навигации.

### 4.2 Таймлайны
Трек анимирует **трансформ-слой** узла: `opacity` и `scale` умножаются на значение prop, `offsetX/offsetY/rotation` — прибавляются. Таймлайн не спорит с биндингами стейта и по окончании держит последнее значение (для входных анимаций оно — нейтральное: 1 / 0).
- Ключи: `{t: мс, v: число, ease}`; `ease` — сегмента, ведущего **в** этот ключ: `linear` (дефолт), `easeIn`, `easeOut`, `easeInOut` (cubic-bezier CSS-стандартов), `spring` (кривая `Motion.spring` Stratoapp: bezier 0.38, 0.70, 0.125, 1.0), `[x1, y1, x2, y2]`.
- `stagger` — задержка на индекс экземпляра `repeat`; `repeat` — число повторов (`-1` — бесконечно), `autoreverse`; `autoplay` — запущен с первого рендера (без мигания входной анимации). На проводе `ease` — всегда массив `[x1, y1, x2, y2]` (имена раскрывает компилятор по `stdlib.json` `easings`) или отсутствует (линейно). Нормативный алгоритм сэмплинга — `contract/README.md`.
- Цель — `id` узла; все материализованные экземпляры узла анимируются вместе (со stagger по индексу).
- Сэмплинг — чистая функция `(timeline, t) → значения`, покрыта conformance-векторами; кадры — `TimelineView(.animation)` на iOS, `withFrameNanos` на Android.
- `env.reduceMotion` ⇒ таймлайны сразу в конечное состояние, `slide/scale/offset`-переходы заменяются на `fade`.

### 4.3 Жесты и скролл
- `scroll.<id>.y` — смещение контента скролла с этим `id`; `drag.<id>.x|y` — текущее смещение перетаскиваемого узла. Любое выражение может читать драйверы; пересчитываются только узлы, которые их читают (наблюдение платформы, §6.2).
- `drag: {axis: "x"|"y"|"both", snap?: [числа по оси], bounds?: [min, max]}` — узел следует за пальцем (смещение прибавляется к `offset*`); на отпускании — пружиной к ближайшей точке `snap` (или к 0), затем событие `dragEnd` с `event.x, event.y, event.vx, event.vy`. Обработчик может сменить стейт (например, убрать карточку).
- `self.pressed` — для press-эффектов.

### 4.4 Ассетная анимация
`lottie` (iOS — lottie-ios, Android — lottie-compose) и `rive` (RiveRuntime / rive-android); `playing`/`progress`/`inputs` — выражения, т.е. управляются стейтом и драйверами. Адаптеры — отдельные продукты SDK, хост подключает по желанию; без адаптера узел деградирует в skip.

## 5. Бэкенд

Go-модуль `backend/`, бинарь `cmd/server`, net/http (маршруты Go 1.22+), Postgres (pgx), WebSocket (`github.com/coder/websocket`). Контракт и сборка админки читаются с диска (`-contract ../contract`, `-admin ../admin/dist`).

### 5.1 Пакеты
- `internal/dsl` — лексер, парсер (Pratt), типы, проверка, понижение в AST §3.8.
- `internal/compile` — исходник экрана → документ: раскрытие шаблонов, проверка props по `primitives.json`, токенов по теме, иконок, лимитов (размер ≤ 512 КБ, глубина дерева ≤ 32, узлов ≤ 2000, вложенность выражения/блока в одном DSL-фрагменте ≤ 500), уникальности `id`, `runtimeMin`. Ошибка = диагностика `{path, line, col, message}`.
- `internal/ops` — реестр операций: `{id, kind: read|write, args: "<тип DSL>", result: "<тип DSL>", handler}`; демо-операции шоукейса.
- `internal/store` — Postgres: `screens(id, title, draft jsonb, updated_at)`, `revisions(screen_id, rev, source, compiled, etag, runtime_min, published_at)`, `idempotency(key, op, response, created_at)`, `audit`. Живая ревизия = максимальная; откат = публикация старого исходника новой ревизией.
- `internal/preview` — хаб сессий превью.
- `internal/api` — HTTP.

### 5.2 Публичный API (клиенты)
Заголовки запроса: `X-Wishbone-Runtime` (semver), `X-Wishbone-Platform`, `X-Wishbone-Brand`, `Accept-Language`, `Authorization` (прокидывается в операции).

- `GET /v1/screens/{id}?params=<json>` (+`If-None-Match`) → `{ "etag", "document" | null (не изменился), "data": { name: {ok, value?, error?} } }`; ошибки — `{error: {code, message, retryable}}` с HTTP-статусом по `errors.json` (`SCREEN_NOT_FOUND` 404, `UNSUPPORTED_RUNTIME` 409, `DATA_REQUIRED_FAILED` 503).
- `POST /v1/data` `{screen, revision, sources: [{name, args}]}` → `{name: {ok, value?, error?}}` (для `refresh`).
- `POST /v1/call` `{screen, revision, op, args, idempotencyKey}` → `{ok, value?, error?, fieldErrors?}`; операция должна быть использована в ревизии (`OPERATION_NOT_BOUND`), write без ключа — `IDEMPOTENCY_REQUIRED`, повтор с тем же ключом отдаёт сохранённый ответ.
- `GET /v1/theme?brand=` (+`If-None-Match`) → тема (§6.4).
- `GET /v1/preview/sessions` → активные сессии; `GET /v1/preview/ws?session=` — WebSocket устройства.
- `GET /assets/*` — статика шоукейса (lottie/rive/картинки) из `contract/assets/`. URL, начинающиеся с `/`, клиенты резолвят относительно базового адреса сервера.

### 5.3 Admin API
`/admin/api/*`, авторизация — пароль из `WISHBONE_ADMIN_PASSWORD` (Bearer). Ресурсы: `GET/POST /screens`, `GET/PUT /screens/{id}` (черновик), `POST /screens/{id}/publish`, `GET /screens/{id}/revisions`, `POST /screens/{id}/rollback/{rev}`, `POST /compile` (диагностика без сохранения), `GET /catalog` (примитивы, stdlib, шаблоны с сигнатурами, токены, иконки, операции), `POST /preview` (новая сессия → код), `POST /preview/{code}/document` (компиляция + рассылка), `POST /preview/{code}/play` (запуск таймлайна на устройствах), `GET /preview/{code}/ws` (устройства и их логи).

### 5.4 Протокол превью
Устройство → сервер: `{"type":"hello","platform","device","runtime"}`, `{"type":"log","level","message"}`. Сервер → устройство: `{"type":"document","document",data}`, `{"type":"play","timeline"}`. Сервер → админка: `{"type":"devices","list"}`, `{"type":"log",…}`. Playground показывает список сессий (`GET /v1/preview/sessions`) — подходит и для симулятора/эмулятора, QR не нужен.

## 6. SDK

### 6.1 Состав
**iOS** (`ios/Package.swift`, iOS 18, Swift 6): `WishboneCore` (AST, интерпретатор, стор, сэмплер таймлайнов, плюрали/форматы, сетевой клиент, кэш) · `WishboneUI` (SwiftUI-рендерер, тема, навигация, превью) · `WishboneLottie` · `WishboneRive`. Playground — Tuist-проект `ios/Playground`.
**Android** (`android/`, minSdk 33, Kotlin 2.3, Compose BOM как в most): `:wishbone-core` (чистый Kotlin/JVM: AST, интерпретатор, сэмплер, плюрали, форматы) · `:wishbone-ui` (стор на snapshot-state, клиент на OkHttp, рендерер, тема, навигация, превью) · `:wishbone-lottie` · `:wishbone-rive` · `:playground`.

### 6.2 Реактивность
Стейт хранится **ячейками** (одна наблюдаемая ячейка на ключ стейта/драйвера): iOS — `@Observable` ячейки, Android — `mutableStateOf`. Выражения вычисляются в `body`/composable — платформа сама отслеживает прочитанные ячейки и перерисовывает только зависимые узлы. Computed вычисляются при чтении. Отдельного графа зависимостей нет.

### 6.3 API для хоста
```swift
let wishbone = Wishbone(baseURL: url, brand: "korpu", host: MyHost())
WishboneScreen(wishbone, id: "showcase.promo", params: [:])

protocol WishboneHost {
  func authHeaders() async -> [String: String]
  func open(url: URL)                       // вне allowlist — игнор
  func track(event: String, params: [String: Any])
  func action(name: String, args: [String: Any]) async
  func nativeView(name: String, props: [String: Any]) -> AnyView?
}
```
Android — зеркально (`WishboneHost` interface, `@Composable fun WishboneScreen(...)`). Кэш — файловый (последний удачный ответ экрана и темы), политика stale-while-revalidate.

### 6.4 Тема
`GET /v1/theme` отдаёт тему бренда: `colors {name: {light, dark}}`, `type {name: {family, size, weight, lineHeight?, tracking?}}`, `space`, `radius` (`pill` = 9999), `motion {name: {kind: "spring", response, damping} | {kind: "curve", duration, bezier}}`. SDK содержит снапшот темы `korpu` как фоллбек и шрифт Manrope. Иконки — статическая таблица в SDK, сверяется с `contract/icons.json` тестом.

## 7. Админка

Vite + React + TypeScript, без UI-фреймворков; редактор кода — CodeMirror 6; drag-and-drop — нативный HTML5.
- **Слева:** список экранов/шаблонов; дерево узлов текущего экрана (вставка примитива/шаблона из палитры, перетаскивание, удаление, дублирование).
- **Справа:** инспектор выбранного узла — контролы по `primitives.json` / сигнатуре шаблона: пикеры токенов, числа, строки, переключатель «литерал / выражение»; события, `when`, `repeat`, моушен-поля.
- **Центр:** вкладки `Script` (DSL с подсветкой диагностики) и `Timeline` (треки × ключи на шкале, редактирование ключей, «проиграть на устройствах»).
- **Верх:** подключённые устройства, статус компиляции, «Сохранить», «Опубликовать», ревизии/откат. **Низ:** консоль — диагностика компилятора и логи устройств.
Каждая правка (debounce 300 мс) уходит в сессию превью. Шаблоны редактируются в git (`contract/templates`), в админке — только просмотр (V1).

## 8. Качество и порядок работ

### 8.1 Паритет
- `contract/conformance/*.json` — векторы: выражения (`source`, `state`, `expect`), обработчики (`script`, `handler`, `before`, `after`, `effects`), сэмплинг таймлайнов, плюрали, форматы. `go run ./cmd/wbc conformance` компилирует их в `compiled/`; Swift (`swift test`) и Kotlin (`:wishbone-core:test`) прогоняют все.
- Go: тесты парсера, типизации, раскрытия шаблонов, golden-компиляция шоукейсов, API (httptest).
- Рендер: запуск шоукейсов в playground на симуляторе/эмуляторе, скриншоты обеих платформ.

### 8.2 Порядок
1. Контракт: примитивы, stdlib, темы, иконки, шаблоны, векторы.
2. Go: компилятор + conformance-компиляция.
3. Go: сервер (store, API, ops, preview).
4. Ядра iOS/Android: интерпретатор, стор, сэмплер — зелёные на векторах. *(параллельно)*
5. Рендереры + моушен + playground. *(параллельно)*
6. Админка. *(параллельно с 4–5)*
7. Шоукейсы + сквозная проверка + доки.

### 8.3 Не делаем в V1
Таргетинг/эксперименты/сегменты, четыре глаза при публикации, SSO, редактирование шаблонов в админке, веб-рендер, видео, placement-слоты, CDN, Redis, omniapp.

### 8.4 Следующая веха — хосты
Stratoapp: SPM-зависимость в Tuist-графе, адаптер `WishboneHost` на `AuthTransport`/`RootExternalRouteParser`/`AnalyticsTracking`, кэш через Postbox, диплинк `bdui/<id>` → `WishboneScreen`. most: composite build / Maven, адаптер на `Navigator`/Apollo-токен, `Screen.Wishbone(id)`.

## 9. Шоукейсы

1. **`showcase.onboarding`** — 3 страницы: входной таймлайн со stagger, смена страниц транзакцией + slide-переходы, индикатор-точки с неявной анимацией ширины, Lottie на каждой странице, кнопка-шаблон с press-эффектом; финал → `navigate("showcase.promo")`.
2. **`showcase.promo`** — скролл-драйвер: параллакс и схлопывание героя, заголовок проявляется по `interpolate(scroll.page.y…)`; сетка офферов из `data` с таймлайном stagger; свайп-карточка (`drag` + `dragEnd`) убирается из списка; countdown от `env.now`; Rive-кнопка со state machine, вход которой — стейт.
3. **`showcase.form`** — форма: поля с валидацией через computed, чипы тарифа, расчёт цены через `reduce`, проверка промокода `await call demo.checkPromo`, отправка `demo.subscribe` с `fieldErrors`, успех — раскрытие карточки через `matchId` + `haptic("success")`.

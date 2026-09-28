# Task 2 — controller decisions (binding, supplement the brief)

Module: `backend/go.mod` → `module wishbone/backend`, Go 1.26, stdlib only for the dsl package.

## Type system (spec §3.4, made precise)
- Types: `bool number string color paint textStyle motion icon`, `list<T>`, records `{a: T, b?: U}` (optional fields), `map<T>` (string keys), `T?`, string-literal types `"a"`, unions `A | B`, `null`, `any` (sink only: accepted as a parameter type, a value of type any cannot be used in operators), internal function types for lambdas.
- `color` is assignable to `paint`. `T` to `T?`; `null` to `T?`; `(T?)?` = `T?`.
- String literal expression has type `string` but is assignable to a literal union containing its value (contextual). Same for literal record/list elements.
- Record literal `{a: 1}` → `{a: number}`. Record assignability to an expected record type: every required field present and assignable, optional fields assignable if present, **extra fields are an error** (catches typos in op args). Record → `map<T>` if every field assignable to T.
- Empty list literal needs an expected type (annotation / parameter / assignment target), else error containing "annot".
- List literal element types must join (identical, or T with null → T?); otherwise type error.
- Ternary branches join the same way; condition must be bool.
- Member access on `T?` record → field type made optional. Index: `list<T>[number]` → `T?`, `map<T>[string]` → `T?`, through optional receivers → `T?`.
- `a ?? b`: a may be `T?` or `T`; result = join(T, type(b)) (non-optional if b non-optional).
- Arithmetic `- * / %` and unary `-`: number only (optional number is an error). `+`: number+number or string+string. `< <= > >=`: both number or both string. `== !=`: joinable types (incl. with null). `&& || !`: bool only.
- Interpolation `${e}`: any non-function type.
- Method sugar: `x.f(args)` → stdlib `f(x, args)` when f is a stdlib method (stdlib.json `method: true`). Properties (`length count isEmpty first last`) without parentheses on string/list receivers; on records, a real field wins.
- Stdlib generics: implement either with a small unifier over the stdlib.json signatures or with per-function Go rules — your choice; both must yield the types in stdlib.json. Lambdas are only legal as stdlib arguments (else error containing "lambda"); map/filter/find/some/every lambdas take 1 or 2 params (item[, index]).
- Tokens `@group.name`: group ∈ color→color, type→textStyle, space→number, radius→number, motion→motion, icon→icon; the checker receives the set of valid names per group from the caller (unknown name → error containing "token", unknown icon → "icon").
- `env.<name>` typed by stdlib.json `env`. `event` exists only when the caller declares an event payload type (else unknown name). Drivers `scroll.<id>.x|y`, `drag.<id>.x|y` → number, `self.pressed` → bool; the caller supplies which ids/self are legal (dsl only resolves through the Env callback).

## Declarations (script)
- One namespace for `state let fn handler data param event` + timeline names are separate (timelines are not values). Duplicate → "duplicate".
- `state` initializers may reference: params, data, env, earlier state, fns, tokens. Not computed, not later state (→ "unknown").
- `let` (computed) may reference state, computed, data, params, env, fns, tokens; cycles → "cycle".
- `fn` bodies may reference only their parameters, other fns, env, tokens (pure); recursion direct or indirect → error containing "recurs". Parameter and return annotations required (return type may be inferred if omitted).
- `data name = op(args) [required]`: op must exist ("operation") and be kind read ("read"); args type-checked against the op's args type; args may reference only literals and params → lowered to plain JSON where params become `{"$param": "<name>"}`. Type of `name`: op result type, optional unless `required`.
- `param name: Type [= default]`; `event name` (template events).
- `handler name(p: T, …) { … }`.

## Statements (spec §3.7)
- Assignment target: screen state (`"s"`) or instance state (`"i"`, caller declares instance-state names), with path of `.field` / `[index]` segments; RHS assignable to the target's (path) type; assigning to computed/param/data/fn/let-local → error containing "assign".
- `let x = e` handler-local; `if/else if/else`; `animate(motionExpr) { … }` (arg type motion).
- `let r = await call op.id(args)` — r has type `{ok: bool, value: R?, error: {code: string, message: string}?, fieldErrors: map<string>?}`; `call op.id(args)` statement without await (fire-and-forget) cannot bind. op unknown → "operation".
- Effects per stdlib.json `effects` (arg types checked). `refresh(x)`: x must be a data name; `play(x)`/`stop(x)`: x must be a declared timeline name (caller supplies the set); `await` is legal before `call` and `play` only.
- `name(args)` statement where name is a handler → `run`; calling a handler inside an expression → type error.
- `emit name` only when the caller says we're inside a template and `event name` is declared → else error containing "emit".

## Lowering (spec §3.8, exact forms)
Expressions: `["v", json]`, `["s", n]`, `["i", n]`, `["c", n]`, `["d", n]`, `["l", n]`, `["p", n]`, `["e", n]`, `["t", group, name]`, `["dr", "scroll"|"drag", id, "x"|"y"]`, `["self", "pressed"]`, `["get", obj, field]`, `["at", obj, index]`, `["op", op, a, b]`, `["un", op, a]`, `["if", c, a, b]`, `["f", name, [args]]` (receiver first for methods/properties), `["u", name, [args]]`, `["lam", [params], body]`, `["list", [..]]`, `["rec", {k: e}]`, `["str", [parts]]` (literal parts as `["v", "text"]`). Constant lists/records may be folded into `["v", json]` — both forms are valid wire.
Statements: `["set", "s"|"i", name, [pathExprs], expr]`, `["let", name, expr]`, `["if", cond, [then], [else]]` (else-if = nested if inside else), `["anim", motionExpr, [body]]`, `["call", opId, argsExpr, resultName|null, await]`, `["do", effect, [argExprs], await]`, `["run", handler, [argExprs]]`. `refresh/play/stop` args lowered as `["v", "<name>"]`. Durations become plain numbers (ms). `emit` is not lowered by dsl (Task 3 inlines it) — represent it in the typed tree and let Task 3 expand; if you must lower it standalone, use `["emit", name]` (Task 3 guarantees none reaches the wire).

## Diagnostics
`Diag{Line, Col int; Msg string}`, 1-based positions relative to the parsed source string, messages in English. Collect multiple errors where cheap. Messages must contain the keywords used by contract/conformance/errors.json (`type unknown token icon assign recurs cycle operation read emit lambda annot duplicate syntax`).

## Tests
Table tests per area + a test that runs every case in `../contract/conformance/errors.json` (script/expr must produce a diagnostic containing `error`, case-insensitive) and asserts every script/expr in `../contract/conformance/{expr,format,handlers}.json` compiles WITHOUT diagnostics (using contract/operations.json for op types, token names from contract/tokens/korpu.json when it exists — if it doesn't exist yet, use the token names from ~/projects/bdui-contracts-wave1/catalog/tokens.json plus motion names press/interactive/snappy/bouncy/gentle/fast/smooth/standard/slow and icon names from contract/icons.json or the 21 names: close back chevronRight chevronDown info check plus share copy ticket gift clock warning retry heart star arrowRight sparkles lock mail user).

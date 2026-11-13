---
status: accepted
---

# List spread in literals

A list literal nests what it holds: with `A: [a, b]`, `[.A, d]` is `[[a, b], d]`. That matches JSON, and we want to keep it. But it leaves no way to build one list from others, such as adding `--verbose` to `CURL_OPTS` under an `if:`. A trailing `...` on an element now splices it in: `[.A..., .B..., d]` is `[a, b, c, d]`. Without `...`, an element nests as before.

The spelling comes from Go's `append(a, b...)`, but Go's rules don't come with it. Go only allows `...` once per call, as the last argument, and never inside a composite literal or a `text/template`. Here a literal can have any number of spreads, in any position.

## Rules

- Spread works in every literal that builds structure: `set:`, `with:`, `into:`, `const:` and `vars:`.
- Spreading an empty list adds nothing.
- Spreading anything that isn't a list (a string, number, bool, object or null) is an error, the same way a wrong-kind accessor is. `| json` fixes text that should have been a list.
- Spread applies to a whole element reference, accessors and filters included: `.QUESTIONS[*].category...`, `(.X | json)...`.
- In argv lists, which already splice arrays, `...` is accepted and changes nothing, so both kinds of list use the same spelling.
- `...` outside a list element is an error when the file loads, in every field that takes a reference (`B:`, `loop:`, `options:`, `default:`, `dir:` and the rest hold a single value). Only a bare reference or parenthesised pipeline followed by `...` counts; `Loading...` and `"{{ .A }}..."` stay text everywhere.
- A spread entry in a list of `KEY: value` maps (`set:`, `into:`) has no meaning there and is an error.

## Considered Options

- **Splice arrays implicitly, as argv lists do.** No new syntax, but it removes nesting, and we want lists to behave like JSON.
- **Sprig-style `append` / `concat` functions.** These match Go-template conventions (Helm) and need no grammar change. But they move list-building out of YAML list syntax into template strings, and the piped value lands as the last argument, so `.A | concat .B` would read as B followed by A.
- **An append form of `set:` (`OPTS+: [...]`).** Covers adding to one list, but not combining two into a third.
- **`...` spread in literals (chosen).** Keeps nesting as the default, makes each splice visible where it happens, and keeps lists written as YAML lists.

## Consequences

The reference grammar gains a suffix. Today a bare field of `.A...` is the plain text `.A...`; under this decision it is a spread inside a list and an error anywhere else (`B: .A...` has no list to splice into). Text that really ends in `...` after a reference has to be written as a template, such as `"{{ .A }}..."`. Go users may expect Go's single-trailing-spread limit, so the reference should say that only the spelling is borrowed.

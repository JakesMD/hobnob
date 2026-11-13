# Templating

Lookup tables for templates. The guide introduces [templates](../guide/templates.md) and explains how values get their [types](../guide/structured-data.md).

## Accessors

| Form                       | Meaning                                                         |
| -------------------------- | --------------------------------------------------------------- |
| `.A.b.c`                   | object keys                                                     |
| `.A[0]`                    | array index                                                     |
| `.A[-1]`                   | negative index, from the end                                    |
| `.A[1:3]`                  | slice (bounds clamp, like Go: `[0:99]` on 3 elements returns 3) |
| `.A[*]`                    | every element of an array, or every value of an object          |
| `.A["key with . or /"]`    | a literal key that is not a valid identifier                    |
| `.A[.KEY]`                 | dynamic key or index, taken from a variable                     |
| `.A[.KEY][0].name`         | any combination, any depth                                      |
| `(.CATEGORIES \| json)[0]` | a pipeline result as the head                                   |

## Filters

Usable anywhere templates are, and in `into:` pipes. Chain with `|`.

| Filter            | Does                                                       |
| ----------------- | ---------------------------------------------------------- |
| `default "x"`     | Falls back when the value is empty or a missing path.      |
| `trim`            | Strips leading and trailing whitespace.                    |
| `upper` / `lower` | Changes case.                                              |
| `split ","`       | Splits a string into a list, dropping empty parts.         |
| `lines`           | Splits on newlines, trimming each and dropping blank ones. |
| `join ","`        | Joins a list's elements into one string with a separator.  |
| `json`            | Parses a string into a real array or object.               |
| `string`          | Forces a value to text; compact JSON for a list or object. |
| `keys`            | Sorted list of an object's top-level keys.                 |
| `len`             | Length of a string (in runes), array, or object.           |
| `quote`           | Wraps in POSIX single quotes for a `run:` string command.  |
| `jsonEscape`      | Escapes a string for embedding inside JSON text.           |

```yaml
tasks:
  fetch-questions:
    steps:
      - set:
          - CATEGORY: '{{ .TRIVIA_CATEGORY | default "general_knowledge" }}'
      - run: curl -s "{{.API}}/questions?limit=10" | jq -r '.[].question.text'
        into:
          - TEXTS: stdout | lines
      - run: echo {{ .QUESTIONS[0].correctAnswer | quote }}
```

- **`json`** leaves structured values alone, so it is always safe before an
  accessor or `keys`.
- **`keys`** works on objects only; parse a string with `| json` first. For values, use `[*]`: with `/metadata` captured as `META`,
  `.META.byDifficulty | keys` gives `["easy","hard","medium"]` and
  `.META.byDifficulty[*]` gives `[2963,4997,7339]`, same order.
- **`join`** is the inverse of `split`: `.CATEGORIES | join ","` gives
  `history,science`. Numbers and bools join as their text. A `null`, list or
  object element errors; access the text you meant first, such as
  `.QUESTIONS[*].category`. Anything but a list errors too; parse a string
  with `| json` first. An empty list joins to `""`.
- **`quote`** escapes for the string `run:` form; the
  [list form](../guide/commands.md#two-ways-to-write-a-command) usually removes the need.

Name intermediate values rather than stacking many pipes inline:

```yaml
tasks:
  fetch-question:
    steps:
      - set:
          - CATEGORIES: "{{ .CATEGORIES_TEXT | json }}"
      - run: [curl, -s, "{{.API}}/questions?limit=1&categories={{ .CATEGORIES[0] }}"]
```

## Comparisons

`eq` / `ne` / `lt` / `le` / `gt` / `ge` compare typed values. `if:` runs the
rendered `true`/`false` as a shell condition:

```yaml
tasks:
  lab-check:
    steps:
      - run: echo "lab coats on"
        if: '{{ eq .CATEGORY "science" }}'
      - run: echo "that is a lot of questions"
        if: "{{ gt (.QUESTIONS | len) 5 }}"
```

- **JSON number text on both sides** (`-3`, `1.5`, `1e9`), whatever its kind,
  compares numerically and exactly. So with CLI args `A=9 B=10`,
  `{{ lt .A .B }}` is `true`.
- **Text that only looks numeric** (`Inf`, `NaN`, `0x10`, `007`, `+3`)
  compares as text.
- **A missing variable** equals `""`.
- **Anything else** compares as text, exactly and lexically.

Bools support only `eq`/`ne`; arrays and objects are not comparable, so access
the field you meant. `eq` is true if any right-hand argument matches:
`eq .CATEGORY "history" "geography"`.

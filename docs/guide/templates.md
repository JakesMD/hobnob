# Templates

Anywhere you write text in a taskfile, such as a command, a condition, a value
or a path, you can drop a variable in with `{{ }}`. That's a template:

```yaml
tasks:
  announce:
    steps:
      - set:
          - CATEGORY: science
      - run: echo "Today's category is {{ .CATEGORY }}"
```

```
$ hobnob announce
run: [announce] echo "Today's category is science"
[announce] Today's category is science
✓ [announce] done
```

A template is filled in when its step runs, not when the file loads, so it
always sees what the steps before it set.

hobnob's templates are Go templates, with extra filters and a way of reaching
into lists and maps (see [Structured data](./structured-data.md)).

## Filters

A filter changes a value on its way into the text. Put it after the variable
with a `|`. You can chain several, and they apply left to right:

```yaml
tasks:
  announce:
    steps:
      - run: echo "Today's category is {{ .CATEGORY | default "general_knowledge" | upper }}"
```

```
$ hobnob announce
[announce] Today's category is GENERAL_KNOWLEDGE

$ hobnob announce CATEGORY=science
[announce] Today's category is SCIENCE
```

You'll use four filters all the time:

- **`default "x"`** fills in a value when the variable is empty or missing.
- **`trim`** removes the newline at the end of a command's output, which
  you'll almost always want when you keep it (see
  [Capturing output](./capturing-output.md)).
- **`quote`** makes a value safe to put in a shell command, even when it
  holds quotes or semicolons. `{{ .Q | quote }}`, with `Q` set to
  `it's a trap; rm -rf`, runs `echo 'it'\''s a trap; rm -rf'`.
- **`json`** turns JSON text into a real list or map (see
  [Structured data](./structured-data.md#text-that-looks-like-json)).

The [filter table](../reference/templating.md#filters) lists them all.

## If and else

A template can choose between two pieces of text:

```yaml
tasks:
  mood:
    steps:
      - run: echo "{{ if eq .CATEGORY "science" }}lab coats on{{ else }}desk day{{ end }}"
```

```
$ hobnob mood CATEGORY=science
[mood] lab coats on

$ hobnob mood
[mood] desk day
```

`eq` checks two values are equal. The other comparisons (`ne`, `lt`, `le`,
`gt`, `ge`) are in the [comparisons table](../reference/templating.md#comparisons),
and `and`, `or` and `not` combine them.

Keep these short. To decide whether a whole step runs, give it an
[`if:`](./conditions.md) instead, which is easier to read.

## Leaving out the braces

When a field holds a single variable and nothing else, you can leave out the
`{{ }}`. Without braces, YAML doesn't need the value in quotes either:

```yaml
tasks:
  pick-category:
    steps:
      - set:
          - CATEGORIES: [history, science]
      - get:
          - CATEGORY:
              options: .CATEGORIES # same as "{{ .CATEGORIES }}"
```

A field like this keeps the variable as it is, so `options:` gets a real
list. Put any other text around the variable and the field becomes text
instead (see [Keeping a type](./structured-data.md#keeping-a-type)).

## Rules

- **A bare variable starts with a `.` and then a capital letter or `_`**, as
  in `.CATEGORIES` or `._RAW`. It can carry accessors and filters:
  `.CATEGORIES | len`. Anything else starting with a dot, such as `./scripts`
  or `.env`, is plain text.
- **Go's `range` and `printf` don't work with hobnob's values.** Use a
  [`loop:`](./loops.md) to repeat steps, and filters to shape text.

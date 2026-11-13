# Structured data

When a command prints JSON, hobnob keeps it as data, not text. You can reach
straight into it, in any later step, without `jq` or a parsing step:

```yaml
tasks:
  fetch-question:
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1"
        into:
          - QUESTIONS: stdout
      - run: echo "{{ .QUESTIONS[0].question.text }} {{ .QUESTIONS[0].correctAnswer }}"
```

```
$ hobnob fetch-question
…
[fetch-question] In which country did the 'Bay of Pigs' invasion take place? Cuba
✓ [fetch-question] done
```

Numbers and `true`/`false` keep their type too. With the API's `/metadata`
captured as `META`, `{{ gt .META.byCategory.science 1000 }}` compares two
numbers, however many steps later you write it.

## Where data comes from

A variable holds a list or a map in only three cases:

1. **You wrote one in YAML**, in `set:`, `vars:`, `const:`, `with:` or
   `into:`, such as `[history, science]` or `{ easy: 1, hard: 3 }`.
2. **A command printed a JSON list or object**, and you kept it with
   [`into:`](./capturing-output.md).
3. **You used the `json` filter** to turn text into data.

Everything else is text, even when it looks like JSON (see
[Text that looks like JSON](#text-that-looks-like-json)).

## Reaching into data

Reach into data the way it's written: `.key` for a field of a map, `[0]` for
an item of a list, and `[1:3]` for a slice of a list. Combine them to any
depth:

```yaml
tasks:
  fetch-question:
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1"
        into:
          - QUESTIONS: stdout
      - set:
          - QUESTION: .QUESTIONS[0].question.text
```

`[-1]` counts from the end, and `[.KEY]` takes the key from another variable.
The [accessor table](../reference/templating.md#accessors) lists every form.

## Every item at once

`[*]` reaches into every item of a list. The rest of the path then applies to
each one, and you get a list back:

```yaml
tasks:
  fetch-questions:
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=5"
        into:
          - QUESTIONS: stdout
      - set:
          - TEXTS: .QUESTIONS[*].question.text # ["How many limbs does an octopus have?", …]
          - FIRST_TWO: .QUESTIONS[0:2].difficulty # ["easy", "hard"]
```

A slice like `[0:2]` works the same way, for just those items.

## Missing values

Reaching for something that isn't there stops the task with an error, so a
typo can't quietly turn into an empty value. The questions have no `author`
field, so `{{ .QUESTIONS[0].author }}` fails with `"author" not found`. Add
`default` where a value really might be missing:

```yaml
tasks:
  show-author:
    steps:
      - run: echo "{{ .QUESTIONS[0].author | default "anonymous" }}" # anonymous
```

## Keeping a type

A field that holds a single variable and nothing else keeps it as it is, so
a list stays a list. Put any other text around it and the field becomes text:

```yaml
tasks:
  show-categories:
    steps:
      - set:
          - A: .CATEGORIES # still a list
          - B: "{{ .CATEGORIES }}" # still a list
          - C: "count: {{ .CATEGORIES | len }}" # text
```

This is why `options: .CATEGORIES` gives a prompt a real list of choices.

## Combining lists

A list inside a list stays a separate item, as it would in JSON. Add `...`
after it to pour its items in instead:

```yaml
tasks:
  build-curl-opts:
    steps:
      - set:
          - CURL_OPTS: [-s, --max-time, "10"]
          - EXTRA: [--retry, "3"]
          - NESTED: [.CURL_OPTS, --verbose] # [["-s", "--max-time", "10"], "--verbose"]
          - FLAT: [.CURL_OPTS..., .EXTRA..., --verbose] # ["-s", "--max-time", "10", "--retry", "3", "--verbose"]
```

That makes it easy to add to a list as you go:

```yaml
tasks:
  build-curl-opts:
    steps:
      - set:
          - CURL_OPTS: [.CURL_OPTS..., --verbose]
        if: '[ "{{.VERBOSE}}" = true ]'
```

## Lists as command arguments

In the [list form](./commands.md#two-ways-to-write-a-command) of `run:`, a list variable
becomes several arguments, one per item. You don't need `...`:

```yaml
tasks:
  fetch-question:
    steps:
      - set:
          - CURL_OPTS: [-s, --max-time, "10"]
      - run: [curl, .CURL_OPTS, "https://the-trivia-api.com/v2/questions?limit=1"]
      # runs: curl -s --max-time 10 https://the-trivia-api.com/v2/questions?limit=1
```

## Text that looks like JSON

Command-line arguments, env files and your shell's environment only ever
give text, however much it looks like JSON. Turn it into data with `| json`:

```bash
hobnob report CATEGORIES='["history","science"]'
```

```yaml
tasks:
  report:
    steps:
      - loop: .CATEGORIES | json
        steps:
          - run: echo "{{.ITEM}}"
```

Without `| json`, `.CATEGORIES` is 21 characters of text, not a list of two.

## Rules

### Reaching into data

- **A key from a variable is matched exactly.** With `K` set to
  `science.v2`, `.LABELS[.K]` finds the key `science.v2`; the dot isn't read
  as a path.
- **`[*]` and slices drop items the rest of the path doesn't match.**
  If one question has no `question.text`, `TEXTS` simply has one fewer item.
  If none match, you get an empty list.
- **`[*]` on a map gives its values**, sorted by key, in the same order as
  `keys`.
- **Treating text as data is always an error**, even with `default`. Indexing
  text, or spreading it with `...`, means it needs `| json` first.
- **Data is only detected once**, when it's captured or written. A later
  filter never reads text as a list, which is why `keys`, `join` and `[0]`
  fail on text.

### Combining lists

- **Spread as many lists as you like, anywhere in a list.** An empty list
  adds nothing.
- **`...` works in any list you write**: in `set:`, `vars:`, `const:`,
  `with:` and `into:`. In `into:` you can spread an output too:
  `[stdout..., exit]`.
- **`...` applies to the whole item**, including its path and filters:
  `.QUESTIONS[*].category...`, `(.TAGS | json)...`.
- **Only a variable spreads.** Plain text such as `Loading...` stays text.
  To put dots after a variable's value, write `"{{ .A }}..."`.
- **Spreading anything but a list is an error.**

### Lists as command arguments

- **An empty list adds no arguments**, but an empty string `""` stays as one
  empty argument, so the arguments after it don't shift.
- **`...` is allowed and changes nothing**, so `[curl, .CURL_OPTS..., .URL]`
  runs the same as `[curl, .CURL_OPTS, .URL]`.
- **A map is an error.** Pick the field you meant, such as `.CONFIG.url`.

# Running once

Sometimes several tasks call the same helper, and it should only run once:
one prompt, one download, one login. Mark it `once: true`. The first call
runs it, and every later call gets the same values back without running it
again:

```yaml
tasks:
  _pick-category:
    once: true
    results: [CATEGORY]
    steps:
      - get:
          - CATEGORY:
              info: Which category?
              options: [history, science, music]

  ask-question:
    steps:
      - call: _pick-category
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"

  count-questions:
    steps:
      - call: _pick-category
      - run: curl -s https://the-trivia-api.com/v2/metadata

  morning:
    steps:
      - call: ask-question
      - call: count-questions
```

`hobnob morning` asks for a category once. When `count-questions` calls
`_pick-category`, hobnob doesn't ask again. It prints the values the first
run set instead:

```
call: [count-questions] _pick-category (cached — CATEGORY=history)
```

Running `ask-question` or `count-questions` on its own still asks, because
each is then the first call.

## What a later call gets

hobnob remembers every variable the first run ended with. A later call takes
from those exactly what it would take from a real run: the task's
`results:`, or whatever its own `into:` lists.

## Rules

- **`once:` belongs to the task**, so it applies to every call, from
  anywhere, for as long as hobnob is running.
- **A later call's `with:` is ignored.** It gets the first run's values, even
  if it passes something different.
- **A later call overwrites.** If the caller changed `CATEGORY` since the
  first call, the next call puts the remembered value back. Only the values
  it brings back are touched.
- **A task that skipped itself with `if:`** is remembered as setting nothing,
  so every later call gets nothing back either.
- **A failure under `soft: true` isn't remembered**, so the next call runs the
  task again.

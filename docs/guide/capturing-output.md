# Capturing output

Add `into:` to a `run:` step to keep what the command printed in a variable.
Later steps can then use it:

```yaml
tasks:
  build:
    steps:
      - run: git rev-parse --short HEAD
        into:
          - SHA: stdout | trim
      - run: echo "building {{.SHA}}"
```

```
$ hobnob build
run: [build] git rev-parse --short HEAD
[build] 54a1ede
run: [build] echo "building 54a1ede"
[build] building 54a1ede
✓ [build] done
```

`stdout` is what the command printed. Commands end their output with a
newline, so `| trim` removes it. Any [filter](./templates.md#filters) works
here, chained with `|` as in a template.

Capturing keeps a copy; it doesn't hide anything. To keep the output off the
screen as well, add [`quiet:`](./commands.md#hiding-noisy-output).

## What you can capture

A command gives you three things:

- **`stdout`**: what it printed.
- **`stderr`**: what it printed as errors.
- **`exit`**: its exit code, as a number. `0` means it succeeded.

When a command prints a JSON list or object, `stdout` is real data, so you can
reach straight into it (see [Structured data](./structured-data.md)):

```yaml
tasks:
  fetch-questions:
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=5"
        into:
          - QUESTIONS: stdout # the whole list
          - FIRST: stdout[0].question.text # one field from it
          - ANSWERS: stdout[*].correctAnswer # every answer, as a list
```

## Using earlier captures

Each entry can use the ones above it:

```yaml
tasks:
  build:
    steps:
      - run: git rev-parse --short HEAD
        into:
          - SHA: stdout | trim
          - TAG: "build-{{.SHA}}" # build-54a1ede
```

## Building a map

An entry can be a map, built from several parts of the output:

```yaml
tasks:
  make-card:
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1"
        into:
          - CARD:
              text: stdout[0].question.text
              niche: stdout[0].isNiche
      # CARD is {"niche": false, "text": "In which US election year…"}
```

Each part keeps its type, so `niche` is a real `false`, not the text
`"false"`.

## Rules

- **Output is captured even when the command fails.** Combined with
  [`soft:`](./errors.md), that's how a task reacts to a failure: capture
  `exit` and `stderr`, then decide what to do (see
  [Reacting to a failure](./errors.md#reacting-to-a-failure)).
- **`into:` works on `call:` steps too**, where it reads the called task's
  variables instead of a command's output (see
  [Calling tasks](./calling-tasks.md#getting-values-back)).

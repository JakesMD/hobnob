# Conditions

Add `if:` to a step to run it only when a condition holds:

```yaml
tasks:
  lab-check:
    steps:
      - run: echo "lab coats on"
        if: '{{ eq .CATEGORY "science" }}'
      - run: echo "let's go"
```

```
$ hobnob lab-check CATEGORY=science
run: [lab-check] echo "lab coats on"
[lab-check] lab coats on
run: [lab-check] echo "let's go"
[lab-check] let's go
✓ [lab-check] done

$ hobnob lab-check CATEGORY=history
run: [lab-check] skipped (if: false)
run: [lab-check] echo "let's go"
[lab-check] let's go
✓ [lab-check] done
```

## How a condition works

`if:` is a shell command. hobnob fills in its template, runs it, and runs the
step only if the command succeeds (exits with `0`).

The template above becomes `true` or `false`, and the shell has commands
with those names, so that's all it takes. hobnob's comparisons are `eq`,
`ne`, `lt`, `le`, `gt` and `ge`. They compare numbers as numbers, so
`{{ gt (.QUESTIONS | len) 5 }}` works as you'd expect. The
[comparisons table](../reference/templating.md#comparisons) explains how
each type compares.

Because it's a shell command, any shell test works too, such as checking
whether a file exists:

```yaml
tasks:
  import:
    steps:
      - run: ./import.sh questions.json
        if: '[ -f questions.json ]'
```

## Skipping a whole task

Put `if:` on the task itself to skip all its steps:

```yaml
tasks:
  explain-answer:
    if: '{{ eq .CATEGORY "science" }}'
    steps:
      - run: echo "here is the science behind it"
      - set:
          - EXPLAINED: "yes"
```

```
$ hobnob explain-answer CATEGORY=history
⊘ [explain-answer] skipped
✓ [explain-answer] done
```

## Calling a task that skips

A skipped task isn't an error, but its steps don't run, so it sets nothing.
When `explain-answer` skips, `EXPLAINED` is never set, so a caller reading it
needs `| default`:

```yaml
tasks:
  quiz:
    steps:
      - call: explain-answer
        into:
          - EXPLAINED: .EXPLAINED | default "no"
```

## Rules

- **Prefer comparisons to shell tests for variables.** A variable goes into
  the shell command as plain text, as in a
  [command written as text](./commands.md#two-ways-to-write-a-command). In
  `'[ "{{.CATEGORY}}" = "science" ]'`, a `CATEGORY` holding a `"` breaks the
  command, and the step is skipped without an error. `{{ eq .CATEGORY
  "science" }}` can't break this way. If you need a shell test, escape the
  value with [`quote`](./templates.md#filters).
- **`if:` runs in the task's working directory**, even on a step with its own
  [`dir:`](./commands.md#running-in-another-directory).

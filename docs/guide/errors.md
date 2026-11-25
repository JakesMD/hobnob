# Errors

When a command fails, the task stops there, and so does everything else
hobnob was running:

```yaml
tasks:
  fetch-question:
    steps:
      - run: curl -sf https://the-trivia-api.com/v2/question/no-such-id
      - run: echo "never runs"
```

```
$ hobnob fetch-question
fetch-question $ curl -sf https://the-trivia-api.com/v2/question/no-such-id
fetch-question ✗ 0.4s curl -sf https://the-trivia-api.com/v2/question/no-such-id: exit status 56
fetch-question ✗ failed after 0.4s
```

The failed step ends with `✗`, its time and the command with why it failed, as
`fetch-question ✗ 0.4s curl -sf …: exit status 56`. Then every task in the call
chain ends with `✗ failed after` and its time, innermost first, one line per
task. hobnob itself exits with `1`, whatever code the command exited with.

## Carrying on after a failure

Add `soft: true` to a step that's allowed to fail. If it fails, the task
carries on with the next step:

```yaml
tasks:
  fetch-question:
    steps:
      - run: curl -sf https://the-trivia-api.com/v2/question/no-such-id
        soft: true
      - run: echo "still running"
```

```
$ hobnob fetch-question
fetch-question $ curl -sf https://the-trivia-api.com/v2/question/no-such-id
fetch-question ! 0.4s curl -sf https://the-trivia-api.com/v2/question/no-such-id: exit status 56
fetch-question $ echo "still running"
fetch-question │ still running
fetch-question ✓ 0.0s
fetch-question ✓ done after 0.5s • 1 soft failure
```

`soft:` works on a `call:` step too: if the called task fails, the caller
carries on. After the called task's own lines, the calling step ends with a
yellow `!` and the failure counts in the caller's total: `deploy ! 1.9s → lint:
failed`.

## Reacting to a failure

A `soft:` step still captures its output when it fails. Capture `exit`, and a
later step can check it with [`if:`](./conditions.md):

```yaml
tasks:
  fetch-question:
    steps:
      - run: curl -sf https://the-trivia-api.com/v2/question/no-such-id
        soft: true
        into:
          - CODE: exit
          - ERRORS: stderr
      - run: echo "no question with that id (curl exited {{.CODE}})"
        if: "{{ ne .CODE 0 }}"
```

```
$ hobnob fetch-question
fetch-question $ curl -sf https://the-trivia-api.com/v2/question/no-such-id
fetch-question ! 0.4s curl -sf https://the-trivia-api.com/v2/question/no-such-id: exit status 56
fetch-question $ echo "no question with that id (curl exited 56)"
fetch-question │ no question with that id (curl exited 56)
fetch-question ✓ 0.0s
fetch-question ✓ done after 0.5s • 1 soft failure
```

## Rules

- **A `soft:` step that fails ends with a yellow `!` where `✗` would be**, and
  no other note: `deploy ! 0.6s golangci-lint run: exit status 1`. The task's
  end line then counts them in grey: `deploy ✓ done after 8.2s • 1 soft
  failure`. That covers any failure, not just a command exiting non-zero. A
  typo in a template or a path that leads nowhere is passed over just as
  quietly. Only use `soft:` on steps you expect might fail.
- **Only `run:` and `call:` steps take `soft:`.** Inside a
  [loop](./loops.md), put it on the steps that might fail.
- **A step that can't start is one `✗` line**: the step as written, then
  what's wrong, with no time: `deploy ✗ echo {{ upper .X }}: upper: expected a
  string, got nil`. That covers a template error, a broken `if:` (`deploy ✗
  echo hi: if {{ nope }}: function "nope" not defined`) and a `call:` to a task
  that doesn't exist (`deploy ✗ nope: no task "nope"`). A `set:`, `loop:` or
  `get:` shows its field as written: `deploy ✗ X={{ upper .Y }}: …`, `deploy ✗
  loop {{ .Items }}: …`, `deploy ✗ get X: …`. The task's end line follows.
- **A command that can't start, or an `into:` that can't take its output,
  ends like any failed step**, with a time: `deploy ✗ 0.0s nosuchcmd a: no such
  command`, `deploy ✗ 0.0s echo hi: no directory ./nowhere`, `deploy ✗ 0.1s
  echo notjson: into X: …`.
- **Before a run, `hobnob` takes the task's place**, in bold: `hobnob ✗
  hobnob.yml line 2: did not find expected ',' or ']'` for an error, `hobnob ✗
  no task "nope"`, and `hobnob !` in yellow for a warning after which hobnob
  carries on, such as `hobnob ! no env file missing.env, skipping it`.
- **A command killed by a signal has an `exit` of `-1`.**
- **A command that never started**, such as one that isn't installed,
  captures nothing, so `CODE` above wouldn't be set. Write
  `.CODE | default -1` if that can happen.

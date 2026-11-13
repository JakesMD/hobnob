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
run: [fetch-question] curl -sf https://the-trivia-api.com/v2/question/no-such-id
✗ [fetch-question] exit status 56
```

hobnob itself then exits with `1`, whatever code the command exited with.

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
run: [fetch-question] curl -sf https://the-trivia-api.com/v2/question/no-such-id
run: [fetch-question] echo "still running"
[fetch-question] still running
✓ [fetch-question] done
```

`soft:` works on a `call:` step too: if the called task fails, the caller
carries on.

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
run: [fetch-question] curl -sf https://the-trivia-api.com/v2/question/no-such-id
run: [fetch-question] echo "no question with that id (curl exited 56)"
[fetch-question] no question with that id (curl exited 56)
✓ [fetch-question] done
```

## Rules

- **A `soft:` step that fails prints nothing**, and that covers any failure,
  not just a command exiting non-zero. A typo in a template or a path that
  leads nowhere is passed over just as quietly. Only use `soft:` on steps
  you expect might fail.
- **Only `run:` and `call:` steps take `soft:`.** Inside a
  [loop](./loops.md), put it on the steps that might fail.
- **A command killed by a signal has an `exit` of `-1`.**
- **A command that never started**, such as one that isn't installed,
  captures nothing, so `CODE` above wouldn't be set. Write
  `.CODE | default -1` if that can happen.

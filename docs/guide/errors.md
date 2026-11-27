---
description: "What happens when a command in a hobnob task fails, and how to carry on after a failure or react to it."
---

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

hobnob itself exits with `1`, whatever code the command exited with.

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
carries on, and the failure counts as a soft failure.

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

- **`soft:` covers any failure**, not just a command exiting non-zero. A
  typo in a template or a path that leads nowhere is passed over just as
  quietly. Only use `soft:` on steps you expect might fail.
- **Only `run:` and `call:` steps take `soft:`.** Inside a
  [loop](./loops.md), put it on the steps that might fail.
- **A command killed by a signal has an `exit` of `-1`.**
- **A command that never started**, such as one that isn't installed,
  captures nothing, so `CODE` above wouldn't be set. Write
  `.CODE | default -1` if that can happen.

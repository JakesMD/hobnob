# Secrets

hobnob prints every command before it runs it. If a command holds a password,
token or private URL, mark that value `secret:` and hobnob prints `****` in
its place:

```yaml
tasks:
  announce:
    steps:
      - set:
          - WEBHOOK:
              value: https://hooks.example.com/abc
              secret: true
      - run: curl -s -X POST "{{.WEBHOOK}}" -d '{"text":"quiz is ready"}'
```

```
$ hobnob announce
run: [announce] curl -s -X POST "****" -d '{"text":"quiz is ready"}'
✓ [announce] done
```

The command itself still gets the real value.

## Marking a value secret

Mark it where the value is first set:

- **In `set:`, `vars:` and `const:`**, write the entry with `value:` and
  `secret: true`, as above.
- **In a prompt**, add `secret: true` to the [`get:`](./prompts.md#prompt-options)
  entry. hobnob hides the answer as you type it, too.
- **In an env file**, add `secret: true` to its `env:` entry. Every value the
  file sets is then masked:

  ```yaml
  env:
    - .env:
        secret: true
    - defaults.txt # not masked
  ```

## What gets masked

hobnob masks what it prints itself: each command before it runs, and
[`quiet:`](./commands.md#hiding-noisy-output) messages. It can't mask what a
command prints, so this reveals the value:

```yaml
tasks:
  leak:
    steps:
      - run: echo "{{.WEBHOOK}}"
```

```
$ hobnob leak
run: [leak] echo "****"
[leak] https://hooks.example.com/abc
```

A secret stays masked wherever it goes: into another variable with `into:`,
or into another task with `call:` and back again. hobnob masks the value, not
the variable's name, so a new name doesn't uncover it.

## Rules

- **Only the exact value is masked.** Anything made from it, such as
  `{{ .WEBHOOK | upper }}` or part of it, is printed in full.
- **`with:` doesn't take `secret:`.** A value passed to another task is
  already masked if it was secret.
- **When two env files set the same name, the one that wins decides.** If
  `.env` is secret but a later `defaults.txt` sets the same name, the value
  from `defaults.txt` is used and isn't masked.

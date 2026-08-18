# hobnob

A YAML task runner built around 3 things most others get wrong:

- **Prompts** — text input or select menus, dropped anywhere in a task.
- **JSON, natively** — query, filter, slice, no `jq` needed.
- **Tasks return values** — capture a command's output or a sub-task's results,
  explicitly, back into the caller.

---

## 📦 Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

---

## 👀 Quick look

```yaml
# hobnob.yml

tasks:
  tell-joke:
    info: Tell a joke from a category you pick
    steps:
      # Returning vars: curl's stdout captured straight into a var
      - run: curl -s https://official-joke-api.appspot.com/types
        into:
          - CATEGORIES: stdout

      # Interactive prompts: options pulled from the step above
      - get:
          - CATEGORY:
              options: .CATEGORIES

      # Returning vars: whole JSON blob pulled back from an isolated sub-task
      - call: _fetch-joke
        into:
          - JOKE: .RESP

      # JSON handling: pluck fields out of the blob, no jq
      - run: 'echo "{{ .JOKE | pluck "[0].setup" }} ... {{ .JOKE | pluck "[0].punchline" }}"'

  _fetch-joke:
    steps:
      - run: curl -s https://official-joke-api.appspot.com/jokes/{{.CATEGORY}}/random
        into:
          - RESP: stdout
```

```shell
% hobnob --file example.yml tell-joke

run: [tell-joke] curl -s https://official-joke-api.appspot.com/types
[tell-joke] ["general","knock-knock","programming","dad"]

Select a value for CATEGORY.
▶ general
  knock-knock
  programming
  dad
↑↓ move  enter select
```

---

## ✨ Other features

- **Modules** — import tasks from other files, namespaced by prefix.
- **Loops** — over a list, a matrix of arrays, or a map's key/value pairs.
- **Env files** — load vars from `.env` files or sourced shell scripts.
- **Working dir inheritance** — set it once, override per-call or per-step.
- **Secret masking** — flag any var as secret to mask it in output.
- **CI mode** — skip prompts, fail fast on missing vars.
- **Task listing** — see or interactively pick from available tasks.
- ...and much more in the [hobnob guide](GUIDE.md).

---

## 📖 Docs

Everything you need to know to write your first file is right here in the
[hobnob guide](GUIDE.md).

## 🤖 GitHub Action

Add hobnob to any workflow:

```yaml
steps:
  - uses: jakesmd/hobnob@v0
    with: # Pin a specific version (optional)
      version: v0.2.3

  - run: hobnob deploy
```

## 🤝 Contributing

Got ideas or fixes? Check out the [Contributing guide](CONTRIBUTING.md) to see
how to get involved and submit a PR.

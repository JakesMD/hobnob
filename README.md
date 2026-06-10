# hobnob

Replace the step-by-step docs nobody reads — write a YAML task once that anyone
can run.

Other task runners are either predictable **_or_** interactive. Hobnob is both.

Built for CI/CD pipelines, interactive developer CLIs, and a drop-in replacement
for the Confluence docs nobody can find and nobody keeps current.

---

## 👀 Quick look

```yaml
# hobnob.yml

tasks:
  get-releases:
    steps:
      - run: curl -s https://api.github.com/repos/jakesmd/hobnob/releases | awk -F'"' '/tag_name/{print $4}' | head -8
        into:
          - RELEASE_LIST: stdout | lines
      - get:
          - RELEASES:
              info: Releases to download
              options: .RELEASE_LIST
              default: .RELEASE_LIST | first
              multi: true
      - call: _pick-dir
        into:
          - DIR: .FOLDER
      - loop: .RELEASES
        steps:
          - run: curl -L -o {{.DIR}}/hobnob-{{.ITEM}}.tar.gz "https://github.com/jakesmd/hobnob/archive/refs/tags/{{.ITEM}}.tar.gz"

  _pick-dir:
    steps:
      - get:
          - FOLDER:
              info: Folder to save the releases to
              default: ./downloads
      - run: mkdir -p {{.FOLDER}}
```

```bash
hobnob --list
hobnob get-releases
hobnob get-releases FOLDER=./releases   # skip the folder prompt
```

Example prompt (without the colors)

```
% hobnob get-releases

Select values for RELEASES.
Releases to download
  ◉ v0.3.0
▶ ○ v0.2.1
  ○ v0.2.0
  ○ v0.1.0
↑↓ move  space toggle  enter confirm
```

---

## ✨ Top features

**Prompts** — text input or selection menus, placed anywhere in the timeline.
Options can pull from a previous step's output.

**Sub-task returns** — run separate tasks and explicitly pull their variables
back into the parent context, ensuring predictable data flow without accidental
leaks.

**Modules** — pull tasks from other files.

**Dynamic everywhere** — set variables at any point, not just upfront. Go
templates in values, conditions, options, ...

---

## 📦 Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

---

## 📖 Docs

Everything you need to know to write your first file is right here in the
[hobnob guide](GUIDE.md).

## 🤝 Contributing

Got ideas or fixes? Check out the [Contributing guide](CONTRIBUTING.md) to see
how to get involved and submit a PR.

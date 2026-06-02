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
  load-stick:
    steps:
      - run: gh release list --repo acme/releases | cut -f1
        into:
          - RELEASE_LIST: stdout | lines
      - get:
          - RELEASES:
              info: Releases to copy
              options: .RELEASE_LIST
              multi: true
      - call: _pick-drive
        into:
          - DRIVE: .DRIVE
      - loop: .RELEASES
        steps:
          - run: gh release download {{.ITEM}} --pattern "*.zip" --dir /media/$USER/{{.DRIVE}}/

  _pick-drive:
    steps:
      - run: ls /media/$USER
        into:
          - DRIVES: stdout | lines
      - get:
          - DRIVE:
              info: Destination USB stick
              options: .DRIVES
```

```bash
hobnob --list
hobnob load-stick
hobnob load-stick DRIVE=MY_STICK   # skip the drive prompt
```

Example prompt (without the colors)

```
% hobnob load-stick

Select values for RELEASES.
Releases to copy
  ◉ v1.2.0
▶ ○ v1.3.0
  ○ v1.4.0
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
curl -fsSL https://raw.githubusercontent.com/jakesmd/hobnob/main/install.sh | bash
```

---

## 📖 Docs

Everything you need to know to write your first file is right here in the
[hobnob guide](GUIDE.md).

## 🤝 Contributing

Got ideas or fixes? Check out the [Contributing guide](CONTRIBUTING.md) to see
how to get involved and submit a PR.

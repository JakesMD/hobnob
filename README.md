<div align="center">
  <a href="https://hobnob.jakesmd.dev/">
    <img src=".github/assets/logo.svg" alt="hobnob logo" width="128">
  </a>

<h1>hobnob</h1>

<p>
    Turn README steps into tasks anyone can run.
  </p>

<p>
    <a href="https://hobnob.jakesmd.dev/">Home</a> &bullet;
    <a href="https://hobnob.jakesmd.dev/docs/quick-start">Quick start</a> &bullet;
    <a href="https://hobnob.jakesmd.dev/docs/guide/">Docs</a> &bullet;
    <a href="https://github.com/jakesmd/hobnob/releases">Releases</a>
  </p>

<p>
    <a href="https://github.com/jakesmd/hobnob/actions/workflows/ci.yml"><img src="https://github.com/jakesmd/hobnob/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://github.com/jakesmd/hobnob/releases/latest"><img src="https://img.shields.io/github/v/release/jakesmd/hobnob" alt="Release"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-BSD--3--Clause-blue" alt="License"></a>
  </p>
</div>

Stop blindly copy-pasting from the README. Just run `hobnob`. It hides the
cryptic shell commands behind a friendly, interactive interface.

A README step like this one hardcodes the disk it was written on:

```bash
sudo wipefs --all /dev/sda && sudo dd if=output/os.img of=/dev/sda bs=4M status=progress conv=fsync
```

As a task in `hobnob.yml`, it only offers removable drives, and asks once more
before it wipes one:

```yaml
tasks:
  flash:
    info: Flash the built image to a disk.
    steps:
      # Removable disks only, so the laptop's own drive is never offered.
      - run: lsblk -dno NAME,SIZE,MODEL,RM | sed -n 's/ *1$//p'
        into:
          - DISKS: stdout | lines
      - get:
          - DISK:
              info: Which disk should the image be flashed to?
              options: .DISKS
          - SURE:
              info: "Wipe '{{.DISK}}'? Everything on it goes."
              options: ["no", "yes"]
      - run: '[ "{{.SURE}}" = yes ]'
      - set:
          - DISK: '{{ (.DISK | split " ")[0] }}'
      - run: sudo wipefs --all /dev/{{.DISK}}
      - run: sudo dd if=output/os.img of=/dev/{{.DISK}} bs=4M status=progress conv=fsync
```

Run it in the terminal, the browser or CI:

- **In your terminal.** `hobnob flash` runs a task by name. Plain `hobnob`
  lists every task in the project to pick from.
- **In the browser.** `hobnob --browser` puts every task on a page, for people
  who never open a terminal.
- **On CI.** Pass the answers as `NAME=value`, and the task runs straight
  through, the same steps you run.

Already use make, just or Task? hobnob is a task runner where prompts are just
steps, JSON stays data and tasks return values.
[See what it adds →](https://hobnob.jakesmd.dev/make-just-task)

[Try it in your browser →](https://hobnob.jakesmd.dev/)

## 📦 Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

One static Go binary, nothing else to install.

## 🤖 GitHub Action

Add hobnob to any workflow:

```yaml
steps:
  - uses: jakesmd/hobnob@v0
    with: # Pin any released version, or omit `with:` for the latest
      version: v0.8.0

  - run: hobnob deploy
```

## 🤝 Contributing

Got ideas or fixes? Check out the [Contributing guide](CONTRIBUTING.md) to see
how to get involved and submit a PR.

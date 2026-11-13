# Installation

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

One static binary into `~/.local/bin`, plus completion for bash, zsh and fish.

Nothing written yet? `hobnob --demo ask-trivia` runs a taskfile baked into the
binary.

## Upgrading

```bash
hobnob --upgrade
```

Replaces the running binary with the latest release. Works outside any
taskfile.

## GitHub Actions

Add hobnob to any workflow:

```yaml
steps:
  - uses: jakesmd/hobnob@v0
    with: # Pin any released version, or omit `with:` for the latest
      version: v0.8.0

  - run: hobnob deploy
```

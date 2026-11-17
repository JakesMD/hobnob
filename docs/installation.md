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

Reruns the latest installer into the running binary's directory, so PATH and
completion get set up even if hobnob got there another way. Needs `bash` and
`curl`, like the installer. Works outside any taskfile.

## GitHub Actions

Add hobnob to any workflow:

```yaml
steps:
  - uses: jakesmd/hobnob@v0
    with: # Pin any released version, or omit `with:` for the latest
      version: v0.8.0

  - run: hobnob deploy
```

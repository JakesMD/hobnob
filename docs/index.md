---
layout: home
title: hobnob
titleTemplate: Build wizards in YAML
---

<div class="landing">

<header class="l-hero">
  <div class="l-hero-text">
    <p class="l-badge">hobnob</p>
    <h1 class="l-title">Build <span class="l-accent">wizards</span> in YAML</h1>
    <p class="l-tagline">A task runner where prompts are steps, JSON stays data and tasks return values. It reads like a book, and runs like one.</p>
    <div class="l-actions">
      <a class="l-btn brand" href="./docs/quick-start">Get started</a>
      <a class="l-btn alt" href="./docs/reference/">Reference</a>
      <a class="l-btn alt" href="https://github.com/jakesmd/hobnob">GitHub</a>
    </div>
  </div>
  <div class="l-logo"><img src="/logo.svg" alt="hobnob" /></div>
</header>

<div class="l-install">

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

<p class="l-note">One static binary. On CI, <a href="./docs/installation#github-actions">add it to GitHub Actions</a>. Add <code>--web</code> to any task to <a href="./docs/guide/browser-mode">run it behind a web page</a>.</p>

</div>

<section class="l-features">
  <div class="l-feature">
    <span class="l-icon">📝</span>
    <h3>Prompts are just steps</h3>
    <p>Ask anywhere in a task: a text box, a menu, a multi-select or a file picker, built from an earlier step's output.</p>
  </div>
  <div class="l-feature">
    <span class="l-icon">🧬</span>
    <h3>JSON stays data</h3>
    <p>Dot into it ten steps later, at any depth. No <code>jq</code>, no parsing step.</p>
  </div>
  <div class="l-feature">
    <span class="l-icon">↩️</span>
    <h3>Tasks return values</h3>
    <p>Call a helper and use what it returns in the next step, like a function.</p>
  </div>
  <div class="l-feature">
    <span class="l-icon">🤖</span>
    <h3>Runs in CI too</h3>
    <p>Pass the answers as <code>NAME=value</code> and the same wizard runs straight through.</p>
  </div>
</section>

<section class="l-demo">

<TryDemo>
<template #trivia>

```yaml
tasks:
  ask-trivia:
    info: Answer a trivia question from a category you pick.
    steps:
      # Query an API. The response is a value now.
      - run: curl -fsS https://the-trivia-api.com/v2/categories
        into:
          - CATEGORIES: stdout

      # The menu is built from what that step returned.
      - get:
          - CATEGORY:
              info: Which trivia category?
              options: .CATEGORIES | keys

      # Look up the chosen category's id by name.
      - run: curl -fsS "https://the-trivia-api.com/v2/questions?limit=1&categories={{ .CATEGORIES[.CATEGORY][0] }}"
        into:
          - TRIVIA: stdout[0]

      # Shuffle the answers, so the right one isn't always first.
      - run: [sh, -c, 'printf "%s\n" "$@" | sort -R', answers, .TRIVIA.correctAnswer, .TRIVIA.incorrectAnswers]
        into:
          - ANSWERS: stdout | lines

      # Dot straight into the JSON. No parsing step.
      - get:
          - GUESS:
              info: "{{ .TRIVIA.question.text }}"
              options: .ANSWERS

      - run: echo "Correct!"
        if: "{{ eq .GUESS (.TRIVIA.correctAnswer | trim) }}"
      - run: 'echo "Not quite. It was {{ .TRIVIA.correctAnswer | trim }}."'
        if: "{{ ne .GUESS (.TRIVIA.correctAnswer | trim) }}"
```

</template>
<template #deploy>

```yaml
tasks:
  deploy:
    info: Build services and roll them out to an environment.
    steps:
      # Ask GitHub which services exist. JSON in, a list out.
      - run: gh api repos/acme/platform/contents/services
        into:
          - ALL_SERVICES: stdout[*].name

      # Three prompts: a multi-select, a menu and a masked token.
      - get:
          - SERVICES:
              info: Which services should be deployed?
              options: .ALL_SERVICES
              multi: true
          - ENV:
              info: Which environment should they go to?
              options: [staging, production]
          - TOKEN:
              info: "Cluster token for {{.ENV}}"
              secret: true

      # Once per service you picked. The token never shows.
      - loop: .SERVICES
        steps:
          - run: docker build -t ghcr.io/acme/{{.ITEM}}:{{.ENV}} services/{{.ITEM}}
          - run: kubectl --token {{.TOKEN}} -n {{.ENV}} rollout restart deploy/{{.ITEM}}
```

</template>
<template #flash>

```yaml
tasks:
  build-and-flash:
    info: Build an OS image with Packer and flash it to a disk.
    steps:
      # Ask everything up front, then leave it to run.
      - call: _pick-image
      - call: _pick-disk
      - call: build
      - call: flash

  build:
    info: Build an OS image with Packer.
    steps:
      - call: _pick-image
      - run: packer build -var out=output/os.img {{.IMAGE}}

  flash:
    info: Flash the built image to a disk.
    steps:
      - call: _pick-disk
      - run: sudo wipefs --all /dev/{{.DISK}}
      - run: sudo dd if=output/os.img of=/dev/{{.DISK}} bs=4M status=progress conv=fsync

  _pick-image:
    once: true
    results: [IMAGE]
    steps:
      # Opens in images/ and offers only Packer templates.
      - get:
          - IMAGE:
              info: Which Packer template should be built?
              type: file
              default: images/
              match: "*.pkr.hcl"

  _pick-disk:
    once: true
    results: [DISK]
    steps:
      - run: lsblk -dno NAME,SIZE,MODEL
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
```

</template>
</TryDemo>

</section>

<section class="l-cta">
  <h2>Ready to write your own?</h2>
  <div class="l-actions">
    <a class="l-btn brand" href="./docs/quick-start">Quick start</a>
    <a class="l-btn alt" href="./docs/guide/">Read the guide</a>
  </div>
</section>

</div>

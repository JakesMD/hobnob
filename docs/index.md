---
layout: home
title: hobnob
titleTemplate: Another YAML task runner, except…
---

<div class="landing">

<header class="l-hero">
  <div class="l-hero-text">
    <p class="l-badge">hobnob</p>
    <h1 class="l-title">Another YAML task runner, <span class="l-accent">except…</span></h1>
    <p class="l-tagline">Tasks return values, prompts are steps and JSON stays data. It reads like a book, and runs like one.</p>
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

<p class="l-note">One static binary. On CI, <a href="./docs/installation#github-actions">add it to GitHub Actions</a>.</p>

</div>

<section class="l-features">
  <div class="l-feature">
    <span class="l-icon">↩️</span>
    <h3>Tasks return values</h3>
    <p>Query an API from any step, then use the response for the rest of the run.</p>
  </div>
  <div class="l-feature">
    <span class="l-icon">📝</span>
    <h3>Prompts are just steps</h3>
    <p>A text box, a menu or a multi-select, built from an earlier step's output.</p>
  </div>
  <div class="l-feature">
    <span class="l-icon">🧬</span>
    <h3>JSON is data, not text</h3>
    <p>Dot into it ten steps later, at any depth. No parsing step.</p>
  </div>
  <div class="l-feature">
    <span class="l-icon">🧭</span>
    <h3>Nothing to memorize</h3>
    <p>Bare <code>hobnob</code> opens a menu of tasks, and whatever a task needs, it prompts for.</p>
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
              info: Which category?
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
              info: Which services?
              options: .ALL_SERVICES
              multi: true
          - ENV:
              info: Where to?
              options: [staging, production]
          - TOKEN:
              info: Cluster token
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
      - run: packer build images/{{.IMAGE}}

  flash:
    info: Flash a built image to a disk.
    steps:
      - call: _pick-image
      - call: _pick-disk
      - run: sudo wipefs --all /dev/{{.DISK}}
      - run: sudo dd if=output/{{.IMAGE}}.img of=/dev/{{.DISK}} bs=4M status=progress conv=fsync

  _pick-image:
    results: [IMAGE]
    steps:
      - run: ls images
        into:
          - IMAGES: stdout | lines
      - get:
          - IMAGE:
              info: Which image?
              options: .IMAGES

  _pick-disk:
    once: true
    results: [DISK]
    steps:
      - run: lsblk -dno NAME,SIZE,MODEL
        into:
          - DISKS: stdout | lines
      - get:
          - DISK:
              info: Which disk?
              options: .DISKS
          - SURE:
              info: "Wipe {{.DISK}}? Everything on it goes."
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

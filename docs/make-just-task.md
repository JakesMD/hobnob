---
layout: home
title: "hobnob: build wizards in YAML, alongside make, just or Task"
titleTemplate: false
description: Already use make, just or Task? hobnob is a task runner where a prompt is just another step, JSON stays data and tasks return values.
---

<div class="landing">

<header class="l-hero">
  <div class="l-hero-text">
    <p class="l-badge">hobnob</p>
    <h1 class="l-title">Build <span class="l-accent">wizards</span> in YAML</h1>
    <p class="l-tagline">A task runner like make, just or Task, where any step can stop and ask: pick from a menu, tick a list, paste a token. Run it in the terminal, on a page in the browser, or straight through on CI.</p>
    <div class="l-actions">
      <a class="l-btn brand" href="./docs/quick-start">Get started</a>
      <a class="l-btn alt" href="./docs/compare/">Compare</a>
    </div>
    <p class="l-switch">New to task runners? <a href="./">Start here →</a></p>
  </div>
  <div class="l-logo"><img src="/logo.svg" alt="hobnob" /></div>
</header>

<section class="l-pair l-pair-code">
  <h2 class="l-heading">When a recipe has to ask, it turns into bash</h2>
  <div class="l-pair-side">
    <p class="l-label">justfile</p>

```make
# Restore a database backup into an environment.
restore-db env="" backup="":
    #!/usr/bin/env bash
    set -euo pipefail
    env="{{env}}"
    until [ "$env" = staging ] || [ "$env" = production ]; do
      read -rp "Which environment? (staging/production) " env
    done
    backup="{{backup}}"
    if [ -z "$backup" ]; then
      backup=$(aws s3api list-objects-v2 \
        --bucket "backups-$env" \
        | jq -r '.Contents[].Key' \
        | fzf --prompt "Which backup? ")
    fi
    read -rp "Overwrite the $env database? (yes/no) " sure
    [ "$sure" = yes ]
    aws s3 cp "s3://backups-$env/$backup" dump.gz
    gunzip -c dump.gz | psql -h "db-$env" app
```

  </div>
  <div class="l-pair-side">
    <p class="l-label">hobnob.yml</p>

```yaml
tasks:
  restore-db:
    info: Restore a database backup into an environment.
    steps:
      - get:
          - ENV:
              info: Which environment?
              options: [staging, production]
      - run: aws s3api list-objects-v2 --bucket backups-{{.ENV}}
        into:
          - BACKUPS: stdout.Contents[*].Key
      - get:
          - BACKUP:
              info: Which backup?
              options: .BACKUPS
          - SURE:
              info: "Overwrite the {{.ENV}} database?"
              options: ["no", "yes"]
      - run: '[ "{{.SURE}}" = yes ]'
      - run: aws s3 cp s3://backups-{{.ENV}}/{{.BACKUP}} dump.gz
      - run: gunzip -c dump.gz | psql -h db-{{.ENV}} app
```

  </div>
  <p class="l-pair-caption">Restoring a backup asks three things. In just that's a <code>read</code> loop, <code>jq</code> to dig the backups out of the JSON, and fzf for the menu, which everyone has to install. In hobnob each question is a <code>get:</code> step, and the menu comes straight from the JSON.</p>
</section>

<section class="l-features">
  <h2 class="l-heading">What the others don't do</h2>
  <div class="l-feature">
    <h3>Prompts are just steps</h3>
    <p>Ask anywhere in a task: a text box, a menu, a multi-select or a file picker, built from an earlier step's output.</p>
  </div>
  <div class="l-feature">
    <h3>JSON stays data</h3>
    <p>Dot into it ten steps later, at any depth. No <code>jq</code>, no parsing step.</p>
  </div>
  <div class="l-feature">
    <h3>Tasks return values</h3>
    <p>Call a helper and use what it returns in the next step, like a function.</p>
  </div>
</section>

<section class="l-demo">

<div class="l-demo-intro">
  <h2 class="l-heading">Try one</h2>
  <p>Pick an example, press Run and answer its questions. The YAML beside it is the whole task.</p>
</div>

<!--@include: ./parts/try-demo.md-->

</section>

<section class="l-uses l-uses-3">
  <h2 class="l-heading">One task, three ways to run it</h2>
  <div class="l-use">
    <h3>In your terminal</h3>
    <p><code>hobnob deploy</code>, like any task runner. Plain <code>hobnob</code> opens a menu of every task.</p>
  </div>
  <div class="l-use">
    <h3>In the browser</h3>
    <p><code>hobnob --browser</code> puts every task on a page, so a client or a manager can click Deploy and fill in the blanks.</p>
  </div>
  <div class="l-use">
    <h3>On CI</h3>
    <p><code>hobnob deploy ENV=staging</code> skips the question it answers and runs straight through, the same steps you run.</p>
  </div>
</section>

<section class="l-cta">
  <h2>You don't have to switch</h2>
  <p class="l-cta-text">Keep make, just or Task for what they're good at, and write the wizards in hobnob.</p>

<div class="l-install">

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

<p class="l-note">One static binary. On CI, <a href="./docs/installation#github-actions">add it to GitHub Actions</a>.</p>

</div>

  <div class="l-actions">
    <a class="l-btn brand" href="./docs/quick-start">Quick start</a>
    <a class="l-btn alt" href="./docs/compare/">See them side by side</a>
  </div>
</section>

</div>

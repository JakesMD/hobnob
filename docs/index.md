---
layout: home
title: "hobnob: turn README steps into tasks anyone can run"
titleTemplate: false
description: Just run hobnob. It hides the cryptic shell commands in your README behind questions anyone can answer.
---

<div class="landing">

<header class="l-hero">
  <div class="l-hero-text">
    <h1 class="l-title l-log">Wiping <span class="l-accent">/dev/sda</span>…<br>…wait, what?</h1>
    <p class="l-tagline">Stop blindly copy-pasting from the README. Just run <code>hobnob</code>. It hides the cryptic shell commands behind a friendly, interactive interface.</p>
    <div class="l-actions">
      <a class="l-btn brand" href="./docs/quick-start">Get started</a>
      <a class="l-btn alt" href="https://github.com/jakesmd/hobnob">GitHub</a>
    </div>
  </div>
  <div class="l-logo"><img src="/logo.svg" alt="hobnob" /></div>
</header>

<section class="l-pair">
  <h2 class="l-heading">Turn README steps into tasks anyone can run</h2>
  <div class="l-pair-side">
    <p class="l-label">Copy-pasted from the README</p>
    <div class="l-term">
      <div class="l-term-bar"><span></span><span></span><span></span><p>zsh</p></div>
<pre><span class="l-term-prompt">$</span> sudo wipefs --all /dev/sda && sudo dd if=output/os.img of=/dev/sda bs=4M status=progress conv=fsync<span class="l-term-cursor"></span></pre>
    </div>
  </div>
  <div class="l-pair-side">
    <p class="l-label">The same steps as a hobnob task</p>
    <PageShot task="build-and-flash" info="Build an OS image with Packer and flash it to a disk." var-name="DISK" question="Which disk should the image be flashed to?" :options="['sdb  28.9G SanDisk Ultra', 'sdc  59.8G Samsung BAR Plus']" />
  </div>
  <p class="l-pair-caption">The README hardcodes the disk it was written on. hobnob only offers removable drives, and asks once more before it wipes one.</p>
</section>


<section class="l-demo">

<div class="l-demo-intro">
  <h2 class="l-heading">A task is a few lines of YAML</h2>
  <p>Tasks live in a <code>hobnob.yml</code> in the repo, so they change when the code does. Pick an example, press Run and answer its questions. The YAML beside it is the whole task.</p>
</div>

<!--@include: ./parts/try-demo.md-->

</section>

<section class="l-uses l-uses-3">
  <h2 class="l-heading">Run it in the terminal, the browser or CI</h2>
  <div class="l-use">
    <h3>In your terminal</h3>
    <p><code>hobnob build-and-flash</code> runs a task by name. Plain <code>hobnob</code> lists every task in the project to pick from.</p>
  </div>
  <div class="l-use">
    <h3>In the browser</h3>
    <p><code>hobnob --browser</code> puts every task on a page, for people who never open a terminal.</p>
  </div>
  <div class="l-use">
    <h3>On CI</h3>
    <p><code>hobnob build IMAGE=images/raspios-lite.pkr.hcl</code> answers the question up front and runs straight through, the same steps you run.</p>
  </div>
</section>

<section class="l-cta">
  <h2>Give your README a <code>hobnob.yml</code></h2>

<div class="l-install">

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

<p class="l-note">One static binary. On CI, <a href="./docs/installation#github-actions">add it to GitHub Actions</a>.</p>

</div>

  <div class="l-actions">
    <a class="l-btn brand" href="./docs/quick-start">Quick start</a>
    <a class="l-btn alt" href="./docs/guide/">Read the guide</a>
  </div>
</section>

</div>

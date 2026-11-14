<script setup lang="ts">
// Browser re-enactments of hobnob runs, one per taskfile in the named slots.
// Output mirrors the real CLI (internal/tui/styles.go, prompt_select.go,
// prompt_text.go, runner.displayArgv, app.execTask's done line). Trivia
// fetches live questions; the others play back scripted command output.
// Each script marks the step it is on by a line of text from that step's
// YAML, so a slot can be edited without renumbering anything here. The trivia
// slot must stay in step with internal/app/demo.yml.
import { computed, nextTick, ref } from 'vue'

type Seg = { t: string; c?: string }
type Line = Seg[]
type Prompt =
  | { kind: 'select'; varName: string; info: string; items: string[]; cursor: number; multi: boolean; picked: boolean[]; resolve: (v: string | string[]) => void }
  | { kind: 'text'; varName: string; info: string; secret: boolean; value: string; resolve: (v: string | string[]) => void }

type DemoId = 'trivia' | 'deploy' | 'flash'
const DEMOS: { id: DemoId; label: string; command: string; caption: string }[] = [
  { id: 'trivia', label: 'Trivia', command: 'hobnob --demo ask-trivia', caption: 'Live, in your browser: the questions come straight from the API.' },
  { id: 'deploy', label: 'Deploy', command: 'hobnob deploy', caption: 'Pick services, an environment and a token. The token never shows.' },
  { id: 'flash', label: 'Flash an image', command: 'hobnob build-and-flash', caption: 'One pipeline from two tasks. Every question comes first; say no and nothing is touched.' },
]

const active = ref<DemoId>('trivia')
const demo = computed(() => DEMOS.find((d) => d.id === active.value)!)
const lines = ref<Line[]>([])
const typed = ref('')
const prompt = ref<Prompt | null>(null)
const state = ref<'idle' | 'running' | 'done'>('idle')
const screen = ref<HTMLElement>()
const code = ref<HTMLElement>()
const textInput = ref<HTMLInputElement>()

// A switch of tab bumps the run id; anything still awaiting from the old run
// throws Cancelled at its next sleep or prompt and unwinds quietly.
let runId = 0
class Cancelled extends Error {}

// ── styling, after internal/tui/styles.go ───────────────────────────────────

const PALETTE = ['2', '3', '4', '5', '6', '10', '11', '12', '13', '14']
const taskColor = (task: string) => 'a' + PALETTE[[...task].reduce((s, ch) => s + ch.codePointAt(0)!, 0) % PALETTE.length]
const prefix = (task: string): Seg[] => [{ t: '[' }, { t: task, c: taskColor(task) }, { t: '] ' }]

const SHELL_METACHARS = ' \t\n"\'$`\\|&;<>()*?[]{}!#~'
const shellQuote = (s: string) => "'" + s.replaceAll("'", `'\\''`) + "'"
const displayArgv = (argv: string[]) =>
  argv.map((a) => (a === '' || [...a].some((ch) => SHELL_METACHARS.includes(ch)) ? shellQuote(a) : a)).join(' ')

function print(...segs: Seg[]) {
  lines.value.push(segs)
  scrollScreen()
}
const runLine = (task: string, cmd: string) => print({ t: 'run:', c: 'a11' }, { t: ' ' }, ...prefix(task), { t: cmd, c: 'a11' })
const skipLine = (task: string) => print({ t: 'run:', c: 'a11' }, { t: ' ' }, ...prefix(task), { t: 'skipped (if: false)', c: 'a8' })
const outLine = (task: string, text: string) => print(...prefix(task), { t: text })
const getLine = (task: string, name: string, val: string) =>
  print({ t: 'get:', c: 'a11' }, { t: ' ' }, ...prefix(task), { t: `${name}: ${val}`, c: 'a11' })
const cachedLine = (task: string, called: string, summary: string) =>
  print({ t: 'call:', c: 'a11' }, { t: ' ' }, ...prefix(task), { t: called, c: 'a11' }, { t: ` (cached — ${summary})`, c: 'a8' })
const doneLine = (task: string) => print({ t: '✓', c: 'a10' }, { t: ' ' }, ...prefix(task), { t: 'done', c: 'a10' })
const failLine = (task: string, msg: string) => print({ t: '✗', c: 'a9' }, { t: ' ' }, ...prefix(task), { t: msg, c: 'a9' })

// A \r-redrawn line (dd status=progress): replace the last line in place.
function redraw(task: string, text: string) {
  lines.value[lines.value.length - 1] = [...prefix(task), { t: text }]
}

function sleep(ms: number) {
  const id = runId
  return new Promise<void>((resolve, reject) =>
    setTimeout(() => (id === runId ? resolve() : reject(new Cancelled())), ms),
  )
}

// Stream command output a line at a time, the way a real command trickles in.
async function stream(task: string, out: string[], ms = 140) {
  for (const line of out) {
    await sleep(ms)
    outLine(task, line)
  }
}

function scrollScreen() {
  nextTick(() => {
    if (screen.value) screen.value.scrollTop = screen.value.scrollHeight
  })
}

// ── YAML step highlight ─────────────────────────────────────────────────────

// A step is the line containing `anchor` plus every line after it indented
// deeper than that line's "- ", up to the first blank line. `within` names
// the task to search in (its "name:" line), for steps that appear in several.
function highlight(anchor: string | null, within?: string) {
  const pane = code.value?.querySelector<HTMLElement>(`[data-demo="${active.value}"]`)
  if (!pane) return
  const rows = [...pane.querySelectorAll<HTMLElement>('pre code .line')]
  rows.forEach((row) => row.classList.remove('demo-active'))
  if (!anchor) return
  const from = within ? rows.findIndex((row) => row.textContent!.trim() === within) : 0
  const start = rows.findIndex(
    (row, i) => i >= from && !row.textContent!.trim().startsWith('#') && row.textContent!.includes(anchor),
  )
  if (start < 0) return
  const indent = (text: string) => text.length - text.trimStart().length
  const dash = indent(rows[start].textContent!)
  let end = start
  while (end + 1 < rows.length) {
    const text = rows[end + 1].textContent ?? ''
    if (!text.trim() || indent(text) <= dash) break
    end++
  }
  for (let i = start; i <= end; i++) rows[i].classList.add('demo-active')
  const pre = pane.querySelector('pre')
  if (pre) pre.scrollTo({ top: rows[start].offsetTop - pre.clientHeight / 3, behavior: 'smooth' })
}

// ── prompts, after prompt_select.go and prompt_text.go ──────────────────────

function ask(p: Omit<Prompt, 'resolve'>): Promise<string | string[]> {
  const id = runId
  return new Promise((resolve, reject) => {
    prompt.value = {
      ...p,
      resolve: (v) => (id === runId ? resolve(v) : reject(new Cancelled())),
    } as Prompt
    scrollScreen()
    nextTick(() => (p.kind === 'text' ? textInput.value : screen.value)?.focus({ preventScroll: true }))
  })
}

const select = (varName: string, info: string, items: string[]) =>
  ask({ kind: 'select', varName, info, items, cursor: 0, multi: false, picked: [] }) as Promise<string>
const multiSelect = (varName: string, info: string, items: string[]) =>
  ask({ kind: 'select', varName, info, items, cursor: 0, multi: true, picked: items.map(() => false) }) as Promise<string[]>
const secretText = (varName: string, info: string) =>
  ask({ kind: 'text', varName, info, secret: true, value: '' }) as Promise<string>

function submit() {
  const p = prompt.value
  if (!p) return
  prompt.value = null
  if (p.kind === 'text') p.resolve(p.value)
  else if (p.multi) p.resolve(p.items.filter((_, i) => p.picked[i]))
  else p.resolve(p.items[p.cursor])
}

function clickItem(i: number) {
  const p = prompt.value
  if (p?.kind !== 'select') return
  p.cursor = i
  if (p.multi) p.picked[i] = !p.picked[i]
  else submit()
}

function onKey(e: KeyboardEvent) {
  const p = prompt.value
  if (!p) {
    if (e.key === 'Enter' && state.value !== 'running') {
      e.preventDefault()
      start()
    }
    return
  }
  if (p.kind === 'text') return
  if (e.key === 'ArrowUp' || e.key === 'k') p.cursor = Math.max(0, p.cursor - 1)
  else if (e.key === 'ArrowDown' || e.key === 'j') p.cursor = Math.min(p.items.length - 1, p.cursor + 1)
  else if (e.key === ' ' && p.multi) p.picked[p.cursor] = !p.picked[p.cursor]
  else if (e.key === 'Enter') submit()
  else return
  e.preventDefault()
}

// ── the runs ────────────────────────────────────────────────────────────────

const TRIVIA_API = 'https://the-trivia-api.com/v2'

async function curl(task: string, url: string, display: string): Promise<string> {
  runLine(task, display)
  const res = await fetch(url)
  if (!res.ok) throw new Error(`exit status 22`)
  const body = (await res.text()).trim()
  outLine(task, body)
  return body
}

function shuffle<T>(items: T[]): T[] {
  const a = [...items]
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[a[i], a[j]] = [a[j], a[i]]
  }
  return a
}

async function trivia() {
  const task = 'ask-trivia'
  print({ t: 'Running the built-in demo. Write your own: https://hobnob.jakesmd.dev/docs/guide/', c: 'a8' })

  highlight('the-trivia-api.com/v2/categories')
  const categoriesUrl = `${TRIVIA_API}/categories`
  const categories: Record<string, string[]> = JSON.parse(await curl(task, categoriesUrl, `curl -fsS ${categoriesUrl}`))
  await sleep(400)

  highlight('- get:')
  const category = await select('CATEGORY', 'Which category?', Object.keys(categories).sort())
  getLine(task, 'CATEGORY', category)
  await sleep(300)

  highlight('questions?limit=1')
  const questionUrl = `${TRIVIA_API}/questions?limit=1&categories=${categories[category][0]}`
  const trivia = JSON.parse(await curl(task, questionUrl, `curl -fsS "${questionUrl}"`))[0]
  await sleep(500)

  highlight('sort -R')
  const correct: string = trivia.correctAnswer
  const answers = shuffle<string>([correct, ...trivia.incorrectAnswers])
  runLine(task, displayArgv(['sh', '-c', 'printf "%s\\n" "$@" | sort -R', 'answers', correct, ...trivia.incorrectAnswers]))
  await stream(task, answers, 60)
  await sleep(500)

  highlight('- GUESS:')
  const guess = await select('GUESS', trivia.question.text, answers)
  getLine(task, 'GUESS', guess)
  await sleep(300)

  highlight('echo "Correct!"')
  if (guess === correct.trim()) {
    runLine(task, 'echo "Correct!"')
    outLine(task, 'Correct!')
    await sleep(400)
    highlight('Not quite')
    skipLine(task)
  } else {
    skipLine(task)
    await sleep(400)
    highlight('Not quite')
    const msg = `Not quite. It was ${correct.trim()}.`
    runLine(task, `echo "${msg}"`)
    outLine(task, msg)
  }
  doneLine(task)
}

async function deploy() {
  const task = 'deploy'
  const services = ['api', 'web', 'worker']

  highlight('gh api')
  runLine(task, 'gh api repos/acme/platform/contents/services')
  await sleep(500)
  outLine(task, JSON.stringify(services.map((name) => ({ name, path: `services/${name}`, type: 'dir' }))))
  await sleep(400)

  highlight('- SERVICES:')
  const picked = await multiSelect('SERVICES', 'Which services?', services)
  getLine(task, 'SERVICES', picked.join(', '))
  highlight('- ENV:')
  const env = await select('ENV', 'Where to?', ['staging', 'production'])
  getLine(task, 'ENV', env)
  highlight('- TOKEN:')
  await secretText('TOKEN', 'Cluster token')
  getLine(task, 'TOKEN', '****')
  await sleep(300)

  for (const svc of picked) {
    highlight('docker build')
    const image = `ghcr.io/acme/${svc}:${env}`
    runLine(task, `docker build -t ${image} services/${svc}`)
    await stream(task, [
      '[+] Building 0.0s (0/12)',
      ' => [internal] load build definition from Dockerfile',
      ' => [build 1/5] FROM docker.io/library/golang:1.24-alpine',
      ' => [build 4/5] RUN go build -o /out/app ./cmd/' + svc,
      ' => [stage-1 2/2] COPY --from=build /out/app /app',
      ' => exporting to image',
      ` => => naming to ${image}`,
      `[+] Building ${(20 + Math.random() * 30).toFixed(1)}s (12/12) FINISHED`,
    ], 220)
    await sleep(300)
    highlight('rollout restart')
    runLine(task, `kubectl --token **** -n ${env} rollout restart deploy/${svc}`)
    await sleep(600)
    outLine(task, `deployment.apps/${svc} restarted`)
    await sleep(400)
  }
  doneLine(task)
}

async function flash() {
  const task = 'build-and-flash'
  const images = ['debian-12', 'raspios-lite', 'ubuntu-24.04']
  const disks = ['nvme0n1 953.9G Samsung SSD 980 PRO', 'sda      28.9G SanDisk Ultra']
  const step = (anchor: string, within?: string) => (highlight(anchor, within), sleep(350))

  // _pick-image has no once:, so each call lists images again; only its
  // prompt is skipped once IMAGE is in scope.
  const listImages = async (within: string) => {
    await step('call: _pick-image', within)
    highlight('ls images')
    runLine('_pick-image', 'ls images')
    await stream('_pick-image', images, 50)
    await sleep(250)
  }

  await listImages('build-and-flash:')
  highlight('- IMAGE:')
  const image = await select('IMAGE', 'Which image?', images)
  getLine('_pick-image', 'IMAGE', image)
  await sleep(250)

  await step('call: _pick-disk', 'build-and-flash:')
  highlight('lsblk -dno')
  runLine('_pick-disk', 'lsblk -dno NAME,SIZE,MODEL')
  await stream('_pick-disk', disks, 120)
  await sleep(300)
  // `| lines` trims each line but keeps the spacing inside it.
  highlight('- DISK:')
  const line = await select('DISK', 'Which disk?', disks)
  getLine('_pick-disk', 'DISK', line)
  highlight('- SURE:')
  const sure = await select('SURE', `Wipe ${line}? Everything on it goes.`, ['no', 'yes'])
  getLine('_pick-disk', 'SURE', sure)
  await sleep(250)

  // The guard: `[ "no" = yes ]` exits 1, and the failed call stops the run.
  highlight('= yes ]')
  runLine('_pick-disk', `[ "${sure}" = yes ]`)
  await sleep(400)
  if (sure !== 'yes') {
    failLine(task, 'call _pick-disk: exit status 1')
    return
  }
  await step('split')
  const disk = line.split(' ')[0]

  await step('call: build', 'build-and-flash:')
  await listImages('build:')
  highlight('packer build')
  const src = `qemu.${image}`
  runLine('build', `packer build images/${image}`)
  await stream('build', [
    `${src}: output will be in this color.`,
    '',
    `==> ${src}: Retrieving ISO`,
    `==> ${src}: Starting VM, booting from CD-ROM`,
    `==> ${src}: Waiting for SSH to become available...`,
    `==> ${src}: Connected to SSH!`,
    `==> ${src}: Provisioning with shell script: scripts/setup.sh`,
    `==> ${src}: Gracefully halting virtual machine...`,
    `==> ${src}: Converting hard drive to raw: output/${image}.img`,
    `Build '${src}' finished after 6 minutes 12 seconds.`,
    '',
    '==> Builds finished. The artifacts of successful builds are:',
    `--> ${src}: VM files in directory: output`,
  ], 280)
  await sleep(400)

  await step('call: flash', 'build-and-flash:')
  await listImages('flash:')
  // once: replays the first answer, so nothing is asked twice.
  highlight('call: _pick-disk', 'flash:')
  cachedLine('flash', '_pick-disk', `DISK=${disk} DISKS=${JSON.stringify(disks)} SURE=yes`)
  await sleep(500)

  highlight('sudo wipefs')
  runLine('flash', `sudo wipefs --all /dev/${disk}`)
  const end = disk === 'sda' ? '0x7363ffe00' : '0xee3d4c5e00'
  await stream('flash', [
    `/dev/${disk}: 8 bytes were erased at offset 0x00000200 (gpt): 45 46 49 20 50 41 52 54`,
    `/dev/${disk}: 8 bytes were erased at offset ${end} (gpt): 45 46 49 20 50 41 52 54`,
    `/dev/${disk}: 2 bytes were erased at offset 0x000001fe (PMBR): 55 aa`,
  ], 200)
  await sleep(400)

  highlight('sudo dd')
  runLine('flash', `sudo dd if=output/${image}.img of=/dev/${disk} bs=4M status=progress conv=fsync`)
  const total = 2147483648
  outLine('flash', '')
  for (let s = 1; s <= 7; s++) {
    await sleep(350)
    const bytes = Math.min(total, Math.round((total / 7.13) * s))
    redraw('flash', `${bytes} bytes (${(bytes / 1e9).toFixed(1)} GB, ${(bytes / 2 ** 30).toFixed(1)} GiB) copied, ${s} s, 301 MB/s`)
  }
  await sleep(350)
  redraw('flash', `${total} bytes (2.1 GB, 2.0 GiB) copied, 7.13 s, 301 MB/s`)
  await stream('flash', ['512+0 records in', '512+0 records out', `${total} bytes (2.1 GB, 2.0 GiB) copied, 7.13 s, 301 MB/s`], 200)
  doneLine(task)
}

const RUNS: Record<DemoId, { task: string; run: () => Promise<void> }> = {
  trivia: { task: 'ask-trivia', run: trivia },
  deploy: { task: 'deploy', run: deploy },
  flash: { task: 'build-and-flash', run: flash },
}

function reset() {
  runId++
  prompt.value = null
  lines.value = []
  typed.value = ''
  state.value = 'idle'
  highlight(null)
}

function pick(id: DemoId) {
  if (id === active.value) return
  reset()
  active.value = id
  nextTick(() => {
    const pre = code.value?.querySelector<HTMLElement>(`[data-demo="${id}"] pre`)
    pre?.scrollTo({ top: 0 })
  })
}

async function start() {
  if (state.value === 'running') return
  reset()
  state.value = 'running'
  const id = runId
  const { task, run } = RUNS[active.value]
  try {
    for (const ch of demo.value.command) {
      typed.value += ch
      await sleep(28)
    }
    await sleep(250)
    print({ t: '$ ', c: 'prompt' }, { t: demo.value.command })
    typed.value = ''
    await run()
  } catch (err) {
    if (err instanceof Cancelled || id !== runId) return
    prompt.value = null
    failLine(task, err instanceof Error ? err.message : String(err))
  }
  await sleep(600).catch(() => {})
  if (id !== runId) return
  highlight(null)
  state.value = 'done'
}
</script>

<template>
  <div class="try-demo">
    <div class="tabs" role="tablist">
      <button
        v-for="d in DEMOS"
        :key="d.id"
        role="tab"
        class="tab-btn"
        :class="{ on: d.id === active }"
        :aria-selected="d.id === active"
        @click="pick(d.id)"
      >
        {{ d.label }}
      </button>
    </div>

    <div class="window">
      <div class="bar">
        <span class="lights"><span class="dot r" /><span class="dot y" /><span class="dot g" /></span>
        <span class="tab">Terminal</span>
        <span class="tab file">hobnob.yml</span>
      </div>
      <div class="panes">
        <div ref="screen" class="screen" tabindex="0" @keydown="onKey" @click="screen?.focus({ preventScroll: true })">
          <div v-for="(line, i) in lines" :key="i" class="ln">
            <span v-for="(seg, j) in line" :key="j" :class="seg.c">{{ seg.t }}</span>
          </div>

          <template v-if="prompt?.kind === 'select'">
            <div class="ln">&nbsp;</div>
            <div class="ln">{{ prompt.multi ? 'Select values' : 'Select a value' }} for <span class="a14 b">{{ prompt.varName }}</span>.</div>
            <div v-if="prompt.info" class="ln a8">{{ prompt.info }}</div>
            <div
              v-for="(item, i) in prompt.items"
              :key="item"
              class="ln item"
              @mouseenter="prompt.cursor = i"
              @click.stop="clickItem(i)"
            ><span class="a14 b">{{ i === prompt.cursor ? '▶ ' : '  ' }}</span><template v-if="prompt.multi"><span v-if="prompt.picked[i]" class="a10">◉ </span><span v-else class="a8">○ </span></template>{{ item }}</div>
            <div v-if="prompt.multi" class="ln a8">{{ '↑↓ move  space toggle  ' }}<span class="hint-key" @click.stop="submit">enter confirm</span></div>
            <div v-else class="ln a8">{{ '↑↓ move  enter select' }}</div>
          </template>

          <template v-if="prompt?.kind === 'text'">
            <div class="ln">&nbsp;</div>
            <div class="ln">Enter a value for <span class="a14 b">{{ prompt.varName }}</span>.</div>
            <div v-if="prompt.info" class="ln a8">{{ prompt.info }}</div>
            <div class="ln field">
              <span class="a14 b">› </span>
              <input
                ref="textInput"
                v-model="prompt.value"
                class="text-input"
                :type="prompt.secret ? 'password' : 'text'"
                autocomplete="off"
                spellcheck="false"
                placeholder="anything will do"
                @keydown.enter.prevent="submit"
                @click.stop
              />
            </div>
            <div class="ln a8">enter ↵</div>
          </template>

          <div v-if="state !== 'running' || typed" class="ln">
            <span class="prompt">$ </span>{{ typed }}<span class="cursor" />
          </div>

          <button v-if="state !== 'running'" class="run" @click.stop="start">
            {{ state === 'idle' ? `▶ Run ${demo.command}` : '↻ Run it again' }}
          </button>
        </div>
        <div ref="code" class="code">
          <div v-for="d in DEMOS" v-show="d.id === active" :key="d.id" :data-demo="d.id" class="code-pane">
            <slot :name="d.id" />
          </div>
        </div>
      </div>
    </div>

    <p class="caption">{{ demo.caption }}</p>
  </div>
</template>

<style scoped>
.window {
  border-radius: 14px;
  border: 1px solid var(--vp-c-border);
  background: #0f131a;
  box-shadow: 0 40px 100px rgba(0, 0, 0, 0.5), inset 0 1px 0 rgba(255, 255, 255, 0.04);
  overflow: hidden;
}

.bar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  position: relative;
  height: 44px;
  background: #151a22;
  border-bottom: 1px solid var(--vp-c-divider);
}
.lights {
  position: absolute;
  left: 16px;
  top: 16px;
  display: flex;
  gap: 8px;
}
.dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}
.dot.r { background: #ff5f57; }
.dot.y { background: #febc2e; }
.dot.g { background: #28c840; }
.tab {
  align-self: center;
  justify-self: center;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.tab.file {
  height: 100%;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  border-left: 1px solid var(--vp-c-divider);
}

.panes {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  grid-template-rows: minmax(0, 1fr);
  height: 520px;
}

/* Grid items grow to fit their content unless told otherwise, which would
   stop both panes from scrolling and let the output spill past the window. */
.screen,
.code {
  min-height: 0;
}

.screen {
  overflow: auto;
  padding: 20px 24px 28px;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  line-height: 1.65;
  color: #e6e6e6;
  outline: none;
}
.ln {
  width: max-content;
  min-width: 100%;
  white-space: pre;
}
.item {
  cursor: pointer;
}
.prompt {
  color: var(--vp-c-brand-1);
}
.cursor {
  display: inline-block;
  width: 8px;
  height: 1.1em;
  margin-left: 1px;
  vertical-align: text-bottom;
  background: var(--vp-c-brand-1);
  animation: blink 1s steps(1) infinite;
}
@keyframes blink {
  50% { opacity: 0; }
}
.run {
  margin-top: 16px;
  padding: 6px 14px;
  border-radius: 20px;
  font-family: var(--vp-font-family-base);
  font-size: 13px;
  font-weight: 600;
  color: var(--vp-button-brand-text);
  background: var(--vp-button-brand-bg);
  transition: background 0.2s;
}
.run:hover {
  background: var(--vp-button-brand-hover-bg);
}

/* Demo switcher */
.tabs {
  display: flex;
  justify-content: center;
  gap: 4px;
  width: max-content;
  max-width: 100%;
  margin: 0 auto 20px;
  padding: 4px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 999px;
  background: var(--vp-c-bg-alt);
}
.tab-btn {
  padding: 6px 16px;
  border-radius: 999px;
  font-size: 14px;
  font-weight: 500;
  color: var(--vp-c-text-2);
  transition: color 0.2s, background 0.2s;
}
.tab-btn:hover {
  color: var(--vp-c-text-1);
}
.tab-btn.on {
  color: var(--vp-button-brand-text);
  background: var(--vp-button-brand-bg);
}

.caption {
  margin: 20px 0 0;
  text-align: center;
  font-size: 14px;
  color: var(--vp-c-text-3);
}

/* Prompt extras */
.field {
  display: flex;
}
.text-input {
  flex: 1;
  min-width: 0;
  padding: 0;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  color: inherit;
  caret-color: var(--vp-c-brand-1);
}
/* iOS Safari zooms the page into any focused field under 16px. */
@media (pointer: coarse) {
  .text-input {
    font-size: 16px;
  }
}
.text-input::placeholder {
  color: #4a5361;
}
/* Clicking the hint confirms a multi-select, for touch screens with no Enter. */
.hint-key {
  cursor: pointer;
  text-decoration: underline dotted;
  text-underline-offset: 3px;
}
.hint-key:hover {
  color: var(--vp-c-text-1);
}

/* ANSI colours, by the numbers internal/tui/styles.go uses. */
.b { font-weight: 700; }
.a2 { color: #4ec27a; }
.a3 { color: #d7b54a; }
.a4 { color: #5b8def; }
.a5 { color: #c678dd; }
.a6 { color: #46b5c6; }
.a8 { color: #7d8794; }
.a9 { color: #ff6b6b; }
.a10 { color: #7ee2a0; }
.a11 { color: #f5d76e; }
.a12 { color: #82aaff; }
.a13 { color: #e39ef7; }
.a14 { color: #6fe3f0; }

.code {
  min-width: 0;
  border-left: 1px solid var(--vp-c-divider);
  background: var(--vp-code-block-bg);
}
.code-pane {
  height: 100%;
}
.code :deep(div[class*='language-']) {
  height: 100%;
  margin: 0 !important;
  border-radius: 0 !important;
  background: transparent;
}
.code :deep(span.lang) {
  display: none;
}
.code :deep(pre) {
  height: 100%;
  overflow: auto;
  padding: 20px 0 28px;
}
.code :deep(.line) {
  display: inline-block;
  width: calc(100% + 48px);
  margin: 0 -24px;
  padding: 0 24px;
  transition: background 0.25s;
}
.code :deep(.line.demo-active) {
  background: var(--vp-c-brand-soft);
  box-shadow: inset 3px 0 0 var(--vp-c-brand-2);
}

@media (max-width: 860px) {
  .bar {
    grid-template-columns: minmax(0, 1fr);
  }
  .tab.file {
    display: none;
  }
  .panes {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: minmax(0, 440px) minmax(0, 320px);
    height: auto;
  }
  .code {
    border-left: none;
    border-top: 1px solid var(--vp-c-divider);
  }
}
</style>

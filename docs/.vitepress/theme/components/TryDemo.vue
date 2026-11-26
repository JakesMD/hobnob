<script setup lang="ts">
// Browser re-enactments of hobnob runs, one per taskfile in the named slots.
// Output mirrors the real CLI: the log lines of internal/tui/runlog.go, the
// status line of live.go, the card of frame.go with the menu and picker bodies
// of list.go and prompt_path.go. Trivia fetches live questions; the others
// play back scripted command output. Each script marks the step it is on by a
// line of text from that step's YAML, so a slot can be edited without
// renumbering anything here. The trivia slot must stay in step with
// internal/app/demo.yml.
//
// The Browser view runs the same scripts behind hobnob's own --web page,
// internal/web/page.html, in a frame. A shim stands in for its events stream
// and its button actions: the run's state goes in as the snapshots of
// internal/web/snapshot.go, and the page's answers come back as the actions
// of internal/web/server.go.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import PAGE_HTML from '../../../../internal/web/page.html?raw'

type Seg = { t: string; c?: string; click?: () => void }
type Line = Seg[]
type Resolve = (v: string | string[]) => void
type Prompt =
  | { kind: 'select'; task: string; varName: string; info: string; items: string[]; filter: string; cursor: number; offset: number; multi: boolean; picked: boolean[]; resolve: Resolve }
  | { kind: 'text'; task: string; varName: string; info: string; secret: boolean; value: string; resolve: Resolve }
  | (PathState & { kind: 'path'; resolve: Resolve })
  | { kind: 'say'; task: string; message: string; resolve: Resolve }

// A type: file picker over a made-up tree (FS), after prompt_path.go.
type PathState = {
  task: string
  varName: string
  info: string
  match: (name: string) => boolean
  text: string
  cursor: number
  offset: number
  errMsg: string
}
type PathRow = { name: string; isDir: boolean; positions: number[] }
type MenuRow = { index: number; positions: number[] }

// What a card shows: one line of its box, and the row it stands for when the
// line can be clicked.
type BodyRow = { segs: Seg[]; row?: number; input?: boolean }
type CardRow = { pre: Seg[]; post?: Seg[]; row?: number; input?: boolean }

type DemoId = 'trivia' | 'deploy' | 'flash'
type View = 'terminal' | 'browser'
const VIEWS: View[] = ['terminal', 'browser']
// Each task's info:, which the page shows while its steps run.
const INFO: Record<string, string> = {
  'ask-trivia': 'Answer a trivia question from a category you pick.',
  deploy: 'Build services and roll them out to an environment.',
  'build-and-flash': 'Build an OS image with Packer and flash it to a disk.',
  build: 'Build an OS image with Packer.',
  flash: 'Flash the built image to a disk.',
}
const DEMOS: { id: DemoId; label: string; command: string; caption: string }[] = [
  { id: 'trivia', label: 'Trivia', command: 'hobnob --demo ask-trivia', caption: 'Live, in your browser: the questions come straight from the API.' },
  { id: 'deploy', label: 'Deploy', command: 'hobnob deploy', caption: 'Pick services, an environment and a token. The token never shows.' },
  { id: 'flash', label: 'Flash an image', command: 'hobnob build-and-flash', caption: 'Pick a template, then a disk. Every question comes first; say no and nothing is touched.' },
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
const setInput = (el: unknown) => (textInput.value = (el as HTMLInputElement) ?? undefined)

// A switch of tab bumps the run id; anything still awaiting from the old run
// throws Cancelled at its next sleep or prompt and unwinds quietly.
let runId = 0
class Cancelled extends Error {}
// A step that failed, already logged, on its way up to the task ends.
class StepFailed extends Error {}
// The page's Cancel: the run stops at its next sleep, as after a CTRL+C.
class Interrupted extends Error {}
let interrupted = false

const view = ref<View>('terminal')
const frame = ref<HTMLIFrameElement>()
const frameKey = ref(0)

// What the page is sent, kept as the run goes: the tasks running, innermost
// last, the current step's last output line, and how the run ended.
const running = ref<string[]>([])
const lastLine = ref('')
const refusal = ref('')
const pickerOpen = ref('')
let promptId = 0
let failedAt = { label: '', lastLine: '' }
type Outcome = { outcome: 'done' | 'failed' | 'cancelled'; durationMs: number; label: string; lastLine: string }
const outcome = ref<Outcome | null>(null)
const label = (task: string) => INFO[task] || task


const GREY = 'a8'
const GREEN = 'a10'
const RED = 'a9'
const YELLOW = 'a11'

const PALETTE = ['2', '3', '4', '5', '6', '10', '11', '12', '13', '14']
const taskColor = (task: string) => 'a' + PALETTE[[...task].reduce((s, ch) => s + ch.codePointAt(0)!, 0) % PALETTE.length]

const SHELL_METACHARS = ' \t\n"\'$`\\|&;<>()*?[]{}!#~'
const shellQuote = (s: string) => "'" + s.replaceAll("'", `'\\''`) + "'"
const displayArgv = (argv: string[]) =>
  argv.map((a) => (a === '' || [...a].some((ch) => SHELL_METACHARS.includes(ch)) ? shellQuote(a) : a)).join(' ')

const formatElapsed = (ms: number) =>
  ms >= 60000 ? `${Math.floor(ms / 60000)}m${String(Math.floor(ms / 1000) % 60).padStart(2, '0')}s` : `${(ms / 1000).toFixed(1)}s`

// A log line starts with the task in its colour, then the mark that says what
// the line is.
const lead = (task: string, mark: Seg): Seg[] => [{ t: task, c: taskColor(task) }, { t: ' ' }, mark, { t: ' ' }]
const taskMark = (task: string, mark: string): Seg => ({ t: mark, c: taskColor(task) + ' b' })
const note = (text: string): Seg => ({ t: ` # ${text}`, c: GREY })

function print(...segs: Seg[]) {
  lines.value.push(segs)
  scrollScreen()
}
const hobnobNote = (text: string) => print({ t: 'hobnob', c: 'b' }, { t: ' ' }, { t: '»', c: GREY }, { t: ' ' }, { t: text, c: GREY })

function runLine(task: string, cmd: string) {
  const [first, ...rest] = cmd.split('\n')
  print(...lead(task, taskMark(task, '$')), { t: first })
  const indent = ' '.repeat(task.length + 3)
  for (const more of rest) print({ t: indent + more })
}
const callLine = (task: string, called: string) =>
  print(...lead(task, taskMark(task, '→')), { t: called, c: taskColor(called) })
const varLine = (task: string, name: string, val: string) =>
  print(...lead(task, taskMark(task, '»')), { t: `${name}=${val}` })
const doneEarlierLine = (task: string) =>
  print(...lead(task, { t: '✓', c: GREEN }), { t: 'done', c: GREEN }, { t: ' earlier', c: GREY })
const sayLine = (task: string, message: string) => print(...lead(task, taskMark(task, '»')), { t: message, c: 'b' })
const skipLine = (task: string, what: string) => print(...lead(task, { t: '⊘', c: GREY }), { t: what, c: GREY })

// The terminal's width: output wraps, and cards stop, at the same column. A
// run sets it from the pane, so a wide screen wraps less; a narrow one keeps
// 60 and scrolls sideways.
let termCols = 60
let cardW = termCols
let cardInner = cardW - 2 - 4

function fitColumns() {
  const el = screen.value
  if (!el) return
  const probe = document.createElement('span')
  probe.textContent = '0'.repeat(100)
  probe.style.position = 'absolute'
  el.appendChild(probe)
  const ch = probe.getBoundingClientRect().width / 100
  probe.remove()
  const css = getComputedStyle(el)
  const room = el.clientWidth - parseFloat(css.paddingLeft) - parseFloat(css.paddingRight)
  termCols = Math.min(110, Math.max(60, Math.floor(room / ch) - 1))
  cardW = termCols
  cardInner = cardW - 2 - 4
}

// Command output passes through a line at a time, wrapped under its own text
// past the prefix.
function outLine(task: string, text: string) {
  lastLine.value = text
  const room = Math.max(20, termCols - task.length - 3)
  const chunks: string[] = []
  for (let i = 0; i < Math.max(text.length, 1); i += room) chunks.push(text.slice(i, i + room))
  for (const chunk of chunks) print(...lead(task, { t: '│', c: GREY }), { t: chunk })
}

// A \r-redrawn line (dd status=progress): replace the last line in place.
function redraw(task: string, text: string) {
  lastLine.value = text
  lines.value[lines.value.length - 1] = [...lead(task, { t: '│', c: GREY }), { t: text }]
}

function sleep(ms: number) {
  const id = runId
  return new Promise<void>((resolve, reject) =>
    setTimeout(() => (id !== runId ? reject(new Cancelled()) : interrupted ? reject(new Interrupted()) : resolve()), ms),
  )
}

// Stream command output a line at a time, the way a real command trickles in.
async function stream(task: string, out: string[], ms = 140) {
  for (const line of out) {
    await sleep(ms)
    outLine(task, line)
  }
}

// A step: its command line, its body, then its own end line with the time.
async function step(task: string, cmd: string, body: () => Promise<void>) {
  runLine(task, cmd)
  const started = performance.now()
  stepStart.value = started
  lastLine.value = ''
  try {
    await body()
  } catch (err) {
    stepStart.value = null
    if (err instanceof Cancelled) throw err
    if (err instanceof Interrupted) {
      print(...lead(task, { t: '✗', c: RED }), { t: formatElapsed(performance.now() - started), c: GREY }, { t: ` ${cmd}: stopped` })
      throw err
    }
    const reason = err instanceof Error ? err.message : String(err)
    failedAt = { label: label(task), lastLine: lastLine.value || reason }
    print(...lead(task, { t: '✗', c: RED }), { t: formatElapsed(performance.now() - started), c: GREY }, { t: ` ${cmd}: ${reason}` })
    throw new StepFailed(reason)
  }
  stepStart.value = null
  print(...lead(task, { t: '✓', c: GREEN }), { t: formatElapsed(performance.now() - started), c: GREY })
}

// A task: its steps, then its own end line.
async function inTask(task: string, body: () => Promise<void>) {
  const started = performance.now()
  const id = runId
  running.value.push(task)
  try {
    await body()
  } catch (err) {
    if (err instanceof Cancelled) throw err
    const how = err instanceof Interrupted ? 'stopped' : 'failed'
    print(...lead(task, { t: '✗', c: RED }), { t: how, c: RED }, { t: ` after ${formatElapsed(performance.now() - started)}`, c: GREY })
    throw err
  } finally {
    if (id === runId) running.value.pop()
  }
  print(...lead(task, { t: '✓', c: GREEN }), { t: 'done', c: GREEN }, { t: ` after ${formatElapsed(performance.now() - started)}`, c: GREY })
}

async function call(caller: string, called: string, body: () => Promise<void>) {
  callLine(caller, called)
  await inTask(called, body)
}

function callDoneEarlier(caller: string, called: string) {
  callLine(caller, called)
  doneEarlierLine(called)
}

function scrollScreen() {
  nextTick(() => {
    if (screen.value) screen.value.scrollTop = screen.value.scrollHeight
  })
}


const runStart = ref<number | null>(null)
const stepStart = ref<number | null>(null)
const clock = ref(0)
let ticker: ReturnType<typeof setInterval> | undefined

const status = computed(() => {
  if (runStart.value === null || prompt.value) return ''
  const total = `total ${formatElapsed(clock.value - runStart.value)}`
  return stepStart.value === null ? total : `step ${formatElapsed(clock.value - stepStart.value)} · ${total}`
})

function startClock() {
  runStart.value = performance.now()
  clock.value = runStart.value
  ticker = setInterval(() => (clock.value = performance.now()), 100)
}

function stopClock() {
  clearInterval(ticker)
  runStart.value = null
  stepStart.value = null
}

onBeforeUnmount(stopClock)


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


const MENU_HEIGHT = 10
const PATH_HEIGHT = 12
const PATH_LIMIT = 40

const width = (segs: Seg[]) => segs.reduce((n, s) => n + [...s.t].length, 0)

function wrapText(text: string, room: number): string[] {
  const out: string[] = []
  for (const para of text.split('\n')) {
    let cur = ''
    for (const word of para.split(' ')) {
      if (cur && [...cur].length + 1 + [...word].length > room) {
        out.push(cur)
        cur = word
      } else cur = cur ? `${cur} ${word}` : word
    }
    out.push(cur)
  }
  return out
}

type Hint = [key: string, action: string, click?: () => void]

function hintSegs(hints: Hint[]): Seg[] {
  return hints.flatMap(([key, action, click], i) => [
    ...(i ? [{ t: ' • ', c: GREY }] : []),
    ...(key ? [{ t: key, c: 'a7 b', click }, { t: ' ' + action, c: GREY }] : [{ t: action, c: GREY }]),
  ])
}

// An empty varName is a say: card, titled by the task alone.
function card(p: { task: string; varName: string; info: string; body: BodyRow[]; err?: string; hints: Hint[] }): CardRow[] {
  const border = p.err ? RED : GREY
  const boxed = (segs: Seg[]): CardRow => ({
    pre: [{ t: '│  ', c: border }, ...segs, { t: ' '.repeat(Math.max(0, cardInner - width(segs))) + '  │', c: border }],
  })
  const blank = boxed([])
  const title: Seg[] = [{ t: p.task, c: taskColor(p.task) }]
  if (p.varName) title.push({ t: ' ' }, { t: '»', c: taskColor(p.task) }, { t: ' ' }, { t: p.varName, c: 'a14 b' })
  const fill = Math.max(1, cardW - width(title) - 5)
  const rows: CardRow[] = [
    { pre: [{ t: ' ' }] },
    { pre: [{ t: '╭─ ', c: border }, ...title, { t: ' ' }, { t: '─'.repeat(fill) + '╮', c: border }] },
    blank,
  ]
  if (p.info) {
    for (const line of wrapText(p.info, cardInner)) rows.push(boxed([{ t: line }]))
    rows.push(blank)
  }
  for (const b of p.body) {
    if (b.input) rows.push({ pre: [{ t: '│  ', c: border }, ...b.segs], post: [{ t: '  │', c: border }], input: true })
    else rows.push({ ...boxed(b.segs), row: b.row })
  }
  if (p.err) rows.push(blank, boxed([{ t: '✗ ' + p.err, c: RED }]))
  rows.push(blank, { pre: [{ t: '╰' + '─'.repeat(cardW - 2) + '╯', c: border }] })
  rows.push({ pre: [{ t: ' ' }, ...hintSegs(p.hints)] })
  return rows
}

const cursorMark = (on: boolean): Seg => ({ t: on ? '▶ ' : '  ', c: 'a14 b' })

// A name in segments, its fuzzy-matched letters picked out.
function highlightSegs(text: string, positions: number[], base?: string): Seg[] {
  const segs: Seg[] = []
  ;[...text].forEach((ch, i) => {
    const c = positions.includes(i) ? 'a11 b' : base
    const last = segs[segs.length - 1]
    if (last && last.c === c) last.t += ch
    else segs.push({ t: ch, c })
  })
  return segs
}

function fuzzyMatch(query: string, target: string): { score: number; positions: number[] } | null {
  if (!query) return { score: 0, positions: [] }
  const q = [...query.toLowerCase()]
  const t = [...target.toLowerCase()]
  const positions: number[] = []
  let qi = 0, last = -2, score = 0
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] !== q[qi]) continue
    positions.push(ti)
    if (ti === last + 1) score += 5
    if (ti === 0 || '_-.'.includes(t[ti - 1])) score += 3
    score++
    last = ti
    qi++
  }
  return qi < q.length ? null : { score: score * 100 - t.length, positions }
}

// The items a query keeps, best match first; with no query, in their order.
function matchRows(names: string[], query: string): (MenuRow & { score: number })[] {
  const hits = names.flatMap((name, index) => {
    const m = fuzzyMatch(query, name)
    return m ? [{ index, positions: m.positions, score: m.score }] : []
  })
  return query ? hits.sort((a, b) => b.score - a.score) : hits
}

const scrollTo = (cursor: number, offset: number, height: number) =>
  cursor < offset ? cursor : cursor >= offset + height ? cursor - height + 1 : offset

function filterBody(text: string): BodyRow[] {
  return [
    { segs: [{ t: '› ', c: 'a14 b' }, { t: text, c: 'b' }, { t: '▏', c: 'a11 b' }] },
    { segs: [{ t: '─'.repeat(cardInner), c: GREY }] },
  ]
}

// The rows [offset, offset+height) of total, framed by "↑ n more" and
// "↓ n more", or "nothing matches" when there are none.
function windowBody(total: number, offset: number, height: number, row: (i: number) => Seg[]): BodyRow[] {
  if (!total) return [{ segs: [{ t: '  nothing matches', c: GREY }] }]
  const end = Math.min(offset + height, total)
  const out: BodyRow[] = []
  if (offset > 0) out.push({ segs: [{ t: `  ↑ ${offset} more`, c: GREY }] })
  for (let i = offset; i < end; i++) out.push({ segs: row(i), row: i })
  if (end < total) out.push({ segs: [{ t: `  ↓ ${total - end} more`, c: GREY }] })
  return out
}


function ask(p: Omit<Prompt, 'resolve'>): Promise<string | string[]> {
  const id = runId
  promptId++
  refusal.value = ''
  if (p.kind === 'path') pickerOpen.value = dirKey(pathDirText(p))
  return new Promise((resolve, reject) => {
    prompt.value = {
      ...p,
      resolve: (v) => (id === runId ? resolve(v) : reject(new Cancelled())),
    } as Prompt
    scrollScreen()
    nextTick(() => (p.kind === 'text' ? textInput.value : screen.value)?.focus({ preventScroll: true }))
  })
}

const select = (task: string, varName: string, info: string, items: string[]) =>
  ask({ kind: 'select', task, varName, info, items, filter: '', cursor: 0, offset: 0, multi: false, picked: [] }) as Promise<string>
const multiSelect = (task: string, varName: string, info: string, items: string[]) =>
  ask({ kind: 'select', task, varName, info, items, filter: '', cursor: 0, offset: 0, multi: true, picked: items.map(() => false) }) as Promise<string[]>
const secretText = (task: string, varName: string, info: string) =>
  ask({ kind: 'text', task, varName, info, secret: true, value: '' }) as Promise<string>

const say = (task: string, message: string) => ask({ kind: 'say', task, message }) as Promise<string>

const menuRows = (p: Extract<Prompt, { kind: 'select' }>) => matchRows(p.items, p.filter)

function setFilter(p: Extract<Prompt, { kind: 'select' }>, filter: string) {
  p.filter = filter
  p.cursor = 0
  p.offset = 0
}

function moveMenu(p: Extract<Prompt, { kind: 'select' }>, to: number) {
  p.cursor = Math.max(0, Math.min(to, menuRows(p).length - 1))
  p.offset = scrollTo(p.cursor, p.offset, MENU_HEIGHT)
}

function submit() {
  const p = prompt.value
  if (!p || p.kind === 'path') return
  if (p.kind === 'say') {
    prompt.value = null
    return p.resolve('')
  }
  if (p.kind === 'text') {
    if (!p.value) return
    prompt.value = null
    return p.resolve(p.value)
  }
  if (p.multi) {
    const picks = p.items.filter((_, i) => p.picked[i])
    if (!picks.length) return
    prompt.value = null
    return p.resolve(picks)
  }
  const row = menuRows(p)[p.cursor]
  if (!row) return
  prompt.value = null
  p.resolve(p.items[row.index])
}

function clickRow(i: number) {
  const p = prompt.value
  if (p?.kind === 'path') return clickPathRow(i)
  if (p?.kind !== 'select') return
  p.cursor = i
  const row = menuRows(p)[i]
  if (p.multi && row) p.picked[row.index] = !p.picked[row.index]
  else submit()
}

function hoverRow(i: number) {
  const p = prompt.value
  if (p?.kind === 'select' || p?.kind === 'path') p.cursor = i
}

function selectCard(p: Extract<Prompt, { kind: 'select' }>): CardRow[] {
  const rows = menuRows(p)
  const body: BodyRow[] = [
    ...(p.filter ? filterBody(p.filter) : []),
    ...windowBody(rows.length, p.offset, MENU_HEIGHT, (i) => [
      cursorMark(i === p.cursor),
      ...(p.multi ? [p.picked[rows[i].index] ? { t: '◉ ', c: GREEN } : { t: '○ ', c: GREY }] : []),
      ...highlightSegs(p.items[rows[i].index], rows[i].positions),
    ]),
  ]
  const hints: Hint[] = [['', 'type to filter'], ['↑↓', 'move']]
  if (p.multi) hints.push(['tab', 'toggle'], ['enter', 'confirm', submit])
  else hints.push(['enter', 'select'])
  return card({ task: p.task, varName: p.varName, info: p.info, body, hints })
}

function textCard(p: Extract<Prompt, { kind: 'text' }>): CardRow[] {
  const body: BodyRow[] = [{ segs: [{ t: '› ', c: 'a14 b' }], input: true }]
  return card({ task: p.task, varName: p.varName, info: p.info, body, hints: [['enter', 'confirm', submit]] })
}

function sayCard(p: Extract<Prompt, { kind: 'say' }>): CardRow[] {
  const body: BodyRow[] = wrapText(p.message, cardInner).map((line) => ({ segs: [{ t: line }] }))
  return card({ task: p.task, varName: '', info: '', body, hints: [['enter', 'continue', submit]] })
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
  if (p.kind === 'say') {
    if (e.key === 'Enter') {
      e.preventDefault()
      submit()
    }
    return
  }
  // stopPropagation keeps a typed "/" from also opening the site search.
  const handled = p.kind === 'path' ? onPathKey(p, e) : onMenuKey(p, e)
  if (handled) {
    e.preventDefault()
    e.stopPropagation()
  }
}

function onMenuKey(p: Extract<Prompt, { kind: 'select' }>, e: KeyboardEvent): boolean {
  if (e.key === 'ArrowUp') moveMenu(p, p.cursor - 1)
  else if (e.key === 'ArrowDown') moveMenu(p, p.cursor + 1)
  else if (e.key === 'Backspace') p.filter && setFilter(p, [...p.filter].slice(0, -1).join(''))
  else if (e.key === 'Escape') setFilter(p, '')
  else if (e.key === 'Tab') {
    const row = menuRows(p)[p.cursor]
    if (!p.multi || !row) return true
    p.picked[row.index] = !p.picked[row.index]
  } else if (e.key === 'Enter') submit()
  else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
    if (e.key !== ' ' || p.filter) setFilter(p, p.filter + e.key)
  } else return false
  return true
}


const HOME = '/home/you'
const PROJECT = `${HOME}/os-images`
// Directories end in "/". A directory not listed here is empty.
const FS: Record<string, string[]> = {
  '/': ['home/'],
  '/home': ['you/'],
  [HOME]: ['Downloads/', 'os-images/', 'notes.txt'],
  [`${HOME}/Downloads`]: ['ubuntu-24.04-live-server-amd64.iso'],
  [PROJECT]: ['images/', 'output/', 'scripts/', 'README.md', 'hobnob.yml'],
  [`${PROJECT}/images`]: ['http/', 'debian-12.pkr.hcl', 'raspios-lite.pkr.hcl', 'ubuntu-24.04.pkr.hcl', 'README.md'],
  [`${PROJECT}/images/http`]: ['preseed.cfg', 'user-data'],
  [`${PROJECT}/scripts`]: ['setup.sh'],
}

const parentDir = (dir: string) => dir.slice(0, dir.lastIndexOf('/')) || '/'
const dirKey = (dirText: string) => (dirText === '/' ? '/' : dirText.replace(/\/$/, ''))

function dirExists(dir: string): boolean {
  if (dir === '/' || dir in FS) return true
  return FS[parentDir(dir)]?.includes(dir.slice(dir.lastIndexOf('/') + 1) + '/') ?? false
}

// The filter line is the only thing that decides which directory is listed:
// the text up to its last "/", and the rest is fuzzy-matched.
const pathDirText = (p: PathState) => p.text.slice(0, p.text.lastIndexOf('/') + 1)
const pathSeg = (p: PathState) => p.text.slice(p.text.lastIndexOf('/') + 1)

function pathEntries(p: PathState): { name: string; isDir: boolean }[] {
  const all = (FS[dirKey(pathDirText(p))] ?? []).map((e) => ({ name: e.replace(/\/$/, ''), isDir: e.endsWith('/') }))
  const byName = (a: { name: string }, b: { name: string }) => a.name.toLowerCase().localeCompare(b.name.toLowerCase())
  return [...all.filter((e) => e.isDir).sort(byName), ...all.filter((e) => !e.isDir && p.match(e.name)).sort(byName)]
}

function pathRows(p: PathState): PathRow[] {
  const entries = pathEntries(p)
  return matchRows(entries.map((e) => e.name), pathSeg(p)).map((hit) => ({ ...entries[hit.index], positions: hit.positions }))
}

function setPathText(p: PathState, text: string) {
  p.text = text
  p.cursor = 0
  p.offset = 0
}

function pathUp(p: PathState) {
  const dir = pathDirText(p)
  if (dir === '/') return
  const parent = dir.slice(0, dir.lastIndexOf('/', dir.length - 2) + 1)
  const left = dir.slice(parent.length, -1)
  setPathText(p, parent)
  const i = pathRows(p).findIndex((r) => r.name === left)
  if (i >= 0) movePath(p, i)
}

function movePath(p: PathState, to: number) {
  p.cursor = Math.max(0, Math.min(to, pathRows(p).length - 1))
  p.offset = scrollTo(p.cursor, p.offset, PATH_HEIGHT)
}

const shortPath = (text: string) => ([...text].length <= PATH_LIMIT ? text : '…' + [...text].slice(-(PATH_LIMIT - 1)).join(''))

function choosePathRow(p: PathState & { resolve: Resolve }, row: PathRow | undefined) {
  if (!row) p.errMsg = 'nothing matches'
  else if (row.isDir) p.errMsg = "that's a directory: → opens it"
  else {
    prompt.value = null
    p.resolve(pathDirText(p) + row.name)
  }
}

function clickPathRow(i: number) {
  const p = prompt.value
  if (p?.kind !== 'path') return
  p.errMsg = ''
  const row = pathRows(p)[i]
  if (row?.isDir) setPathText(p, pathDirText(p) + row.name + '/')
  else choosePathRow(p, row)
}

function clickPathUp() {
  const p = prompt.value
  if (p?.kind !== 'path') return
  p.errMsg = ''
  pathUp(p)
}

function pathCard(p: PathState): CardRow[] {
  const rows = pathRows(p)
  const body: BodyRow[] = filterBody(shortPath(p.text))
  if (dirExists(dirKey(pathDirText(p)))) {
    body.push(...windowBody(rows.length, p.offset, PATH_HEIGHT, (i) => [
      cursorMark(i === p.cursor),
      ...highlightSegs(rows[i].name, rows[i].positions, rows[i].isDir ? 'a12' : undefined),
      ...(rows[i].isDir ? [{ t: '/', c: 'a12' }] : []),
    ]))
  } else body.push({ segs: [{ t: `  no directory "${pathDirText(p)}"`, c: GREY }] })
  const hints: Hint[] = [['', 'type to filter'], ['↑↓', 'move'], ['←', 'up', clickPathUp], ['→', 'open'], ['tab', 'complete'], ['enter', 'pick']]
  return card({ task: p.task, varName: p.varName, info: p.info, body, err: p.errMsg, hints })
}

function onPathKey(p: PathState & { resolve: Resolve }, e: KeyboardEvent): boolean {
  p.errMsg = ''
  const rows = pathRows(p)
  const row = rows[p.cursor]
  if (e.key === 'ArrowUp') movePath(p, p.cursor - 1)
  else if (e.key === 'ArrowDown') movePath(p, p.cursor + 1)
  else if (e.key === 'ArrowLeft') pathUp(p)
  else if (e.key === 'ArrowRight') row?.isDir && setPathText(p, pathDirText(p) + row.name + '/')
  else if (e.key === 'Backspace') p.text !== '/' && setPathText(p, p.text.slice(0, -1))
  else if (e.key === 'Escape') pathSeg(p) && setPathText(p, pathDirText(p))
  else if (e.key === 'Tab') row && setPathText(p, pathDirText(p) + row.name)
  else if (e.key === 'Enter') choosePathRow(p, row)
  else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) setPathText(p, p.text + e.key)
  else return false
  return true
}

// A type: file picker opening in a directory default, the way open() does.
const pickFile = (task: string, varName: string, info: string, start: string, match: (name: string) => boolean) =>
  ask({ kind: 'path', task, varName, info, match, text: `${PROJECT}/${start}/`, cursor: 0, offset: 0, errMsg: '' }) as Promise<string>

const cardRows = computed<CardRow[]>(() => {
  const p = prompt.value
  if (p?.kind === 'select') return selectCard(p)
  if (p?.kind === 'text') return textCard(p)
  if (p?.kind === 'say') return sayCard(p)
  if (p?.kind === 'path') return pathCard(p)
  return []
})

watch(() => cardRows.value.length, scrollScreen)


const TRIVIA_API = 'https://the-trivia-api.com/v2'
const DEMO_NOTICE = 'Running the built-in demo. Write your own: https://jakesmd.github.io/hobnob/docs/guide/'

async function curl(task: string, url: string, display: string): Promise<string> {
  let body = ''
  await step(task, display, async () => {
    const id = runId
    const res = await fetch(url)
    if (id !== runId) throw new Cancelled()
    if (!res.ok) throw new Error('exit status 22')
    body = (await res.text()).trim()
    if (id !== runId) throw new Cancelled()
    outLine(task, body)
  })
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
  hobnobNote(DEMO_NOTICE)
  startClock()

  await inTask(task, async () => {
    highlight('the-trivia-api.com/v2/categories')
    const categoriesUrl = `${TRIVIA_API}/categories`
    const categories: Record<string, string[]> = JSON.parse(await curl(task, categoriesUrl, `curl -fsS ${categoriesUrl}`))
    await sleep(400)

    highlight('- get:')
    const category = await select(task, 'CATEGORY', 'Which trivia category?', Object.keys(categories).sort())
    varLine(task, 'CATEGORY', category)
    await sleep(300)

    highlight('questions?limit=1')
    const questionUrl = `${TRIVIA_API}/questions?limit=1&categories=${categories[category][0]}`
    const trivia = JSON.parse(await curl(task, questionUrl, `curl -fsS "${questionUrl}"`))[0]
    await sleep(500)

    highlight('sort -R')
    const correct: string = trivia.correctAnswer
    const answers = shuffle<string>([correct, ...trivia.incorrectAnswers])
    await step(task, displayArgv(['sh', '-c', 'printf "%s\\n" "$@" | sort -R', 'answers', correct, ...trivia.incorrectAnswers]), () =>
      stream(task, answers, 60),
    )
    await sleep(500)

    highlight('- GUESS:')
    const guess = await select(task, 'GUESS', trivia.question.text, answers)
    varLine(task, 'GUESS', guess)
    await sleep(300)

    const message = guess === correct.trim() ? 'Correct!' : `Not quite. It was ${correct.trim()}.`
    highlight(guess === correct.trim() ? 'say: Correct!' : 'Not quite')
    await say(task, message)
    sayLine(task, message)
  })
}

async function deploy() {
  const task = 'deploy'
  const services = ['api', 'web', 'worker']
  startClock()

  await inTask(task, async () => {
    highlight('gh api')
    await step(task, 'gh api repos/acme/platform/contents/services', async () => {
      await sleep(500)
      outLine(task, JSON.stringify(services.map((name) => ({ name, path: `services/${name}`, type: 'dir' }))))
    })
    await sleep(400)

    highlight('- SERVICES:')
    const picked = await multiSelect(task, 'SERVICES', 'Which services should be deployed?', services)
    varLine(task, 'SERVICES', picked.join(', '))
    highlight('- ENV:')
    const env = await select(task, 'ENV', 'Which environment should they go to?', ['staging', 'production'])
    varLine(task, 'ENV', env)
    highlight('- TOKEN:')
    await secretText(task, 'TOKEN', `Cluster token for ${env}`)
    varLine(task, 'TOKEN', '****')
    await sleep(300)

    for (const svc of picked) {
      highlight('docker build')
      const image = `ghcr.io/acme/${svc}:${env}`
      await step(task, `docker build -t ${image} services/${svc}`, () =>
        stream(task, [
          '[+] Building 0.0s (0/12)',
          ' => [internal] load build definition from Dockerfile',
          ' => [build 1/5] FROM docker.io/library/golang:1.24-alpine',
          ' => [build 4/5] RUN go build -o /out/app ./cmd/' + svc,
          ' => [stage-1 2/2] COPY --from=build /out/app /app',
          ' => exporting to image',
          ` => => naming to ${image}`,
          `[+] Building ${(20 + Math.random() * 30).toFixed(1)}s (12/12) FINISHED`,
        ], 220),
      )
      await sleep(300)
      highlight('rollout restart')
      await step(task, `kubectl --token **** -n ${env} rollout restart deploy/${svc}`, async () => {
        await sleep(600)
        outLine(task, `deployment.apps/${svc} restarted`)
      })
      await sleep(400)
    }
  })
}

async function flash() {
  const task = 'build-and-flash'
  const disks = ['nvme0n1 953.9G Samsung SSD 980 PRO', 'sda      28.9G SanDisk Ultra']
  const at = (anchor: string, within?: string) => (highlight(anchor, within), sleep(350))
  startClock()

  await inTask(task, async () => {
    await at('call: _pick-image', 'build-and-flash:')
    let image = ''
    await call(task, '_pick-image', async () => {
      highlight('- IMAGE:')
      image = await pickFile('_pick-image', 'IMAGE', 'Which Packer template should be built?', 'images', (name) => name.endsWith('.pkr.hcl'))
      varLine('_pick-image', 'IMAGE', image)
      await sleep(250)
    })

    await at('call: _pick-disk', 'build-and-flash:')
    let disk = ''
    await call(task, '_pick-disk', async () => {
      highlight('lsblk -dno')
      await step('_pick-disk', 'lsblk -dno NAME,SIZE,MODEL', () => stream('_pick-disk', disks, 120))
      await sleep(300)
      highlight('- DISK:')
      const line = await select('_pick-disk', 'DISK', 'Which disk should the image be flashed to?', disks)
      varLine('_pick-disk', 'DISK', line)
      highlight('- SURE:')
      const sure = await select('_pick-disk', 'SURE', `Wipe '${line}'? Everything on it goes.`, ['no', 'yes'])
      varLine('_pick-disk', 'SURE', sure)
      await sleep(250)

      highlight('= yes ]')
      await step('_pick-disk', `[ "${sure}" = yes ]`, async () => {
        await sleep(400)
        if (sure !== 'yes') throw new Error('exit status 1')
      })
      await at('split')
      disk = line.split(' ')[0]
    })

    await at('call: build', 'build-and-flash:')
    await call(task, 'build', async () => {
      highlight('call: _pick-image', 'build:')
      callDoneEarlier('build', '_pick-image')
      await sleep(400)
      highlight('packer build')
      const src = `qemu.${image.slice(image.lastIndexOf('/') + 1).replace(/\.pkr\.hcl$/, '')}`
      await step('build', `packer build -var out=output/os.img ${image}`, () =>
        stream('build', [
          `${src}: output will be in this color.`,
          '',
          `==> ${src}: Retrieving ISO`,
          `==> ${src}: Starting VM, booting from CD-ROM`,
          `==> ${src}: Waiting for SSH to become available...`,
          `==> ${src}: Connected to SSH!`,
          `==> ${src}: Provisioning with shell script: scripts/setup.sh`,
          `==> ${src}: Gracefully halting virtual machine...`,
          `==> ${src}: Converting hard drive to raw: output/os.img`,
          `Build '${src}' finished after 6 minutes 12 seconds.`,
          '',
          '==> Builds finished. The artifacts of successful builds are:',
          `--> ${src}: VM files in directory: output`,
        ], 280),
      )
      await sleep(400)
    })

    await at('call: flash', 'build-and-flash:')
    await call(task, 'flash', async () => {
      highlight('call: _pick-disk', 'flash:')
      callDoneEarlier('flash', '_pick-disk')
      await sleep(500)

      highlight('sudo wipefs')
      await step('flash', `sudo wipefs --all /dev/${disk}`, () => {
        const end = disk === 'sda' ? '0x7363ffe00' : '0xee3d4c5e00'
        return stream('flash', [
          `/dev/${disk}: 8 bytes were erased at offset 0x00000200 (gpt): 45 46 49 20 50 41 52 54`,
          `/dev/${disk}: 8 bytes were erased at offset ${end} (gpt): 45 46 49 20 50 41 52 54`,
          `/dev/${disk}: 2 bytes were erased at offset 0x000001fe (PMBR): 55 aa`,
        ], 200)
      })
      await sleep(400)

      highlight('sudo dd')
      await step('flash', `sudo dd if=output/os.img of=/dev/${disk} bs=4M status=progress conv=fsync`, async () => {
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
      })
    })
  })
}

const RUNS: Record<DemoId, () => Promise<void>> = { trivia, deploy, flash }


// The page as hobnob serves it, but talking to this component. The site is
// always dark, so the page's light theme goes. Focus never scrolls the
// homepage to the frame.
const SHIM = `<script>
(() => {
  const focus = HTMLElement.prototype.focus;
  HTMLElement.prototype.focus = function (options) { focus.call(this, { ...options, preventScroll: true }); };
  window.EventSource = class {
    constructor() {
      addEventListener('message', (e) => {
        if (e.source === parent && e.data && e.data.hobnobSnapshot) this.onmessage && this.onmessage({ data: e.data.hobnobSnapshot });
      });
      parent.postMessage({ hobnobReady: true }, '*');
    }
    close() {}
  };
  window.fetch = (url, init) => {
    parent.postMessage({ hobnobAction: String(url).split('?')[0].replace('/actions/', ''), body: init && init.body }, '*');
    return Promise.resolve(new Response(null, { status: 204 }));
  };
})();
<` + `/script>`
const PAGE = PAGE_HTML.replace('@media (prefers-color-scheme: light)', '@media not all').replace('<script>', SHIM + '<script>')

const webCommand = computed(() => demo.value.command.replace(/^hobnob /, 'hobnob --web '))
const ADDRESS = '127.0.0.1:52814/?token=3f9c…'

// A folder the picker lists, after ListDir in internal/tui/prompt_path.go.
function listDir(dir: string, match: (name: string) => boolean) {
  const all = (FS[dir] ?? []).map((e) => ({ name: e.replace(/\/$/, ''), dir: e.endsWith('/') }))
  const byName = (a: { name: string }, b: { name: string }) => a.name.toLowerCase().localeCompare(b.name.toLowerCase())
  return [
    ...all.filter((e) => e.dir).sort(byName).map((e) => ({ name: e.name, dir: true })),
    ...all.filter((e) => !e.dir && match(e.name)).sort(byName).map((e) => ({ name: e.name, pickable: true })),
  ]
}

// The columns from the home folder (or / when open isn't under it) down to
// open, after view in internal/web/picker.go.
function pickerView(open: string, match: (name: string) => boolean) {
  const root = open === HOME || open.startsWith(HOME + '/') ? HOME : '/'
  const chain = [open]
  for (let dir = open; dir !== root; ) {
    dir = parentDir(dir)
    chain.unshift(dir)
  }
  const columns = chain.map((dir, i) => {
    const next = chain[i + 1]
    return { path: dir, ...(next ? { next: next.slice(next.lastIndexOf('/') + 1) } : {}), entries: listDir(dir, match) }
  })
  return { type: 'file', open, columns, selected: [] }
}

function webPrompt(p: Prompt) {
  const base = { id: promptId, ...(refusal.value ? { refusal: refusal.value } : {}) }
  if (p.kind === 'select') return { ...base, kind: 'menu', varName: p.varName, info: p.info, options: p.items, multi: p.multi }
  if (p.kind === 'text') return { ...base, kind: 'text', varName: p.varName, info: p.info, secret: p.secret }
  if (p.kind === 'say') return { ...base, kind: 'say', varName: '', info: p.message }
  return { ...base, kind: 'path', varName: p.varName, info: p.info, picker: pickerView(pickerOpen.value, p.match) }
}

const runLog = () => lines.value.slice(1).map((line) => line.map((seg) => seg.t).join('')).join('\n')

// The snapshot, but for the clock, which goes in as it is sent. Null until
// the run's first task starts.
const snapshot = computed(() => {
  const task = running.value[0] ?? snapTask
  if (!task) return null
  const base = { task: { name: task, info: INFO[task] ?? '' }, rail: { answers: [], setup: [] } }
  const o = outcome.value
  if (o) return { ...base, screen: o.outcome === 'done' ? 'done' : 'failed', result: { ...o, log: runLog(), home: false } }
  if (prompt.value) return { ...base, screen: 'prompt', prompt: webPrompt(prompt.value) }
  return { ...base, screen: 'progress', progress: { label: label(running.value[running.value.length - 1] ?? task), lastLine: lastLine.value, elapsedMs: 0, cancelling: false } }
})
let snapTask = ''
watch(() => running.value[0], (task) => task && (snapTask = task))

function sendSnapshot() {
  const s = snapshot.value
  const win = frame.value?.contentWindow
  if (!s || !win) return
  if (s.progress && runStart.value !== null) s.progress.elapsedMs = Math.round(performance.now() - runStart.value)
  win.postMessage({ hobnobSnapshot: JSON.stringify(s) }, '*')
}
watch(() => JSON.stringify(snapshot.value), sendSnapshot)

// An answer goes through the rules of internal/web/prompt.go: a refusal
// keeps the prompt open with the reason under it.
function answer(value: unknown) {
  const p = prompt.value
  if (!p) return
  const take = (v: string | string[]) => {
    prompt.value = null
    p.resolve(v)
  }
  if (p.kind === 'say') return take('')
  if (p.kind === 'select' && p.multi) {
    const picks = Array.isArray(value) ? value.filter((v) => p.items.includes(v)) : []
    return picks.length ? take(picks) : (refusal.value = 'pick at least one option')
  }
  const text = typeof value === 'string' ? value : ''
  if (!text) return (refusal.value = 'an answer is required')
  if (p.kind === 'select') return p.items.includes(text) ? take(text) : (refusal.value = `${text} isn't one of the options`)
  if (p.kind === 'path') {
    const name = text.slice(text.lastIndexOf('/') + 1)
    return FS[parentDir(text)]?.includes(name) && p.match(name) ? take(text) : (refusal.value = `${text} doesn't match`)
  }
  take(text)
}

function onFrameMessage(e: MessageEvent) {
  if (!frame.value || e.source !== frame.value.contentWindow || !e.data) return
  if (e.data.hobnobReady) return sendSnapshot()
  let body: { id?: number; value?: unknown; path?: string } = {}
  try {
    body = JSON.parse(e.data.body || '{}')
  } catch {}
  const action = e.data.hobnobAction
  if (action === 'cancel') {
    if (state.value === 'running' && !prompt.value && !interrupted) {
      interrupted = true
      print(...lead(running.value[running.value.length - 1] ?? 'hobnob', { t: '!', c: YELLOW }), { t: 'stopping, ctrl+c again to force', c: YELLOW })
    }
    return
  }
  if (body.id !== promptId) return
  if (action === 'answer') answer(body.value)
  else if (action === 'open' && prompt.value?.kind === 'path' && body.path && dirExists(body.path)) pickerOpen.value = body.path
}

onMounted(() => window.addEventListener('message', onFrameMessage))
onBeforeUnmount(() => window.removeEventListener('message', onFrameMessage))

function setView(v: View) {
  if (v === view.value) return
  reset()
  view.value = v
}

function reset() {
  runId++
  stopClock()
  prompt.value = null
  lines.value = []
  typed.value = ''
  state.value = 'idle'
  highlight(null)
  interrupted = false
  running.value = []
  snapTask = ''
  lastLine.value = ''
  refusal.value = ''
  outcome.value = null
  failedAt = { label: '', lastLine: '' }
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
  fitColumns()
  state.value = 'running'
  const id = runId
  const web = view.value === 'browser'
  if (web) frameKey.value++
  let ended: Outcome['outcome'] = 'done'
  try {
    const command = web ? webCommand.value : demo.value.command
    if (!web) {
      for (const ch of command) {
        typed.value += ch
        await sleep(28)
      }
      await sleep(250)
    }
    print({ t: '$ ', c: 'prompt' }, { t: command })
    typed.value = ''
    if (web) hobnobNote(`Browser mode: http://${ADDRESS}`)
    await RUNS[active.value]()
  } catch (err) {
    if (err instanceof Cancelled || id !== runId) return
    prompt.value = null
    ended = err instanceof Interrupted ? 'cancelled' : 'failed'
    if (!(err instanceof StepFailed) && !(err instanceof Interrupted)) {
      const reason = err instanceof Error ? err.message : String(err)
      failedAt = { label: label(running.value[running.value.length - 1] ?? snapTask), lastLine: lastLine.value || reason }
      print({ t: 'hobnob', c: 'b' }, { t: ' ' }, { t: '✗', c: RED }, { t: ' ' }, { t: reason })
    }
  }
  const durationMs = runStart.value === null ? 0 : Math.round(performance.now() - runStart.value)
  outcome.value = { outcome: ended, durationMs, ...(ended === 'failed' ? failedAt : { label: '', lastLine: '' }) }
  stopClock()
  await sleep(600).catch(() => {})
  if (id !== runId) return
  highlight(null)
  state.value = 'done'
  scrollScreen()
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
        <span class="tab views" role="tablist" aria-label="Where it runs">
          <button v-for="v in VIEWS" :key="v" role="tab" :class="{ on: view === v }" :aria-selected="view === v" @click="setView(v)">
            {{ v === 'terminal' ? 'Terminal' : 'Browser' }}
          </button>
        </span>
        <span class="tab file">hobnob.yml</span>
      </div>
      <div class="panes">
        <div v-if="view === 'browser'" class="browser">
          <div class="address">
            <span class="url">{{ state === 'idle' ? '' : ADDRESS }}</span>
            <button v-if="state === 'done'" class="again" @click="start">↻ Run it again</button>
          </div>
          <iframe ref="frame" :key="frameKey" class="page" :srcdoc="PAGE" title="hobnob's browser page" allow="clipboard-write" />
          <div v-if="state === 'idle'" class="cover">
            <button class="run" @click="start">▶ Run {{ webCommand }}</button>
          </div>
        </div>
        <div v-show="view === 'terminal'" ref="screen" class="screen" tabindex="0" @keydown="onKey" @click="screen?.focus({ preventScroll: true })">
          <div v-for="(line, i) in lines" :key="i" class="ln">
            <span v-for="(seg, j) in line" :key="j" :class="seg.c">{{ seg.t }}</span>
          </div>

          <div
            v-for="(r, i) in cardRows"
            :key="'card' + i"
            class="ln"
            :class="{ item: r.row !== undefined, field: r.input }"
            @mouseenter="r.row !== undefined && hoverRow(r.row)"
            @click.stop="r.row !== undefined && clickRow(r.row)"
          >
            <span v-for="(seg, j) in r.pre" :key="'a' + j" :class="[seg.c, { 'hint-key': seg.click }]" @click="seg.click && ($event.stopPropagation(), seg.click())">{{ seg.t }}</span>
            <input
              v-if="r.input && prompt?.kind === 'text'"
              :ref="setInput"
              v-model="prompt.value"
              class="text-input"
              :style="{ '--cols': cardInner - 2 }"
              :type="prompt.secret ? 'password' : 'text'"
              autocomplete="off"
              spellcheck="false"
              placeholder="anything will do"
              @keydown.enter.prevent="submit"
              @click.stop
            />
            <span v-for="(seg, j) in r.post" :key="'b' + j" :class="seg.c">{{ seg.t }}</span>
          </div>
          <div v-if="status" class="status">
            <div class="ln">&nbsp;</div>
            <div class="rule">
              <span class="rule-fill">{{ '─'.repeat(300) }}</span>
              <span class="rule-text">{{ ` ${status} ──` }}</span>
            </div>
          </div>
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

/* Terminal or Browser, in the window's bar */
.views {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 8px;
  background: #0f131a;
}
.views button {
  padding: 2px 12px;
  border-radius: 6px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  color: var(--vp-c-text-3);
  transition: color 0.2s, background 0.2s;
}
.views button:hover {
  color: var(--vp-c-text-1);
}
.views button.on {
  color: var(--vp-c-text-1);
  background: #2c3542;
}

/* The browser view: an address bar over hobnob's page */
.browser {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: #1a2029;
}
.address {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: none;
  height: 40px;
  padding: 0 10px;
  background: #151a22;
  border-bottom: 1px solid var(--vp-c-divider);
}
.url {
  flex: 1;
  min-width: 0;
  height: 26px;
  padding: 0 12px;
  border-radius: 13px;
  background: #0f131a;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  line-height: 26px;
  color: var(--vp-c-text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.again {
  flex: none;
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-brand-1);
}
.page {
  flex: 1;
  min-height: 0;
  width: 100%;
  border: none;
  color-scheme: dark;
}
.cover {
  position: absolute;
  inset: 40px 0 0;
  display: grid;
  place-items: center;
  background: #1a2029;
}
.cover .run {
  margin: 0;
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

/* The status line: a grey rule under the log, pinned to the bottom of the
   screen while a run is on, with the clock on its right. */
.status {
  position: sticky;
  bottom: -28px;
  margin-bottom: -28px;
  padding-bottom: 28px;
  background: #0f131a;
  color: #7d8794;
}
.rule {
  display: flex;
  white-space: pre;
}
.rule-fill {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

/* Prompt extras */
.field {
  display: flex;
}
.text-input {
  flex: none;
  width: calc(var(--cols) * 1ch);
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
    /* --cols at the screen's 13px, so the row still ends at the border. */
    width: calc(var(--cols) * 1ch * 13 / 16);
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
.a7 { color: #d0d4da; }
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

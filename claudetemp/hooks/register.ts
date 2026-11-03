import type { EngineInterface, PluginOptions, Register } from 'claude-code'

// Shows a temperature sensor's reading in the status line, every 30 seconds
// (the `interval` setting).
//
// The reading comes from the claudetemp program (cmd/claudetemp in this
// repo), which finds an SHT41 or SHTC3 on the MCP2221 or FT232H adapter and
// prints one number as %0.1f in the unit asked for (F by default). With no
// sensor, nothing is shown. The latest reading is also written to a file as
// "unix-seconds valid-seconds text" (valid for three intervals), so the progress-pie plugin can keep it in the status
// line over the pie.
//
// The adapter can be open in one program at a time. While another (such as
// claudematrix) has it, claudetemp prints "busy" and the last reading stays.
//
// The `enabled` setting turns all of it off.

export const DEFAULT_INTERVAL_S = 30
export const MIN_INTERVAL_S = 5
export const MAX_INTERVAL_S = 3600

// The seconds between readings from the `interval` setting: a whole number
// from 5 to 3600, else 30.
export function intervalSeconds(options: PluginOptions): number {
  const n = Math.floor(Number(options.interval ?? DEFAULT_INTERVAL_S))
  if (!Number.isFinite(n)) return DEFAULT_INTERVAL_S
  return Math.min(MAX_INTERVAL_S, Math.max(MIN_INTERVAL_S, n))
}

let timer: ReturnType<typeof setTimeout> | undefined
let shown: string | undefined
let isReading = false
let lastError = ''

// Formats claudetemp's output (a number with one decimal) with its unit.
export function temperatureText(output: string, unit: string): string | undefined {
  const value = output.trim().split('\n').at(-1)?.trim() ?? ''
  if (!/^-?\d+\.\d$/.test(value)) return undefined
  return `${value}°${unit}`
}

function unitOf(options: PluginOptions): 'F' | 'C' {
  return String(options.unit ?? 'F').trim().toUpperCase() === 'C' ? 'C' : 'F'
}

// Logs to the debug log, skipping repeats.
function logOnce($: EngineInterface, line: string) {
  if (line === '' || line === lastError) return
  lastError = line
  $.ui.log(line, { to: 'debug' })
}

// A bare program name is looked for in Go's install folders (where `go
// install` puts it, often not on PATH), and otherwise left for PATH.
const resolved = new Map<string, string>()

async function resolveBinary($: EngineInterface, name: string): Promise<string> {
  if (name === '' || /[\\/]/.test(name)) return name
  const known = resolved.get(name)
  if (known !== undefined) return known
  const gobin = await $.env.get('GOBIN')
  const gopath = await $.env.get('GOPATH')
  const home = await $.env.get('HOME')
  for (const dir of [gobin, gopath && `${gopath}/bin`, home && `${home}/go/bin`]) {
    if (!dir) continue
    for (const file of [`${dir}/${name}`, `${dir}/${name}.exe`]) {
      if (await $.fs.exists(file)) {
        resolved.set(name, file)
        return file
      }
    }
  }
  return name
}

// A file setting left empty lives in ~/.claude.
async function resolveFile($: EngineInterface, path: string): Promise<string> {
  if (path !== '') return path
  const home = (await $.env.get('HOME')) ?? (await $.env.get('USERPROFILE')) ?? ''
  return `${home}/.claude/claudetemp`
}

async function publish($: EngineInterface, options: PluginOptions, text: string | undefined) {
  shown = text
  $.ui.status(text)
  const seconds = Math.floor((await $.clock.now()) / 1000)
  const file = await resolveFile($, String(options.temperatureFile))
  try {
    await $.fs.write(file, `${seconds} ${3 * intervalSeconds(options)} ${text ?? ''}\n`)
  } catch (err) {
    logOnce($, `claudetemp: couldn't write ${file}: ${String(err)}`)
  }
}

async function read($: EngineInterface, options: PluginOptions) {
  if (isReading) return
  isReading = true
  try {
    const unit = unitOf(options)
    let out = ''
    try {
      const binary = await resolveBinary($, String(options.binary))
      const child = $.process.spawn({ argv: [binary, '-unit', unit] })
      for await (const { stream, text } of child) {
        if (stream === 'stdout') out += text
      }
    } catch (err) {
      logOnce($, `claudetemp: couldn't run ${String(options.binary)}: ${String(err)}`)
    }
    // Busy: keep the last reading, and stamp it again so progress-pie still
    // counts it as current.
    const next = out.trim().split('\n').at(-1)?.trim() === 'busy' ? shown : temperatureText(out, unit)
    if (next !== undefined || shown !== undefined) await publish($, options, next)
  } finally {
    isReading = false
  }
}

// Bumped by every stop, so a reading that was still running when the timer
// was restarted doesn't start a second chain of timers.
let generation = 0

function stop() {
  generation++
  if (timer !== undefined) clearTimeout(timer)
  timer = undefined
}

// Stops the running timer and starts a new one, waiting the `interval`
// setting as it is now before each reading. The next reading is at once.
// A changed setting takes effect by the next reading at the latest.
function restart($: EngineInterface, options: PluginOptions) {
  stop()
  const mine = generation
  const tick = async () => {
    await read($, options)
    if (mine !== generation) return
    timer = setTimeout(() => void tick(), intervalSeconds(options) * 1000)
  }
  timer = setTimeout(() => void tick(), 0)
}

export const register: Register = (on, options) => {
  // Turned off, every hook passes straight through. Changing the setting
  // reloads this module, so session.start clears a status an earlier load
  // left up.
  const isOff = options.enabled === false || String(options.binary) === ''

  on('session.start', async ($, e, next) => {
    stop()
    if (isOff) {
      $.ui.status(undefined)
      return next(e)
    }
    restart($, options)
    return next(e)
  })

  on('session.end', async ($, e, next) => {
    if (isOff) return next(e)
    stop()
    if (shown !== undefined) await publish($, options, undefined)
    return next(e)
  })
}

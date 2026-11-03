import type { EngineInterface, PluginOptions, Register } from 'claude-code'

// Shows a temperature sensor's reading in the status line, every 30 seconds.
//
// The reading comes from the claudetemp program (cmd/claudetemp in this
// repo), which finds an SHT41 or SHTC3 on the MCP2221 or FT232H adapter and
// prints one number as %0.1f in the unit asked for (F by default). With no
// sensor, nothing is shown. The latest reading is also written to a file as
// "unix-seconds text", so the progress-pie plugin can keep it in the status
// line over the pie.
//
// The adapter can be open in one program at a time. While another (such as
// claudematrix) has it, claudetemp prints "busy" and the last reading stays.
//
// The `enabled` setting turns all of it off.

export const INTERVAL_MS = 30_000

let timer: ReturnType<typeof setInterval> | undefined
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
    await $.fs.write(file, `${seconds} ${text ?? ''}\n`)
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
    if (out.trim().split('\n').at(-1)?.trim() === 'busy') return // keep the last reading
    const next = temperatureText(out, unit)
    if (next !== shown) await publish($, options, next)
  } finally {
    isReading = false
  }
}

function stop() {
  if (timer !== undefined) clearInterval(timer)
  timer = undefined
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
    void read($, options)
    timer = setInterval(() => void read($, options), INTERVAL_MS)
    return next(e)
  })

  on('session.end', async ($, e, next) => {
    if (isOff) return next(e)
    stop()
    if (shown !== undefined) await publish($, options, undefined)
    return next(e)
  })
}

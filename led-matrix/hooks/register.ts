import type { EngineInterface, PluginOptions, Register, SessionRateLimit } from 'claude-code'

// Runs claudematrix (cmd/claudematrix in this repo) for as long as Claude is
// working on a prompt. claudematrix animates the LED matrix and blanks it
// when it is stopped, so stopping the child is all "clear" takes.
//
// While Claude works the matrix always shows the working light, never the
// progress-pie plugin's pie: that pie is in the status line already.
//
// When a usage limit is hit, the time it resets is written to a file as unix
// seconds ("0" once nothing is limited). Between turns this plugin runs
// claudematrix -countdown, which counts down to the reset and exits there.

type Animation = { kind: 'working' | 'countdown'; stop: () => void }

let current: Animation | undefined
let lastError = ''
let isTurnRunning = false
// When the hit limit resets, in milliseconds; 0 when none is hit.
let limitResetsAt = 0
// What was last written to the limit file, so unchanged readings aren't.
let lastWritten: string | undefined
// Bumped whenever the animation is stopped or replaced, so a spawn that was
// still looking for the program knows it has been overtaken.
let generation = 0

// A setting left at its default is worked out here, so the plugin has no
// path of one person's machine in it. A bare program name is looked for in
// Go's install folders (where `go install` puts it, often not on PATH), and
// otherwise left for the system to find on PATH.
const resolved = new Map<string, string>()

async function resolveBinary($: EngineInterface, name: string): Promise<string> {
  if (name === '' || /[\\/]/.test(name)) return name
  const known = resolved.get(name)
  if (known !== undefined) return known
  const gobin = await $.env.get('GOBIN')
  const gopath = await $.env.get('GOPATH')
  const home = await $.env.get('HOME')
  const dirs = [gobin, gopath && `${gopath}/bin`, home && `${home}/go/bin`]
  for (const dir of dirs) {
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
async function resolveFile($: EngineInterface, path: string, name: string): Promise<string> {
  if (path !== '') return path
  const home = (await $.env.get('HOME')) ?? (await $.env.get('USERPROFILE')) ?? ''
  return `${home}/.claude/${name}`
}

function stopAnimation() {
  generation++
  current?.stop()
  current = undefined
}

// Logs to the debug log, skipping repeats (claudematrix repeats the same
// error while it retries).
function logOnce($: EngineInterface, line: string) {
  if (line === lastError) return
  lastError = line
  $.ui.log(line, { to: 'debug' })
}

// When the limit that is hit resets: the latest reset among the windows at or
// past 100%, in milliseconds; 0 when none is.
export function limitReset(windows: readonly SessionRateLimit[], now: number): number {
  let latest = 0
  for (const w of windows) {
    if (w.percentUsed < 100 || w.resetsAt === undefined) continue
    const at = Date.parse(w.resetsAt)
    if (Number.isFinite(at) && at > now && at > latest) latest = at
  }
  return latest
}

async function spawn($: EngineInterface, options: PluginOptions, kind: Animation['kind'], flags: string[]) {
  stopAnimation()
  const mine = generation

  const binary = await resolveBinary($, String(options.binary))
  if (mine !== generation) return
  // -heartbeat makes claudematrix print a line every half second, so the
  // loop below wakes up regularly and can notice it has been asked to stop.
  const child = $.process.spawn({
    argv: [binary, '-brightness', String(options.brightness), '-addr', String(options.addr), '-heartbeat', '500ms', ...flags],
  })
  let isStopped = false
  const animation: Animation = {
    kind,
    stop: () => {
      if (isStopped) return
      isStopped = true
      // Ending the stream sends the child SIGTERM; it blanks the matrix.
      void child.return(undefined).catch(() => {})
    },
  }
  current = animation

  void (async () => {
    try {
      for await (const { stream, text } of child) {
        if (isStopped) break
        if (stream === 'stderr') logOnce($, text.trim())
      }
    } catch (err) {
      logOnce($, `led-matrix: couldn't run ${binary}: ${String(err)}`)
    } finally {
      if (current === animation) current = undefined
    }
  })()
}

// The working light: the spinner only, no pie and no countdown.
function startWorking($: EngineInterface, options: PluginOptions) {
  void spawn($, options, 'working', ['-progress', '', '-limit', ''])
}

// Between turns, while a limit is hit: the countdown, which exits at the reset.
async function updateCountdown($: EngineInterface, options: PluginOptions) {
  if (isTurnRunning) return
  const isLimited = limitResetsAt > (await $.clock.now())
  if (isLimited && current?.kind !== 'countdown') {
    void spawn($, options, 'countdown', ['-countdown', '-limit', await resolveFile($, String(options.limitFile), 'usage-limit')])
  } else if (!isLimited && current?.kind === 'countdown') {
    stopAnimation()
  }
}

async function writeLimit($: EngineInterface, options: PluginOptions) {
  const text = `${Math.floor(limitResetsAt / 1000)}\n`
  if (text === lastWritten) return
  const file = await resolveFile($, String(options.limitFile), 'usage-limit')
  try {
    await $.fs.write(file, text)
    lastWritten = text
  } catch (err) {
    logOnce($, `led-matrix: couldn't write ${file}: ${String(err)}`)
  }
}

export const register: Register = (on, options) => {
  // Main conversation only: a subagent's turns run inside the main turn.
  on('turn.start', ($, e, next) => {
    if (e.agentId === undefined) {
      isTurnRunning = true
      startWorking($, options)
    }
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    if (e.agentId === undefined) {
      isTurnRunning = false
      stopAnimation()
      await updateCountdown($, options)
    }
    return next(e)
  })

  on('session.measure', async ($, e, next) => {
    // No readings (off a subscription, or none yet): nothing to go on.
    if (e.changed.includes('rateLimits') && e.rateLimits.length > 0) {
      limitResetsAt = limitReset(e.rateLimits, await $.clock.now())
      await writeLimit($, options)
      await updateCountdown($, options)
    }
    return next(e)
  })

  on('session.end', ($, e, next) => {
    stopAnimation()
    return next(e)
  })
}

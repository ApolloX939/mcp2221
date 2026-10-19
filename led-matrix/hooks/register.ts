import type { EngineInterface, PluginOptions, Register } from 'claude-code'

// Runs claudematrix (cmd/claudematrix in this repo) for as long as Claude is
// working on a prompt. claudematrix animates the LED matrix and blanks it
// when it is stopped, so stopping the child is all "clear" takes.
//
// While Claude works the matrix always shows the working light, never the
// progress-pie plugin's pie: that pie is in the status line already.

type Animation = { stop: () => void }

let current: Animation | undefined
let lastError = ''

function stopAnimation() {
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

function startAnimation($: EngineInterface, options: PluginOptions) {
  stopAnimation()

  const binary = String(options.binary)
  // -heartbeat makes claudematrix print a line every half second, so the
  // loop below wakes up regularly and can notice it has been asked to stop.
  // -progress "" keeps it on the spinner instead of drawing a progress pie.
  const child = $.process.spawn({
    argv: [binary, '-brightness', String(options.brightness), '-heartbeat', '500ms', '-progress', ''],
  })
  let isStopped = false
  const animation: Animation = {
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

export const register: Register = (on, options) => {
  // Main conversation only: a subagent's turns run inside the main turn.
  on('turn.start', ($, e, next) => {
    if (e.agentId === undefined) startAnimation($, options)
    return next(e)
  })

  on('turn.complete', ($, e, next) => {
    if (e.agentId === undefined) stopAnimation()
    return next(e)
  })

  on('session.end', ($, e, next) => {
    stopAnimation()
    return next(e)
  })
}

import type { EngineInterface, PluginOptions, Register } from 'claude-code'

// Tracks a multi-part job's progress and shows it as a pie: in the status
// line, and on the LED matrix through claudematrix (cmd/claudematrix).
//
// Progress comes from the `progress` tool this plugin gives the model, and
// from the built-in task tools (TodoWrite, TaskCreate/TaskUpdate) when the
// model uses those. It is written to a file as "done total unix-seconds".
// While Claude works, the led-matrix plugin's claudematrix reads that file
// and draws the pie in place of its spinner. Between turns, while the job is
// unfinished (background work is running), this plugin runs claudematrix
// itself so the pie stays up.

const TOOL = 'progress'

type Job = { done: number; total: number; label: string }
type Display = { stop: () => void }

let job: Job | undefined
let isTurnRunning = false
let display: Display | undefined
let lastError = ''
// Task tool ids and their status, for TaskCreate/TaskUpdate.
const tasks = new Map<string, string>()

const PIE = ['○', '◔', '◑', '◕', '●']

export function pieGlyph(done: number, total: number): string {
  if (total <= 0) return PIE[0]
  return PIE[Math.round((4 * done) / total)]
}

function isUnfinished(j: Job | undefined): j is Job {
  return j !== undefined && j.done < j.total
}

// Shows the job in the status line and writes it for claudematrix.
async function publish($: EngineInterface, options: PluginOptions) {
  $.ui.status(job ? `${pieGlyph(job.done, job.total)} ${job.done}/${job.total}${job.label ? ' ' + job.label : ''}` : undefined)

  const seconds = Math.floor((await $.clock.now()) / 1000)
  const text = job ? `${job.done} ${job.total} ${seconds}\n` : `0 0 ${seconds}\n`
  try {
    await $.fs.write(String(options.progressFile), text)
  } catch (err) {
    logOnce($, `progress-pie: couldn't write ${String(options.progressFile)}: ${String(err)}`)
  }
  updateDisplay($, options)
}

// Between turns, keeps the pie on the matrix while the job is unfinished.
function updateDisplay($: EngineInterface, options: PluginOptions) {
  if (!isTurnRunning && isUnfinished(job) && String(options.binary) !== '') startDisplay($, options)
  else stopDisplay()
}

function setJob($: EngineInterface, options: PluginOptions, next: Job | undefined) {
  job = next
  return publish($, options)
}

// Logs to the debug log, skipping repeats.
function logOnce($: EngineInterface, line: string) {
  if (line === '' || line === lastError) return
  lastError = line
  $.ui.log(line, { to: 'debug' })
}

function stopDisplay() {
  display?.stop()
  display = undefined
}

function startDisplay($: EngineInterface, options: PluginOptions) {
  if (display) return // already up; it rereads the file every frame
  const binary = String(options.binary)
  // -heartbeat: a line every half second lets the loop below notice a stop.
  const child = $.process.spawn({
    argv: [
      binary, '-brightness', String(options.brightness), '-heartbeat', '500ms',
      '-progress', String(options.progressFile),
    ],
  })
  let isStopped = false
  const shown: Display = {
    stop: () => {
      if (isStopped) return
      isStopped = true
      // Ending the stream sends SIGTERM; claudematrix blanks the matrix.
      void child.return(undefined).catch(() => {})
    },
  }
  display = shown

  void (async () => {
    try {
      for await (const { stream, text } of child) {
        if (isStopped) break
        if (stream === 'stderr') logOnce($, text.trim())
      }
    } catch (err) {
      logOnce($, `progress-pie: couldn't run ${binary}: ${String(err)}`)
    } finally {
      if (display === shown) display = undefined
    }
  })()
}

function jobFromTasks(): Job | undefined {
  if (tasks.size < 2) return undefined
  const done = [...tasks.values()].filter(s => s === 'completed').length
  return { done, total: tasks.size, label: '' }
}

const GUIDANCE = `# Progress pie
When a request has several distinct parts (steps, files, checks, items), report progress with the \`mcp__progress-pie__${TOOL}\` tool so the person can see it as a pie: call it with done=0 and the number of parts when you start, again each time a part is finished, and with done=total when everything is done. Keep calling it while background work (background shells, agents) finishes parts between turns. If the plan changes, call it with the new total. If you abandon the job, call it with total=0 to clear the pie. Skip it for single-step requests. You don't need it when you are already tracking the work with a task list tool; the pie follows that list.`

export const register: Register = (on, options) => {
  on('session.start', async ($, e, next) => {
    await $.tool.register({
      name: TOOL,
      description:
        'Reports progress on a multi-part job; shown to the person as a pie (status line and LED matrix). ' +
        'done = parts finished, total = parts in all. done = total finishes the job; total = 0 clears it.',
      inputSchema: {
        type: 'object',
        properties: {
          done: { type: 'integer', minimum: 0, description: 'Parts finished so far.' },
          total: { type: 'integer', minimum: 0, description: 'Parts in all; 0 clears the pie.' },
          label: { type: 'string', description: 'Optional few words on the current part, e.g. "running tests".' },
        },
        required: ['done', 'total'],
      },
      isDeferred: false,
    })
    return next(e)
  })

  on('prompt.compose', async ($, e, next) => {
    const composed = await next(e)
    return {
      sections: [...composed.sections, { id: 'progress-pie:guidance', text: GUIDANCE, scope: 'session' as const }],
    }
  })

  on('tool.call', { tool: 'mcp__progress-pie__progress' }, async ($, e) => {
    const input = e as unknown as { done?: unknown; total?: unknown; label?: unknown }
    const total = Math.floor(Number(input.total))
    const done = Math.floor(Number(input.done))
    if (!Number.isFinite(total) || !Number.isFinite(done) || total < 0 || done < 0) {
      return { deny: 'done and total must be whole numbers, 0 or more.' }
    }
    if (total === 0) {
      await setJob($, options, undefined)
      return { result: 'Progress cleared.' }
    }
    const label = typeof input.label === 'string' ? input.label.trim().slice(0, 40) : ''
    await setJob($, options, { done: Math.min(done, total), total, label })
    return { result: `Progress: ${Math.min(done, total)} of ${total}.` }
  }).catch(() => ({ deny: 'progress-pie failed to record that.' }))

  on('tool.call', { tool: 'TodoWrite' }, async ($, e, next) => {
    const ran = await next(e)
    if (ran.deny === undefined && ran.isError !== true && e.todos.length >= 2) {
      const done = e.todos.filter(t => t.status === 'completed').length
      const current = e.todos.find(t => t.status === 'in_progress')
      await setJob($, options, { done, total: e.todos.length, label: current?.activeForm ?? '' })
    }
    return ran
  }).catch(($, e, next) => next(e)) // tracking failed: run the tool untouched

  on('tool.call', { tool: 'TaskCreate' }, async ($, e, next) => {
    const ran = await next(e)
    if (ran.deny === undefined && ran.isError !== true && ran.result?.task?.id) {
      tasks.set(ran.result.task.id, 'pending')
      const fromTasks = jobFromTasks()
      if (fromTasks) await setJob($, options, fromTasks)
    }
    return ran
  }).catch(($, e, next) => next(e)) // tracking failed: run the tool untouched

  on('tool.call', { tool: 'TaskUpdate' }, async ($, e, next) => {
    const ran = await next(e)
    if (ran.deny === undefined && ran.isError !== true && e.status !== undefined && tasks.has(e.taskId)) {
      if (e.status === 'deleted') tasks.delete(e.taskId)
      else tasks.set(e.taskId, e.status)
      const fromTasks = jobFromTasks()
      if (fromTasks) await setJob($, options, { ...fromTasks, label: e.status === 'in_progress' ? (e.activeForm ?? '') : '' })
    }
    return ran
  }).catch(($, e, next) => next(e)) // tracking failed: run the tool untouched

  // Main conversation only: a subagent's turns run inside the main turn.
  on('turn.start', ($, e, next) => {
    if (e.agentId === undefined) {
      isTurnRunning = true
      stopDisplay() // led-matrix's claudematrix takes over the matrix
    }
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    if (e.agentId === undefined) {
      isTurnRunning = false
      // Interrupted (Esc), or finished: clear the pie. Otherwise the job is
      // waiting on background work, and publish keeps the pie on the matrix.
      if (job && (e.reason === 'aborted' || !isUnfinished(job))) {
        tasks.clear()
        await setJob($, options, undefined)
      } else {
        updateDisplay($, options)
      }
    }
    return next(e)
  })

  on('session.end', async ($, e, next) => {
    stopDisplay()
    if (job) await setJob($, options, undefined)
    return next(e)
  })
}

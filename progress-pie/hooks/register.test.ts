import type { On, ProcessSpawnChunk, ProcessSpawnResult } from 'claude-code'
import { expect, test } from 'claude-code/testing'
import { pieGlyph } from './register'

// Fakes the engine's turn events, file writes, and claudematrix.
function fakes(on: On) {
  on('turn.start', (_$, e) => ({ turnId: e.turnId }))
  on('turn.complete', () => ({ text: '' }))

  on('clock.now', () => ({ value: 1_700_000_000_000 }))
  const statuses: (string | undefined)[] = []
  on('ui.status', (_$, e) => {
    statuses.push(e.text)
    return { value: undefined }
  })

  const writes: string[] = []
  on('fs.write', (_$, e) => {
    writes.push(e.text)
    return { value: undefined }
  })

  const started: (readonly string[])[] = []
  let stopped = 0
  const waiting = new Set<() => void>()
  on('process.spawn', async function* (_$, e): AsyncGenerator<ProcessSpawnChunk, ProcessSpawnResult> {
    started.push(e.argv)
    try {
      for (;;) {
        await new Promise<void>(resolve => waiting.add(resolve))
        yield { stream: 'stdout', text: 'working\n' }
      }
    } finally {
      stopped++
    }
  })
  return {
    statuses,
    writes,
    started,
    stopped: () => stopped,
    beat: () => {
      for (const resolve of [...waiting]) resolve()
      waiting.clear()
    },
  }
}

async function settle() {
  await new Promise(resolve => setTimeout(resolve, 50))
}

const complete = (turnId: string, reason: 'answer' | 'aborted' = 'answer') => ({
  answer: '', text: '', durationMs: 1, isAborted: reason === 'aborted', reason, turnId,
})

const progress = ($: any, done: number, total: number, label?: string) =>
  $.tool.call({ tool: 'mcp__progress-pie__progress', done, total, ...(label ? { label } : {}) })

test('pie glyphs go from empty to full', async () => {
  expect([pieGlyph(0, 4), pieGlyph(1, 4), pieGlyph(2, 4), pieGlyph(3, 4), pieGlyph(4, 4)]).toEqual(['○', '◔', '◑', '◕', '●'])
})

test('the progress tool writes done and total for claudematrix', async ($, on) => {
  const f = fakes(on)
  await $.turn.start({ text: 'go', turnId: 't1' })
  await progress($, 1, 4, 'building')
  await settle()
  expect(f.writes.at(-1)).toBe('1 4 1700000000\n')
  expect(f.statuses.at(-1)).toBe('◔ 1/4 building')
  expect(f.started.length).toBe(0) // during a turn led-matrix owns the matrix
})

test('between turns an unfinished job keeps the pie on the matrix', async ($, on) => {
  const f = fakes(on)
  await $.turn.start({ text: 'go', turnId: 't1' })
  await progress($, 2, 4)
  await $.turn.complete(complete('t1'))
  await settle()
  expect(f.started.length).toBe(1)
  expect(f.started[0]).toEqual([
    '/home/apollo/go/bin/claudematrix', '-brightness', '0.1', '-heartbeat', '500ms',
    '-progress', '/home/apollo/.claude/progress-pie',
  ])

  // Background work wakes Claude: the turn takes the matrix back.
  await $.turn.start({ text: '', turnId: 't2' })
  f.beat()
  await settle()
  expect(f.stopped()).toBe(1)
})

test('a finished job is cleared when the turn ends', async ($, on) => {
  const f = fakes(on)
  await $.turn.start({ text: 'go', turnId: 't1' })
  await progress($, 3, 3)
  await $.turn.complete(complete('t1'))
  await settle()
  expect(f.writes.at(-1)).toMatch(/^0 0 \d+\n$/)
  expect(f.started.length).toBe(0)
})

test('interrupting clears the pie', async ($, on) => {
  const f = fakes(on)
  await $.turn.start({ text: 'go', turnId: 't1' })
  await progress($, 1, 5)
  await $.turn.complete(complete('t1', 'aborted'))
  await settle()
  expect(f.writes.at(-1)).toMatch(/^0 0 \d+\n$/)
  expect(f.started.length).toBe(0)
})

test('a quiet turn leaves the progress file alone', async ($, on) => {
  const f = fakes(on)
  await $.turn.start({ text: 'hi', turnId: 't1' })
  await $.turn.complete(complete('t1'))
  await settle()
  expect(f.writes.length).toBe(0)
})

test('a todo list drives the pie', async ($, on) => {
  const f = fakes(on)
  on('tool.call', { tool: 'TodoWrite' }, () => ({ result: { oldTodos: [], newTodos: [] } }))
  await $.turn.start({ text: 'go', turnId: 't1' })
  await $.tool.call({
    tool: 'TodoWrite',
    todos: [
      { content: 'a', status: 'completed', activeForm: 'Doing a' },
      { content: 'b', status: 'in_progress', activeForm: 'Doing b' },
      { content: 'c', status: 'pending', activeForm: 'Doing c' },
    ],
  })
  await settle()
  expect(f.writes.at(-1)).toBe('1 3 1700000000\n')
  expect(f.statuses.at(-1)).toBe('◔ 1/3 Doing b')
})

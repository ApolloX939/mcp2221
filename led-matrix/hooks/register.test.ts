import type { On, ProcessSpawnChunk, ProcessSpawnResult } from 'claude-code'
import { expect, test } from 'claude-code/testing'
import { limitReset } from './register'

// A fake claudematrix: records each start and stop, and only sends its
// heartbeat line when the test calls beat(). Also stands in for the engine's
// own turn events.
function fakeMatrix(on: On) {
  on('turn.start', (_$, e) => ({ turnId: e.turnId }))
  on('turn.complete', () => ({ text: '' }))
  on('session.measure', (_$, e) => ({ changed: e.changed }))
  on('clock.now', () => ({ value: NOW }))
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
    writes,
    started,
    stopped: () => stopped,
    beat: () => {
      for (const resolve of [...waiting]) resolve()
      waiting.clear()
    },
  }
}

const NOW = 1_700_000_000_000
const HOUR = 3_600_000

// A rate-limit reading: the five-hour window at percent, resetting in hours.
const measure = (percent: number, hours = 2) => ({
  context: { window: 200_000 },
  rateLimits: [
    { kind: 'five_hour', percentUsed: percent, resetsAt: new Date(NOW + hours * HOUR).toISOString() },
    { kind: 'seven_day', percentUsed: 40, resetsAt: new Date(NOW + 90 * HOUR).toISOString() },
  ],
  changed: ['rateLimits' as const],
})

// The mod runs in an environment of its own; give messages time to cross.
async function settle() {
  await new Promise(resolve => setTimeout(resolve, 50))
}

const complete = (turnId: string, agentId?: string) => ({
  answer: '',
  text: '',
  durationMs: 1,
  isAborted: false,
  reason: 'answer' as const,
  turnId,
  ...(agentId === undefined ? {} : { agentId }),
})

test('starts the animation when a turn starts and stops it when the turn ends', async ($, on) => {
  const matrix = fakeMatrix(on)

  await $.turn.start({ text: 'hello', turnId: 't1' })
  await settle()
  expect(matrix.started.length).toBe(1)
  expect(matrix.started[0]).toEqual([
    '/home/apollo/go/bin/claudematrix', '-brightness', '0.1', '-addr', '0x70', '-heartbeat', '500ms', '-progress', '', '-limit', '',
  ])

  await $.turn.complete(complete('t1'))
  matrix.beat() // the next heartbeat lets the loop see the stop
  await settle()
  expect(matrix.stopped()).toBe(1)
})

test("ignores subagents' turns", async ($, on) => {
  const matrix = fakeMatrix(on)

  await $.turn.start({ text: 'main', turnId: 't1' })
  await settle()
  await $.turn.start({ text: 'sub', turnId: 't2', agentId: 'a1' })
  await $.turn.complete(complete('t2', 'a1'))
  matrix.beat()
  await settle()
  expect(matrix.started.length).toBe(1)
  expect(matrix.stopped()).toBe(0)
})

test('a new turn replaces a running animation instead of adding one', async ($, on) => {
  const matrix = fakeMatrix(on)

  await $.turn.start({ text: 'one', turnId: 't1' })
  await settle()
  await $.turn.start({ text: 'two', turnId: 't2' })
  matrix.beat()
  await settle()
  expect(matrix.started.length).toBe(2)
  expect(matrix.stopped()).toBe(1)
})

test('passes the configured program, brightness and address', { options: { binary: '/opt/cm', brightness: 0.5, addr: '0x71' } }, async ($, on) => {
  const matrix = fakeMatrix(on)

  await $.turn.start({ text: 'hello', turnId: 't1' })
  await settle()
  expect(matrix.started[0]).toEqual(['/opt/cm', '-brightness', '0.5', '-addr', '0x71', '-heartbeat', '500ms', '-progress', '', '-limit', ''])
})

test('the limit that is hit is the one whose reset counts', async () => {
  const windows = measure(100, 3).rateLimits
  expect(limitReset(windows, NOW)).toBe(NOW + 3 * HOUR)
  expect(limitReset(measure(99.9).rateLimits, NOW)).toBe(0)
  expect(limitReset(windows, NOW + 4 * HOUR)).toBe(0) // already reset
})

test('hitting the limit writes the reset and counts down between turns', async ($, on) => {
  const matrix = fakeMatrix(on)
  await $.turn.start({ text: 'go', turnId: 't1' })
  await $.turn.complete(complete('t1'))
  await $.session.measure(measure(100, 2))
  await settle()
  expect(matrix.writes.at(-1)).toBe(`${(NOW + 2 * HOUR) / 1000}\n`)
  expect(matrix.started.at(-1)).toEqual([
    '/home/apollo/go/bin/claudematrix', '-brightness', '0.1', '-addr', '0x70', '-heartbeat', '500ms',
    '-countdown', '-limit', '/home/apollo/.claude/usage-limit',
  ])

  // A new prompt takes the matrix back for the working light.
  await $.turn.start({ text: 'again', turnId: 't2' })
  matrix.beat()
  await settle()
  expect(matrix.started.length).toBe(3)
  expect(matrix.started.at(-1)).toContain('-progress')
})

test('a reading under the limit clears the file and the countdown', async ($, on) => {
  const matrix = fakeMatrix(on)
  await $.session.measure(measure(100))
  await settle()
  expect(matrix.started.length).toBe(1)
  await $.session.measure(measure(5))
  matrix.beat()
  await settle()
  expect(matrix.writes.at(-1)).toBe('0\n')
  expect(matrix.stopped()).toBe(1)
})

test('a reading during a turn waits for the turn to end', async ($, on) => {
  const matrix = fakeMatrix(on)
  await $.turn.start({ text: 'go', turnId: 't1' })
  await $.session.measure(measure(100))
  await settle()
  expect(matrix.started.length).toBe(1) // just the working light
  await $.turn.complete(complete('t1'))
  matrix.beat()
  await settle()
  expect(matrix.started.length).toBe(2)
  expect(matrix.started[1]).toContain('-countdown')
})

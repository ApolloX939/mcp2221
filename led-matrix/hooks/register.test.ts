import type { On, ProcessSpawnChunk, ProcessSpawnResult } from 'claude-code'
import { expect, test } from 'claude-code/testing'

// A fake claudematrix: records each start and stop, and only sends its
// heartbeat line when the test calls beat(). Also stands in for the engine's
// own turn events.
function fakeMatrix(on: On) {
  on('turn.start', (_$, e) => ({ turnId: e.turnId }))
  on('turn.complete', () => ({ text: '' }))

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
    started,
    stopped: () => stopped,
    beat: () => {
      for (const resolve of [...waiting]) resolve()
      waiting.clear()
    },
  }
}

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
    '/home/apollo/go/bin/claudematrix', '-brightness', '0.1', '-heartbeat', '500ms', '-progress', '',
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

test('passes the configured program and brightness', { options: { binary: '/opt/cm', brightness: 0.5 } }, async ($, on) => {
  const matrix = fakeMatrix(on)

  await $.turn.start({ text: 'hello', turnId: 't1' })
  await settle()
  expect(matrix.started[0]).toEqual(['/opt/cm', '-brightness', '0.5', '-heartbeat', '500ms', '-progress', ''])
})

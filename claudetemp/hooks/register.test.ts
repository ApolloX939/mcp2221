import type { On, ProcessSpawnChunk, ProcessSpawnResult } from 'claude-code'
import { expect, test } from 'claude-code/testing'
import { intervalSeconds, temperatureText } from './register'

function fakes(on: On) {
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  on('session.end', () => ({}))
  on('ui.log', () => ({ value: undefined }))
  on('clock.now', () => ({ value: 1_700_000_000_000 }))
  on('env.get', (_$, e) => ({ value: e.name === 'HOME' ? '/home/test' : undefined }))
  on('fs.exists', (_$, e) => ({ value: e.path === '/home/test/go/bin/claudetemp' }))
  const statuses: (string | undefined)[] = []
  on('ui.status', (_$, e) => {
    statuses.push(e.text)
    return { value: undefined }
  })
  const writes: string[] = []
  on('fs.write', (_$, e) => {
    writes.push(`${e.path}: ${e.text}`)
    return { value: undefined }
  })
  const argv: (readonly string[])[] = []
  const sensor = { output: undefined as string | undefined }
  on('process.spawn', async function* (_$, e): AsyncGenerator<ProcessSpawnChunk, ProcessSpawnResult> {
    argv.push(e.argv)
    if (sensor.output !== undefined) yield { stream: 'stdout', text: sensor.output }
    return { value: { exitCode: 0 } } as unknown as ProcessSpawnResult
  })
  return { statuses, writes, argv, sensor }
}

const start = ($: any) => $.session.start({ cwd: '/tmp', surface: 'terminal', isInteractive: true })
const settle = () => new Promise(resolve => setTimeout(resolve, 50))

test('temperature text is the reading with its unit', () => {
  expect(temperatureText('72.4\n', 'F')).toBe('72.4°F')
  expect(temperatureText('-3.0\n', 'C')).toBe('-3.0°C')
  expect(temperatureText('', 'F')).toBeUndefined()
  expect(temperatureText('busy\n', 'F')).toBeUndefined()
})

test('a sensor shows the temperature, in Fahrenheit by default', async ($, on) => {
  const f = fakes(on)
  f.sensor.output = '72.4\n'
  await start($)
  await settle()
  expect(f.argv[0]).toEqual(['/home/test/go/bin/claudetemp', '-unit', 'F'])
  expect(f.statuses.at(-1)).toBe('72.4°F')
  expect(f.writes.at(-1)).toBe('/home/test/.claude/claudetemp: 1700000000 90 72.4°F\n')
})

test('Celsius can be chosen', { options: { unit: 'C' } }, async ($, on) => {
  const f = fakes(on)
  f.sensor.output = '22.4\n'
  await start($)
  await settle()
  expect(f.argv[0].slice(1)).toEqual(['-unit', 'C'])
  expect(f.statuses.at(-1)).toBe('22.4°C')
})

test('without a sensor nothing is shown', async ($, on) => {
  const f = fakes(on)
  await start($)
  await settle()
  expect(f.statuses.length).toBe(0)
  expect(f.writes.length).toBe(0)
})

test('turned off, nothing is read or shown', { options: { enabled: false } }, async ($, on) => {
  const f = fakes(on)
  f.sensor.output = '72.4\n'
  await start($)
  await settle()
  expect(f.argv.length).toBe(0)
  expect(f.statuses.every(s => s === undefined)).toBe(true)
})

test('the interval is 30 seconds unless set between 5 and 3600', () => {
  expect(intervalSeconds({})).toBe(30)
  expect(intervalSeconds({ interval: 10 })).toBe(10)
  expect(intervalSeconds({ interval: '45' })).toBe(45)
  expect(intervalSeconds({ interval: 1 })).toBe(5)
  expect(intervalSeconds({ interval: 99999 })).toBe(3600)
  expect(intervalSeconds({ interval: 'soon' })).toBe(30)
})

test('the file says how long the reading is good for', { options: { interval: 60 } }, async ($, on) => {
  const f = fakes(on)
  f.sensor.output = '72.4\n'
  await start($)
  await settle()
  expect(f.writes.at(-1)).toBe('/home/test/.claude/claudetemp: 1700000000 180 72.4°F\n')
})

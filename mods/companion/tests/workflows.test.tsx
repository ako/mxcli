import { expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'

const BAND = {
  hasSurvey: false,
  isWorking: false,
  maxRows: 6,
  bodyColumns: 100,
  scroll: { offset: 0, bodyRows: 6 },
  view: {},
} as const

const START = { cwd: '/repo', surface: 'terminal', isInteractive: true } as const

const VIBE = { id: 'vibe', title: 'Vibe code', summary: 'Small changes, gates kept', command: '/workflow-vibe', skill: 'workflow-vibe', gates: [] }

const RUN = {
  id: '2026-10-13-vibe-add-a-status',
  title: 'Vibe code',
  intent: 'Add a status to orders',
  last: [
    { gate: 'security', status: 'pass' },
    { gate: 'tests', status: 'fail' },
    { gate: 'quality', status: 'warn' },
  ],
  blocked: ['tests'],
}

// A project already welcomed (so the band shows workflows, not the welcome),
// with an mxcli that answers `workflow list` and `workflow status`.
const world = (on: On, opts: { runs?: unknown[]; welcomed?: boolean } = {}) => {
  const filled: string[] = []
  const calls: string[][] = []
  let runs = opts.runs ?? []
  const clock = mock.clock(on)
  const store: Record<string, unknown> = opts.welcomed === false ? {} : { 'welcomed:/repo': true }
  on('store.get', (_$, e) => ({ value: store[e.key] }))
  on('store.set', (_$, e) => {
    store[e.key] = e.value
    return { value: undefined }
  })
  on('command.register', () => ({ value: { command: 'mxcli-project' } }))
  on('fs.exists', () => ({ value: false }))
  on('fs.list', () => ({ value: [{ name: 'App.mpr', kind: 'file', size: 1, mtimeMs: 0, isLink: false }] }))
  on('prompt.fill', (_$, e) => {
    filled.push(e.text)
    return { isFilled: true }
  })
  on('process.run', (_$, e) => {
    const argv = [...e.argv]
    calls.push(argv)
    const stdout =
      argv[1] === 'workflow' && argv[2] === 'list'
        ? JSON.stringify([VIBE])
        : argv[1] === 'workflow' && argv[2] === 'status'
          ? JSON.stringify(runs)
          : '[]'
    return { value: { exitCode: 0, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('tool.call', () => ({ result: { stdout: '', stderr: '', interrupted: false }, text: 'ok' }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  on('ui.render', ($, e) => {
    const { Box } = $.ui.resolve(e)
    return <Box key="engine-band" />
  })
  return { filled, calls, clock, setRuns: (r: unknown[]) => (runs = r) }
}

for (const surface of ['terminal', 'desktop'] as const) {
  test(`${surface}: the band offers the workflows mxcli lists, and a button drafts the command`, async ($, on) => {
    const { filled } = world(on)
    await $.session.start(START)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'AbovePrompt', props: BAND })
    expect(await ui.find({ key: 'workflow-vibe' })).toBeDefined()
    await ui.press({ key: 'workflow-vibe' })
    expect(filled).toEqual(['/workflow-vibe '])
    await ui.unmount()
  })
}

test('the welcome band offers the workflows too', async ($, on) => {
  world(on, { welcomed: false })
  await $.session.start(START)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'AbovePrompt', props: BAND })
  expect(await ui.find({ key: 'welcome-open' })).toBeDefined()
  expect(await ui.find({ key: 'workflow-vibe' })).toBeDefined()
  await ui.unmount()
})

test('an open run shows its intent and where its gates stand, instead of the offers', async ($, on) => {
  world(on, { runs: [RUN] })
  await $.session.start(START)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'AbovePrompt', props: BAND })
  expect(await ui.find({ text: /Add a status to orders/ })).toBeDefined()
  expect(await ui.find({ text: /✓ security {2}✗ tests {2}! quality/ })).toBeDefined()
  expect(await ui.find({ key: 'workflow-vibe' })).toBeUndefined()
  await ui.unmount()
})

test('the agent running mxcli workflow re-reads the runs', async ($, on) => {
  const { calls, clock, setRuns } = world(on)
  await $.session.start(START)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'AbovePrompt', props: BAND })
  expect(await ui.find({ text: /Add a status/ })).toBeUndefined()

  setRuns([RUN])
  await $.tool.call({ tool: 'Bash', command: './mxcli workflow start vibe "Add a status to orders" -p App.mpr' })
  await clock.settle()
  expect(calls.filter(c => c[2] === 'status').length).toBeGreaterThan(1)
  expect(await ui.find({ text: /Add a status to orders/ })).toBeDefined()
  await ui.unmount()
})

test('the offers can be hidden for the session', async ($, on) => {
  world(on)
  await $.session.start(START)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'AbovePrompt', props: BAND })
  await ui.press({ key: 'workflows-hide' })
  expect(await ui.find({ key: 'workflow-vibe' })).toBeUndefined()
  await ui.unmount()
})


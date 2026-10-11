import { expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'

const TREE = JSON.stringify([
  {
    label: 'Sales',
    type: 'module',
    qualifiedName: 'Sales',
    children: [
      { label: 'ACT_Process', type: 'microflow', qualifiedName: 'Sales.ACT_Process', describable: true },
      { label: 'Tmpl', type: 'pagetemplate', qualifiedName: 'Sales.Tmpl' },
    ],
  },
])

const MDL = 'mdl 1;\ncreate microflow Sales.ACT_Process ()\nbegin\nend;\n'

const PANE_PROPS = {
  title: 'Project',
  isFocused: true,
  bodyColumns: 80,
  placement: 'dock',
  scroll: { offset: 0, bodyRows: 40 },
  view: {},
} as const

// The world beneath the mod: one app here, and an mxcli that answers
// project-tree and describe. `calls` records every argv it was run with.
const world = (on: On, opts: { describeFails?: boolean; notPlaced?: string; tree?: string; mdl?: string } = {}) => {
  const calls: string[][] = []
  const clock = mock.clock(on)
  on('fs.exists', () => ({ value: false }))
  on('fs.list', () => ({ value: [{ name: 'App.mpr', kind: 'file', size: 1, mtimeMs: 0, isLink: false }] }))
  on('ui.open', () => ({
    value: opts.notPlaced === undefined ? { isPlaced: true } : { isPlaced: false, reason: opts.notPlaced },
  }))
  on('command.register', () => ({ value: { command: 'mxcli-project' } }))
  on('process.run', (_$, e) => ({ value: answer([...e.argv]) }))
  const answer = (argv: string[]) => {
    calls.push(argv)
    if (argv[1] === 'project-tree') {
      return { exitCode: 0, stdout: opts.tree ?? TREE, stderr: '', isStdoutTruncated: false, isStderrTruncated: false }
    }
    if (argv[1] === 'describe') {
      return opts.describeFails
        ? { exitCode: 1, stdout: '', stderr: 'Error: not found\n', isStdoutTruncated: false, isStderrTruncated: false }
        : {
            exitCode: 0,
            stdout: JSON.stringify({ name: 'Sales.ACT_Process', type: 'microflow', mdl: opts.mdl ?? MDL, executable: true }),
            stderr: '',
            isStdoutTruncated: false,
            isStderrTruncated: false,
          }
    }
    return { exitCode: 2, stdout: '', stderr: 'unexpected', isStdoutTruncated: false, isStderrTruncated: false }
  }
  return { calls, clock }
}

// The person typing /mxcli-project at the prompt.
const RUN = {
  command: 'mxcli-project',
  args: '',
  origin: { kind: 'composer' },
  presentation: { isFullscreen: true, columns: 160 },
} as const

const rowKey = (...path: string[]) => path.map(p => `/${p}`).join('')

for (const surface of ['terminal', 'desktop'] as const) {
  test(`${surface}: opens, expands a module and shows a document's MDL`, async ($, on) => {
    const { calls } = world(on)
    await $.command.run(RUN)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })

    expect(calls[0]).toEqual(['mxcli', 'project-tree', '-p', 'App.mpr'])
    expect(await ui.find({ text: /ACT_Process/ })).toBeUndefined()

    await ui.press({ key: rowKey('module:Sales') })
    expect(await ui.find({ text: /ACT_Process/ })).toBeDefined()

    await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
    expect(calls.at(-1)).toEqual(['mxcli', 'describe', '-p', 'App.mpr', 'microflow', 'Sales.ACT_Process', '--json'])
    const code = await ui.find({ type: 'Code' })
    expect(code).toBeDefined()
    expect(JSON.stringify(code)).toContain('create microflow Sales.ACT_Process')
    await ui.unmount()
  })

  test(`${surface}: a type without MDL is not described`, async ($, on) => {
    const { calls } = world(on)
    await $.command.run(RUN)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
    await ui.press({ key: rowKey('module:Sales') })
    await ui.press({ key: rowKey('module:Sales', 'pagetemplate:Tmpl') })
    expect(calls.some(c => c[1] === 'describe')).toBe(false)
    expect(await ui.find({ text: /has no MDL source/ })).toBeDefined()
    await ui.unmount()
  })

  test(`${surface}: a describe failure is shown, not a blank pane`, async ($, on) => {
    world(on, { describeFails: true })
    await $.command.run(RUN)
    const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface, component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
    await ui.press({ key: rowKey('module:Sales') })
    await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
    expect(await ui.find({ text: /not found/ })).toBeDefined()
    await ui.unmount()
  })
}

test('an agent write reloads the tree once; a read does not', async ($, on) => {
  const { calls, clock } = world(on)
  on('tool.call', () => ({ result: { stdout: '', stderr: '', interrupted: false }, text: 'ok' }))
  await $.command.run(RUN)
  await clock.settle()
  const trees = () => calls.filter(c => c[1] === 'project-tree').length
  expect(trees()).toBe(1)

  await $.tool.call({ tool: 'Bash', command: './mxcli -p App.mpr -c "show entities"' })
  await clock.advance(5_000)
  expect(trees()).toBe(1)

  await $.tool.call({ tool: 'Bash', command: './mxcli exec a.mdl -p App.mpr' })
  await $.tool.call({ tool: 'Bash', command: './mxcli exec b.mdl -p App.mpr' })
  await clock.advance(5_000)
  expect(trees()).toBe(2)
})

test('going back to a document reuses its MDL until the model changes', async ($, on) => {
  const { calls, clock } = world(on)
  on('tool.call', () => ({ result: { stdout: '', stderr: '', interrupted: false }, text: 'ok' }))
  await $.command.run(RUN)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
  const describes = () => calls.filter(c => c[1] === 'describe').length
  await ui.press({ key: rowKey('module:Sales') })
  await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
  await ui.press({ key: rowKey('module:Sales', 'pagetemplate:Tmpl') })
  await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
  expect(describes()).toBe(1)
  expect(await ui.find({ type: 'Code' })).toBeDefined()

  // The agent changes the model: the reload re-describes the selection afresh.
  await $.tool.call({ tool: 'Bash', command: './mxcli exec a.mdl -p App.mpr' })
  await clock.advance(5_000)
  expect(describes()).toBe(2)
  await ui.unmount()
})

test('says why when the surface does not show the pane', async ($, on) => {
  const { clock } = world(on, { notPlaced: 'the attached desktop places no panes' })
  const answer = await $.command.run(RUN)
  await clock.settle()
  expect(JSON.stringify(answer)).toContain('not shown here: the attached desktop places no panes')
})

// A docked pane wide enough for two columns, and short: 8 rows a page.
const WIDE = { ...PANE_PROPS, bodyColumns: 140, scroll: { offset: 0, bodyRows: 12 } } as const

test('a wide pane puts the tree and the source side by side, each paging on its own', async ($, on) => {
  const many = Array.from({ length: 30 }, (_, i) => ({
    label: `ACT_${String(i).padStart(2, '0')}`,
    type: 'microflow',
    qualifiedName: `Sales.ACT_${String(i).padStart(2, '0')}`,
    describable: true,
  }))
  const longMdl = Array.from({ length: 50 }, (_, i) => `-- line ${i + 1}`).join('\n')
  mock.clock(on)
  on('fs.exists', () => ({ value: false }))
  on('fs.list', () => ({ value: [{ name: 'App.mpr', kind: 'file', size: 1, mtimeMs: 0, isLink: false }] }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('process.run', (_$, e) => ({
    value: {
      exitCode: 0,
      stdout:
        e.argv[1] === 'project-tree'
          ? JSON.stringify([{ label: 'Sales', type: 'module', qualifiedName: 'Sales', children: many }])
          : JSON.stringify({ name: 'x', type: 'microflow', mdl: longMdl, executable: true }),
      stderr: '',
      isStdoutTruncated: false,
      isStderrTruncated: false,
    },
  }))
  await $.command.run(RUN)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'Pane', requestId: 'mxcli-project', props: WIDE })

  expect(await ui.find({ key: 'columns' })).toBeDefined()
  await ui.press({ key: rowKey('module:Sales') })
  // 31 rows, 8 a page: the tree shows a page and says what is below.
  const row = (n: string) => ui.find({ key: rowKey('module:Sales', `microflow:ACT_${n}`) })
  expect(await row('06')).toBeDefined()
  expect(await row('20')).toBeUndefined()
  await ui.press({ key: 'tree-down' })
  expect(await row('10')).toBeDefined()
  expect(await row('00')).toBeUndefined()
  expect(await ui.find({ key: 'tree-up' })).toBeDefined()

  // The source pages without moving the tree.
  await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_10') })
  expect(JSON.stringify(await ui.find({ type: 'Code' }))).toContain('line 1')
  await ui.press({ key: 'src-down' })
  const code = JSON.stringify(await ui.find({ type: 'Code' }))
  expect(code).not.toContain('-- line 1\\n')
  expect(code).toContain('line 10')
  expect(await row('10')).toBeDefined()
  await ui.unmount()
})

test('a narrow pane stacks the tree above the source', async ($, on) => {
  world(on)
  await $.command.run(RUN)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
  expect(await ui.find({ key: 'columns' })).toBeUndefined()
  expect(await ui.find({ key: 'tree' })).toBeDefined()
  await ui.unmount()
})

test('an mxcli older than the describable field still gets documents described', async ($, on) => {
  // project-tree as it printed before `describable` existed.
  const old = JSON.stringify([
    { label: 'Sales', type: 'module', qualifiedName: 'Sales', children: [{ label: 'ACT_Process', type: 'microflow', qualifiedName: 'Sales.ACT_Process' }] },
  ])
  const { calls } = world(on, { tree: old })
  await $.command.run(RUN)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
  await ui.press({ key: rowKey('module:Sales') })
  await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })
  expect(calls.some(c => c[1] === 'describe')).toBe(true)
  expect(await ui.find({ type: 'Code' })).toBeDefined()
  await ui.unmount()
})

test('annotation lines are dimmed outside the code, so the statements stand out', async ($, on) => {
  const mdl = 'mdl 1;\ncreate microflow Sales.ACT_Process ()\nbegin\n  @start(-21, 200)\n  @position(150, 200)\n  retrieve $L from Sales.Order;\n  @position(340, 200)\n  return;\nend;\n'
  world(on, { mdl })
  await $.command.run(RUN)
  const ui = await $.ui.mount({ plugin: 'mxcli-companion', surface: 'terminal', component: 'Pane', requestId: 'mxcli-project', props: PANE_PROPS })
  await ui.press({ key: rowKey('module:Sales') })
  await ui.press({ key: rowKey('module:Sales', 'microflow:ACT_Process') })

  const code = JSON.stringify(await ui.findAll({ type: 'Code' }))
  expect(code).toContain('retrieve $L from Sales.Order;')
  expect(code).not.toContain('@position')
  const note = await ui.find({ type: 'Text', text: /@position\(150, 200\)/ })
  expect(note).toBeDefined()
  expect(JSON.stringify(note)).toContain('"dimColor":true')
  expect(note?.text).toMatch(/^5 /)
  await ui.unmount()
})

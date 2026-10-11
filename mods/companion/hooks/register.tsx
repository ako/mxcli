// mxcli companion, slice 1: the Project view.
//
// A pane showing the app's structure from `mxcli project-tree`; selecting a
// document shows its MDL from `mxcli describe --json`. The mod only draws:
// every fact about the project comes from an mxcli command
// (docs/11-proposals/proposal-mxcli-companion-mod.md, P1).

import { atom, read, update } from 'claude-code'
import type { EngineInterface as Engine, FsEntry, Register } from 'claude-code'

import type { Project, Source, TreeNode } from '../types'
import {
  annotationRuns,
  assumeDescribable,
  changesModel,
  firstLine,
  nodeId,
  pageOf,
  parseDescribe,
  parseTree,
  pickMpr,
  touchesProjectFile,
  visibleRows,
} from './tree'

const PLUGIN = 'mxcli-companion'
const PANE = 'mxcli-project'

const project = atom({ plugin: 'mxcli-companion', key: 'project' } as const, { state: 'unlocated' } as Project)
const expanded = atom({ plugin: 'mxcli-companion', key: 'expanded' } as const, [] as string[])
const selected = atom({ plugin: 'mxcli-companion', key: 'selected' } as const, '')
const source = atom({ plugin: 'mxcli-companion', key: 'source' } as const, { state: 'none' } as Source)
// The first visible row of each column: they page on their own.
const treeTop = atom({ plugin: 'mxcli-companion', key: 'treeTop' } as const, 0)
const sourceTop = atom({ plugin: 'mxcli-companion', key: 'sourceTop' } as const, 0)
// Whether the band above the prompt offers the Project pane: once per project,
// until the person opens the pane or says not now.
const welcome = atom({ plugin: 'mxcli-companion', key: 'welcome' } as const, false)

// project-tree reads the whole model; on a large app that is the slow call.
const TREE_TIMEOUT_MS = 120_000
const DESCRIBE_TIMEOUT_MS = 60_000
// Below this many columns the tree and the source stack instead of sitting
// side by side.
const SIDE_BY_SIDE_MIN_COLUMNS = 90
// Rows the pane's own chrome takes (header, the gap, a pager line per column).
const CHROME_ROWS = 4
// A burst of agent writes reloads once, this long after the last one.
const RELOAD_DEBOUNCE_MS = 1_500

const basename = (path: string) => path.split('/').pop() ?? path

let mprOption = ''
let mxcliOption = ''
let reloadGeneration = 0
// The store key that remembers a project was welcomed; set at session start.
let welcomedKey = ''
// MDL already described since the tree last loaded, by qualified name. A
// describe re-reads the whole model, seconds on a large app, so going back to a
// document should not pay it again; a reload (the model changed) clears it.
const described = new Map<string, string>()


// The mxcli to run: the option, else ./mxcli (where the bootstrap script puts
// it), else whatever is on PATH.
const mxcli = async ($: Engine): Promise<string> => {
  if (mxcliOption !== '') return mxcliOption
  return (await $.fs.exists('mxcli')) ? './mxcli' : 'mxcli'
}

const run = async ($: Engine, args: string[], timeoutMs: number) =>
  $.process.run([await mxcli($), ...args], { env: { MXCLI_QUIET: '1' }, timeoutMs })

const mprFiles = async ($: Engine, dir: string): Promise<string[]> => {
  const entries = await $.fs.list(dir).catch((): FsEntry[] => [])
  return entries
    .filter(e => e.kind === 'file' && e.name.endsWith('.mpr'))
    .map(e => (dir === '.' ? e.name : `${dir}/${e.name}`))
}

const locate = async ($: Engine): Promise<Project> => {
  const here = await mprFiles($, '.')
  let below: string[] = []
  if (mprOption === '' && here.length === 0) {
    const dirs = (await $.fs.list('.').catch((): FsEntry[] => [])).filter(
      e => e.kind === 'dir' && !e.name.startsWith('.') && e.name !== 'node_modules',
    )
    for (const d of dirs) below = below.concat(await mprFiles($, d.name))
  }
  const picked = pickMpr(mprOption, here, below)
  if ('mpr' in picked) return { state: 'loading', mpr: picked.mpr }
  if ('candidates' in picked) return { state: 'choose', candidates: picked.candidates }
  return { state: 'none' }
}

const describeSelected = async ($: Engine, node: TreeNode) => {
  const current = await read($, project)
  if (current.state !== 'loaded' || !node.qualifiedName) return
  const qn = node.qualifiedName
  const cached = described.get(qn)
  if (cached !== undefined) {
    await update($, source, (): Source => ({ state: 'shown', label: node.label, qn, mdl: cached }))
    return
  }
  await update($, source, (): Source => ({ state: 'loading', label: node.label, qn }))
  try {
    const out = await run($, ['describe', '-p', current.mpr, node.type, qn, '--json'], DESCRIBE_TIMEOUT_MS)
    let next: Source
    if (out.exitCode === 0) {
      const mdl = parseDescribe(out.stdout)
      described.set(qn, mdl)
      next = { state: 'shown', label: node.label, qn, mdl }
    } else {
      next = { state: 'failed', label: node.label, qn, error: firstLine(out.stderr || out.stdout) }
    }
    // Only land the answer if the person has not moved on meanwhile.
    await update($, source, (s): Source => (s.state === 'loading' && s.qn === qn ? next : s))
  } catch (err) {
    await update($, source, (s): Source =>
      s.state === 'loading' && s.qn === qn
        ? { state: 'failed', label: node.label, qn, error: String(err) }
        : s,
    )
  }
}

const findNode = (nodes: readonly TreeNode[], id: string, parent = ''): TreeNode | undefined => {
  for (const node of nodes) {
    const nid = nodeId(parent, node)
    if (nid === id) return node
    if (id.startsWith(`${nid}/`) && node.children) {
      const hit = findNode(node.children, id, nid)
      if (hit) return hit
    }
  }
  return undefined
}

const load = async ($: Engine, mpr?: string) => {
  const target = mpr ? ({ state: 'loading', mpr } as Project) : await locate($)
  await update($, project, () => target)
  if (target.state !== 'loading') return
  const startedMs = await $.clock.now()
  try {
    const out = await run($, ['project-tree', '-p', target.mpr], TREE_TIMEOUT_MS)
    if (out.exitCode !== 0) {
      await update($, project, (): Project => ({ state: 'failed', mpr: target.mpr, error: firstLine(out.stderr || out.stdout) }))
      return
    }
    const tree = assumeDescribable(parseTree(out.stdout))
    described.clear()
    const loadedMs = await $.clock.now()
    await update($, project, (): Project => ({ state: 'loaded', mpr: target.mpr, tree, loadedMs, tookMs: loadedMs - startedMs }))
    // A reload keeps the selection; refresh its source, which may have changed.
    const sel = await read($, selected)
    const node = sel === '' ? undefined : findNode(tree, sel)
    if (node?.describable) await describeSelected($, node)
  } catch (err) {
    await update($, project, (): Project => ({ state: 'failed', mpr: target.mpr, error: String(err) }))
  }
}

const scheduleReload = async ($: Engine) => {
  const current = await read($, project)
  if (current.state !== 'loaded') return // never opened, or already busy
  const mine = ++reloadGeneration
  await $.clock.sleep(RELOAD_DEBOUNCE_MS)
  if (mine === reloadGeneration) await load($, current.mpr)
}

const press = async ($: Engine, id: string) => {
  const current = await read($, project)
  if (current.state !== 'loaded') return
  const node = findNode(current.tree, id)
  if (!node) return
  if (node.children && node.children.length > 0) {
    await update($, expanded, list => (list.includes(id) ? list.filter(x => x !== id) : [...list, id]))
  }
  if ((await read($, selected)) !== id) await update($, sourceTop, () => 0)
  await update($, selected, () => id)
  if (node.describable) {
    await describeSelected($, node)
  } else {
    await update($, source, (): Source => ({
      state: 'container',
      label: node.label,
      type: node.type,
      children: (node.children ?? []).map(c => c.label),
    }))
  }
}

// background runs work nobody awaits. Its failures have nowhere to go (the
// pane shows load errors itself), and one landing after an unload must not
// surface as an unhandled rejection.
const background = (work: Promise<unknown>) => {
  work.catch(() => undefined)
}

// The store outlives sessions and spans projects, so the key names the project.
const startWelcome = async ($: Engine, cwd: string) => {
  welcomedKey = `welcomed:${cwd}`
  if ((await $.store.get(welcomedKey)) !== true) await update($, welcome, () => true)
}

const retireWelcome = async ($: Engine) => {
  if (!(await read($, welcome))) return
  await update($, welcome, () => false)
  if (welcomedKey !== '') await $.store.set(welcomedKey, true)
}

// open seats the pane and starts loading; it answers what the person should
// read, which is not always "opened": a session whose attached surfaces place
// no panes (an older desktop, some remote surfaces) keeps it waiting undrawn.
const open = async ($: Engine): Promise<string> => {
  const opened = await $.ui.open({ id: PANE, title: 'Project' })
  await retireWelcome($)
  const current = await read($, project)
  if (current.state === 'unlocated' || current.state === 'none' || current.state === 'failed') {
    background(load($, current.state === 'failed' ? current.mpr : undefined))
  }
  return opened.isPlaced
    ? 'Project pane opened.'
    : `The Project pane is open but not shown here: ${opened.reason}`
}

export const register: Register = (on, options) => {
  mprOption = typeof options.mprPath === 'string' ? options.mprPath : ''
  mxcliOption = typeof options.mxcliPath === 'string' ? options.mxcliPath : ''

  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'mxcli-project', description: 'Show the Mendix project structure and MDL source in a pane' })
    background(startWelcome($, e.cwd))
    return next(e)
  })

  // The first session after `mxcli init` installed the companion: say it is
  // here and offer the pane. A band, not a pane, because a band needs no room
  // and shows unasked at any width.
  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    if (e.props.hasSurvey || !(await read($, welcome))) return next(e)
    const { Box, Text, Button } = $.ui.resolve(e)
    return (
      <Box flexDirection="row" gap={1} flexWrap="wrap">
        <Text>
          <Text bold>mxcli companion</Text> is installed: the Project pane shows this app's structure and MDL.
        </Text>
        <Button key="welcome-open" variant="primary" label="Open Project pane" onPress={() => void open($)} />
        <Button key="welcome-dismiss" role="dismiss" label="Not now" onPress={() => void retireWelcome($)} />
      </Box>
    )
  })

  on('command.run', { command: 'mxcli-project' }, async $ => ({ text: await open($) }))

  on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
    const ran = await next(e)
    if (changesModel(e.command)) background(scheduleReload($))
    return ran
  }).catch(($, e, next) => next(e)) // an observer: its failure must never cost the call

  on('tool.call', { tool: 'Edit' }, async ($, e, next) => {
    const ran = await next(e)
    if (touchesProjectFile(e.file_path)) background(scheduleReload($))
    return ran
  }).catch(($, e, next) => next(e)) // an observer: its failure must never cost the call

  on('tool.call', { tool: 'Write' }, async ($, e, next) => {
    const ran = await next(e)
    if (touchesProjectFile(e.file_path)) background(scheduleReload($))
    return ran
  }).catch(($, e, next) => next(e)) // an observer: its failure must never cost the call

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Button, Code } = $.ui.resolve(e)
    const proj = await read($, project)
    const open = new Set(await read($, expanded))
    const sel = await read($, selected)
    const src = await read($, source)
    const sideBySide = e.props.bodyColumns >= SIDE_BY_SIDE_MIN_COLUMNS
    // Side by side, each column is one screen tall and pages on its own; stacked,
    // the pane scrolls as usual and the columns draw whole.
    const pageRows = sideBySide ? Math.max(8, e.props.scroll.bodyRows - CHROME_ROWS) : Number.MAX_SAFE_INTEGER

    const pager = (key: string, label: string, onPress: () => void) => (
      <Button key={key} plain dimColor label={label} onPress={onPress} />
    )

    const header = (
      <Box key="header" flexDirection="row" gap={1}>
        <Text bold>Project</Text>
        {'mpr' in proj && <Text dimColor>{basename(proj.mpr)}</Text>}
        {proj.state === 'loaded' && <Text dimColor>({(proj.tookMs / 1000).toFixed(1)}s)</Text>}
        <Button key="refresh" label="Refresh" onPress={() => void load($, 'mpr' in proj ? proj.mpr : undefined)} />
        <Button
          key="collapse"
          label="Collapse"
          onPress={() => void update($, expanded, () => []).then(() => update($, treeTop, () => 0))}
        />
      </Box>
    )

    let tree
    switch (proj.state) {
      case 'unlocated':
      case 'loading':
        tree = <Text dimColor>Reading the project…</Text>
        break
      case 'none':
        tree = (
          <Text>
            No .mpr found here or one level down. Set the mprPath option of {PLUGIN}, or run this from the app's
            folder.
          </Text>
        )
        break
      case 'choose':
        tree = (
          <Box flexDirection="column">
            <Text>Several apps found; pick one:</Text>
            {proj.candidates.map(c => (
              <Button key={`mpr:${c}`} label={c} onPress={() => void load($, c)} />
            ))}
          </Box>
        )
        break
      case 'failed':
        tree = <Text color="error">mxcli project-tree failed: {proj.error}</Text>
        break
      case 'loaded': {
        const { rows, more } = visibleRows(proj.tree, open, sideBySide ? 100_000 : 400)
        const pg = pageOf(rows.length, await read($, treeTop), pageRows)
        tree = (
          <Box flexDirection="column">
            {pg.above > 0 && pager('tree-up', `▲ ${pg.above} more`, () => void update($, treeTop, () => Math.max(0, pg.first - pageRows + 1)))}
            {rows.slice(pg.first, pg.end).map(({ id, node, depth }) => {
              const hasChildren = (node.children?.length ?? 0) > 0
              const glyph = hasChildren ? (open.has(id) ? '▾' : '▸') : node.describable ? '·' : ' '
              return (
                <Box key={`row:${id}`} flexDirection="row" overflow="hidden">
                  <Text>{sel === id ? '›' : ' '}{'  '.repeat(depth)}</Text>
                  <Button key={id} plain label={`${glyph} ${node.label}`} onPress={() => void press($, id)} />
                  <Text dimColor wrap="truncate"> {node.type}</Text>
                </Box>
              )
            })}
            {pg.below > 0 && pager('tree-down', `▼ ${pg.below} more`, () => void update($, treeTop, () => pg.end - 1))}
            {more > 0 && <Text dimColor>… {more} more rows; collapse a module to see them</Text>}
          </Box>
        )
        break
      }
    }

    let detail
    switch (src.state) {
      case 'none':
        detail = <Text dimColor>Select a document to see its MDL.</Text>
        break
      case 'loading':
        detail = <Text dimColor>Describing {src.qn}…</Text>
        break
      case 'failed':
        detail = <Text color="error">describe {src.qn} failed: {src.error}</Text>
        break
      case 'container':
        detail = (
          <Box flexDirection="column">
            <Text dimColor>
              {src.label} ({src.type}) has no MDL source of its own.
            </Text>
            {src.children.length > 0 && <Text dimColor>Contains: {src.children.slice(0, 40).join(', ')}{src.children.length > 40 ? ', …' : ''}</Text>}
          </Box>
        )
        break
      case 'shown': {
        const lines = src.mdl.replace(/\n$/, '').split('\n')
        const pg = pageOf(lines.length, await read($, sourceTop), pageRows - 1)
        detail = (
          <Box flexDirection="column">
            <Text bold>{src.qn}</Text>
            {pg.above > 0 && pager('src-up', `▲ ${pg.above} lines`, () => void update($, sourceTop, () => Math.max(0, pg.first - pageRows + 2)))}
            {annotationRuns(lines.slice(pg.first, pg.end), pg.first + 1).map(run =>
              run.kind === 'code' ? (
                <Code key={`code:${run.startLine}`} source={run.text} language="sql" startLine={run.startLine} />
              ) : (
                <Text key={`note:${run.line}`} dimColor wrap="truncate">
                  {String(run.line).padStart(String(pg.end).length)} {run.text}
                </Text>
              ),
            )}
            {pg.below > 0 && pager('src-down', `▼ ${pg.below} lines`, () => void update($, sourceTop, () => pg.end - 1))}
          </Box>
        )
        break
      }
    }

    if (!sideBySide) {
      return (
        <Box flexDirection="column" gap={1}>
          {header}
          <Box key="tree" flexDirection="column">{tree}</Box>
          <Box key="source" flexDirection="column">{detail}</Box>
        </Box>
      )
    }
    const treeColumns = Math.min(56, Math.max(28, Math.floor(e.props.bodyColumns * 0.38)))
    return (
      <Box flexDirection="column" gap={1}>
        {header}
        <Box key="columns" flexDirection="row" gap={2}>
          <Box key="tree" flexDirection="column" width={treeColumns} flexShrink={0} overflow="hidden">
            {tree}
          </Box>
          <Box key="source" flexDirection="column" flexGrow={1} overflow="hidden">
            {detail}
          </Box>
        </Box>
      </Box>
    )
  })
}

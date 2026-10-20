// Pure helpers for the Project view: no `$`, so the tests can call them
// directly. Everything that reads the project goes through mxcli (the mod
// draws; mxcli knows).

import type { TreeNode } from '../types'

// A row of the tree as drawn: the node, its path id and its depth.
export type Row = { id: string; node: TreeNode; depth: number }

// nodeId is the node's path of labels. Labels are unique among siblings in
// project-tree's output (a folder and a document of the same name sort apart
// by type, so the type is part of the id too).
export const nodeId = (parent: string, node: TreeNode): string =>
  `${parent}/${node.type}:${node.label}`

// visibleRows flattens the expanded part of the tree, in order. `limit` keeps a
// drawing under the engine's per-tree text budget; `more` says what was left.
export const visibleRows = (
  nodes: readonly TreeNode[],
  expanded: ReadonlySet<string>,
  limit = 400,
): { rows: Row[]; more: number } => {
  const rows: Row[] = []
  let more = 0
  const walk = (list: readonly TreeNode[], parent: string, depth: number) => {
    for (const node of list) {
      const id = nodeId(parent, node)
      if (rows.length >= limit) {
        more += 1
        continue
      }
      rows.push({ id, node, depth })
      if (node.children && node.children.length > 0 && expanded.has(id)) {
        walk(node.children, id, depth + 1)
      }
    }
  }
  walk(nodes, '', 0)
  return { rows, more }
}

// pageOf clamps a column's first row to what `total` rows allow at `size` rows
// a page, and says what lies above and below the page. The pane scrolls as one
// body, so each column pages on its own to scroll independently.
export const pageOf = (total: number, top: number, size: number) => {
  const page = Math.max(1, size)
  const first = Math.max(0, Math.min(top, total - page))
  const end = Math.min(total, first + page)
  return { first, end, above: first, below: total - end }
}

// assumeDescribable copes with an mxcli older than project-tree's `describable`
// field: a project can commit a newer companion than the mxcli its session hook
// downloads. With no node carrying the field at all, offer MDL for every named
// leaf and let describe answer, as clients did before the field existed.
// Containers stay out: describing a module prints the whole module.
export const assumeDescribable = (tree: TreeNode[]): TreeNode[] => {
  const hasField = (nodes: readonly TreeNode[]): boolean =>
    nodes.some(n => 'describable' in n || hasField(n.children ?? []))
  if (hasField(tree)) return tree
  const mark = (nodes: readonly TreeNode[]): TreeNode[] =>
    nodes.map(n => ({
      ...n,
      describable: n.qualifiedName !== undefined && (n.children?.length ?? 0) === 0,
      ...(n.children ? { children: mark(n.children) } : {}),
    }))
  return mark(tree)
}

// How a diagram is shown: the whole drawing scaled to the column ('fit'), or
// at actual size ('actual'), cropped to the column and panned by whole
// column widths (`x` counts them from the left).
export type DiagramZoom = { zoom: 'fit' | 'actual'; x: number }

const VIEWBOX = /viewBox="(-?[\d.]+) (-?[\d.]+) ([\d.]+) ([\d.]+)"/

// diagramView rewrites the root's viewBox (and width/height, to keep the
// aspect) so the surface draws the chosen window of the drawing. A surface
// scales an Svg to its slot, so a wide drawing shown whole is too small to read.
export const diagramView = (svg: string, view: DiagramZoom, columnPx: number) => {
  const m = VIEWBOX.exec(svg)
  if (!m) return { svg, canPan: false, atStart: true, atEnd: true, width: undefined, height: undefined }
  const [x0, y0, w, h] = [Number(m[1]), Number(m[2]), Number(m[3]), Number(m[4])]
  // The size to draw it at, in CSS pixels, given to the Svg element: an
  // interactive Svg draws in a frame, and a frame does not size itself to its
  // content the way an image does — without these it is a thumbnail.
  if (view.zoom === 'fit' || w <= columnPx) {
    const width = Math.round(Math.min(w, columnPx))
    return { svg, canPan: false, atStart: true, atEnd: true, width, height: Math.round((h * width) / w) }
  }
  const vw = Math.round(columnPx)
  const maxX = w - vw
  const x = Math.max(0, Math.min(view.x * vw, maxX))
  const head = svg.slice(0, svg.indexOf('>') + 1)
  const rest = svg.slice(head.length)
  const newHead = head
    .replace(VIEWBOX, `viewBox="${x0 + x} ${y0} ${vw} ${h}"`)
    .replace(/ width="[\d.]+"/, ` width="${vw}"`)
    .replace(/ height="[\d.]+"/, ` height="${h}"`)
  return { svg: newHead + rest, canPan: true, atStart: x <= 0, atEnd: x >= maxX, width: vw, height: Math.round(h) }
}

// touchesWorkflow is a Bash command running `mxcli workflow …`: the open runs
// may have changed, so the band re-reads them.
export const touchesWorkflow = (command: string): boolean => /(^|[\s/;&|(])mxcli\s+(.*\s)?workflow\b/.test(command)

// gateLine renders a run's last gate results as one short line.
export const gateLine = (last: readonly { gate: string; status: string }[] | null): string =>
  last === null || last.length === 0
    ? 'not checked yet'
    : last.map(r => `${r.status === 'pass' ? '✓' : r.status === 'warn' ? '!' : '✗'} ${r.gate}`).join('  ')

// parseTree reads `mxcli project-tree` output.
export const parseTree = (stdout: string): TreeNode[] => {
  const start = stdout.indexOf('[')
  if (start < 0) throw new Error('project-tree printed no JSON')
  const parsed: unknown = JSON.parse(stdout.slice(start))
  if (!Array.isArray(parsed)) throw new Error('project-tree did not print a list')
  return parsed as TreeNode[]
}

// parseDescribe reads `mxcli describe --json` output: { name, type, mdl }.
export const parseDescribe = (stdout: string): string => {
  const start = stdout.indexOf('{')
  if (start < 0) throw new Error('describe printed no JSON')
  const parsed = JSON.parse(stdout.slice(start)) as { mdl?: unknown }
  if (typeof parsed.mdl !== 'string') throw new Error('describe printed no mdl')
  return parsed.mdl
}

// firstLine trims an mxcli error to what fits a row of the pane.
export const firstLine = (text: string): string => {
  const line = text
    .split('\n')
    .map(l => l.trim())
    .find(l => l !== '')
  return line ?? 'mxcli failed with no message'
}

// MUTATING_VERB matches the mxcli invocations that can change the model: a
// script (exec), a one-liner (-c) that writes, a new project, a rename, a
// marketplace install. A read-only `show` or `describe` must not reload the
// tree: on a large project the reload is the slow part.
const MXCLI = /(^|[\s/;&|(])mxcli(\s|$)/
const WRITES = /\b(exec|new|rename|marketplace|import)\b/
const ONE_LINER = /\s(-c|--command)\s/
const WRITING_MDL = /\b(create|alter|drop|rename|move|grant|revoke|update|delete|insert)\b/i

export const changesModel = (command: string): boolean => {
  if (!MXCLI.test(command)) return false
  if (ONE_LINER.test(command)) return WRITING_MDL.test(command)
  return WRITES.test(command)
}

// touchesProjectFile is an Edit or Write of the MPR or a unit under
// mprcontents/ — rare, but a change all the same.
export const touchesProjectFile = (path: string): boolean =>
  /\.mpr$/.test(path) || /(^|\/)mprcontents\//.test(path)

// pickMpr chooses the project from what was found, in the order the
// proposal fixes: the option, else the single .mpr here, else the single one a
// level down. Several candidates leave the choice to the person.
export const pickMpr = (
  option: string,
  here: readonly string[],
  below: readonly string[],
): { mpr: string } | { candidates: string[] } | { none: true } => {
  if (option !== '') return { mpr: option }
  if (here.length === 1) return { mpr: here[0]! }
  if (here.length > 1) return { candidates: [...here] }
  if (below.length === 1) return { mpr: below[0]! }
  if (below.length > 1) return { candidates: [...below] }
  return { none: true }
}

import { useLayoutEffect, useMemo, useReducer, useRef, useState } from 'react'
import type { TaskNode, TaskTree } from '../types'
import { compactTokens, formatDuration, stateMeta, traverse } from '../lib/tree'
import { useStore } from '../store'
import { BranchIcon, CheckIcon, DocIcon, SpinnerIcon, XIcon } from './icons'

function StateGlyph({ node }: { node: TaskNode }) {
  const meta = stateMeta[node.state]
  if (node.state === 'COMPLETED')
    return <CheckIcon width={11} height={11} className="shrink-0 text-emerald-400" />
  if (node.state === 'FAILED')
    return <XIcon width={11} height={11} className="shrink-0 text-rose-400" />
  if (meta.spin)
    return <SpinnerIcon width={11} height={11} className={`shrink-0 ${meta.color}`} />
  return node.type === 'LEAF' ? (
    <DocIcon width={11} height={11} className="shrink-0 text-slate-600" />
  ) : (
    <BranchIcon width={11} height={11} className="shrink-0 text-slate-600" />
  )
}

function NodeRow({ node, collapsed, onToggle }: {
  node: TaskNode
  collapsed: boolean
  onToggle: () => void
}) {
  const selectedId = useStore((s) => s.selectedNodeId)
  const selectNode = useStore((s) => s.selectNode)
  const meta = stateMeta[node.state]
  const hasChildren = (node.children_ids?.length ?? 0) > 0
  const selected = selectedId === node.id
  const usage = node.token_usage
  const retries = node.retry_count ?? 0

  return (
    <div
      role="treeitem"
      aria-selected={selected}
      aria-expanded={hasChildren ? !collapsed : undefined}
      onClick={() => selectNode(node.id)}
      className={`group cursor-pointer rounded-md border px-2 py-1.5 transition ${
        selected
          ? 'border-brand-600/70 bg-brand-600/10'
          : 'border-transparent hover:border-line hover:bg-ink-800/70'
      }`}
      style={{ marginLeft: node.depth * 14 }}
    >
      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            if (hasChildren) onToggle()
          }}
          className={`flex h-4 w-4 shrink-0 items-center justify-center rounded text-slate-500 ${
            hasChildren ? 'hover:text-slate-300' : 'invisible'
          }`}
          aria-label={collapsed ? '展开' : '折叠'}
        >
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
            {hasChildren && <path d={collapsed ? 'M9 6l6 6-6 6' : 'M6 9l6 6 6-6'} />}
          </svg>
        </button>

        <StateGlyph node={node} />

        <span className={`min-w-0 flex-1 truncate text-[12.5px] ${meta.color}`} title={node.title}>
          {node.title}
        </span>

        <span className="shrink-0 font-mono text-[10px] text-slate-600">{node.id}</span>
      </div>

      <div className="ml-5 mt-0.5 flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-[10.5px] text-slate-500">
        <span className={`rounded px-1 py-px ${meta.ring} border ${meta.color}`}>{meta.label}</span>
        {node.type === 'COMPOUND' && (
          <span className="text-slate-600">
            {node.children_ids?.length ?? 0} 个子节点
          </span>
        )}
        {retries > 0 && <span className="text-amber-400/90">重试 {retries}</span>}
        {(usage?.calls ?? 0) > 0 && (
          <span className="font-mono">{compactTokens(usage.total_tokens ?? 0)} tok</span>
        )}
        {node.state !== 'PENDING' && (
          <span className="font-mono text-slate-600">
            {formatDuration(node.created_at, node.state === 'COMPLETED' || node.state === 'FAILED'
              ? node.updated_at
              : undefined)}
          </span>
        )}
        {node.integration_verified && (
          <span className="rounded border border-emerald-700/50 px-1 text-emerald-400">
            端到端验收
          </span>
        )}
      </div>

      {node.error_msg && node.state === 'FAILED' && (
        <p className="ml-5 mt-1 line-clamp-2 break-words text-[11px] leading-snug text-rose-300/80">
          {node.error_msg}
        </p>
      )}
    </div>
  )
}

// Auto-collapse fully completed, integration-verified compounds; the user
// can still toggle them explicitly.
function autoCollapsed(n: TaskNode): boolean {
  return (
    n.type === 'COMPOUND' && n.state === 'COMPLETED' && n.integration_verified === true
  )
}

// --- Windowed (virtual) rendering -------------------------------------------
// Zero-dependency virtualization (NFR-7): the flattened visible list is laid
// out on a measured-offset canvas and only the rows intersecting the viewport
// (+ overscan) mount, so the DOM stays bounded no matter how many nodes the
// tree holds (NFR-5). Heights are measured after mount and cached per node
// id; unmeasured rows use a close estimate that self-corrects on first paint.
const ROW_GAP = 2 // rhythm between rows (was space-y-0.5)
const PAD_Y = 8 // pane padding above/below the row canvas (was py-2)
const OVERSCAN = 600 // px rendered beyond the viewport in each direction
const EST_ROW_H = 50 // title line + meta line, before first measurement
const EST_ERR_H = 30 // extra for a clamped two-line error paragraph

// Smallest i such that offsets[i+1] > pos: first row whose bottom edge lies
// below pos. offsets has length n+1 with offsets[0] = 0.
function firstVisible(offsets: Float64Array, n: number, pos: number): number {
  let lo = 0
  let hi = n
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (offsets[mid + 1] > pos) hi = mid
    else lo = mid + 1
  }
  return lo
}

// Smallest j such that offsets[j] >= pos: first row starting at/after pos.
function endVisible(offsets: Float64Array, n: number, pos: number): number {
  let lo = 0
  let hi = n
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (offsets[mid] >= pos) hi = mid
    else lo = mid + 1
  }
  return lo
}

export function TaskTreeView({ tree }: { tree: TaskTree }) {
  // Default-expanded compound ids. Failed/active branches stay open so the
  // user immediately sees what needs attention; finished branches can fold.
  const nodes = useMemo(() => traverse(tree), [tree])
  const [manualCollapsed, setManualCollapsed] = useState<Record<string, boolean>>({})

  // Flip the EFFECTIVE state: an auto-collapsed row's first click must expand
  // it (naively flipping an unset flag would write true and need two clicks).
  const toggle = (n: TaskNode) =>
    setManualCollapsed((m) => ({ ...m, [n.id]: !(m[n.id] ?? autoCollapsed(n)) }))

  const visible = useMemo(() => {
    const collapsedSet = new Set<string>()
    const out: TaskNode[] = []
    for (const n of nodes) {
      // Hidden if any ancestor is collapsed.
      let hidden = false
      let pid = n.parent_id
      while (pid) {
        if (collapsedSet.has(pid)) {
          hidden = true
          break
        }
        pid = tree.nodes[pid]?.parent_id
      }
      if (!hidden) out.push(n)
      const isCollapsed = manualCollapsed[n.id] ?? autoCollapsed(n)
      if (isCollapsed && (n.children_ids?.length ?? 0) > 0) collapsedSet.add(n.id)
    }
    return out
  }, [nodes, manualCollapsed, tree.nodes])

  const isCollapsed = (n: TaskNode) => manualCollapsed[n.id] ?? autoCollapsed(n)

  const scrollRef = useRef<HTMLDivElement | null>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewportH, setViewportH] = useState(0)
  // Measured heights persist across scrolling (cache keyed by node id);
  // rowEls only tracks currently mounted rows for re-measurement.
  const rowHeights = useRef(new Map<string, number>())
  const rowEls = useRef(new Map<string, HTMLElement>())
  const [heightsVer, bumpHeights] = useReducer((x: number) => x + 1, 0)

  // Prefix offsets over the flattened visible list. O(n) per recomputation
  // is negligible even at 10k rows and keeps the scroll math exact.
  const offsets = useMemo(() => {
    const off = new Float64Array(visible.length + 1)
    for (let i = 0; i < visible.length; i++) {
      const n = visible[i]
      const est =
        n.error_msg && n.state === 'FAILED' ? EST_ROW_H + EST_ERR_H : EST_ROW_H
      off[i + 1] = off[i] + (rowHeights.current.get(n.id) ?? est) + ROW_GAP
    }
    return off
    // heightsVer bumps whenever a measured height drifts from the cache.
  }, [visible, heightsVer])

  const total = visible.length
  const first = firstVisible(offsets, total, scrollTop - OVERSCAN)
  const last = endVisible(offsets, total, scrollTop + viewportH + OVERSCAN)

  // Track the viewport size so the window covers exactly what is on screen.
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    setViewportH(el.clientHeight)
    const ro = new ResizeObserver(() => setViewportH(el.clientHeight))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // Re-measure mounted rows on every render: heights converge right after
  // the row first paints and self-heal when its content (e.g. error text)
  // changes. The bump settles in one extra render.
  useLayoutEffect(() => {
    let changed = false
    for (const [id, el] of rowEls.current) {
      const h = el.offsetHeight
      const prev = rowHeights.current.get(id)
      if (prev === undefined || Math.abs(prev - h) > 0.5) {
        rowHeights.current.set(id, h)
        changed = true
      }
    }
    if (changed) bumpHeights()
  })

  const rows: number[] = []
  for (let i = first; i < last; i++) rows.push(i)

  return (
    <div
      ref={scrollRef}
      role="tree"
      onScroll={() => setScrollTop(scrollRef.current?.scrollTop ?? 0)}
      className="h-full overflow-y-auto"
    >
      <div
        role="presentation"
        className="relative mx-3"
        style={{ height: offsets[total] + PAD_Y * 2 }}
      >
        {rows.map((i) => {
          const n = visible[i]
          return (
            <div
              key={n.id}
              ref={(el) => {
                if (el) rowEls.current.set(n.id, el)
                else rowEls.current.delete(n.id)
              }}
              style={{
                position: 'absolute',
                top: PAD_Y + offsets[i],
                left: 0,
                right: 0,
              }}
            >
              <NodeRow node={n} collapsed={isCollapsed(n)} onToggle={() => toggle(n)} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

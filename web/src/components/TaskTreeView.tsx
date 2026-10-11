import { useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState } from 'react'
import type { TaskNode, TaskTree } from '../types'
import { autoCollapsed, compactTokens, flattenVisible, formatDuration, stateMeta } from '../lib/tree'
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

// --- Windowed (virtual) rendering -------------------------------------------
// Zero-dependency virtualization (NFR-5): the flattened visible list is laid
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

// States that demand the supervisor's attention; the quick-find controls walk
// visible nodes in these states in visible order.
const ATTENTION_STATES = new Set(['FAILED', 'RUNNING', 'VERIFYING', 'DECOMPOSING'])

export function TaskTreeView({ tree }: { tree: TaskTree }) {
  // Failed/active branches stay open so the user immediately sees what needs
  // attention; finished+verified branches auto-fold (see flattenVisible).
  const [manualCollapsed, setManualCollapsed] = useState<Record<string, boolean>>({})
  const selectedNodeId = useStore((s) => s.selectedNodeId)
  const selectNode = useStore((s) => s.selectNode)

  // Viewport anchor for collapse/expand (AC-7): the toggled row must keep its
  // distance from the pane top. A node's own canvas offset cannot move when
  // its descendants are revealed or pruned (they sit below it); the real
  // disturbance is the browser clamping scrollTop after a prune shrinks the
  // canvas. The anchor is captured here and consumed once in a layout effect.
  // Only toggle sets it, so programmatic scrolling (quick find) never anchors.
  const anchorRef = useRef<{ id: string; viewportTop: number } | null>(null)

  // Flip the EFFECTIVE state: an auto-collapsed row's first click must expand
  // it (naively flipping an unset flag would write true and need two clicks).
  const toggle = (n: TaskNode) => {
    const pane = scrollRef.current
    const rowEl = rowEls.current.get(n.id)
    if (pane && rowEl) {
      anchorRef.current = {
        id: n.id,
        viewportTop:
          rowEl.getBoundingClientRect().top - pane.getBoundingClientRect().top,
      }
    } else {
      anchorRef.current = null
    }
    setManualCollapsed((m) => ({ ...m, [n.id]: !(m[n.id] ?? autoCollapsed(n)) }))
  }

  // One iterative O(n) DFS: ordering and collapsed-subtree pruning live in
  // the pure lib function so deep chains cannot blow the call stack or cost
  // O(n*depth).
  const visible = useMemo(
    () => flattenVisible(tree, manualCollapsed),
    [tree, manualCollapsed],
  )

  const isCollapsed = (n: TaskNode) => manualCollapsed[n.id] ?? autoCollapsed(n)

  const scrollRef = useRef<HTMLDivElement | null>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewportH, setViewportH] = useState(0)
  // Trackpads/wheels emit 200+ scroll events per second. Coalesce them to at
  // most one state commit per animation frame: events just ensure a frame is
  // scheduled; the frame reads the then-latest scrollTop.
  const scrollRaf = useRef(0)
  const onScroll = () => {
    if (scrollRaf.current !== 0) return
    scrollRaf.current = requestAnimationFrame(() => {
      scrollRaf.current = 0
      setScrollTop(scrollRef.current?.scrollTop ?? 0)
    })
  }
  // A pending frame must not touch state after the pane unmounted.
  useEffect(
    () => () => {
      if (scrollRaf.current !== 0) cancelAnimationFrame(scrollRaf.current)
    },
    [],
  )
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

  // Quick find (AC-8): visible attention nodes in visible order, with a cursor
  // that survives snapshot replacements as long as its node still exists.
  const attention = useMemo(
    () =>
      visible
        .map((node, index) => ({ node, index }))
        .filter(({ node }) => ATTENTION_STATES.has(node.state)),
    [visible],
  )
  // The cursor is the store selection itself: quick-find, row clicks and
  // keyboard navigation all move it, so it can never diverge from the label.
  const attentionPos = attention.findIndex(({ node }) => node.id === selectedNodeId)
  const labelPos = attentionPos >= 0 ? attentionPos + 1 : '–'
  // A jump whose target lands in never-rendered area parks with estimated row
  // heights; replay it once after measurements converge (see measure effect).
  const pendingJumpRef = useRef<{ id: string } | null>(null)
  const parkRow = (index: number) => {
    const pane = scrollRef.current
    if (!pane) return
    const gap = Math.min(80, Math.round(pane.clientHeight / 4))
    const top = Math.min(
      Math.max(0, PAD_Y + offsets[index] - gap),
      Math.max(0, pane.scrollHeight - pane.clientHeight),
    )
    pane.scrollTop = top
    setScrollTop(top)
  }
  const jumpToAttention = (dir: 1 | -1) => {
    if (attention.length === 0) return
    const len = attention.length
    let idx = attentionPos
    if (idx < 0) idx = dir === 1 ? 0 : len - 1
    else idx = (idx + dir + len) % len
    const { node, index } = attention[idx]
    selectNode(node.id)
    // Park the row just below the pane top (small breathing gap). Programmatic
    // only: no toggle anchor is recorded; a replay is armed in case the target
    // region had only estimated row heights so far.
    pendingJumpRef.current = { id: node.id }
    parkRow(index)
  }

  // Minimal scroll so the row at visible index i is inside the viewport: moves
  // only when it is above or below the current view (plus a small gap).
  const ensureIndexVisible = (i: number) => {
    const pane = scrollRef.current
    if (!pane || i < 0 || i >= total) return
    const canvasTop = PAD_Y + offsets[i]
    const rowBottom = PAD_Y + offsets[i + 1] - ROW_GAP
    const gap = 12
    let want = pane.scrollTop
    if (canvasTop < want + gap) want = canvasTop - gap
    else if (rowBottom > want + pane.clientHeight - gap) {
      want = rowBottom - pane.clientHeight + gap
    }
    want = Math.min(Math.max(0, want), Math.max(0, pane.scrollHeight - pane.clientHeight))
    if (want !== pane.scrollTop) {
      pane.scrollTop = want
      setScrollTop(want)
    }
  }

  // Keyboard navigation (AC-9): arrows move selection along the visible order;
  // right/left expand/collapse compounds through the same anchored toggle path
  // as the mouse. Boundary presses are no-ops (no wrap). Only active while the
  // tree pane itself holds focus.
  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (total === 0) return
    const idx = visible.findIndex((n) => n.id === selectedNodeId)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const i =
        e.key === 'ArrowDown'
          ? Math.min(total - 1, idx < 0 ? 0 : idx + 1)
          : Math.max(0, idx < 0 ? total - 1 : idx - 1)
      selectNode(visible[i].id)
      ensureIndexVisible(i)
    } else if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
      if (idx < 0) return
      const n = visible[idx]
      if ((n.children_ids?.length ?? 0) === 0) return
      const collapsed = isCollapsed(n)
      if ((e.key === 'ArrowRight' && collapsed) || (e.key === 'ArrowLeft' && !collapsed)) {
        e.preventDefault()
        toggle(n)
      }
    }
  }

  // Track the viewport size so the window covers exactly what is on screen.
  // A width-only change reflows wrapped error text, so cached row heights are
  // invalidated and remeasured (clientHeight alone may stay identical).
  const paneWidth = useRef(0)
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    setViewportH(el.clientHeight)
    paneWidth.current = el.clientWidth
    const ro = new ResizeObserver(() => {
      setViewportH(el.clientHeight)
      if (el.clientWidth !== paneWidth.current) {
        paneWidth.current = el.clientWidth
        rowHeights.current.clear()
        bumpHeights()
      }
    })
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
    const jump = pendingJumpRef.current
    if (changed) {
      bumpHeights()
    } else if (jump) {
      // Heights in the jumped-to window have settled: repark against the
      // measured offsets so estimate drift cannot strand the target row.
      pendingJumpRef.current = null
      const idx = visible.findIndex((n) => n.id === jump.id)
      if (idx >= 0) parkRow(idx)
    }
  })

  // Consume the toggle anchor once, after the pruned/revealed subtree has
  // committed: put the toggled row back at its pre-toggle viewport position.
  // Rows above it never change identity on toggle, so this only corrects the
  // browser's scrollTop clamp against the shrunken canvas. One-shot: later
  // snapshots/scrolling must not re-trigger compensation (TR-4.2).
  useLayoutEffect(() => {
    const a = anchorRef.current
    const pane = scrollRef.current
    if (!a || !pane) return
    anchorRef.current = null
    if (!visible.some((n) => n.id === a.id)) return
    const rowEl = rowEls.current.get(a.id)
    if (!rowEl) return
    const current =
      rowEl.getBoundingClientRect().top - pane.getBoundingClientRect().top
    const delta = Math.round(current - a.viewportTop)
    if (Math.abs(delta) <= 2) return
    const max = Math.max(0, pane.scrollHeight - pane.clientHeight)
    const next = Math.min(Math.max(0, pane.scrollTop + delta), max)
    pane.scrollTop = next
    setScrollTop(next)
  })

  const rows: number[] = []
  for (let i = first; i < last; i++) rows.push(i)

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-line bg-ink-850/40 px-3 py-1 text-[11.5px]">
        <button
          type="button"
          onClick={() => jumpToAttention(-1)}
          disabled={attention.length === 0}
          title="定位上一个失败/进行中节点（到首后回绕到末尾）"
          className="rounded border border-line px-1.5 py-0.5 text-slate-400 hover:bg-ink-800 hover:text-slate-200 disabled:cursor-not-allowed disabled:opacity-40"
        >
          ‹ 上一个
        </button>
        <button
          type="button"
          onClick={() => jumpToAttention(1)}
          disabled={attention.length === 0}
          title="定位下一个失败/进行中节点（到末尾后回绕到首个）"
          className="rounded border border-line px-1.5 py-0.5 text-slate-400 hover:bg-ink-800 hover:text-slate-200 disabled:cursor-not-allowed disabled:opacity-40"
        >
          下一个 ›
        </button>
        {attention.length > 0 ? (
          <span className="text-slate-500">
            失败/进行中{' '}
            <span className="font-mono text-slate-300">
              {labelPos}/{attention.length}
            </span>
          </span>
        ) : (
          <span className="text-slate-600">无失败/进行中节点</span>
        )}
      </div>
      <div
        ref={scrollRef}
        role="tree"
        tabIndex={0}
        onScroll={onScroll}
        onKeyDown={onKeyDown}
        className="min-h-0 flex-1 overflow-y-auto outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-brand-600/70"
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
    </div>
  )
}

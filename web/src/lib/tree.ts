import type { NodeState, TaskNode, TaskTree } from '../types'

// Ordered root-down traversal, following children_ids (the tree stores a
// map; ordering explicitly keeps rendering stable as snapshots replace each
// other). Iterative with an explicit stack: a pathological single-chain
// decomposition thousands of levels deep would overflow the call stack
// under recursion (Node/V8 caps around 7-8k frames).
export function traverse(tree: TaskTree): TaskNode[] {
  const out: TaskNode[] = []
  const seen = new Set<string>()
  // Push children in reverse so the lowest-index child pops first: this
  // yields the same order as the old recursive pre-order walk.
  const stack: string[] = [tree.root_id]
  while (stack.length > 0) {
    const id = stack.pop() as string
    if (seen.has(id)) continue
    seen.add(id)
    const n = tree.nodes[id]
    if (!n) continue
    out.push(n)
    const kids = n.children_ids ?? []
    for (let i = kids.length - 1; i >= 0; i--) stack.push(kids[i])
  }
  return out
}

// Auto-collapse fully completed, integration-verified compounds; the user
// can still toggle them explicitly.
export function autoCollapsed(n: TaskNode): boolean {
  return (
    n.type === 'COMPOUND' && n.state === 'COMPLETED' && n.integration_verified === true
  )
}

// Visible (non-pruned) nodes in root-down order, computed in one iterative
// DFS pass. A frame carries whether its subtree is already hidden by a
// collapsed ancestor: when so, the node is skipped without pushing its
// children (subtree pruning), which keeps the total cost O(n) instead of
// walking each node's ancestor chain (O(n*depth), seconds on deep chains).
// The collapsed node itself stays visible; only its descendants are pruned.
export function flattenVisible(
  tree: TaskTree,
  manualCollapsed: Record<string, boolean> = {},
): TaskNode[] {
  const out: TaskNode[] = []
  const seen = new Set<string>()
  const stack: Array<{ id: string; hidden: boolean }> = [{ id: tree.root_id, hidden: false }]
  while (stack.length > 0) {
    const frame = stack.pop() as { id: string; hidden: boolean }
    if (seen.has(frame.id)) continue
    seen.add(frame.id)
    const n = tree.nodes[frame.id]
    if (!n) continue
    if (!frame.hidden) out.push(n)
    const kids = n.children_ids ?? []
    const selfCollapsed = (manualCollapsed[n.id] ?? autoCollapsed(n)) && kids.length > 0
    // Hidden is monotone down the tree: a folded subtree cannot become
    // visible again deeper on, so do not even enqueue its descendants.
    if (frame.hidden || selfCollapsed) continue
    for (let i = kids.length - 1; i >= 0; i--) {
      stack.push({ id: kids[i], hidden: false })
    }
  }
  return out
}

export interface TreeStats {
  total: number
  leaves: number
  leavesDone: number
  tokens: number
  calls: number
  failed: number
  running: number
  pending: number
}

export function treeStats(tree: TaskTree): TreeStats {
  const s: TreeStats = {
    total: 0,
    leaves: 0,
    leavesDone: 0,
    tokens: 0,
    calls: 0,
    failed: 0,
    running: 0,
    pending: 0,
  }
  for (const n of Object.values(tree.nodes)) {
    s.total++
    s.tokens += n.token_usage?.total_tokens ?? 0
    s.calls += n.token_usage?.calls ?? 0
    if (n.type === 'LEAF') {
      s.leaves++
      if (n.state === 'COMPLETED') s.leavesDone++
    }
    if (n.state === 'FAILED') s.failed++
    if (n.state === 'RUNNING' || n.state === 'VERIFYING' || n.state === 'DECOMPOSING')
      s.running++
    if (n.state === 'PENDING') s.pending++
  }
  return s
}

export function formatDuration(fromIso?: string, toIso?: string): string {
  if (!fromIso) return ''
  const from = Date.parse(fromIso)
  if (!Number.isFinite(from)) return ''
  const to = toIso ? Date.parse(toIso) : Date.now()
  const ms = Math.max(0, to - from)
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)}s`
  const m = Math.floor(s / 60)
  const rs = Math.round(s % 60)
  return `${m}m ${rs}s`
}

export function compactTokens(n: number): string {
  if (n <= 0) return '0'
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`
  return `${(n / 1_000_000).toFixed(2)}M`
}

export const stateMeta: Record<
  NodeState,
  { color: string; ring: string; label: string; spin?: boolean }
> = {
  PENDING: { color: 'text-slate-500', ring: 'border-slate-600 bg-ink-750', label: '待运行' },
  DECOMPOSING: { color: 'text-amber-300', ring: 'border-amber-500/60 bg-amber-500/10', label: '拆解中', spin: true },
  RUNNING: { color: 'text-brand-400', ring: 'border-brand-500/60 bg-brand-600/10', label: '运行中', spin: true },
  VERIFYING: { color: 'text-cyan-300', ring: 'border-cyan-500/60 bg-cyan-500/10', label: '验收中', spin: true },
  COMPLETED: { color: 'text-emerald-300', ring: 'border-emerald-600/50 bg-emerald-600/10', label: '完成' },
  FAILED: { color: 'text-rose-300', ring: 'border-rose-700/60 bg-rose-700/15', label: '失败' },
  SKIPPED: { color: 'text-slate-600', ring: 'border-slate-700 bg-ink-800', label: '跳过' },
}

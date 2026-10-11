import { useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState } from 'react'
import { ApiError, api } from '../api/client'
import { useStore } from '../store'
import type { FsFileResponse } from '../types'
import { DocIcon, SpinnerIcon, XIcon } from './icons'

function humanSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

// --- Windowed text rendering -------------------------------------------------
// The server caps previewable text files at 1 MiB, but that is still tens of
// thousands of lines (and long lines wrap to several rows). Rendering every
// line as a table row made opening such files jank or hang. The same measured
// windowing as TaskTreeView applies: only viewport-intersecting lines (+
// overscan) mount, on an absolutely positioned canvas. Unlike tree rows,
// wrapped-line heights depend on container width, so the cache is invalidated
// whenever the viewport width changes.
const EST_LINE_H = 20 // font-mono 12px * leading 1.6
const LINE_OVERSCAN = 600 // px above/below the viewport

function lowerBound(offsets: Float64Array, n: number, pos: number): number {
  let lo = 0
  let hi = n
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (offsets[mid + 1] > pos) hi = mid
    else lo = mid + 1
  }
  return lo
}

function startBound(offsets: Float64Array, n: number, pos: number): number {
  let lo = 0
  let hi = n
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (offsets[mid] >= pos) hi = mid
    else lo = mid + 1
  }
  return lo
}

function WindowedText({ lines }: { lines: string[] }) {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewport, setViewport] = useState({ h: 0, w: 0 })
  const heights = useRef(new Map<number, number>())
  const els = useRef(new Map<number, HTMLElement>())
  const [heightsVer, bumpHeights] = useReducer((x: number) => x + 1, 0)
  const widthRef = useRef(0)

  // Line indices are only meaningful for the current file content.
  const linesKey = useMemo(() => lines.length, [lines])
  useEffect(() => {
    heights.current.clear()
    els.current.clear()
    widthRef.current = 0
    setScrollTop(0)
    bumpHeights()
  }, [linesKey])

  const offsets = useMemo(() => {
    const off = new Float64Array(lines.length + 1)
    for (let i = 0; i < lines.length; i++) {
      off[i + 1] = off[i] + (heights.current.get(i) ?? EST_LINE_H)
    }
    return off
  }, [lines, heightsVer])

  const total = lines.length
  const first = lowerBound(offsets, total, scrollTop - LINE_OVERSCAN)
  const last = startBound(offsets, total, scrollTop + viewport.h + LINE_OVERSCAN)

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    setViewport({ h: el.clientHeight, w: el.clientWidth })
    const ro = new ResizeObserver(() => {
      const w = el.clientWidth
      // A width change re-wraps every line: discard measured heights so the
      // canvas and window recompute instead of overlaying stale offsets.
      if (widthRef.current !== 0 && Math.abs(w - widthRef.current) > 1) {
        heights.current.clear()
        bumpHeights()
      }
      widthRef.current = w
      setViewport((v) => (v.h === el.clientHeight && v.w === w ? v : { h: el.clientHeight, w }))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  useLayoutEffect(() => {
    let changed = false
    for (const [i, el] of els.current) {
      const h = el.offsetHeight
      const prev = heights.current.get(i)
      if (prev === undefined || Math.abs(prev - h) > 0.5) {
        heights.current.set(i, h)
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
      data-file-scroll
      onScroll={() => setScrollTop(scrollRef.current?.scrollTop ?? 0)}
      className="min-h-0 flex-1 overflow-auto bg-ink-950/60"
    >
      <div role="presentation" className="relative font-mono text-[12px] leading-[1.6]">
        <div style={{ height: offsets[total] }} />
        {rows.map((i) => (
          <div
            key={i}
            data-file-row={i + 1}
            ref={(el) => {
              if (el) els.current.set(i, el)
              else els.current.delete(i)
            }}
            className="absolute left-0 right-0 flex hover:bg-ink-900/60"
            style={{ top: offsets[i] }}
          >
            <span className="w-14 shrink-0 select-none border-r border-ink-800 px-3 text-right text-[10.5px] leading-[1.6] text-slate-700">
              {i + 1}
            </span>
            <span className="min-w-0 flex-1 whitespace-pre-wrap break-all px-3 leading-[1.6] text-slate-300">
              {lines[i] || ' '}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

// Read-only, sandbox-confined text viewer for workspace files. The server
// already enforces path confinement, a 1 MiB size cap (413) and a binary
// sniff (415); this component renders those outcomes as friendly panels.
export function FileViewer({ sessionId, path }: { sessionId: string | null; path: string }) {
  const closeFile = useStore((s) => s.closeFile)
  const [file, setFile] = useState<FsFileResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [problem, setProblem] = useState<{ status: number; message: string } | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setProblem(null)
    setFile(null)
    api
      .fsFile(path, sessionId ?? undefined)
      .then((f) => {
        if (!cancelled) setFile(f)
      })
      .catch((e) => {
        if (cancelled) return
        if (e instanceof ApiError) {
          setProblem({
            status: e.status,
            message:
              e.status === 413
                ? '文件超过 1 MiB 的只读查看上限，请在编辑器或终端中打开。'
                : e.status === 415
                  ? '二进制文件不支持文本预览。'
                  : e.status === 403
                    ? '该路径通过软链接指向工作区之外，已被安全策略阻止。'
                    : e.message,
          })
        } else {
          setProblem({ status: 0, message: String(e) })
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [path, sessionId])

  const lines = useMemo(() => {
    if (!file) return []
    const out = file.content.split('\n')
    if (out.length > 1 && out[out.length - 1] === '') out.pop()
    return out
  }, [file])

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center gap-2 border-b border-line bg-ink-850 px-4 py-2">
        <DocIcon width={14} height={14} className="text-slate-500" />
        <span className="truncate font-mono text-[12px] text-slate-200" title={path}>
          {path}
        </span>
        {file && (
          <span className="shrink-0 font-mono text-[11px] text-slate-500">
            {humanSize(file.size)} · {lines.length} 行 · 只读
          </span>
        )}
        <button
          onClick={closeFile}
          className="ml-auto flex shrink-0 items-center gap-1 rounded border border-line px-2 py-0.5 text-[11.5px] text-slate-400 hover:bg-ink-800 hover:text-slate-200"
        >
          <XIcon width={12} height={12} />
          关闭
        </button>
      </div>

      {loading && (
        <div className="min-h-0 flex-1 overflow-auto">
          <p className="flex items-center gap-2 px-5 py-6 text-[12.5px] text-slate-500">
            <SpinnerIcon width={14} height={14} />
            正在读取 {path}…
          </p>
        </div>
      )}
      {problem && (
        <div className="min-h-0 flex-1 overflow-auto">
          <div className="mx-5 my-6 max-w-[560px] rounded-lg border border-rose-900/50 bg-rose-950/25 px-4 py-3">
            <p className="text-[13px] font-medium text-rose-300">无法预览文件</p>
            <p className="mt-1 text-[12px] leading-relaxed text-rose-200/80">{problem.message}</p>
            <p className="mt-2 font-mono text-[11px] text-slate-500">
              {path}
              {problem.status > 0 ? ` · HTTP ${problem.status}` : ''}
            </p>
          </div>
        </div>
      )}
      {file && lines.length === 0 && (
        <div className="min-h-0 flex-1 overflow-auto">
          <p className="px-5 py-6 font-mono text-[12px] text-slate-600">（空文件）</p>
        </div>
      )}
      {file && lines.length > 0 && <WindowedText key={path} lines={lines} />}
    </div>
  )
}

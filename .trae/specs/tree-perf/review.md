# tree-perf 切片独立代码审查记录

- 审查方式：spec-mode 独立 fresh-context 只读子代理（不接触实施过程上下文），对照 spec.md 的 AC-1~11 / NFR-1~6 逐条核实，另跑独立断言与 tsc。
- 审查对象：5eee956（树优化主体）+ c70f727（FileViewer 上下文）。
- 初始结论：**可关闭**，Blocker 0 / Major 0 / Minor 4 / Nit 6。

## 审查方独立验证

- `git diff` 核实：package.json/package-lock.json 两个提交 **0 diff**；`*.go` 0 diff；无 Worker；dist 含新代码。
- esbuild 打包真实 web/src/lib/tree.ts 后独立断言 **825 条全绿**：与朴素递归+祖先回溯参考实现逐索引等价（16 组 manual 折叠组合 + 40 棵随机树），覆盖断链、根缺失、root_id undefined、环、重复引用、跨父引用、children_ids 缺失/null；二分边界 + 500 行随机偏移暴力交叉；20k 链 flatten 8.18ms、10k 链 3.38ms、10k 平衡 3.46ms。
- `tsc --noEmit`（strict + noUnusedLocals）通过。
- 浏览器性能类 AC（AC-5/6）结论采信 tasks.md 中 CDP 原始采样证据。

## AC/NFR 核实结论

| 项 | 结论 | 要点 |
|---|---|---|
| AC-1 深链不崩溃 | pass | 显式栈迭代 + 挂载行有界；20k 链不抛错 |
| AC-2 O(n) 语义不变 | pass | 单趟 DFS 剪枝，825 断言等价 |
| AC-3 折叠零回归 | pass | manual ?? auto，首点即展开；选中消失回退 root |
| AC-4 滚动合帧/统计 memo | pass | rAF ref 守卫 + 卸载清理；treeStats 仅 tree 替换重算 |
| AC-5 2k/10fps 60fps | pass（采信实测） | TR-7.1 avg 16.67ms、长任务 0 |
| AC-6 10k 可用 | pass（采信实测+代码） | TR-7.3 rubric 5/5 |
| AC-7 折叠锚定 | pass | anchor 仅 toggle 写、一次性消费、StrictMode 安全 |
| AC-8 快速定位 | pass（附 Minor-1/2，已修） | 状态集合/回绕/空态/不经锚定均正确 |
| AC-9 键盘导航 | pass | 边界 clamp、叶子 no-op、仅树内焦点响应 |
| AC-10 FileViewer 窗口化 | pass | c70f727，本切片仅追加同款 rAF |
| AC-11 零依赖/构建闸门 | pass | 依赖/Go 双零 diff，tsc 全绿 |
| NFR-1~6 | pass | NFR-3 审查方实测 flatten 远低于 100ms 预算 |

## 问题处置

| 编号 | 问题 | 处置 |
|---|---|---|
| Minor-1 | 长距离快速定位落入从未渲染区域时，估计行高漂移可能使目标停在视口外沿 | **已修（8c12121）**：pendingJumpRef 在测量收敛（changed=false）帧按实测 offsets 重放一次停车后清除，目标不可见时丢弃；不经过 toggle 锚定 |
| Minor-2 | 独立 cursorId 与鼠标行选中分叉：标签位置与「下一个」起点可能停留在旧 cursor | **已修（8c12121）**：删除 cursorId，关注位置统一从 store 选中节点派生，行点击/键盘/定位天然一致，同时消除一次 findIndex |
| Minor-3 | 仅宽度变化（clientHeight 不变）时 FAILED 行折行高度缓存不失效，可能短暂重叠（ed0ce17 既有） | **已修（8c12121）**：ResizeObserver 跟踪 clientWidth，宽度变化清空 rowHeights 并 bump 重测 |
| Minor-4 | rowHeights/manualCollapsed 无淘汰 | 记录不改：键以会话内节点总数（≤20k，每条 number）为硬上界，切会话卸载即 GC |
| Nit-1 | 两次 attention.findIndex O(n) | 随 Minor-2 消除为一次 |
| Nit-2 | attention memo 为每节点分配对象 | 不改：10fps×10k 下 GC 压力可忽略（实测零长任务） |
| Nit-3 | onKeyDown 每键一次 findIndex | 不改：20k 亚毫秒 |
| Nit-4 | 注释引用不存在的 "NFR-7" | **已修（8c12121）**：改为 NFR-5 |
| Nit-5 | FileViewer linesKey 切换只 setState 不直写 pane.scrollTop | 不改：c70f727 既有，key={path} 已兜底重挂载 |
| Nit-6 | 焦点在行内折叠按钮上时方向键冒泡仍生效 | 不改：行为良性（焦点仍在树内），保留与 chevron+键盘的连贯操作 |

## 修复后回归（CDP，2193/2200 平衡树与混合态夹具）

- 混合态：下一个×4 序列 failgrp→f1→run1→root(回绕)、标签 2/4…1/4 不变；直接点击 f1 标签立即显示 3/4；点击非关注行 done 显示「–/4」；done 展开/折叠视口锚定 delta=0px（行 7↔5）。
- 平衡树：从默认选中起按「上一个」回绕到末尾关注节点 n02191（423/423，物理 clamp 下 top=440 仍在视口内），继续上一个 n02190（422/423，top=364 在视口），下一个回 n02191；测量收敛后无二次漂移、无渲染循环。
- 宽度循环 1440→1000→640→1000→1440：挂载行零重叠，失败错误段落仍渲染，页面异常 0。
- tsc -b + vite build 全绿，dist 重建 sha256 一致（index-BQCTGh4D.js）。

提交：5eee956（实现）、8637e19（规格/证据）、8c12121（评审修复）。

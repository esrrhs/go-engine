# 长任务树性能与导航优化 - 产品需求文档

## Overview
- **Summary**: desktop-ui 已完成窗口化虚拟渲染（1021 节点挂载行有界，commit ed0ce17），但在超过现有 NFR-5 基线（200 节点）的长树上，前端管线仍存在两类已实测坐实的病理问题：递归遍历在深链上栈溢出、可见性过滤的祖先回溯在深链上 O(n²)，以及高频全量快照下的整树重算/渲染浪费。本期为**纯前端**切片：消除崩溃路径、把派生计算降到 O(n)、节流滚动与统计重算，并补齐长树导航能力（视口锚定、失败/进行中快速定位、键盘导航）。
- **Purpose**: 弱模型拆解大型目标时树可达数百至数千节点；深链（逐层拆解的任务链）虽不常见但当前直接整树崩溃白屏。用户需要在长树上不卡死、不崩溃，并能快速定位需要干预的失败/进行中叶子。
- **Target Users**: 在 `divvy serve` 浏览器界面或 Tauri 界面中监督长任务树运行的开发者。

## Goals
- 任意形态的树（平衡树、深链、混合）不崩溃，挂载 DOM 行数始终有界。
- 2,000 节点平衡树（10× 现 NFR-5 基线 200 节点）下持续滚动 60fps，快照高频更新（10 帧/秒合并上限）期间交互无明显卡顿。
- 10,000 节点平衡树功能可用：可浏览、折叠/展开、选中、定位，不出现秒级冻结。
- 长树导航：折叠/展开不跳位；一键在失败/进行中节点间跳转；键盘可达。
- 在途的 FileViewer 万行文本窗口化改动单独收口提交。

## Non-Goals
- **不改后端事件协议**：不引入节点级 patch/增量事件；tree_snapshot 仍为全量。10k 节点时 JSON.parse 约 39ms/帧的成本本期由前端结构优化摊薄，不做 worker 解析（2k 目标规模下解析仅占主线程约 8%，worker 的结构化克隆与构建复杂度不划算）。
- 不引入任何新的 npm 依赖（含 vitest 等测试框架）；规模证据沿用 node 脚本 + 浏览器实测的既有方式。
- 不改动树的视觉设计、三栏布局与审批/问答主流程交互。
- 不做树形数据的搜索框过滤、多选、拖拽排序。
- 不做 FileViewer 的语法高亮、行列跳转。

## Background & Context
- 现状代码：
  - [traverse()](file:///Volumes/My%20Shared%20Files/project/divvy/web/src/lib/tree.ts#L6-L19) 为递归 DFS，Node 实测单链约 7-8 千深度即 `RangeError: Maximum call stack size exceeded`，整棵树渲染崩溃。
  - [TaskTreeView visible 过滤](file:///Volumes/My%20Shared%20Files/project/divvy/web/src/components/TaskTreeView.tsx#L164-L183) 对每个节点沿 parent_id 回溯整条祖先链判断是否折叠，深链全展开时为 O(n²)：node 微基准实测 2,000 链 102ms、5,000 链 1,211ms。
  - 滚动 onScroll 每次事件直接 setState（惯性滚动可 >100 事件/秒），每次都重算窗口并整组重渲挂载行。
  - TreeHeader 的 treeStats(tree) 在 SessionView 每次渲染时全量 O(n) 重算，而 SessionView 订阅了 events（llm_call/tool_call 等事件频率远高于树快照），即每条无关事件都全树扫描一次。
  - 快照经 treeBroadcaster 100ms 合并（[treebroadcast.go](file:///Volumes/My%20Shared%20Files/project/divvy/pkg/agent/treebroadcast.go)），上限约 10 全量帧/秒。
- node 微基准（复刻当前管线，2026-10-11）：

  | 节点数（平衡树） | JSON.parse/帧 | traverse+visible | 快照体积 |
  | --- | --- | --- | --- |
  | 1,021 | 4.1ms | ~0.5ms | 353KiB |
  | 5,000 | 17.0ms | 4.1ms | 1.7MiB |
  | 10,000 | 39.1ms | 7.6ms | 3.4MiB |
  | 20,000 | 145.6ms | 15.2ms | 6.9MiB |
- 既有资产：TaskTreeView 的测量高度缓存、二分窗口、overscan 机制保持不变并作为本期基础；FileViewer 已有一份完成但未提交的同款窗口化文本实现（宽度变化使高度缓存失效）。
- 既有证据方式：node 侧用 esbuild（web/node_modules 内已有）打包 TS 纯函数后跑断言/基准；浏览器侧用规模夹具（pkg/server 现有 scale 测试）+ 自动化实测 DOM 数/帧时。

## Functional Requirements

- **FR-1 无崩溃遍历**：树的顺序化与可见性计算必须使用迭代（显式栈）实现；20,000 节点单链树可完成计算并渲染，不得抛出栈溢出。
- **FR-2 线性可见性计算**：可见扁平列表由一次根向下 DFS 产出（折叠状态在遍历中传递，命中折叠祖先则剪枝整棵子树），总复杂度 O(n)，消除逐节点祖先回溯。
- **FR-3 派生纯函数化**：遍历与可见性计算为 web/src/lib 下的纯函数，不依赖 React，可被 node 脚本直接断言：输出顺序、折叠剪枝、manualCollapsed 与 autoCollapsed 优先级与现状逐字节一致。
- **FR-4 滚动节流**：滚动位置更新经 requestAnimationFrame 合帧，一帧内至多一次状态提交；组件卸载时取消待执行帧。
- **FR-5 统计记忆化**：treeStats 仅在树对象变化时重算，无关 SSE 事件不触发全树扫描。
- **FR-6 折叠视口锚定**：折叠或展开某节点后，该节点行相对视口的屏幕位置保持不变（容差 ±2px），文档总高度变化通过滚动位置补偿吸收。
- **FR-7 快速定位**：树区提供「上一个/下一个关注节点」入口，关注节点 = FAILED 或进行中（RUNNING/VERIFYING/DECOMPOSING）；点击依次选中目标节点并滚动至可视区；不存在关注节点时控件禁用并给出当前状态提示。
- **FR-8 键盘导航**：树滚动容器获得焦点时，↑/↓ 在可见节点间移动选中（必要时自动滚动入视），→ 展开折叠节点、← 折叠展开节点（对叶子或已是目标态的按键为无害 no-op）。

## Non-Functional Requirements

- **NFR-1 规模-流畅**：2,000 节点平衡树、快照以 10 帧/秒持续替换期间，连续滚动 1 秒的平均帧时 ≤ 16.7ms（≥60fps），长任务（>50ms）计数为 0；浏览器实测。
- **NFR-2 规模-可用**：10,000 节点平衡树可完成打开（首屏可交互 ≤ 1s，不含 HTTP 传输）、折叠/展开、选中与定位；单次快照更新主线程阻塞可感知但不出现 >500ms 冻结。
- **NFR-3 深链健壮**：20,000 节点单链从历史会话打开不崩溃；扁平化计算 ≤ 100ms（现状 5,000 链即 1.2s）。
- **NFR-4 行为零回归**：autoCollapsed（完成且 integration_verified 的 COMPOUND 默认折叠）、手动翻转语义（自动折叠节点首次点击即展开）、跨快照选中保留（节点消失回退 root）、高度测量自修正、DOM 挂载行有界等既有行为全部保持。
- **NFR-5 零新依赖与构建闸门**：不新增 npm 依赖；`tsc -b` 严格检查与 `vite build` 通过；提交的 web/dist 可由源码确定性重建（CI web job 口径不变）。
- **NFR-6 证据可重复**：规模数字必须来自自动化实测（node 断言/基准脚本输出 + 浏览器性能采样），不接受仅凭代码走查的性能结论；临时脚本/夹具用后从仓库删除。

## Constraints
- **Technical**: React 18 + Vite + TypeScript + Tailwind 现状；无测试框架，纯函数验证经 esbuild 打包后 node 执行；浏览器证据经现有自动化浏览器能力采集。
- **Business**: 界面文案中文为主；不改变桌面端 Tauri 外壳与后端任何行为。
- **Dependencies**: 仅使用 web/node_modules 中已有依赖（react/react-dom/zustand、构建期 esbuild via vite）。

## Assumptions
- 真实使用中长树形态以数百至 2,000 节点为主；10,000 节点为防御性上限而非日常目标，故全量快照协议维持不变。
- 深链场景下用户通常关注链末端，默认折叠策略（autoCollapsed 只针对完成且验收的 COMPOUND）维持现状，不新增「自动折叠中间层级」策略。
- 规模浏览器实测可复用 pkg/server 现有规模夹具生成大会话（历史会话 GET 详情返回树 JSON，无需真实 LLM）。
- 测量环境为本机 macOS + 自动化浏览器，帧时绝对值以同环境前后对比为准，阈值按实测采集口径写入证据。

## Acceptance Criteria

### AC-1: 深链不崩溃
- **Type**: `rule`
- **Given**: 一棵 20,000 节点、每节点仅一个子节点的单链树（最大深度 20,000）
- **When**: 在界面中通过历史会话打开该树
- **Then**: 不抛出 RangeError，树正常渲染（挂载行有界），可滚动到链末端并选中末节点
- **Pass Condition**: 浏览器实测打开成功且控制台无 RangeError；node 纯函数断言 20,000 链遍历不抛错
- **Evidence**: 浏览器控制台与 DOM 采样记录；node 断言脚本输出

### AC-2: 可见性计算为 O(n) 且语义不变
- **Type**: `rule`
- **Given**: 含 COMPOUND/LEAF、多级 children、completed+verified 与 failed/running 混合状态的夹具树，以及多组 manualCollapsed 组合
- **When**: 执行新的迭代扁平化纯函数
- **Then**: 输出的节点顺序、可见集合与朴素参考实现（递归 + 祖先回溯）逐索引一致；10,000 节点单链计算 ≤ 100ms
- **Pass Condition**: esbuild 打包 TS 源码后由 node 断言脚本全绿，基准输出达标
- **Evidence**: 断言脚本（含反例：折叠祖先整棵子树被剪枝）与耗时输出

### AC-3: 既有树交互零回归
- **Type**: `rule`
- **Given**: 一个包含自动折叠分支、失败分支、进行中分支的会话树
- **When**: 手动展开自动折叠节点、折叠/展开普通分支、等待树快照多次替换、选中节点后切换再切回
- **Then**: 自动折叠节点首次点击即展开；手动优先于自动；选中在快照替换后保留（节点被删除时回退 root）；挂载行数始终 ≈ 视口/行高 + overscan 决定的有界值
- **Pass Condition**: 浏览器走查各项行为；DOM 行数采样
- **Evidence**: 浏览器操作记录与 DOM 计数

### AC-4: 滚动合帧且统计不随无关事件重算
- **Type**: `rule`
- **Given**: 2,000 节点树高频接收无关 SSE 事件（llm_call/tool_call）期间进行惯性滚动
- **When**: 以编程方式派发滚动事件风暴（≥200 事件/秒持续 1 秒）
- **Then**: 每动画帧至多一次窗口状态提交（rAF 节流可见于代码与提交次数采样）；treeStats 在树引用未变时不重算（React DevTools/profiler 或插桩计数）
- **Pass Condition**: 代码走查确认 rAF 与卸载取消；浏览器插桩计数达标
- **Evidence**: 插桩输出与代码引用

### AC-5: 2,000 节点平衡树滚动流畅
- **Type**: `rule`
- **Given**: 2,000 节点平衡树、快照以 10 帧/秒持续替换
- **When**: 自动化连续滚动 1 秒以上并采集帧时
- **Then**: 平均帧时 ≤ 16.7ms；无 >50ms 长任务；挂载行数始终有界
- **Pass Condition**: 浏览器性能采样输出满足阈值
- **Evidence**: 帧时/长任务采样记录

### AC-6: 10,000 节点树可用
- **Type**: `rubric`
- **Dimension**: 超大规模树的交互可用性（打开、折叠展开、选中、定位是否可操作而非仅仅不崩）
- **Scale**: 1-5
- **Anchors**: 1 = 打开即冻结或白屏；3 = 可打开但每次更新明显停顿、操作经常丢失/迟滞数秒；5 = 全部操作即时响应、与 2,000 节点体验无主观差异
- **Pass Threshold**: >= 3（首屏可交互 ≤ 1s，无 >500ms 冻结，主路径操作可完成）
- **Evidence**: 浏览器实测各操作耗时与录屏/记录

### AC-7: 折叠/展开视口锚定
- **Type**: `rule`
- **Given**: 长列表中某节点位于视口中部且其下有大量节点
- **When**: 展开或折叠该节点
- **Then**: 该节点行操作前后的视口垂直位置差 ≤ 2px；视口下方内容按高度差平滑伸缩，页面不跳位
- **Pass Condition**: 浏览器在操作前后采样节点 getBoundingClientRect().top
- **Evidence**: 前后坐标采样记录

### AC-8: 关注节点快速定位
- **Type**: `rule`
- **Given**: 树中存在若干 FAILED 与 RUNNING/VERIFYING/DECOMPOSING 节点且分布在视口外
- **When**: 点击「下一个关注节点」多次再点「上一个」
- **Then**: 选中按可见顺序循环/折返遍历关注节点，目标自动滚动到可视区内；无关注节点时两个按钮均禁用
- **Pass Condition**: 浏览器走查选中 id 序列与滚动位置；空态按钮 disabled
- **Evidence**: 自动化操作记录

### AC-9: 键盘导航
- **Type**: `rule`
- **Given**: 树区域已获得键盘焦点
- **When**: 按 ↑/↓/←/→
- **Then**: ↑↓ 按可见顺序移动选中且目标进入视口；→ 展开折叠中的 COMPOUND；← 折叠已展开 COMPOUND；对叶子按方向键为 no-op，不报错
- **Pass Condition**: 浏览器键盘事件自动化断言选中 id 与折叠态
- **Evidence**: 自动化操作记录

### AC-10: FileViewer 万行文本窗口化收口
- **Type**: `rule`
- **Given**: 一份约 1 万行、行长不一的文本文件（≤1MiB 服务端上限内）
- **When**: 在文件浏览器中打开、滚动、拖拽改变窗口宽度
- **Then**: 挂载行有界；滚动流畅；宽度变化后换行与高度重排正确，不错位不空白；行号连续
- **Pass Condition**: 浏览器实测 DOM 行数、宽度变化前后首尾行内容正确；tsc/vite 构建通过
- **Evidence**: 浏览器实测记录；该改动独立提交

### AC-11: 构建闸门与零新依赖
- **Type**: `rule`
- **Given**: 本期全部改动完成
- **When**: 执行 web 构建与依赖核对
- **Then**: package.json/package-lock.json 无新增依赖；`npm run build`（tsc -b + vite build）通过；web/dist 为当前源码的确定性重建产物；Go 侧 `go build ./... && go test ./...` 不受影响仍全绿
- **Pass Condition**: 上述命令输出全绿，dist 哈希与本地重建一致
- **Evidence**: 构建输出与 git diff

## Open Questions
- 无（Q1 目标规模=2k 流畅/10k 可用/深链 20k 不崩；Q2 纯前端不动协议；Q3 纳入锚定+快速定位+键盘导航；Q4 FileViewer 在途改动先行独立提交——均已按推荐确认）。

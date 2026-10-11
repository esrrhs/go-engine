# 长任务树性能与导航优化 - 实施计划

## Task 0: FileViewer 万行窗口化在途改动收口
- **Status**: `completed`
- **Completion Evidence**:
  - `npm run build`（tsc -b + vite build）零错误，产物 index-C2jknR1X.js/index-Cb2Z1RCP.css 与重建哈希一致（TR-0.3）。
  - 浏览器实测 10000 行 bigfile.txt（638KB，含长行/空行混合）：稳定挂载 48 行（约 22 行可见）；滚至中部见 4999~5003、底部见第 10000 行，行号与「行 N」内容逐行对应无错位（TR-0.1）。
  - 直接压缩容器宽度 408→199px（-51%）触发 ResizeObserver：长行折行后高度重算，相邻行重叠 0、异常间隙 0、行号连续；撤销后复原一致（TR-0.2）；控制台无错误。
  - 已独立提交 c70f727（仅 FileViewer.tsx + web/dist）；临时生成器 zz_tmpgen 与临时数据已删除，git status 无残留。
- **Priority**: high
- **Depends On**: None
- **Description**:
  - 验证工作区未提交的 FileViewer.tsx 窗口化文本实现（含宽度变化失效高度缓存）行为完整；补必要的边界（文件切换、空文件已在现状中）。
  - `tsc -b` + vite build 通过后，将 FileViewer.tsx 与重建的 web/dist 作为一个独立提交先行收口（不与后续树优化混杂）。
- **Acceptance Criteria Addressed**: AC-10、AC-11
- **Test Requirements**:
  - `rule` TR-0.1: 浏览器打开约 1 万行文本，挂载行 DOM 数有界（约视口/20px + overscan 量级），行号连续，滚动后首尾行内容与行号对应正确。
  - `rule` TR-0.2: 拖拽改变窗口宽度 ≥ 20% 后，折行行高重排正确（无重叠、无大片空白、不错位）；宽度复原后内容仍正确。
  - `rule` TR-0.3: `npm run build` 零 TypeScript 错误；该提交仅含 FileViewer.tsx 与 web/dist 重建产物。
- **Notes**: 实测后记录挂载行数与宽度变化前后采样；证据记入本任务 Completion Evidence。

## Task 1: 迭代遍历与 O(n) 扁平化纯函数
- **Status**: `completed`
- **Completion Evidence**:
  - web/src/lib/tree.ts：traverse 改为显式栈迭代（逆序压栈保持前序）；新增导出 autoCollapsed 与 flattenVisible（单趟 DFS 携带 hidden 态、折叠子树直接剪枝不入栈）。
  - esbuild 打包真实源码后 node 脚本（/tmp/tp-treecheck.mjs，不进仓库）8 组 manualCollapsed 组合 + 混合态/断链/重复引用/跨父引用夹具，与朴素参考实现（递归+祖先回溯）逐索引全部一致；默认可见集合 = root,a,b,b1,b2,b21,c（TR-1.1）。
  - 20,000 单链 traverse/flattenVisible 不抛错且长度 20000；全 COMPLETED 链自动折叠仅余 root（TR-1.2）。
  - 基准实测：10k 链 flattenVisible 3.72ms（预算 ≤100ms，旧实现 5k 链即 1,211ms）；10k 平衡树 traverse 0.96ms / flatten 7.42ms（≤10ms）；root 折叠 0.00ms（剪枝生效）。
  - tsc -b 通过；验证脚本与夹具均在 /tmp，git status 无残留（TR-1.3）。
- **Priority**: high
- **Depends On**: Task 0
- **Description**:
  - 在 web/src/lib/tree.ts 以迭代（显式栈）重写 traverse，消除递归调用栈；保持根向下、children_ids 顺序、seen 去重语义。
  - 新增纯函数 `flattenVisible(tree, manualCollapsed)`：一次迭代 DFS 内携带「祖先是否已折叠」状态做子树剪枝，产出可见节点有序数组；折叠判定沿用 `manualCollapsed[id] ?? autoCollapsed(node)`，autoCollapsed 语义（COMPOUND+COMPLETED+integration_verified）原样导出复用。
  - TaskTreeView 中的内联 autoCollapsed/visible 逻辑改为引用 lib 纯函数（组件接线在 Task 2，本任务只交付 lib 函数）。
  - 临时 node 验证脚本（用 web/node_modules 的 esbuild 打包 lib/tree.ts 后执行，用后删除）：
    - 正确性：构造多级夹具树（含 leaf/compound、failed/running/completed-verified 混合、多 children 顺序、断链/孤儿保护），与朴素参考实现（递归遍历 + 逐节点祖先回溯）逐索引比对多组 manualCollapsed 组合；
    - 深链：20,000 单链不抛错且输出长度正确；
    - 基准：10,000 单链 flattenVisible ≤ 100ms，10,000 平衡树 ≤ 10ms。
- **Acceptance Criteria Addressed**: AC-1、AC-2、NFR-3
- **Test Requirements**:
  - `rule` TR-1.1: 新 traverse/flattenVisible 与朴素参考实现在全部夹具组合上输出完全一致（脚本断言退出码 0）。
  - `rule` TR-1.2: 20,000 单链遍历与扁平化不抛 RangeError；10,000 单链 flattenVisible 实测 ≤ 100ms；10,000 平衡树 ≤ 10ms（输出实测毫秒数）。
  - `rule` TR-1.3: 临时验证脚本与夹具不进入提交（git status 无其痕迹）。

## Task 2: TaskTreeView 接线纯函数并保持既有行为
- **Status**: `completed`
- **Completion Evidence**:
  - TaskTreeView 删除内联 autoCollapsed 与 nodes memo/祖先回溯，改引用 lib：`flattenVisible(tree, manualCollapsed)` 单 memo 驱动；toggle/isCollapsed 复用导出的 autoCollapsed；高度缓存/二分窗口/overscan 常量原样保留。
  - CDP 无头 Chrome（1440x900，脚本在 /tmp/tp-cdp*.mjs，不进仓库）实测，证据均为脚本原始 JSON：
    - TR-2.3：实时 2k 会话（12 组×10 组×17 叶=2040 叶，TP_DELAY_MS=120 慢速夹具）持续快照替换中，挂载行恒定 20、容器 clientHeight 505、scrollHeight 113117；头部叶子计数 943→955→965→974→984/2040 连续增长证明替换在发生。
    - TR-2.2：选中 g_00 后 3s、6s 两次复查 aria-selected 均为 g_00，期间头部持续增长；删除回退 root 路径沿用 store 既有 onEvent 逻辑（未改动）。
    - 折叠剪枝：root 折叠 20 行/113117px→1 行/505px（aria-expanded=false），展开恢复 20 行/113117px（true）。
    - TR-2.1：混合态夹具默认可见行恰为 root/done(collapsed)/failgrp/f1/run1；done 三连点击（渲染后延迟读取）：展开→d1/d2 出现、折叠→消失、再展开→出现，终态 aria-expanded=true（首次点击即展开）。
    - 失败/进行中：failgrp/f1 徽章 class border-rose-700/60 bg-rose-700/15 text-rose-300、文本「失败」，f1 错误段落原文「验收命令退出码 1：expected ok」，X 图标 text-rose-400；run1 含 animate-spin、徽章「运行中」。
    - 干净页面加载全程 pageErrors/consoleErrors 均为 []（此前浏览器代理注入坏脚本报的 SyntaxError 经 node --check 页面 bundle 证实非应用代码）。
  - tsc -b 通过。
- **Priority**: high
- **Depends On**: Task 1
- **Description**:
  - TaskTreeView 删除内联 autoCollapsed/visible 计算，改用 lib 的纯函数；nodes/visible memo 依赖项相应收敛。
  - 保持高度缓存（按 node id）、二分窗口、overscan、测量自修正、PAD/ROW_GAP 常量与现状一致。
  - 浏览器走查既有行为：自动折叠节点首次点击即展开、手动优先、跨快照选中保留（含节点消失回退 root）、挂载行有界。
- **Acceptance Criteria Addressed**: AC-3、NFR-4
- **Test Requirements**:
  - `rule` TR-2.1: 含自动折叠分支/失败分支/进行中分支的会话中，首次点击自动折叠节点即展开；刷新折叠态后普通 COMPOUND 默认态符合 autoCollapsed 规则。
  - `rule` TR-2.2: 选中某节点后多次 tree_snapshot 替换，选中 id 保持；选中节点被删除后回退 root_id。
  - `rule` TR-2.3: 2,000 节点树上挂载行 DOM 数在任意滚动位置均有界（记录最大采样值与计算窗口值的关系）。

## Task 3: 滚动 rAF 合帧与 treeStats 记忆化
- **Status**: `completed`
- **Completion Evidence**:
  - 改动：TaskTreeView 与 FileViewer 的 onScroll 统一改为「事件只调度、rAF 内读最新 scrollTop 并单次 setState」，ref 守卫保证一帧至多一次；卸载 useEffect 里 cancelAnimationFrame 挂起帧。TreeHeader 的 treeStats 包 `useMemo([tree])`。
  - TR-3.1（CDP，2200 节点静态平衡树夹具 sess_fixt_bal，容器 scrollHeight 55657）：1 秒内派发 315 个 scroll 事件、期间 62 个动画帧；插桩 requestAnimationFrame 计数：滚动期间调度 123 次，空闲基线 62 帧=62 次（React 调度器每帧 1 次），差值 61 即滚动状态提交 ≤62/秒；rAF 峰值并发仅 2。同帧连发 100 个 scroll 事件只调度 1 次 rAF（合帧硬证据）。FileViewer 万行文件（scrollHeight 199307、挂载 94 行）同样风暴 252 事件/62 帧，额外提交全部来自新挂载行的一次性高度测量自修正（heightsVer），非滚动状态提交。
  - TR-3.2（临时插桩 window.__tsCalls，测后已还原重建）：实时会话 F 上 4.09s、200ms 轮询 20 次：tree 引用替换 14 次、treeStats 重算恰好 14 次；6 个采样点 events 增长但 tree 未变，计数不增加；窗口内 events 共增长 104 条（如 ts 卡在 2、4 的采样）。
  - TR-3.3：挂起 rAF 帧的同时通过 openFile 切换中心面板卸载 TaskTreeView，500ms 内树消失/文件视图出现，pageErrors 与 consoleErrors 均 []；清理函数取消挂帧（代码走查）。
  - 插桩代码（__tsCalls/__store 暴露）已从 CenterMain.tsx 移除并重建 dist；tsc -b 与 vite build 全绿。
- **Priority**: medium
- **Depends On**: Task 2
- **Description**:
  - TaskTreeView（及 FileViewer，如其滚动仍为直写）onScroll 改为 rAF 节流：事件只更新 ref，一帧内至多一次 setState；卸载/重订阅时 cancelAnimationFrame。
  - TreeHeader 的 treeStats 包 useMemo([tree])；SessionView 因订阅 events 而重渲染时不得触发全树统计扫描（插桩或 React Profiler 验证）。
- **Acceptance Criteria Addressed**: AC-4
- **Test Requirements**:
  - `rule` TR-3.1: 编程派发 ≥200 事件/秒、持续 1 秒的滚动风暴，窗口状态提交次数 ≤ 动画帧数（约 60 次/秒量级），记录实测提交次数。
  - `rule` TR-3.2: tree 引用不变、仅 events 增长时 treeStats 调用次数为 0（插桩计数输出）；tree 替换后恰好重算 1 次。
  - `rule` TR-3.3: 组件卸载后无残留 rAF 回调执行（代码走查 + 卸载后派发滚动无 setState 警告/异常）。

## Task 4: 折叠/展开视口锚定
- **Status**: `completed`
- **Completion Evidence**:
  - 实现：toggle 时在 anchorRef 记录被操作行相对滚动容器顶的像素距离（仅 toggle 写，程序化定位不经过）；新增提交后 useLayoutEffect 一次性消费——按行实测 rect 差值补偿 scrollTop（直接写 DOM + setScrollTop 同步窗口状态，clamp 到合法区间），随后立即清除；锚点 id 已从 visible 消失（快照删除）则丢弃。
  - TR-4.1（CDP，sess_fixt_bal 2200 节点）：scrollTop 分位 0.05/0.5/0.95 三处，各取视口中部最近的展开 COMPOUND（n00074/n01061/n02039）：折叠后行 top 差 0/0/0px，画布高度实际变化 -2730/-451/-467px；再展开差 0/0/0px，画布 +2772/+475/+475px（证明子树确有显隐而非空操作），均 ≤2px。
  - TR-4.2：折叠补偿后再派发无关 scroll 事件并等 200ms，三节点漂移均 0；锚点一次性消费代码走查确认。
  - 物理边界说明：已在底部时折叠视口外子树导致的浏览器 clamp 无法把行钉回原位（下方无内容填充），补偿按新 maxScroll 收敛到底部贴合，属不可避免行为，中部节点不受影响。
  - pageErrors/consoleErrors []；tsc -b + vite build 通过。
- **Priority**: medium
- **Depends On**: Task 2
- **Description**:
  - toggle 时记录锚点（被操作节点 id 与其当前距视口顶部的像素）；visible/offsets 变化后的 useLayoutEffect 中按新 offsets 重算 scrollTop，使该节点距视口顶部保持原值（容差 ±2px）。
  - 锚点一次性消费；程序化滚动（快速定位）不得触发锚定补偿。
- **Acceptance Criteria Addressed**: AC-7
- **Test Requirements**:
  - `rule` TR-4.1: 视口中部节点展开/折叠前后 getBoundingClientRect().top 差值 ≤ 2px（分别采样展开、折叠两个方向各 3 个不同位置节点）。
  - `rule` TR-4.2: 锚点在补偿后清除，后续无关快照/滚动不产生二次位移。

## Task 5: 失败/进行中节点快速定位
- **Status**: `completed`
- **Completion Evidence**:
  - 实现：visible 派生 attention（FAILED/RUNNING/VERIFYING/DECOMPOSING），TaskTreeView 顶部新增「‹ 上一个 / 下一个 ›」控件与计数；cursor 优先跟随显式跳转、其次行选中、快照替换后节点消失则回绕到首/尾；定位 = selectNode + 程序化滚动到距顶 80px（clamp，不经锚定）；空态按钮 disabled + 文案「无失败/进行中节点」。
  - TR-5.1（混合态夹具 CDP）：初始选中 root（RUNNING 计入关注）计数 1/4；连点 5 次下一个选中序列 failgrp→f1→run1→root(回绕)→failgrp，标签 2/4、3/4、4/4、1/4、2/4；连点 3 次上一个 root→run1(回绕)→f1，顺序与可见顺序一致。
  - TR-5.2（2200 平衡树，先滚到 0.7 分位 scrollTop=38603）：下一个×8（n00000/n00013/.../n00027）目标行全部 inViewport=true、行顶统一距容器 80px（首节点 65px 近顶 clamp），首次即从 38603 跳回 0；上一个×4 反向同序且全部在视口内。
  - TR-5.3：已完成的 2k 会话（全 COMPLETED+verified 折叠）显示「无失败/进行中节点」，两按钮 disabled=true。
  - pageErrors/consoleErrors []；tsc -b + vite build 通过。
- **Priority**: medium
- **Depends On**: Task 2
- **Description**:
  - 由 visible 派生关注节点索引列表（FAILED；RUNNING/VERIFYING/DECOMPOSING），树区头部提供「上一个/下一个关注节点」控件。
  - 定位动作：selectNode(id) + 将 scrollTop 设为目标 offsets 位置（视口中上部留白），循环折返；无关注节点时按钮禁用并显示「无失败/进行中节点」提示。
  - 快照更新后关注列表随之刷新，当前定位指针在节点消失时回落到合法位置。
- **Acceptance Criteria Addressed**: AC-8
- **Test Requirements**:
  - `rule` TR-5.1: 自动化连续点击「下一个」断言选中 id 序列等于关注节点按可见顺序排列；到末尾后行为为回绕到首个（记录实现选择并与提示文案一致）。
  - `rule` TR-5.2: 每次定位后目标行进入视口（采样其边界与视口关系）；「上一个」反向同样成立。
  - `rule` TR-5.3: 无关注节点（全部 COMPLETED/SKIPPED/PENDING 以外无失败进行中）时两按钮 disabled，提示文案出现。

## Task 6: 键盘导航
- **Status**: `completed`
- **Completion Evidence**:
  - 实现：滚动容器 tabIndex=0 + focus-visible ring；onKeyDown 仅容器聚焦时响应；↑↓ 沿可见顺序移选中（无选中时 ↓ 从首个、↑ 从末个起，边界 clamp 不回绕）并最小滚动入视；→/← 仅对折叠/展开中的复合节点走与鼠标相同的 toggle（含视口锚定）路径，叶子与重复按键 no-op。
  - TR-6.1（混合态夹具 CDP）：pane.focus() 后 activeElement 即树容器；root 起 ↓↓ 选中 root→done→failgrp，↑ 回 done；done 上 → 展开（行 5→7、aria-expanded true）、再 → no-op；← 折叠回 5 行、再 ← no-op；下移到叶子 run1（无 aria-expanded）后 →← 均无变化；连续 ↑ 到 root 后再 ↑ 停在 root。
  - TR-6.1 失焦负例：焦点移到「下一个」工具栏按钮后向 body 派发 ArrowDown，事件未被 preventDefault、选中保持 failgrp、树容器 focused=false。
  - TR-6.2（2200 平衡树，先滚到 0.85 分位、无选中）：首次 ↓ 选中首可见节点 n00000 并最小回滚（scrollTop→53），连续 ↓ 选 n00001/n00002/n00011/n00012（折叠复合节点的隐藏子孙正确跳过），五行全部 inViewport；后续选中未出视口时不产生额外滚动。
  - pageErrors/consoleErrors []；tsc -b + vite build 通过。
- **Priority**: low
- **Depends On**: Task 5
- **Description**:
  - 树滚动容器 tabIndex={0}，聚焦态有可见焦点样式；↑/↓ 在 visible 中移动选中（无选中时从首个可见节点起），目标在视口外时滚动到最小必要位置；→ 展开折叠 COMPOUND（走与点击相同的 toggle/锚定路径），← 折叠展开的 COMPOUND；叶子与已处于目标态的按键为 no-op。
  - 避免与页面级滚动/按钮焦点冲突：仅容器聚焦时响应。
- **Acceptance Criteria Addressed**: AC-9
- **Test Requirements**:
  - `rule` TR-6.1: 自动化聚焦容器后派发 ↑/↓ 键，选中 id 按 visible 顺序移动且目标进入视口；边界处不报错。
  - `rule` TR-6.2: → 对折叠 COMPOUND 触发展开、← 对已展开 COMPOUND 触发折叠；对叶子按两键无状态变化、无异常。

## Task 7: 规模实测、dist 重建与提交
- **Status**: `completed`
- **Completion Evidence**:
  - TR-7.1（2193 节点平衡树，CDP 注入 SSE shim 回放真实 tree_snapshot，10 次/秒、每次克隆整树并轮转 7 个叶子 RUNNING/COMPLETED 状态，同步连续滚动 10 秒）：599 帧 / 9998ms（avg 16.67ms、p50 16.70、p95 16.70、p99 16.80、max 16.80ms），>50ms 长任务 0，挂载行恒 29，异常 0。满足平均帧时 ≤16.7ms 且无 >50ms 长任务（原始采样脚本 /tmp/tp-cdp9.mjs）。
  - TR-7.2（sess_fixt_chain20k，20000 单链全 RUNNING）：首行挂载 113ms；滚动高 1,040,121px（估计行高首次到底后自修正为 1,043,191px，二次滚到底为真实末端，属窗口化常规自修正）；末端挂出 c19994..c19999，点击选中末叶子 c19999（relTop 365），↑ 到 c19998、↓ 回 c19999；RangeError / 栈溢出 / console error 均为 0。
  - TR-7.3（sess_fixt_bal10k，10000 平衡树默认折叠视图）：点击到首行 39ms（≤1s），挂载 19 行、画布 117,002px；打开后 3 秒 settle 窗口 >500ms 冻结 0（长任务 0）；root 折叠 11ms / 展开 9ms（行 19↔1）；关注定位 ×3 均 9-10ms（n00000/n00013/n00014 入视）；键盘 ↓×5 9-11ms；4 秒连续滚动 avg 16.63 / p95 16.70 / max 16.80ms、>500ms 冻结 0。结合 Task 1 万节点 flatten 7.42ms 纯函数基准，rubric 自评 5/5（全部操作即时响应，与 2,000 节点体验无主观差异），≥3 达标。
  - TR-7.4：package.json/package-lock.json 零 diff（无新增依赖）；tsc -b + vite build 全绿；dist 重建前后 sha256 完全一致（index-BkWcLDMn.js f4ade25d…、index-BIOZdGO3.css 39e68f9b…）；`go build ./...` 通过；`go test ./...` 仅 pkg/server 一次性 TempDir RemoveAll 清理 flaky（SSE 延迟断言本身 571µs 通过），重跑包测试与 -count=3 全绿。zz_live/zz_fixt 临时夹具在提交前删除。
- **Priority**: high
- **Depends On**: Task 3、Task 4、Task 5、Task 6
- **Description**:
  - 复用/扩展 pkg/server 现有规模夹具，构造 2,000 平衡树、10,000 平衡树、20,000 单链三棵历史会话（临时夹具/脚本用后删除，不污染仓库）。
  - 浏览器自动化实测：打开耗时、滚动帧时（rAF 时长均值/最大值、长任务计数）、挂载 DOM 数、折叠/定位/键盘操作、控制台错误（重点确认无 RangeError）。
  - 采集 10,000 树首屏可交互时间与单次快照更新阻塞，按 AC-6 rubric 评分。
  - `npm run build` 确定性重建 web/dist 并核对哈希；`go build ./... && go test ./...` 复跑确认无回归；提交全部前端改动与 dist（若 Task 0 已先行提交，则本提交只含树优化）。
- **Acceptance Criteria Addressed**: AC-5、AC-6、AC-11、NFR-1、NFR-2、NFR-5、NFR-6
- **Test Requirements**:
  - `rule` TR-7.1: 2,000 平衡树、10 帧/秒快照注入下连续滚动 ≥1 秒：平均帧时 ≤ 16.7ms、无 >50ms 长任务；输出采样原始数据。
  - `rule` TR-7.2: 20,000 单链打开成功、控制台无 RangeError，可滚动至末端选中末节点。
  - `rubric` TR-7.3: 10,000 平衡树可用性；scale 1-5；anchors 同 AC-6；threshold >= 3；证据为首屏可交互耗时、操作耗时与无 >500ms 冻结的实测记录。
  - `rule` TR-7.4: package.json/package-lock.json 无新增依赖；tsc -b + vite build 全绿；dist 重建哈希一致；go build/test 全绿；临时脚本/夹具已删除（git status 干净）。

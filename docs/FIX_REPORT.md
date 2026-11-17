# Batch-0 修复报告（2026-11-16）

> 执行：5 个修复代理（W1~W5）+ 主审集成；共 6 个任务（task-6…task-12）
> 依据：`docs/AUDIT_FULL.md` §3 的 P0 清单
> 性质：**代码已修改并通过构建/类型检查**；未做真机 GUI 验证（见 §6）
> 回滚基线：`.fixbackup/20261116-1945/`（改动前的 18 个文件原样备份，含 Go 与前端）

---

## 0 概览

| 工作流 | 对应审计项 | 改动文件 | 行数(+/-) | 自测 | 状态 |
|---|---|---|---|---|---|
| W1 helper 安全加固 | S1 / S2 | `cmd/goclashz-helper/main.go` | +367 / -40 | build+vet+PASS 行为验证 | ✅ |
| W2 资产自愈加固 | U2 / U3 | `core/runtimeassets/{sync,check}.go`、`core/utils/atomic.go` | +173 / -29 | build+vet+7/7 单测 | ✅ |
| W3 日志最小闭环 | O1~O4 | `core/logger/{buffer,file}.go`、`core/appcore/log_dispatcher.go`、`wails_event_sink.go`、`main.go` | +460 / -70 | build+vet+冒烟测试 | ✅ |
| W4 前端提示面 | U4 / C1 | `frontend/src/{store.ts,App.vue,components/*,utils/errors.ts,locales/*}` | +280 / -110 | vue-tsc 0 + 状态机单测 | ✅ |
| W5 核心错误语义 | S3 / S4 / U6 / C2 | `core/tasks/manager.go`、`core/appcore/{subscriptions,controller}.go`、`core/sys/admin_win.go` | +179 / -47 | build+vet+panic 存活实验 | ✅ |
| W4b 文案修正 | 回归修复 | `frontend/src/utils/errors.ts`、`components/Overview.vue` | 小 | vue-tsc 0 + 16/16 用例 | ✅ |

**集成结果**：`go build` 0、`go vet` 0、`go test`（appcore/root）通过、前端 `pnpm run build` 0（含 vue-tsc 与 vite）。

---

## 1 逐工作流

### W1 helper 安全（S1 本地提权 / S2 白名单目录）

1. **管道 DACL**（`main.go:1392-1424`）：去掉 `(A;;GA;;;IU)`；仅 SY/BA + 服务注册的 `AllowedSids`（安装者 SID），且对 SID 做字面量校验防注入（`:1379-1390`）。`AllowedSids` 为空时**显式降级**为 `(A;;GRGW;;;IU)` 并写 stderr 警告（不静默）。
2. **start-core 白名单**（`:793-853`）：`corePath` 必须等于白名单目录下的 `clash.exe`（Clean+EqualFold）、`binDir` 必须等于该目录、PE 体积校验通过；**命令行改由 helper 重建**（`-d/-f`），客户端 `Args` 不再直通。
3. **dataDir 解析**（`:256-401`）：优先 `install-profile.json`（self-contained/portable 直取、standard 走 `os.ExpandEnv`），并新增「从服务键 `AllowedSids` → 用户 ProfileImagePath 反推 `%LOCALAPPDATA%\GoclashZ`」的补位（helper 在 SYSTEM 下算不出 UserCacheDir）；最终 `resolveDataDir` 依次 env → install-profile → .portable → 安装者 profile → `<appDir>/data`。`handleReplaceCoreFile` / `handleInstallWintun` / `handleRepairPermission` 全部改用新白名单。
4. 额外修正（为使链路真能跑通）：`allowStagedSource` 放行同目录 `<name>.new`（真实调用方就是传这个），否则替换永远被拒。

**验证**：build/vet/gofmt 全绿；把 go.mod+core+cmd 复制到临时模块做行为测试 3/3 PASS（解析出本机真实 dataDir、SDDL 无 IU-GA 且安装者获 GA、注入串被过滤、真实 clash.exe/wintun.dll 通过 PE 校验）。

**残余风险**：① 去 IU 后**第二个 Windows 用户**无法访问管道（TUN 对其不可用）——需多 SID 支持；② `<dataDir>/core/bin` 仍在用户可写目录，安装者可换掉 clash.exe 让 SYSTEM 启动 → 真正闭环需"管理员可写目录 + 签名/指纹校验"（Batch-1）；③ seed 恢复路径（`updater_orchestrator.go:355`）仍会被替换白名单拒绝——**改动前也拒绝，非回归**。

### W2 资产自愈（U2 必然失败 / U3 并发互踩）

1. **wintun shouldCopy**（`sync.go:246-265`）：`RepairInvalid` 下 wintun 改用 `!Exists || !Valid`，不再依赖恒为 false 的 `VersionProbeOK` → **TUN 运行中不再重拷 DLL**。
2. **串行化**（`sync.go:118-193`）：新增包级 `repairMu` 包住整个修复流程（锁外快判 + 进锁复核），避免两个 goroutine 同时写同一目标。
3. **分资产冷却**（`sync.go:111-172`）：包级 60s → per-asset 15s；本次要求的资产**全部**在冷却期才复用旧错误，单资产失败不再拖累 geo/core。
4. **CopyFile 加固**（`core/utils/atomic.go:59-157`）：唯一 tmp（`dst.<rand>.tmp`）+ `O_RDWR` 占用探测 + 6×250ms 退避 + 新增 `ErrAssetBusy`，失败必清理 tmp。

**验证**：build/vet 0；临时 overlay 测试 7/7 PASS（并发唯一 tmp、忙碌→ErrAssetBusy 且目标不被改写、无 tmp 残留、目录目标快速失败、冷却按资产隔离/过期失效/窗口=15s）。**仓库内未落任何测试文件**（曾误建 `undefined/temp/...` 已由主审删除）。

**残余**：`ctx` 仍不能中断复制（审计 F-A1-08）→ 等待者最坏阻塞一次 95 MB 复制；建议 Batch-1 改 `TryLock` + ctx 上限。

### W3 日志最小闭环（O1~O4）

1. 新增 `core/logger/file.go`（415 行）：`InitFileSink`、按天 `app-YYYYMMDD.jsonl`、单文件 8 MB 切分、保留 7 天、队列 4096（满则丢 debug 并计数）、`Flush()`、`Dropped()`、`StdWriter()`。
2. 落盘唯一入口挂在 `RingBuffer.Add`（锁外 `enqueueToSink`）——`logger.*`、dispatcher 两条分支、stdlib 收编都经它，避免重复写行。
3. `log_dispatcher`：先归一 → 写环（含落盘）→ 再按 UI 级别 Emit；nil 兜底补时间/来源。
4. `wails_event_sink`：`ctx==nil` 不再丢事件，进 256 容量队列，`SetContext` 后按序补发并统计失败。
5. `main.go`：**最先**初始化 sink；`log.SetOutput(io.MultiWriter(logger.StdWriter(), os.Stderr))` 收编 stdlib log（runtimeassets 的 10 处 `log.Printf` 由此入盘）；`writeBootStart` / `beginRunMarker`（残留标记 → `boot.previousRunUnclean`）/ `finishRunMarker` / `exitProcess`（15+9 处 os.Exit 统一走这里）；panic 分支先落栈再 Flush。

**验证**：build/vet 0；临时冒烟测试：JSONL 4 行可解析、ts 毫秒可还原、保留期清理、队列满丢弃计数、补发 256 条并记录失败数，全 PASS；临时测试已删除。

**残余**：① 内存环仍 500 条 + O(n) 内存搬移（F-A3-09）；② core 日志仍不过滤就推 UI（F-A3-11）；③ helper 侧无日志（Batch-1）；④ `LogEntry` 加 `Event` 字段，若发版需重新生成 bindings（本轮按要求未动）。

### W4 前端提示面（U4 开关卡死 / C1 文案匹配）

1. **模态队列**（`store.ts:476-700`）：`ModalEntry` + 单活动槽 + FIFO 队列；`showAlert`/`showConfirm` 入队；被顶掉/被外部关闭的受管模态一律以 cancel 结算（**Promise 必 settle**）；旧式直写由 `reconcileModalDisplacement()` 每 5s 兜底；新增 `openModal/confirmModal/cancelModal`，App.vue 8 处直写已迁移。
2. **去文案匹配**（`utils/errors.ts:92-129` + `Overview.vue`）：删除 `includes('Wintun')`；新增 `classifyError` 只做**分支决策**（wintun_missing / 缺少 Wintun 依赖 / ASSET_WINTUN_MISSING / helper_* / no active config → 已知），展示文案继续走 `toUserError` 的 8 条人话规则（`displayErrorMessage`，仅 unknown 且未命中规则才加「（已记录日志）」）。**装驱动只由 wintun_missing 触发**。
3. **开关看门狗**（`store.ts:1076-1108`）：`tunPending`/`systemProxyPending` 超 60s 强制复位。
4. SettingsTun：mips 回退改为 loading 复位后 `saveTunCore(true)` 落盘（修 F-A4-05）；SettingsLan/Network 保存失败补 toast（修 F-A4-06）。

**验证**：`vue-tsc --noEmit` 0；`pnpm run build` 0（manifest 94 锚点 + 169 模块 + 产物生成）；模态状态机 16/16、错误分类 16/16 用例 PASS。

**残余**：`Subscriptions.vue` 两处直写、「取消也导出」留 Batch-1；8 语言 `common.unknownError` 键已成死键（未清理）。

### W5 核心错误语义（S3 任务 panic / S4 退出清理 / U6 订阅假成功 / C2 修复失败升级）

1. `core/tasks/manager.go`：goroutine 首行 `defer recover()`，panic → 记栈 + 释放任务名 + 发 `-error`；抽出 `run()/release()/emit()`，事件名与顺序零变化。
2. `subscriptions.go:58-86`：逐个收集失败 `errors.Join` 返回；有失败不发 `-success`。
3. `core/sys/admin_win.go`：新增 `SetBeforeElevatedExit`，两处 `os.Exit(0)` 前调用；`controller.go:245` 注册清理钩子（3s 上限，超时至少摘系统代理）。
4. `controller.go:667-684`：资产修复报错时，只有 core/wintun **真正 !Ready** 才硬失败；否则 `logWarn` 继续（修复瑕疵不再变成开关失败）。
5. `controller.go:1384-1405`：`notify()` 去重分支也写日志（`[通知已去重]`），未去重按级别写 `logError/logWarn` 后再 Emit。

**验证**：build/vet 0；临时模块行为测试：任务 panic 后进程存活、`-error` 事件带原因、任务名释放、取消/失败语义不变，全 PASS。

**残余**：清理钩子是全局函数变量（多 Controller 时后者胜出，生产单实例可接受）；3s 超时后内核可能残留到下次启动由 watchdog 收敛。

---

## 2 集成验证（主审执行）

| 检查 | 命令 | 结果 |
|---|---|---|
| Go 构建 | `go build ./core/... ./cmd/... .` | **exit 0** |
| Go 静态检查 | `go vet ./core/... ./cmd/... .` | **exit 0** |
| Go 测试 | `go test ./core/appcore/ ./core/logger/ ./core/runtimeassets/ ./core/tasks/ ./core/utils/ .` | **exit 0**（appcore 0.87s、root 1.79s） |
| 格式 | `gofmt -l` 全部改动文件 | 空（controller.go 的 gofmt 差异为改动前既有） |
| 前端类型 | `vue-tsc --noEmit`（workdir=frontend） | **exit 0** |
| 前端构建 | `pnpm -C frontend run build` | **exit 0**（manifest OK / 169 模块 / dist 产出） |
| 越界检查 | 变更文件清单 vs 各任务写范围 | 全部在范围内（见 §3） |
| 产物检查 | 仓库根目录 / `undefined/` 残留 | 无（自测遗留目录已删除） |

> 说明：`go build ./...` 会命中 `build/ios`（预存在的跨平台占位包，Windows 上无 main），与本轮改动无关；集成改用 `./core/... ./cmd/... .`。

---

## 3 变更文件清单（vs 备份）

| 文件 | +/− |
|---|---|
| `cmd/goclashz-helper/main.go` | +367 / −40 |
| `core/runtimeassets/sync.go` | +98 / −17 |
| `core/runtimeassets/check.go` | +5 / −0（仅 TODO 注释） |
| `core/utils/atomic.go` | +70 / −12 |
| `core/logger/file.go`（新） | 415 行 |
| `core/logger/buffer.go` | +9 / −2 |
| `core/appcore/log_dispatcher.go` | +23 / −21 |
| `wails_event_sink.go` | +72 / −3 |
| `main.go` | +144 / −25 |
| `core/tasks/manager.go` | +64 / −42 |
| `core/appcore/subscriptions.go` | +23 / −3 |
| `core/sys/admin_win.go` | +36 / −1 |
| `core/appcore/controller.go` | +56 / −1 |
| `frontend/src/store.ts` | +236 / −24 |
| `frontend/src/App.vue` | +28 / −65 |
| `frontend/src/components/Overview.vue` | +23 / −15 |
| `frontend/src/components/SettingsTun.vue` | +15 / −4 |
| `frontend/src/utils/errors.ts`、`SettingsLan.vue`、`SettingsNetwork.vue`、`locales/*.ts`×8 | 小改 |
| `frontend/dist/*` | 由 `vite build` 重新生成 |

---

## 4 回滚

- 基线：`.fixbackup/20261116-1945/`（保留相对路径）。
- 单文件回滚：把备份文件复制回同名路径即可；`core/logger/file.go` 为新增文件，回滚时删除并还原 `buffer.go/main.go`。
- 前端回滚：还原 `store.ts/App.vue/Overview.vue/SettingsTun.vue/errors.ts` 后重跑 `pnpm -C frontend run build`；locales 只需删掉 `unknownError` 键。
- 仓库无 git 提交历史，**不要**用 `git checkout` 回滚。

---

## 5 未完成 → Batch-1（已在 AUDIT_FULL §6 排期）

1. helper：多 SID 支持、core 目录签名/指纹校验、helper 自身日志、`install-wintun` 前停内核。
2. runtimeassets：`ctx` 可中断、`TryLock`、PendingReplace（运行中不替换被映射文件）、移除检查路径里的 `adoptAssetFromFallback`。
3. 日志：内存环 500→2000 + 分级配额 + 事件合并推送、core 日志按级别过滤、诊断包（`docs/FIX_REPORT` 外）。
4. 核心：4 轮 `EnsureReady` 收敛（`GetStatusCached` + L0/L1 非阻塞）、两段式 TUN + phase 事件。
5. 前端：`Subscriptions.vue` 直写与「取消也导出」、8 语言错误文案、bindings 重新生成（`LogEntry.Event`）。

---

## 6 需要真机人工验证（本轮无法自动验证）

| # | 场景 | 期望 |
|---|---|---|
| 1 | 冷启动后首次开 TUN | 不再弹「缺少 Wintun 驱动」；失败时最多是 C 类明确错误 |
| 2 | 设置→组件与库更新 → 装驱动 | 成功且 DLL 版本刷新；失败有 toast + 日志 |
| 3 | 以管理员重启（开系统代理时） | 退出前系统代理被摘除，重启后不残留 |
| 4 | 断网「更新全部订阅」 | 报错而不是"成功"；任务行显示失败 |
| 5 | 日志目录 | `<dataDir>/logs/app-YYYYMMDD.jsonl` 有内容；退出后仍可查 |
| 6 | 异常退出（任务管理器杀进程） | 下次启动日志出现 `boot.previousRunUnclean` |
| 7 | 多 Windows 用户 | **预期风险**：非安装者无法用 TUN（需 Batch-1 多 SID） |
| 8 | 开关连点/弹窗打断 | TUN 开关不再永久卡死（60s 看门狗兜底） |

---

## 7 风险登记

| 风险 | 影响 | 缓解 |
|---|---|---|
| SDDL 去 IU 后第二用户不能用 TUN | 中（多用户场景） | 文档说明 + Batch-1 多 SID |
| 修复串行化最坏阻塞一次 95 MB 复制 | 低（只在修复路径） | Batch-1 ctx 可中断 |
| 模态队列让后台提示延迟到确认框之后 | 低 | 队列 FIFO + 5s 兜底 |
| 看门狗 60s 复位可能导致重复执行一次安装/重试 | 低（幂等） | 保留，观察日志 |
| 日志写盘在 `RingBuffer.Add` 上挂钩 | 无（唯一入口，避免重复行） | 已在报告中说明，如需改回 3 行 |

---

*本轮只做 Batch-0：安全止血 + 可复盘 + 误报面收敛。所有改动均可用 `.fixbackup/` 单文件回滚。*

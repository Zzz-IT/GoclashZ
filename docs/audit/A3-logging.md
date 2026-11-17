# A3 日志与可观测性审计（严格检查 · 静默自愈 · 可回放日志）

> 审计对象：GoclashZ（Go + Wails v3 后端 / Vue3 前端），仓库根 `D:\GOLANG\GoclashZv3`
> 审计范围：`core/logger/**`、`core/appcore/{logs,log_dispatcher,diagnostics,events}.go`、`wails_event_sink.go`、`v3_events.go`、`main.go`、`app.go` 的日志/诊断代码、`frontend/src/App.vue` 日志面板段；并核实 `data/logs` 是否真的落盘
> 性质：**只读审计**。未运行应用、未构建、未修改任何源码/配置；唯一写入为本文件
> 证据等级：**A** = 源码/文件系统可确证；**B** = 代码 + 依赖实现推断（未运行时实测）；**C** = 假设，待实测
> 路径约定：除特别标注 `[外部依赖]` 的条目外，均为仓库相对路径

---

## 0 覆盖范围与方法

### 0.1 实际读过的文件（全文件精读，非抽样）

| 文件 | 行数 | 读法 |
|---|---|---|
| `core/logger/buffer.go` | 125 | 全文 |
| `core/logger/level.go` | 46 | 全文 |
| `core/appcore/logs.go` | 91 | 全文 |
| `core/appcore/log_dispatcher.go` | 66 | 全文 |
| `core/appcore/diagnostics.go` | 97 | 全文 |
| `core/appcore/events.go` | 28 | 全文 |
| `wails_event_sink.go` | 52 | 全文 |
| `v3_events.go` | 184 | 全文（日志相关注册点 :50） |
| `main.go` | 408 | 全文 |
| `app.go` | 1472 | 全文（日志/诊断/退出段逐行） |
| `frontend/src/App.vue` | — | 日志面板段 :119-143、:367-408、:676-691、:841-844 |
| `core/utils/paths.go` | 277 | 全文（数据目录与 `logs` 目录创建） |
| `core/sys/data_dir.go` | 125 | 全文 |
| `core/utils/data_migration.go` | — | :215-290（`logs` 被迁移逻辑跳过） |
| `core/clash/api_client.go` | — | :30-70（client 超时）、:205-299（`FetchLogs` 全量） |
| `core/clash/api_extra.go` | 70 | :1-70（第二条 WS 日志流，死代码） |
| `core/clash/runner.go` | — | :20-80（`ExitEvent`）、:110-220（启动路径）、:240-360（`-t` 与 spawn） |
| `cmd/goclashz-helper/main.go` | — | :1-120（控制台/日志面）、:160-260（`superviseCore`）、:560-640（内核 spawn）、:755-795（`core-status`） |
| `core/sys/helper_protocol.go` | — | :90-140（`CoreStatusData`） |
| `core/sys/helper_client_windows.go` | — | :20-90（超时表）、:220-262（`CheckHelperService`） |
| `core/sys/admin_win.go` | — | :195-231（`WriteAdminTaskResult`） |
| `core/tasks/manager.go` | 122 | 全文（任务失败只发事件） |
| `core/appcore/controller.go` | — | :200-290（dispatcher 接线）、:1330-1380（`notify` 去重）、:2060-2100（级别变更）、:2138-2183（日志 API）、:2438-2470（诊断导出） |
| `core/appcore/behavior.go` | — | :16-60（`AppLogLevel`/`LogLevel`）、:87-130（`Get`） |
| `core/appcore/streams.go` | — | :315-335（traffic 错误去重 + `AddApp`） |
| `frontend/src/components/SettingsAbout.vue` | — | :438-462（导出诊断交互） |
| `frontend/src/store.ts` | — | :740-790（`app-notification` 处理） |
| `docs/SELF_HEAL_AND_LOGGING_PLAN.md` | 621 | 全文（逐条与代码对账） |
| `[外部依赖]` wails v3.1.0 `pkg/application/{events,event_manager,transport_event_ipc,webview_window,application,application_options}.go`、`internal/mailbox/mailbox.go`、`logger_prod.go` | — | 关键段（Emit → mailbox → JS 注入 全链） |

### 0.2 检索手段

1. 正则全仓检索（ripgrep 语义）：`logger\.(Infof|Errorf|Warnf|Debugf)`（57 处）、`c\.(logInfo|logWarn|logError)|AddApp`（42 处）、`os\.OpenFile|os\.Create|log\.New\(|log/slog|"log"`、`rotate|rotation|MaxSize|retention`（**0 命中**）、`redact`（**0 命中**）、`"logs"`、`Stdout|Stderr|Stdin`。
2. 调用点图：对 `AppLogs`、`GetRecentLogs/SearchLogs/ClearLogs`、`StartLogStream/StopLogStream`、`FetchLogs`、`ExitEvent`、`CoreStatusData` 逐一确认调用方，识别死代码。
3. **只读实测**（未启动应用）：
   - `data/logs` 落盘核实：`D:\GoclashZ\data\logs`（安装版数据目录）与 `C:\Users\Jiguang\AppData\Local\GoclashZ\logs`（缓存版数据目录，由 `core/utils/paths.go:115-118` 推导）**两者均为 0 个条目**；仓库内 `data/` 只有 `core/`，无 `logs/`。
   - 二进制子系统：读 `bin\GoclashZ.exe` PE 头 `Subsystem = 2 (WINDOWS_GUI)`、`bin\GoclashZHelper.exe = 3 (CONSOLE)`。
   - `.gitignore:17` 已忽略 `*.log`、`.gitignore:24-27` 忽略 `/data/*`（说明"日志不该进库"，但代码也从未写过）。
4. 与主方案逐条对账：把 `docs/SELF_HEAL_AND_LOGGING_PLAN.md` §1.3 / §6 / §7 / §10.3 / §11.3 的每条断言拉出来，用源码行号验证（见 §3.3）。

### 0.3 覆盖度自评

| 子系统 | 覆盖度 | 等级 | 说明 |
|---|---|---|---|
| 日志内核（logger 包 / 环 / 级别） | **100%** | A | 2 文件合计 171 行全读 |
| 日志分发（dispatcher / stream manager / 事件出口） | **100%** | A | 含 v3 `Emit` 到前端的完整链路（依赖侧读到 mailbox 与 window 队列） |
| 诊断快照与导出 | **100%** | A | 后端 + 前端交互全读 |
| 崩溃 / 异常退出路径 | **95%** | A | 覆盖 `main`/`app.go`/tray 的 recover 与 `os.Exit`；托盘线程细节未逐行 |
| 内核进程日志（stdout/stderr、`/logs` 流） | **90%** | A | 两条启动路径（helper / 直连）均确认；运行期流式实现读全 |
| helper 侧日志 | **85%** | A | 全仓 24 处输出点已枚举；服务态 stderr 去向待实机（B） |
| 前端日志面板 | **100%** | A | 模板 + 状态 + 事件订阅 + i18n key 全读 |
| 日志容量/轮转/量级实测 | **30%** | C | 不能运行应用，只能由代码推断量级 |

### 0.4 一句话结论

> **"可回放"目前是从 0 开始：没有任何一条日志会离开进程内存，而进程内存里那 500 条，还漏掉了三类最关键证据（资产自检、内核启动失败、helper 与框架错误）。**

---

## 1 链路清单表

### 1.1 日志/诊断链路（入口 → 实际做了什么）

| # | 入口 / 场景 | file:line | 严格度 | 单次成本 | 是否阻塞用户 | 误报面 | 触发频率 |
|---|---|---|---|---|---|---|---|
| 1 | App 侧写日志 `logger.Infof/Errorf/Warnf` | `core/logger/buffer.go:91-116` | — | 4 字符串拼接 + 环写入（满后 O(500) 复制，见 F-A3-09） | 否 | — | 全仓 57 处调用点 |
| 2 | App 侧写日志（经 dispatcher）`AddApp` | `core/appcore/log_dispatcher.go:30-47` | 按 `AppLogLevel` 过滤后 emit | 读 `Behavior.Get()`（RLock + 结构体拷贝）+ 环写入 + 可能 1 次 `Event.Emit` | 否 | 级别只影响是否推前端，不影响入环 | 全仓 42 处 |
| 3 | Core 侧写日志 `AddCore` | `core/appcore/log_dispatcher.go:49-66` | **不过滤** | 环写入 + **无条件** 1 次 `Event.Emit` | 否 | 用户把 core 日志调到 info/debug 时 UI 与 IPC 直接打满（F-A3-10/11） | 内核每行日志（默认 `log-level: error`，`core/clash/config.go:622`） |
| 4 | 内核日志流接入 `FetchLogs` | `core/clash/api_client.go:206-299` | 严（无 batch、无缓冲） | 1 条长连接；空闲 70s 断连；失败每 2s 重连 | 否 | 内核未起时静默重连（无提示，F-A3-17） | UI 挂载时启动一次（`frontend/src/App.vue:679`） |
| 5 | 日志流生命周期 `StartLogStream`/`StopLogStream` | `core/appcore/controller.go:2148-2158`、`core/appcore/logs.go:26-85` | 中（generation 防并发） | 1 协程 + 1 长连接 | 否 | 重复 Start 会重建流（`controller.go:2073-2075`）；`StopLogStream` **无调用方**（死接口） | 挂载 / 改级别 |
| 6 | 前端取历史 `GetRecentLogs` | `core/appcore/controller.go:2160-2162` → `frontend/src/App.vue:676` | 全量 | 500 条深拷贝 + 跨进程 JSON（约 100 KB 级，B） | 是（一次 await，毫秒级） | 环里已有内容可能已被 core 日志挤掉 | 应用挂载 1 次 |
| 7 | 前端搜索 `SearchLogs` | `core/appcore/controller.go:2164-2166`、`core/logger/buffer.go:66-79` | 全量线性 | O(n) × 每条 `ToLower` 分配 | 是 | 前端**从未调用**（死接口，F-A3-16） | 0 |
| 8 | 前端清空 `ClearLogs` | `core/appcore/controller.go:2168-2170`、`frontend/src/App.vue:841-844` | 全量销毁 | O(1) 重新分配 | 否 | 无二次确认即抹掉唯一证据（P2） | 用户点击 |
| 9 | 实时推送 `EventLogMessage` | `core/appcore/log_dispatcher.go:44-46,63-65`；`wails_event_sink.go:26-52`；`v3_events.go:50` | 每条一次 | JSON 序列化 + 每条一次 JS 注入（`[外部依赖] webview_window.go:1517`） | 否（异步 mailbox） | UI 跟不上时 v3 mailbox **无界增长**（F-A3-10） | 每条日志 |
| 10 | 早期事件拦截 `WailsEventSink.Emit` | `wails_event_sink.go:35-39` | **丢弃** | — | 否 | `ctx==nil` 时直接 return，**不计数、不落盘**（F-A3-12） | 启动期全部事件 |
| 11 | 用户可见通知 `notify` | `core/appcore/controller.go:1362-1375`（去重 :1332-1360） | 30s 去重 | 1 次 map 查找 + 2 次 emit | 否 | **完全不写日志**；去重时连"被抑制"都不记（F-A3-05） | 9 处调用点 |
| 12 | 崩溃兜底 | `main.go:37-43`；`app.go:1436-1472` | 尽力 | 1 次环写入 | 否 | 只写内存环、不抓栈；`panic(r)` 后进程即死（F-A3-02） | 崩溃时 |
| 13 | 正常/信号退出 | `main.go:391-408`、`app.go:251-276`、`app.go:1456-1472` | — | — | 否 | 无 `boot.exit{clean:true}` 标记（F-A3-02） | 每次退出 |
| 14 | 诊断快照 `GetDiagnosticInfo` | `core/appcore/diagnostics.go:49-88` | 全量 | SCM 查询 ×2 + helper ping（≤350ms，`core/sys/helper_client_windows.go:20,30-32`）+ 6×`os.Stat` + manifest/asset-state 读取 | 是（同步 RPC） | 内容仅为路径与布尔值，无日志/版本/系统信息（F-A3-14） | 打开"关于"诊断弹窗（`SettingsAbout.vue:444-451`） |
| 15 | 诊断导出 `ExportDiagnostics` | `core/appcore/controller.go:2444-2464` → `core/appcore/diagnostics.go:90-97` | 全量 | 系统保存对话框（用户耗时）+ 上述快照 + 一次 `os.WriteFile`（0644、非原子） | 是（模态对话框） | 取消时返回 nil → 前端弹"导出成功"（F-A3-15） | 用户点击 |
| 16 | 前端日志面板渲染 | `frontend/src/App.vue:119-143,367-408,676-691` | — | 每条 push 触发响应式 + 每次变更 O(500) 过滤 + 100ms 合并滚动 | 否 | 无级别下拉/搜索/暂停/导出（F-A3-16） | 每条日志 |
| 17 | 资产自检/自愈日志 | `core/runtimeassets/{check.go:83,86,222; migrate.go:129,138; sync.go:166,168,196,200,216}` | — | 1 次 `log.Printf` | 否 | **写入 os.Stderr，GUI 进程无控制台 → 全部丢失**（F-A3-03） | 每次 EnsureReady |

### 1.2 现状能力矩阵（"可回放"六要素）

| 能力 | 现状 | 证据 |
|---|---|---|
| 落盘 | **无**。`data/logs` 由 `core/utils/paths.go:61` 创建，全仓 0 处写入；两个真实数据目录实测为空 | `core/utils/paths.go:55-63`；`core/utils/data_migration.go:230,274`（迁移逻辑主动 Skip `logs` 目录） |
| 容量 | 内存环 **500 条固定**，无分级配额、无按类别保留，先进先出 | `core/logger/buffer.go:27-51,82` |
| 轮转/保留 | **完全不存在**（`rotate|MaxSize|MaxAge|retention` 全仓 0 命中） | 检索结果 |
| 时间精度 | `HH:MM:SS`，无日期/毫秒/时区/PID | `core/logger/buffer.go:96,105,114,123`；`core/appcore/log_dispatcher.go:39,58`（共 6 处） |
| 结构字段 | 仅 `type/source/payload/time` 4 个字符串；无 category/event/code/corr/dur/attempt/pid/caller | `core/logger/buffer.go:19-24` |
| 崩溃可回放 | 无栈、无退出码、无 clean 标记、无 `previousRunUnclean` | `main.go:37-43`、`app.go:1456-1472` |
| 诊断包 | 单点 JSON 快照，无日志、无 zip、无脱敏模块 | `core/appcore/diagnostics.go:49-97`；`redact` 全仓 0 命中 |
| 用户自助 | 仅"另存为 JSON"；无打开日志目录、无日志导出、无搜索 | `frontend/src/App.vue:137-142`、`frontend/src/components/SettingsAbout.vue:453-462` |

---

## 2 问题清单（21 条，按严重度排序）

### P0（阻断"可回放"目标本身）

#### F-A3-01 全部日志只存在于内存，进程退出即归零；`data/logs` 只建不写
- **严重度**：P0 ｜ **证据等级**：A
- **file:line**：`core/logger/buffer.go:26-82`（`RingBuffer(500)` + 全局单例 `:82`）、`core/utils/paths.go:55-63`（`:61` 创建 `logs` 目录）、`core/utils/data_migration.go:230,274`（迁移把 `logs` 当可丢弃目录）、`core/appcore/log_dispatcher.go:42,61`（唯一写入点，均为 `AppLogs.Add`）
- **现象**：全仓没有任何文件日志实现——`os.OpenFile`/`os.Create` 的 8 处调用点全部用于资产/备份/原子写，与日志无关；`log.New(`、`log/slog`、`rotate` 均 0 命中。日志唯一归宿是 500 条内存环。只读实测：安装版 `D:\GoclashZ\data\logs` 与缓存版 `%LOCALAPPDATA%\GoclashZ\logs` 均 **0 个条目**。
- **影响**：事故（如"开 TUN 首判缺少 Wintun"）在进程退出后**任何证据都不存在**，只能靠文件系统时间戳反推——这正是主方案 §0.2 复盘卡住的直接原因。同时"500 条被 core 日志冲爆"意味着连进程内也不可靠。
- **修法**：新增 `core/logger/file.go`，提供 `type Sink interface{ Write(Entry) }`；在 `core/logger/buffer.go:82` 的 `AppLogs` 旁增加 `var FileSink *JSONLWriter`，并在 `RingBuffer.Add`（`core/logger/buffer.go:41`）末尾同步投递（channel 异步，容量 4096，满则丢弃 debug 并计数）。写入路径固定为 `filepath.Join(utils.GetDataDir(),"logs")`（复用 `core/utils/paths.go:61` 已创建的目录）。**不要**在 `runtimeassets` 里继续用 `log.Printf`（见 F-A3-03）。

#### F-A3-02 崩溃与所有异常退出路径的日志必然丢失：不抓栈、无退出标记、`os.Exit` 前只写内存
- **严重度**：P0 ｜ **证据等级**：A
- **file:line**：`main.go:37-43`（`recover` → `logger.Errorf` → `panic(r)`）、`main.go:385-387`（`Run()` 失败只 `logger.Errorf`）、`main.go:56-58,65-100,102-148,150-210,391-408`（多处 `os.Exit` 前后仅环写入）、`app.go:1456-1472`（`safeQuit` 8s 兜底 + `os.Exit(0)`）
- **现象**：panic 处理只写 `%v`，没有 `debug.Stack()`；随后 `panic(r)` 让进程直接终止，环内容随进程消失；GUI 子系统（实测 `bin\GoclashZ.exe` Subsystem=2）没有 stderr，panic 也进不了任何控制台。所有 `--repair-*`、`--install-helper` 等 `os.Exit(1)` 路径同理：错误只进环，进程立即退出。启动阶段也没有任何 boot.start / boot.exit{clean} 记录，因此**无法判断上一次是否异常退出**。
- **影响**：主方案 §6.4 要求的"崩溃标记 + previousRunUnclean"完全没有基础；用户报"双击没反应"时零线索。
- **修法**：`main.go:37-43` 改为 `logger.Errorf("panic: %v\n%s", r, debug.Stack())` + `logger.Flush()`（新增，同步写盘）后再 `sys.ClearOwnedSystemProxy()`；新增 `core/logger/lifecycle.go`：启动写 `boot.start{version,appDir,dataDir,isAdmin,silent,startup}`、正常退出写 `boot.exit{clean:true}`，下次启动读到无 clean 则补 `boot.previousRunUnclean` 并常驻 warn。所有 `os.Exit` 前统一调用 `logger.Flush()`（可做成 `exitWith` 辅助函数）。

#### F-A3-03 资产自检/自愈日志走标准库 `log`，输出到 GUI 进程不存在的 stderr——最需要复盘的一段完全不可见
- **严重度**：P0 ｜ **证据等级**：A
- **file:line**：`core/runtimeassets/check.go:9`、`core/runtimeassets/migrate.go:4`、`core/runtimeassets/sync.go:11`（`import "log"`）；写点 `check.go:83,86,222`、`migrate.go:129,138`、`sync.go:166,168,196,200,216`
- **现象**：`runtimeassets` 是"严格检查 + 静默自愈"的核心子系统，它的日志全部通过 `log.Printf("[runtimeassets] ...")` 输出。全仓 **没有 `log.SetOutput`**（`log.New(` 也 0 命中），所以默认写 `os.Stderr`；而主进程是 `Subsystem=2`（GUI），stderr 句柄无效，写失败被 log 包静默忽略。这些行**既不入环、也不推前端、更不落盘**。
- **影响**：事故中"到底哪一行判了 missing / 哪次 CopyFile 失败"属于该子系统的输出，恰好**一行都留不下来**。这条比"没有文件日志"更致命——即使马上加了文件日志，只要不换掉 `log.Printf`，自愈过程仍然是黑的。
- **修法**：把 `core/runtimeassets/{check.go:83,86,222; migrate.go:129,138; sync.go:166,168,196,200,216}` 的 10 处 `log.Printf` 换成 `logger.Infof/Warnf`（`core/logger` 已 import，成本极低）；并在 `core/runtimeassets` 内部加 probe/repair/verify 三个结构化事件（asset、level、ready、dur_ms、attempt），供 F-A3-01 的 JSONL 落盘。

#### F-A3-04 内核进程 stdout/stderr 被主动丢弃，启动失败原因永久消失
- **严重度**：P0 ｜ **证据等级**：A
- **file:line**：`cmd/goclashz-helper/main.go:607-613`（`cmd.Stdout = devNull` / `cmd.Stderr = devNull`）、`core/clash/runner.go:313-323`（直连模式 `exec.Command` + `utils.HideCommandWindow`，从不设置 Stdout/Stderr）、`core/utils/process_windows.go:19-21`（`CREATE_NO_WINDOW` = 0x08000000）
- **现象**：两条启动路径都拿不到内核输出：helper 显式重定向到 `NUL`；直连模式让内核继承 GUI 父进程的无效句柄。运行期唯一的日志通道是内核的 `/logs` HTTP 流（`core/clash/api_client.go:206-217`），而它**只有在内核成功起来、external-controller 监听之后才可用**。
- **影响**：构成完整的证据缺口——内核**起不来**（配置错、端口占用、wintun 初始化失败、被杀软拦）时，原因既不在 stdout（被丢到 NUL），也不在 `/logs`（连不上），也不在 UI（无记录）。`core/clash/runner.go:185-187` 只回一句固定文案"内核已异常退出"，无退出码、无耗时。
- **修法**：`helper/main.go:609-616` 改为把 `cmd.Stdout/cmd.Stderr` 接到 `io.Pipe`，由 helper 用一个最小 ring + file（写入 `<dataDir>/logs/helper-core-*.log`）落盘，并通过 `core-status` 暴露最近 N 行；直连路径 `core/clash/runner.go:319-323` 同样接 pipe，交给主进程 logger 落盘。

#### F-A3-05 用户能看到的每一条错误通知，日志里都不存在
- **严重度**：P0 ｜ **证据等级**：A
- **file:line**：`core/appcore/controller.go:1362-1375`（`notify` 只 `Emit`，无 `AddApp`）、`core/appcore/controller.go:1332-1360`（`shouldSuppressNotify` 命中即 return）、`frontend/src/store.ts:749-777`（前端只决定是否弹模态）
- **现象**：`notify(level, code, message, source)` 是用户可见层的唯一出口（9 处调用点），它把 `Notification` 发成事件后就结束了；既没有写环，也没有写文件。去重命中时更是直接 return，连"这条被抑制了"都不记。
- **影响**：用户说"弹了个错误"，日志面板里找不到对应行；时间线在最关键的一跳断开。这与主方案 §2.2 原则 5"可回放优先"直接冲突，也是方案 §7 #3 想解决但**低估了范围**的地方（方案只要求去重时补记，实际连非去重也不记）。
- **修法**：`core/appcore/controller.go:1362` 函数体开头插入 `c.appLogger.AddApp(logger.LogEntry{Type: level, Payload: fmt.Sprintf("[%s][%s] %s", source, code, message)})`（结构化版本见 F-A3-08 的 err.raise 事件），并在 `:1363-1365` 的去重分支写一条 `suppressed:true` 记录。注意 `notify` 可能从无 `appLogger` 的早期路径被调用，需 nil 保护（参考 `controller.go:267` 的写法）。

#### F-A3-06 Wails 框架层的日志与错误被默认丢弃，事件类型不匹配也静默
- **严重度**：P0 ｜ **证据等级**：A
- **file:line**：`main.go:297-320`（`application.Options` 只设了 Name/Description/Icon/Services/Assets/Windows/ShouldQuit/SingleInstance）、`[外部依赖] application_options.go:55-60,103-104`（`Logger *slog.Logger`、`LogLevel`、`ErrorHandler`）、`[外部依赖] application.go:62-66`（未设置时用 `DefaultLogger`）、`[外部依赖] logger_prod.go:10-12`（production 构建 = `slog.NewTextHandler(io.Discard, nil)`）、`[外部依赖] application.go` 的 `handleError` → `a.Logger.Error(...)`
- **现象**：`Options.Logger` 与 `Options.ErrorHandler` 都未接线。dev 构建默认写 stderr（GUI 无控制台）；production 构建默认 `io.Discard`——**框架错误直接丢弃**。`v3_events.go:12-21` 的类型校验一旦与 payload 不符，`Emit` 返回 error → `handleError` → 丢弃，事件静默消失（仅靠 `wails_event_sink.go:41-45` 的 recover 兜住 panic 后再写环，而环终归要丢）。
- **影响**：WebView2 初始化失败、绑定方法异常、事件类型漂移、单实例回调异常等，全部无痕；这层恰是"双击没反应"最可能的落点。
- **修法**：`main.go:297` 的 `application.Options` 增加两个字段：`Logger: slog.New(<转发到 core/logger 的 handler>)`、`ErrorHandler: func(err error){ logger.Errorf("[wails] %v", err) }`（adapter 可写在 `core/logger/slog.go`，`Handle` 里调 `AppLogs.Add` + 文件 sink）。

### P1（结构缺陷，直接限制可诊断性）

#### F-A3-07 时间精度不足：无日期、无毫秒、无时区、无 PID
- **严重度**：P1 ｜ **证据等级**：A
- **file:line**：`core/logger/buffer.go:96,105,114,123`、`core/appcore/log_dispatcher.go:39,58`（共 6 处 `time.Now().Format("15:04:05")`）
- **现象**：时间只有一个 `HH:MM:SS` 字符串。跨天、跨次启动无法排序；与文件 mtime（如事故里 19:00:33）、与内核/helper/事件时间**无法做毫秒级对齐**；多进程（app / helper / core）也没有 pid 区分。
- **影响**：事故复盘的核心动作是"把判据按时间排成因果链"，秒级且无日期的时间戳做不到；主方案 §6.2 要求的 `ts: ISO8601 带毫秒与时区` 完全缺失。
- **修法**：`core/logger/buffer.go:19-24` 的 `LogEntry.Time` 改为 `time.Time`（JSON 序列化为 RFC3339Nano），或保留 string 但改 `time.Now().Format("2006-01-02T15:04:05.000Z07:00")`；6 处格式化收敛为 logger 包内一个 `nowStamp()`；同时给 `LogEntry` 增加 `PID int` 与 `Role string`。

#### F-A3-08 日志条目没有结构：无类别、无事件名、无错误码、无关联 ID、无耗时/重试/调用点
- **严重度**：P1 ｜ **证据等级**：A
- **file:line**：`core/logger/buffer.go:19-24`（`Type/Source/Payload/Time` 四个 string）、`core/appcore/log_dispatcher.go:36-40,55-59`（只做 level 归一与 source 赋值）、全仓 99 处自由文本调用（57 处 `logger.*` + 42 处 `c.log*`）
- **现象**：所有诊断信息都压进 `Payload` 自由文本，中文与英文混排，变量用 `%v` 内联（如 `controller.go:1782` 的 6 个占位符）。没有 cat（asset/helper/core/tun/proxy/update/ui/boot/config）、没有稳定 event 名、没有 code、没有 corr，也没有"谁写的"（caller）与"耗时多少"（dur_ms）。
- **影响**：无法机器化聚合（"ASSET_WINTUN_BUSY 今天发生几次"）、无法按 code 决定 UI 策略（主方案 §5 的地基）、无法把一次用户动作的多条日志串起来。前端也只能做子串匹配。
- **修法**：`core/logger/buffer.go:19-24` 扩展为 `LogEntry{TS time.Time; Level, Cat, Event, Code, Corr, Payload string; DurMS int; Attempt int; PID int; Caller string; Fields map[string]any}`；新增 `core/logger/fields.go` 提供 `func Event(cat, name, code, corr string, fields map[string]any)`；先给 `core/appcore/controller.go:626-700`（起内核）、`control_coordinator.go:246-407`（配置切换/回滚）、`core/runtimeassets/*`（自愈）三处埋 corr。

#### F-A3-09 环形缓冲满后每条日志都要 O(500) 复制，并周期性分配约 50 KB
- **严重度**：P1 ｜ **证据等级**：A（代码）/ B（量级）
- **file:line**：`core/logger/buffer.go:41-51`（`r.data = append(r.data[1:], entry)`）
- **现象**：容量满时用切片前移模拟环形。`r.data[1:]` 的 len==cap==max-1，再 append 一个元素必然触发 growslice（Go 对 cap≥256 的切片按 `cap + (cap+768)/4` 增长），即**每次满环写入**：新建约 815 项的后备数组（`LogEntry` 64 B → 约 52 KB）+ 复制 499 项。即便偶尔命中就地写入，也仍需 memmove 499×64 B ≈ 32 KB。全程持写锁。
- **影响**：500 条在几秒内就会写满，此后**每条日志**都走这条路。用户把 core 日志开到 info/debug 时（数十~数百条/秒），日志本身成为 UI 卡顿的候选来源之一；同时与 `GetAll`（`buffer.go:54-63`，深拷贝 500 条）竞争读锁。
- **修法**：`core/logger/buffer.go:27-51` 改为真正的环形：`data []LogEntry` 固定长度 max、`head int`、`size int`，`Add` 用 `r.data[r.head] = entry; r.head = (r.head+1)%max`，零分配零 memmove；`GetAll` 按 head/size 顺序拷贝一次。

#### F-A3-10 日志推送逐条、无批量、无节流、无背压，且与流量事件共用同一条串行前端通道
- **严重度**：P1 ｜ **证据等级**：A（机制）/ B（量级）
- **file:line**：`core/appcore/log_dispatcher.go:44-46,63-65`（每条一次 Emit）→ `wails_event_sink.go:51`（`app.Event.Emit`）→ `frontend/src/App.vue:681-691`（逐条 push + 响应式）
- **现象**：每条日志一次跨进程事件。v3 侧链路为：Emit → 类型校验 → **无界 mailbox**（`[外部依赖] internal/mailbox/mailbox.go:6-7,20-29`，注释明写 "The queue is unbounded and safe for concurrent senders"）→ 单个 drain 协程 → `window.DispatchWailsEvent` → 每条 `event.ToJSON()` + 一次 JS 注入（`[外部依赖] webview_window.go:1492,1517`），窗口队列满 64 条后 drain 协程**阻塞等待 UI 线程**（`webview_window.go:1576-1584`）。发端（`AddCore`）因 mailbox 异步而永不被反压，所以 UI 跟不上时队列只会在 mailbox 里无限增长。
- **影响**：(a) 日志洪水 → 内存单调增长（无上限）；(b) 所有前端事件共用这一条串行 mailbox，**日志会把 traffic-data / connections-update 等实时事件挤在后面**，表现为界面流量数字卡顿；(c) `frontend/src/App.vue:683` 的 500 条上限只约束前端数组，约束不了 Go 侧队列。
- **修法**：`core/appcore/log_dispatcher.go:44-46,63-65` 改为写入本地下标 `pending []logger.LogEntry`，由 50 ms ticker 合并成一个 log-batch 事件（新事件名需在 `v3_events.go` 注册）；前端 `App.vue:681` 改为接收数组并一次性 splice。若维持逐条，则必须在 dispatcher 内做**有界丢弃**（同类同级别 100 ms 内合并、超过 N/s 丢弃 debug 并计数）。

#### F-A3-11 core 日志不过滤就推前端，且与 app 日志共用一个 500 条环——错误会被淹没，前端还要再过滤一次
- **严重度**：P1 ｜ **证据等级**：A
- **file:line**：`core/appcore/log_dispatcher.go:49-66`（`AddCore` 无 `ShouldShow` 判断，对比 `:44` 的 `AddApp` 有）、`core/logger/buffer.go:82`（单一环）、`frontend/src/App.vue:397-406`（前端按 `globalState.logLevel/appLogLevel` 再过滤一遍）
- **现象**：core 日志无论级别都 emit 到前端，前端拿到后再按本地配置过滤丢弃 → 白付跨进程成本；两侧共用同一个 500 条 FIFO，core 的 info/debug 会把此前写入的 error 挤出去。
- **影响**：主方案 §6.1 要求的"内存环 2000 条、error/warn 至少保 500 条"完全不存在；用户开 debug 复现问题后，**错误行往往已经被冲掉**，这是"越复现越查不到"的典型。
- **修法**：`core/appcore/log_dispatcher.go:49-66` 加上与 `AddApp` 相同的 `if d.emit != nil && logger.ShouldShow(...)` 判断（core 用 `Behavior.LogLevel`）；`core/logger/buffer.go:82` 改为两个环：`AppLogs = NewRingBuffer(1500)` + `ErrLogs = NewRingBuffer(500)`（error/warn 同时写两个），`GetAll`（`controller.go:2160-2162`）合并去重返回。

#### F-A3-12 启动早期事件被丢弃，且没有任何 boot 记录；`runtime-assets-status` 发了没人收
- **严重度**：P1 ｜ **证据等级**：A
- **file:line**：`wails_event_sink.go:35-39`（`ctx==nil` 直接 return）、`app.go:188`（`emitEvent("runtime-assets-status", status)`）、`v3_events.go:52`（注册了该事件）、前端全仓无监听（`frontend/src` 内 `runtime-assets-status` 0 命中）、`app.go:204`（`runStartupRuntimeAssetMaintenance` 同步跑在窗口加载前）
- **现象**：启动资产维护在 webview 加载前同步执行，其结果事件既没有接收者（前端无监听；且 `WebviewWindow.DispatchWailsEvent` 在 `w.impl == nil` 时直接 return，`[外部依赖] webview_window.go:1487-1490`），也没有落盘；`ctx==nil` 期间的日志类事件同样直接 discard 且不计数。整个启动阶段没有任何 boot.phase 记录（版本、路径、管理员、helper、seed 状态、静默参数）。
- **影响**：主方案最关心的"启动阶段发生了什么"只有环里零散几行（且 `app.go:196-245` 的 ServiceStartup 全段没有任何 `logger.*` 调用），窗口一开就已经丢完；`--silent`/`--startup` 这类参数在日志里完全不可见。
- **修法**：`wails_event_sink.go:37-39` 改为：`ctx==nil` 时把事件塞进有界 pending 队列（≤256）**并计数/落盘（作为 boot.event.buffered）**，`SetContext`（`app.go:200-202`）后补发；`app.go:204` 前后各写一条 `boot.phase{phase:"asset-maint",dur_ms}`；前端在 `frontend/src/App.vue` 挂载时主动调 `GetRuntimeAssetStatus()` 兜底（比依赖事件可靠）。

#### F-A3-13 helper 侧是纯黑盒：只有 stderr，内核退出码被丢弃，状态接口不含退出原因
- **严重度**：P1 ｜ **证据等级**：A（代码）/ B（服务态 stderr 去向）
- **file:line**：`cmd/goclashz-helper/main.go:98,295,302,316,699,711,956,985,1016,1026,1032,1044,1063,1070,1102,1109,1121`（全部 `fmt.Fprintf(os.Stderr,...)`，无 logger import）、`cmd/goclashz-helper/main.go:206`（`_ = p.cmd.Wait()` —— 退出码直接丢弃）、`core/sys/helper_protocol.go:104-114`（`CoreStatusData` 只有 running/stopping/state/pid/consoleError/error）、`core/clash/runner.go:25-29`（`ExitEvent{Intentional, Message}`）
- **现象**：helper 没有任何结构化日志，也没有任何日志文件；以服务身份运行时 stderr 没有接收者（PE 为 CONSOLE 子系统，但 SCM 拉起时无控制台）。内核停止/崩溃的退出码在 `superviseCore` 里被 `_ =` 丢掉，`core-status` 也不回传"上次退出码/退出时间/启动时刻"。
- **影响**："内核自己退了"这类最高频故障无法归因（是崩溃？被 kill？配置错 exit 1？），主进程只会显示 `core/clash/runner.go:186` 的固定文案"内核已异常退出"。
- **修法**：新增 `cmd/goclashz-helper/logging.go`（JSONL 写 `<dataDir>/logs/helper-YYYYMMDD.jsonl`，轮转同主进程），把上述 17 处 Fprintf 改为结构化调用并附带 method/pid/dur；`main.go:206` 改为 `err := p.cmd.Wait()` 并记录 exitCode/err/dur，同时在 `core/sys/helper_protocol.go:104-114` 的 `CoreStatusData` 增加 `LastExitCode int`/`LastExitAt`/`StartedAt`，并在 `cmd/goclashz-helper/main.go:775-786` 的 core-status 响应里带出。

#### F-A3-14 诊断包太薄且无脱敏：只有路径与布尔值，没有日志、没有版本、没有系统信息
- **严重度**：P1 ｜ **证据等级**：A
- **file:line**：`core/appcore/diagnostics.go:24-39`（`DiagnosticInfo` 字段全集）、`:49-88`（`GetDiagnosticInfo`）、`:90-97`（`ExportDiagnosticsToFile`，直接 `os.WriteFile(path, data, 0644)`）、`redact` 全仓 0 命中
- **现象**：快照只含 AppDir/DataDir/两种 core bin 目录/runtime config 路径/IsAdmin/helper 状态/helper 路径/6 个资产布尔/seed manifest/asset state。**不含**：应用与内核版本、OS 与架构、helper `consoleError`（`core/sys/helper_protocol.go:112` 明明有）、内核 last-exit、任何日志、当前 desired/actual 状态、网络适配器摘要、最近错误。导出为单文件 JSON、非原子写、权限 0644，且直接暴露 `D:\GoclashZ\...` 这类含用户名的绝对路径，无任何脱敏。
- **影响**：用户"导个诊断给我看看"之后仍拿不到任何时间线；一旦贴到公开 issue 就泄露本机路径与用户名。
- **修法**：`core/appcore/diagnostics.go:90-97` 改为产出 zip（`archive/zip`）：`diagnostics.json`（补 version/os/helper.consoleError/core.lastExit）+ `logs/app-*.jsonl`（最近 3 天）+ `logs/helper-*.jsonl` + `asset-state.json` + `runtime/config.yaml`（脱敏副本）；新增 `core/utils/redact.go` 提供 `func RedactYAML([]byte) []byte` 与 `func RedactPath(string) string`（家目录替换为 `%USERPROFILE%`），打包前统一过一遍。文件名带时间戳（见 F-A3-21）。

### P2（体验与卫生）

#### F-A3-15 用户在保存对话框点"取消"，界面却弹"诊断信息导出成功"
- **严重度**：P2 ｜ **证据等级**：A
- **file:line**：`core/appcore/controller.go:2452-2462`（取消 → `return nil`）、`frontend/src/components/SettingsAbout.vue:453-456`（`await App.ExportDiagnostics(); showAlert('诊断信息导出成功')` 无条件执行）、`:458`（`indexOf('User cancelled')` 判断是死代码，因为 Go 侧返回 nil）
- **现象**：取消 → Go 返回 nil → 前端认为成功。且成功/失败文案硬编码中文，未走 i18n（8 语言里没有这两个字符串）。
- **影响**：用户被明确地误导一次；同目录 `frontend/src/locales/*.ts` 的 `settings.about.diagnosticsExport` 等 key 已有，说明是遗漏而非设计。
- **修法**：`controller.go:2444` 的 `ExportDiagnostics` 改为返回 `(string, error)`（实际导出路径；取消返回空串 + nil），`SettingsAbout.vue:453-461` 改为：空串 → 静默返回；否则 toast 提示路径；失败 toast 而非模态。新增对应 i18n key（8 个 locale 文件均需补）。

#### F-A3-16 日志面板缺所有自助能力：无搜索、无级别下拉、无暂停、无复制、无导出、无打开日志目录；"清空"无二次确认
- **严重度**：P2 ｜ **证据等级**：A
- **file:line**：`frontend/src/App.vue:119-143`（模板只有 all/core/app 三个 tab + 一个清空按钮）、`:367-408`（`filteredLogLines` 只按 source + 全局级别过滤）、`:841-844`（`handleClearLogs` 直接清 + 调 `App.ClearLogs()`）、`frontend/src/locales/zh-CN.ts:29-34`（`logs` 段只有 all/core/app/clear 四个 key）
- **现象**：后端提供 `SearchLogs`（`core/appcore/controller.go:2164-2166`）但前端**从未调用**（全仓 0 引用），是死接口；面板也不提供"打开日志目录"，用户即使知道 `data/logs` 也无入口。清空会直接抹掉后端唯一的证据池，无确认、无"清空发生在何时"的记录。
- **影响**：用户自助排查能力的上限就是"滚动看 500 行"；客服/开发者侧也无法引导用户导出。
- **修法**：`frontend/src/App.vue:137-142` 的 footer 增加：搜索框（回车调 `App.SearchLogs(kw)` 替换 `logLines`）、暂停/继续（本地开关，不停止流）、"打开日志目录"（新增 Go 方法 `OpenLogsDir()`，内部 `sys.OpenFolder(filepath.Join(utils.GetDataDir(),"logs"))`）；`handleClearLogs`（`:841`）改为先 `showConfirm`，并在清空前把该动作本身写一条 app 日志。

#### F-A3-17 日志流自身的健康不可见：断流静默重连、状态不外露，且存在两条互斥的死实现
- **严重度**：P2 ｜ **证据等级**：A
- **file:line**：`core/clash/api_client.go:209-253`（失败每 2s 重连、**不打日志不外露**）、`core/appcore/logs.go:87-91`（`IsRunning` 无调用方）、`core/appcore/controller.go:2156-2158`（`StopLogStream` 无调用方）、`core/clash/api_extra.go:27-70`（WS 版 `StartLogStream` 无调用方，且 `:35` 硬编码 `level=info`）
- **现象**：内核没起来（或 external-controller 未监听）期间，日志流只是每 2 秒静默重连，用户看到"日志面板空的"却不知道"根本连不上"；`IsRunning/StopLogStream` 与整套 WS 实现都是死代码，第二条通道还把级别写死 info（与 `behavior.LogLevel` 无关）。
- **影响**：把"没有日志"与"日志正常但没有事件"混为一谈——这是事故复盘的第二种误判来源。
- **修法**：`core/clash/api_client.go:232-241` 在重连分支首次失败写一次 `logger.Warnf("内核日志流未连通: %v", err)`（后续同类 30s 去重）；`core/appcore/logs.go` 增加 `func (m *LogStreamManager) Health() (ok bool, lastErr string)` 并挂在 `GetAppState` 的 `AppState` 上，前端日志面板显示"流已断开"角标；删除 `core/clash/api_extra.go` 的 WS 实现或与 `FetchLogs` 合一。

#### F-A3-18 `Debugf` 与 `Level` 枚举是死代码——app 侧 debug 档永远为空
- **严重度**：P2 ｜ **证据等级**：A
- **file:line**：`core/logger/buffer.go:118-125`（`Debugf`）、`core/logger/level.go:5-12`（`Level` 枚举，全仓 0 引用）
- **现象**：`logger.Debugf` 调用点 **0 处**（57 处 `logger.*` 里没有 Debugf）；`Level/LevelDebug..LevelError` 常量全仓无引用。用户把 `AppLogLevel` 设成 debug，得到的仍是 info 以上——面板上"什么都没变"。
- **影响**：写"可回放"最需要的细粒度日志根本不会产生；用户以为开了 debug 其实没有。
- **修法**：短期把 `core/logger/level.go:5-12` 与 `buffer.go:118-125` 删掉或标注 Deprecated，避免误以为有 debug 通道；中期在 F-A3-08 的结构化改造里给 `Event` 加 Level 参数，并在 `core/appcore/controller.go:1753-1800`（TUN 开）与 `control_coordinator.go:246-346`（配置切换）补 debug 级细粒度埋点。

#### F-A3-19 dispatcher 为 nil 的兜底路径不补时间，会产出空时间行
- **严重度**：P2 ｜ **证据等级**：A
- **file:line**：`core/appcore/log_dispatcher.go:31-34`（`AddApp` nil 分支直接 Add）、`:50-52`（`AddCore` 同）、`core/appcore/logs.go:63-66`（构造 `LogEntry` 时 `Time` 为空）
- **现象**：nil dispatcher 时绕过 `:36-40`/`:55-59` 的归一化与时间补全，条目以空 Time/Type 入环，前端渲染成空时间行。当前 `logs.go:68-70` 只在 dispatcher 非空时调用，属潜在缺陷；但一旦有人按 `log_dispatcher.go:31` 的写法复用就会踩到。
- **影响**：日志面板出现无时间戳的幽灵行，破坏"按时间读"的可用性。
- **修法**：`core/appcore/log_dispatcher.go:31-34,50-52` 改为先做 `Type` 归一、`Source` 赋值、`Time` 补全，再 `AppLogs.Add`（抽 `func normalizeEntry(entry, source) logger.LogEntry`，两处共用）。

#### F-A3-20 所有丢弃都是静默的：没有 dropped 计数，也就没有"日志系统自身是否健康"的指标
- **严重度**：P2 ｜ **证据等级**：A
- **file:line**：`wails_event_sink.go:37-39`（ctx 未就绪丢弃）、`:47-50`（`application.Get()==nil` 丢弃）、`app.go:76-82`（`emitEvent` 返回 false 丢弃）、`core/appcore/streams.go:320-327`（10s 内重复错误静默 return）、`core/appcore/controller.go:1363-1365`（30s 去重静默 return）
- **现象**：5 处丢弃路径都没有计数、没有日志、没有暴露接口。用户/开发者无法区分"没有发生"与"发生了但被吃掉"。
- **影响**：排障时把"静默"误判为"正常"，与 F-A3-17 是同一类认知陷阱。
- **修法**：在 `core/logger` 增加 `var Dropped atomic.Uint64` 与 `func IncDropped(reason string)`（按 reason 分桶），上述 5 处在 return 前调用；`AppState` 增加 `LogDropped map[string]uint64` 暴露给前端并在日志面板角标显示。

#### F-A3-21 没有任何容量/轮转/保留策略，日志目录从未暴露给用户；诊断文件名固定会被覆盖
- **严重度**：P2 ｜ **证据等级**：A
- **file:line**：`core/appcore/controller.go:2446`（`Filename: "goclashz_diagnostics.json"` 固定名）、`core/utils/paths.go:61`（目录已建）、轮转相关检索 0 命中；`app.go:663-664` 的 `GetDataDirInfo` 只返回路径字符串，无打开目录入口
- **现象**：`rotate/MaxSize/MaxAge/retention` 全仓 0 命中；连续导出两次诊断会用同名文件（是否覆盖取决于系统对话框，B）；"打开数据目录/日志目录"没有任何 Go 方法或前端入口。
- **影响**：一旦开始落盘（F-A3-01），没有轮转就会变成磁盘无限增长；没有时间戳文件名，用户多次导出后会拿错版本。
- **修法**：`core/appcore/controller.go:2446` 改为 `fmt.Sprintf("goclashz-diag-%s.json", time.Now().Format("20060102-150405"))`；新增 `core/logger/rotate.go`（单文件 8 MB / 保留 7 天 / 总量 ≤80 MB，按 `app-YYYYMMDD.jsonl` 分日）；新增 Go 方法 `OpenLogsDir()`/`OpenDataDir()` 并在 `frontend/src/components/SettingsAbout.vue:150-156` 增加入口。

---

## 3 与主方案对应关系

### 3.1 方案已覆盖且经本次复核**成立**的部分

| 方案条目 | 复核结果 | 复核依据 |
|---|---|---|
| §0.1 "仅 500 条内存环" | 成立 | `core/logger/buffer.go:82` |
| §1.3 "存储：全局单例 RingBuffer(500)，纯内存" | 成立 | `core/logger/buffer.go:26-82` |
| §1.3 "时间 15:04:05，无日期/毫秒/时区" | 成立（**处数应为 6 而非 4**） | `buffer.go:96,105,114,123` + `log_dispatcher.go:39,58` |
| §1.3 "级别 NormalizeLevel 四档，无类别/子系统" | 成立 | `core/logger/level.go:14-27` |
| §1.3 "双通道：AddApp 过滤后 emit；AddCore 不过滤；共用一个环" | 成立 | `log_dispatcher.go:44` vs `:63-65`；`buffer.go:82` |
| §1.3 "早期事件：ctx == nil 直接 return" | 成立（行号精确） | `wails_event_sink.go:37-39` |
| §1.3 "通知去重 30s 内重复 error 直接 return，连日志都不写" | 成立（且**低估范围**：非去重路径也不写） | `controller.go:1362-1375` |
| §1.3 "落盘：无；data/logs 由 paths.go:61 创建但从不写入" | 成立，且本次**实测复核**：两个真实数据目录均为空 | `paths.go:61`；`D:\GoclashZ\data\logs`、`%LOCALAPPDATA%\GoclashZ\logs` 均 0 条目 |
| §1.3 "崩溃：recover 只写内存环" | 成立 | `main.go:37-43` |
| §1.3 "helper 完全黑盒" | 成立（本次给出 24 处输出点与退出码丢弃的确证） | `cmd/goclashz-helper/main.go` 全文；`:206` |
| §1.3 "诊断只有单点快照" | 成立 | `core/appcore/diagnostics.go:49-97` |
| §11.3-⑫ "core 日志逐条 emit + 前端逐条 push + 自动滚动" | 成立（本次补充"无界队列/共用通道"这一层） | `log_dispatcher.go:63-65`；`App.vue:681-691` |
| §11.3-③ "任务失败只 Emit 不写日志" | 成立 | `core/tasks/manager.go:56-67,98-109` |
| §6.1/§6.2/§6.3/§6.5/§6.6 的**目标形态** | 方向正确，与本次发现一一对上 | 见 §3.2 |

### 3.2 方案**漏掉**的（本次新增）

| # | 新增发现 | 位置 | 为什么重要 |
|---|---|---|---|
| N1 | 资产自检/自愈用 **stdlib log** 而非 logger，输出到 GUI 进程不存在的 stderr | `core/runtimeassets/{check,migrate,sync}.go`（:83,:86,:222 / :129,:138 / :166,:168,:196,:200,:216） | 方案 §1.3 只说"500 条环"，没意识到**最关键的自愈日志连环都进不去**。只加文件日志而不改这 10 行，自愈仍然是黑的 |
| N2 | 内核 stdout/stderr 被**主动丢弃**（helper→NUL，直连→无效句柄） | `cmd/goclashz-helper/main.go:607-613`；`core/clash/runner.go:319-323`；`core/utils/process_windows.go:19-21` | 方案 §6.4 只说"main 的 panic 不落盘"；实际连**内核启动失败的原始输出**都拿不到，这是"配置错/端口占用"类故障的唯一证据源 |
| N3 | Wails `Options.Logger`/`ErrorHandler` 未接线，生产构建默认 io.Discard | `main.go:297-320`；`[外部依赖] logger_prod.go:10-12`；`application.go` 的 handleError | 框架层（WebView2/绑定/事件校验）错误全静默；`v3_events.go` 的类型校验失败也走这条路——方案 §10.3 只提醒"要注册事件"，没提**注册错也是静默丢弃** |
| N4 | `notify()` **任何情况**都不写日志（不止去重分支） | `controller.go:1362-1375` | 用户可见错误与日志完全脱钩；方案 §7 #3 只要求"去重时补一行"，范围不够 |
| N5 | 环满后写入是 **O(n) 复制 + 周期约 50 KB 分配**，非 O(1) 环形 | `core/logger/buffer.go:41-51` | 方案 §6.1 只提"2000 条容量"，未提实现方式；直接调容量会把成本放大 4 倍 |
| N6 | v3 前端事件 **mailbox 无界**，且日志与 traffic/connections 共用同一条串行通道 | `[外部依赖] internal/mailbox/mailbox.go:6-7,20-29`；`events.go:188-189`；`webview_window.go:1576-1584` | 方案 §11.3-⑫ 只预判"UI 打满"，没预判**内存单调增长**与**实时流量事件被日志挤后** |
| N7 | `runtime-assets-status` 事件**发了但前端没有任何监听** | `app.go:188`；`frontend/src` 0 命中 | 启动自检结果既没落盘也没被消费，等于白算 |
| N8 | 导出诊断**取消也提示成功** | `controller.go:2452-2462` + `SettingsAbout.vue:453-456` | 方案 §6.5 只设计"导出什么"，没审"导出交互" |
| N9 | `data_migration` **主动 Skip logs 目录** | `core/utils/data_migration.go:230,274` | 说明"日志可丢弃"是既定假设；落盘落地时要一并把 logs 从 skip 列表移除，否则迁移/备份语义与"日志是资产"自相矛盾 |
| N10 | `SearchLogs`、`StopLogStream`、`IsRunning`、`clash.StartLogStream`(WS) 均为死代码 | `controller.go:2164-2166`、`controller.go:2156-2158`、`core/appcore/logs.go:87-91`、`core/clash/api_extra.go:27-70` | 方案 §7 #25 想加"分级筛选 + 搜索"，但**后端接口早就有了，只是前端没接**——工作量比方案预估小 |
| N11 | `Debugf` / `Level` 枚举死代码，app 侧 debug 轨道从未存在 | `core/logger/buffer.go:118-125`、`core/logger/level.go:5-12` | 方案 §6.3 埋了一堆 debug 级事件，需先确认"级别通道"真实存在 |
| N12 | 诊断快照缺 `helper.core-status.consoleError`（helper 明明已回传） | `core/sys/helper_client_windows.go:220-262` vs `diagnostics.go:24-39` | TUN 优雅停止降级（→ 强制 Kill 内核）的唯一线索没进诊断包 |
| N13 | 日志流断连完全不可见（静默 2s 重连） | `core/clash/api_client.go:232-253` | "面板空"有两种含义，方案未区分 |

### 3.3 方案**说错 / 不准确**的地方（逐条给出正确版本）

| # | 方案原话 | 实际情况 | 依据 |
|---|---|---|---|
| E1 | §11.3-⑤："RPC 默认 deadline **10s** → CheckHelperService 的 Ping 就挂在 UI 链上……服务假死会让设置页卡 10s" | **不准确**。`getMethodTimeout` 对 ping 返回 **200ms**（`helper_client_windows.go:30`），连接超时 150ms（`:20`）；10s 是**其它方法**的默认值（`:42`），start-core 才是 10s（`:23`）。因此 CheckTunEnv/GetDataDirInfo 的 helper 探测上限约 350ms，不是 10s。**真正会卡 10s+ 的是 StopCore（`:37-40`）与文件替换类方法** | `core/sys/helper_client_windows.go:20-44` |
| E2 | §10.3："新增事件必须在 v3_events.go 注册，否则会被 v3 的类型校验丢弃" | **不准确**。未注册事件**不会**被丢弃：`validateCustomEvent` 对未注册名直接 return nil（只打一条 warn），事件照常投递，只是没有 TS 类型 → 前端拿到 any。真正被取消的是**已注册名 + payload 类型不匹配**。另外"无 payload 却注册成 any"会在校验里 panic（`nil.Implements`），恰被 `wails_event_sink.go:41-45` 的 recover 兜住 → 事件静默消失 | `[外部依赖] pkg/application/events.go:339-344,352-367` |
| E3 | §1.3："时间 15:04:05（buffer.go:96 等 **4 处**）" | 实为 **6 处**：`buffer.go:96,105,114,123` + `log_dispatcher.go:39,58`。后者才是 core 日志真正用的那条（`logs.go:63-66` 构造时不带 Time） | 同左 |
| E4 | §6.4："WailsEventSink 改成 pending 队列 + 落盘，SetContext 后补发" | **不完整**。补发只解决 ctx 判据；**webview 尚未就绪时事件仍会丢**——`WebviewWindow.DispatchWailsEvent` 在 `w.impl == nil` 时直接 return（`[外部依赖] webview_window.go:1487-1490`）。可靠做法是"启动期事件一律先落盘/入环，前端挂载后主动拉取"，而不是指望补发 | 同左 |
| E5 | §0.1 "日志 空：仅 500 条内存环……data\logs 从不写入" | 成立，但归因需补一层：不只是"没实现文件写入"，而是**三个日志源根本不在同一条管线上**（logger 管线 / stdlib log→stderr / 内核 stdout→NUL）。只补文件 sink 只能覆盖第一条 | §3.2 N1/N2 |

### 3.4 与方案 `§7` 优先级的对齐建议

方案 §7 把日志改造的 P0 定为 #1 #2 #3 #4 #26。按本次证据，建议**插入两项、调整一项**：

1. **新增（P0 前置）**：把 `core/runtimeassets` 的 10 处 `log.Printf` 换成 `logger`（N1）——否则 §7 #1 的文件 sink 建好了也看不到自愈过程。
2. **新增（P0 前置）**：接上 `application.Options.Logger/ErrorHandler`（N3）——两行改动，换回整个框架层的可见性。
3. **调整**：§7 #2（`log_dispatcher.go:30-66` 改分级）应先落地 **F-A3-09 的真正环形**，否则容量从 500 提到 2000 会把每条的复制成本放大 4 倍。

---

## 4 未覆盖与待实测

| # | 项目 | 状态 | 证据等级 | 补齐方式 / 成本 |
|---|---|---|---|---|
| 1 | 单位时间日志量（core info/debug 下每秒条数、每日字节数） | **未测**（禁止运行应用） | C | 开 debug 跑 10 分钟，统计 `logs/app-*.jsonl` 行数与体积；或先用 `GetRecentLogs` 两次采样差估算（1 h） |
| 2 | v3 mailbox 实际是否增长（`eventQueueHigh` 水位） | 机制 A，量级**未测** | B/C | 依赖侧 `webview_window.go:1640-1645` 有 `eventQueueHighWater()`，可临时暴露一行日志（1 h） |
| 3 | 前端 500 行 × 逐条 push 的实际帧耗 | **未测** | C | DevTools Performance，debug 级持续灌流观察长任务（0.5 h） |
| 4 | helper 以服务身份运行时 stderr 到底落到哪里 | 代码可证"无接收者"，实机**未验证** | B | `sc start GoclashZHelper` 后观察是否可被 procdump/ETW 捕获（1 h） |
| 5 | `log.Printf` 在 GUI 进程写无效句柄的返回值 | 代码可证写 `os.Stderr`、失败被忽略；句柄有效性未实机验证 | A（行为）/ B（细节） | 在测试分支临时打印 `os.Stderr.Stat()`（0.2 h，**本次禁止改代码**） |
| 6 | 内核 `/logs` 流在窗口最小化/被遮挡时是否继续投递 | **未测** | C | 最小化 5 分钟，比对 `logLines.length` 增量 |
| 7 | WebView2 崩溃时 v3 的重建行为与日志 | 平台层实现未读 | C | 读 `webview_window_windows*.go`（1 h） |
| 8 | 8 语言诊断/日志文案缺口清单 | 只抽查 zh-CN/en-US | B | 逐 locale 比对 `logs*`/`diagnostics*` key（0.5 h） |
| 9 | 标准安装模式与绿色模式（`.portable`）下 `data/logs` 路径差异 | 代码已读（`paths.go:93-121`），实测只覆盖两个目录 | A/B | 绿色模式再验一次（0.2 h） |
| 10 | 托盘/热键线程的 recover 是否有日志（`tray_runtime_windows.go`） | **未读**（超出分配范围） | C | 建议由 A4 审计覆盖（0.3 h） |
| 11 | `GetRecentLogs` 跨进程 JSON 体积与耗时 | 未测 | C | 500 条 × 平均 payload 估算（当前远小于 1 MB，风险低） |
| 12 | 落盘后对启动耗时的影响（按 F-A3-01 实现后） | 设计目标 ≤50 µs/条 | C | P0 落地后压测 |

---

**审计结论（供汇总）**：日志子系统当前处于"**有面板、无管道**"状态——三条日志源（logger 环 / `runtimeassets` 的 stdlib log→stderr / 内核 stdout→NUL）互不相通，且没有任何一条离开进程内存。主方案 §1.3 的现状判断基本成立（复核 13 项、13 项为真），但有 5 处表述需修正（§3.3）、13 项关键缺口未被覆盖（§3.2），其中"自愈日志走 stdlib log 而丢失"（N1）与"内核 stdio 被丢弃"（N2）是事故复盘失败的**第二层根因**，应并入 P0 一起做。

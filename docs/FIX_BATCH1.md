# Batch-1 修复报告（P0 清零）

> 依据：docs/audit/ISSUE_LEDGER.md；执行 6 个工作流（W6/W7a/W7b/W8/W9/W10）
> 集成验证：Go build/vet/test 全绿；前端 vue-tsc + vite build 全绿
> 回滚点：`.fixbackup/20261116-b1/`（各代理编辑前快照，保持相对路径）

## 0 结果

| 指标 | B0 后 | B1 后 |
|---|---|---|
| P0 未关闭 | 11 | **0** ✅ |
| P1 未关闭 | 55 | 44（部分修复 3） |
| P2 未关闭 | 37 | 37 |
| 合计未关闭 | 104 | **81** |

本批关闭 **19 条**：F-A1-01、F-A1-04、F-A2-01、F-A2-02、F-A2-11、F-A3-04、F-A3-06、F-A3-07、F-A3-12、F-A4-02、F-A4-03、F-A4-06、F-A5-04、F-A5-07、F-A5-09、F-A5-10、F-A5-11、F-A0-01、F-A0-07。

## 1 关键变化

### 检查分层（W6，性能根因）

- 新增 `GetStatusFast`（stat+PE 头+版本 API，**不 spawn、不哈希**）与 `GetStatusCached`（60s 快照，按 path+size+mtime 失效）。
- 6 个 UI 方法与 `GetDataDirInfo` 的 seed 探测改走缓存；`CheckCore`/`CheckWintun` 变纯只读（写盘移到 `EnsureReady` 修复分支前的 `AdoptFallback`，per-asset once）。
- 实测（本机热缓存）：`GetStatus` 61 ms → `GetStatusFast` 0.71 ms → `GetStatusCached` 0.21 ms。

### TUN 收敛（W7b，结构根因）

- `prepareTunEnable`：helper 就绪改 800ms 快 ping（超时后台唤醒并放行），资产门禁改 `GetStatusFast`，只有「缺失/目录/PE 损坏」才拒绝；深检/修复交 Reconcile。
- `ensureCoreRunningWithDesiredState` 与 `clash.PrepareEnv` 同改 fast，仅在 fast 明确不可用时回落全量 `EnsureReady`（保留 seed 自愈）。
- watchdog 5s 心跳上的 90.9 MiB 哈希移除；新增 `fastAssetBlocked` 语义（`!Ready && !Valid` 才算明确不可用）。

### 内核输出可观测（W7a）

- 直连模式 stdout/stderr 逐行进日志并保留 200 行环形尾巴，启动失败错误自动附带最近 20 行。
- helper 把内核输出追加到 `<dataDir>/logs/core-helper-YYYYMMDD.log`，`superviseCore` 记录退出码/时间，`core-status` 增加 `exitCode/exitAt/lastExitPid`（仅加字段）。
- helper 超时预算拆分：start-core 25s、replace-core-file / install-wintun 65s、repair-permission 35s；`install-wintun` 前先停内核。

### 日志结构化（W8）

- Wails v3 `application.Options.Logger + ErrorHandler` 接线，框架日志与「事件类型不匹配」入盘。
- `LogEntry` 增 `TS`(RFC3339 毫秒)/`PID`，三处时间戳统一；前端日志面板 16ms 批量合并、显示毫秒时间列。
- 前端终于订阅 `runtime-assets-status`（此前发了没人收）。

### 前端一致性（W9）

- 抽出 `useTunToggle()` 唯一实现（Overview/SettingsTun 共用），`ToggleTunMode` 在组件层零引用；全仓 `globalState.modal = {` **0 处**。
- 导出确认框取消不再执行导出；SettingsTun 保存失败改 toast；8 语言 `common.unknownError` 死键清理。

### 外围可靠性（W10）

- 提权子进程 10 分钟超时（超时可读错误，不再无限等待）。
- 小文件下载：空响应体/读错误/超限截断全部报错。
- 备份还原 ctx 生效、回滚失败如实返回、zip.Close 错误上抛、半成品删除。
- 删除配置事务化（失败回滚 yaml + 索引 + desired）。

## 2 集成验证

| 检查 | 结果 |
|---|---|
| `go build ./core/... ./cmd/... .` | exit 0 |
| `go vet` 同范围 | exit 0 |
| `go test`（appcore/logger/runtimeassets/tasks/utils/backup/downloader/root） | exit 0 |
| `vue-tsc --noEmit` | exit 0 |
| `pnpm -C frontend run build` | exit 0（169 模块，dist 已更新） |
| gofmt（改动文件） | 空（controller.go 差异为改动前既有） |

## 3 行为变更（需知会）

1. 装驱动现在会**先停内核**且不自动拉起（watchdog 下一轮 Reconcile 恢复，有短暂中断窗口）。
2. wintun `invalid_pe` 不再由门禁静默修复，返回 B 类错误让用户决定是否重装。
3. 导出订阅只剩「确定=带规则导出 / 取消=不导出」，不带规则的入口暂缺（记入 B2 议题）。
4. 直连内核输出进日志：`log-level=debug` 时日志量上升（无采样，B2 处理）。
5. helper 侧日志无容量上限（`core-helper-*.log` 会累积，B2 处理）。

## 4 风险与遗留（进 B2）

- `control_coordinator.setLocked` 仍同步等 Reconcile，"两段式返回"未完成（F-A2-05/06 等）。
- `GetStatusFast` 无法发现「PE 有效但内容损坏」，需 F-A1-05/F-A1-06 的 manifest 校验补齐。
- `MigrateLegacyAssets` 每次启动仍多算 6 次 SHA256（F-A5-14）。
- `LogEntry` 的 cat/code/corr/dur 仍无生产者（F-A3-08 partial），需结构化事件 API。
- 真机 8 项验证仍未做（见 FIX_REPORT.md §6）。

# Batch-2 修复报告（P1 主体）

> 台账：docs/audit/ISSUE_LEDGER.md（本批后：P0 25/25、P1 49/55 fixed + 3 partial、P2 1/38）
> 执行：W11 资产 / W12 TUN / W13 日志 / W14 前端 / W15 外围
> 集成：go build/vet/test 全绿；vue-tsc + vite build 全绿
> 回滚：`.fixbackup/20261116-b2/`

## 0 结果

| 指标 | B1 后 | B2 后 |
|---|---|---|
| P0 open | 0 | 0 |
| P1 open | 44（+3 partial） | **3 open + 3 partial** |
| P2 open | 37 | 37 |
| 合计 open | 81 | **40** |

本批关闭 **41 条**（含 2 条复核为「已被 B0/B1 解决」）。

## 1 主要变化

### 资产（W11）

- `-v` 三态化：成功 / 超时（`ErrProbeTimeout`，不再重拷 61 MB）/ 非超时失败（`ErrExecFailed`，判 Ready=false 可自愈）；0 字节或截断规则库判 `ErrCorrupt`。
- manifest 哈希**复制前实测校验**（此前抄清单）；wintun 只认运行目录，旁路副本降级为 Hint。
- `ctx` 贯通复制/探测：`CopyFileContext`（1 MB 分块检查 + 取消清 tmp）、`repairMu` 3s 预算（`ErrAssetLocked`）；`ReadCoreVersion` 不再持全局锁 spawn。
- 新增进程内 + 跨进程资产锁（`.asset-<name>.lock`，O_EXCL + 30s 陈旧回收），接入 repair/adopt/migrate/cleanup。
- 深检结果 2s 共享窗（`sharedDeepStatus`），一次用户动作的深检从 3~4 次收敛到 1~2 次。
- `RepairRuntimeAssetsAsync` + 逐资产进度（`AssetRepairProgress`），Hint 全部改指真实入口（"更新内核"/"重装驱动"）。

### TUN / 控制面（W12）

- 关闭 TUN **两段式**：提交意图即返回，停止在后台（`runAsyncReconcile` 独立 45s）；托盘调用统一补 25s deadline。
- StopCore 失败不再吞、不再伪造"已停止"：签名改返回 error，失败保留 `userCoreRunning/ActualTun`。
- 系统代理改为**归属感知清理**（只清自己写的）；提权退出、内核异常退出、disableAll 三处统一。
- A 类瞬时失败不再撤销 desired，转后台重试；`isTransientControlError` 排除 helper/配置/权限/端口硬错误。
- `EnsureHelperReadyWithContext`：singleflight 改 `DoChan` + select，可被 ctx 打断。
- `ValidateConfigCached`：按配置内容哈希复用 `-t` 结论（首次 82.3ms → 二次 0s，失败不缓存）。
- helper 替换文件改为原子 `replaceFileAtomic`（备份→写 .new→同目录 rename），并修掉"删掉待安装文件"的真 bug。

### 日志（W13）

- `LogEntry` 结构化补全：`cat/code/corr/durMs/attempt/caller` + `Eventf/Record` API，4 条关键路径已实埋；JSONL 全字段落盘。
- 环形缓冲重写为真环形（满环零分配/零 memmove）；分级配额 `2000 + 500`（warn/error 保留区）。
- core→UI 限速 120 条/秒（error 豁免，被抑制时 2s 汇总一条 warn）；core 日志按 `Behavior.LogLevel` 过滤。
- 诊断导出升级为 **zip**：日志 3 天 + helper 内核日志 + 资产清单 + 脱敏配置 + system.txt；新增 `diag.export.*` 结构化事件。

### 前端（W14）

- 33 个 catch 点统一 `reportError`（console + 50 条内存环）；`.catch(()=>{})` 全仓归零。
- 成功提示全部改 toast（22 处）；测速失败不再静默，单点测速只认 `DELAY_TEST_BUSY`。
- 设置页错误文案槽位修正（人话进正文、原文进 detail）；SettingsTun 首屏 IO 减半且错误不再被覆盖。
- 自建"重置"确认改全局模态；i18n 780 键 8 语言对齐。

### 外围（W15）

- 组件更新锁从"整段下载"下移到 commit 窗口；`TryLock` + `ErrComponentUpdateBusy`（不再无限阻塞 UI）。
- 订阅抓取 3min 硬上限、整链路 4min；超时可读（10min → fail-fast）。
- 组件/geo/软件更新失败全部进日志（此前这些路径 0 行日志）。
- SCM/提权路径 9 处固定 `time.Sleep` 全部改 ctx 感知 + 超时 warn。

## 2 验证

| 检查 | 结果 |
|---|---|
| `go build ./core/... ./cmd/... .` | exit 0 |
| `go vet` 同范围 | exit 0 |
| `go test ./core/... .` | 全 ok（appcore/backup/clash/jsonutil/sys/root） |
| `vue-tsc --noEmit` | exit 0 |
| `pnpm -C frontend run build` | exit 0 |

## 3 行为变更（累计）

1. 关闭 TUN 立即返回、失败只 warn；系统代理只清自己写的。
2. PE 有效但跑不起来的内核、0 字节规则库现在会被判不可用并自愈（此前静默）。
3. UI 日志出现上限（2000 行）与限速（core info 只在日志文件里全量）。
4. 诊断包变 zip，且自动补 `.zip` 扩展名。
5. 组件更新不同组件可并行下载；撞上 commit 时返回 busy 而不是排队。
6. 订阅慢链路（<280KB/s 的 50MB 订阅）由"10 分钟内可能成功"变为 3 分钟 fail-fast。

## 4 遗留（进 B3）

- **B2 期间发现的 4 个新项**：诊断保存对话框仍是 `.json`（controller.go:2583）；`RepairRuntimeAssetsAsync` 未进 bindings/服务面；资产锁未接入 `clash/{component_replace,wintun}.go` 与 `updater_orchestrator.go`；`controller.go:655` 会把 `ErrAssetLocked` 升级成硬失败。
- **P1 剩余 3 条**：F-A4-08（App.vue 启动顺序）、F-A4-14（后端 code 化）、F-A5-13（退出预算）。
- **P1 partial 3 条**：F-A0-06（seed `-v` / 启动维护）、F-A2-10（前端 source 白名单）、F-A5-06（toast/startup-task 事件）。
- **P2 37 条**（A0 1 / A1 3 / A2 6 / A3 6 / A4 9 / A5 12）。
- 真机 8 项验证仍未做。

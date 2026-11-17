# Batch-3 修复报告 + 最终验收（118 条收口）

> 台账：docs/audit/ISSUE_LEDGER.md（最终版）
> 执行：W16 资产 / W17 TUN / W18 日志 / W19 前端 / W20 外围 + 主审集成收尾
> 集成：Go build/vet/test 全绿；前端 vue-tsc + vite build 全绿
> 回滚：`.fixbackup/20261116-b0..b3/`（各批编辑前快照，按相对路径）

## 0 最终结果

| 严重度 | 总数 | 已修 | 未修（有理由） |
|---|---|---|---|
| P0 | 25 | **25** | 0 |
| P1 | 55 | **55** | 0 |
| P2 | 38 | 36 | 2 |
| **合计** | **118** | **116** | **2** |

- **P0 / P1 全部清零**。
- 未修 2 条均为 P2，理由见 §4。
- 本批关闭 41 条（含复核后被前批解决的 2 条、以及 B2 期间新发现的 4 项遗留）。

## 1 本批主要变化

### 资产（W16）

- 删除 2 个死接口（`GetWintunVersion` / `GetComponentFileInfo`，均为 `GetRuntimeAssetStatus` 的严格子集）；`GetCoreVersion` 标 Deprecated。
- 修 `resolveAppDir` 的临时目录判断参数颠倒（此前 exe 在任意目录都可能被判为临时目录）。
- **补齐 runtimeassets 单测**（此前 0 测试）：`decision_test.go`（20 例 shouldCopy 矩阵）、`cache_test.go`、`lock_test.go`（串行/陈旧锁/ctx 取消）。
- 抽 `decideRepairCopy` 时发现并修掉**升级覆盖分支不可达**的真缺陷（此前升级后内核/驱动永不刷新）。
- `Ready` 语义三层化（`Ready` / `GeoReady` / `ReadyForAll`），`Required` 不再对 geo 恒 false。
- 启动维护后台化 + 迁移哈希按需（每次启动 6 次 SHA256 ≈91 MB → 正常启动 0 次）。

### TUN / 控制面（W17）

- P2 六条收口（信号处理、退出预算、状态广播、日志流清理等）。
- **F-A5-13 退出预算统一**：`main.go` + `ServiceShutdown` 三套预算（8s 硬退 / 5s 上限 / 30s 优雅）收敛为一套。
- 诊断保存对话框由 `.json` 改 `.zip`；新增 `ExportDiagnosticsWithPath()`（成功返回路径、取消返回空）。
- `controller.go` 对 `ErrAssetLocked` 改可重试 A 类（不再升级硬失败）。
- 资产锁接进 `clash/component_replace.go` 与 `wintun.go`。
- 删除 `core/clash/api_extra.go` 的 WS 日志死实现（顺带清掉 `gorilla/websocket` 全部引用）。
- 后端补 `DEFERRED_RESTART_*` / `ROUTE_SWITCHING` 等错误码。

### 日志（W18）

- 日志面板补齐**搜索 / 级别筛选 / 暂停 / 复制筛选结果 / 复制日志目录 / 清空二次确认**。
- 日志流健康度：60s 无数据 → `LOG_STREAM_IDLE` warn；写盘丢弃 → `LOG_SINK_DROPPED` 汇总；`GetStats()` 进诊断包。
- 删死代码 `logger.Level` 与 4 个常量；诊断包文件名自动追加时间戳（不再覆盖）。

### 前端（W19）

- 通知处理改为 **code 优先 + A/B/C 分级**（`resolveNotificationUi`），`appcore`/`config-switch` 的失败不再被整段丢弃。
- 33 个 catch → `reportError`；成功提示全部 toast（22 处）；测速失败可见；分享链接详情脱敏。
- UWP「全不选」修正；偏差条失败保留上次结果；启动期事件监听补上。
- 主审收尾：`App.vue` 启动链改为**非阻塞重调用**（订阅先注册，修 F-A4-08）；诊断取消不再误报成功（F-A3-15）；绑定层同步（签名改 `string`、删 2 个死导出）。

### 外围（W20）

- 下载失败写 `.tmp.meta.json`（换源保护从"死代码"变真生效）、限速器 ctx 感知、schtasks 3s 超时。
- `BehaviorStore` 先落盘再改内存 + 新增原子 `Update`；配置 JSON 损坏自动备份 `*.corrupt-<ts>`。
- 组件更新事件不再双发；`CleanLegacyFiles` 改白名单（不再通配删用户 zip/tmp）；`Shutdown` 事件失败不再导致启动即退出。
- 托盘「退出/显示」旁路不再排队；热键注册失败改为 warn 通知。

## 2 集成验证（最终）

| 检查 | 结果 |
|---|---|
| `go build ./core/... ./cmd/... .` | exit 0 |
| `go vet` 同范围 | exit 0 |
| `go test -count=1 ./core/... .` | 全 ok（appcore/backup/clash/jsonutil/runtimeassets/sys/root） |
| `vue-tsc --noEmit` | exit 0 |
| `pnpm -C frontend run build` | exit 0（dist 已重建） |
| 手改绑定一致性 | `appservice.ExportDiagnostics` → `Promise<string>`；删 2 个死导出；tsc 通过 |
| 越界检查 | 各批写范围核对，无跨范围改动（W20 的 v3_events.go 已报备并重建回滚点） |

## 3 累计行为变更（发版需知）

1. 关闭 TUN 立即返回、后台停止；系统代理只清理自己写入的。
2. PE 有效但跑不起来的内核、0 字节规则库会判不可用并自愈（此前静默）。
3. 装驱动会先停内核，装完由 watchdog 恢复。
4. UI 日志上限 2000 行 + core→UI 限速 120 条/秒（文件仍全量）。
5. 诊断包为 zip 且文件名带时间戳；取消保存不再提示成功。
6. 组件更新可并行下载、commit 串行；撞锁返回 busy。
7. 订阅抓取 3min 硬上限（原 10min）。
8. 通知分级：A 静默 / B toast / C 模态（`appcore` 失败此前完全静默，现在可见）。
9. 升级时会按 seed 刷新"未被用户修改过"的内核/驱动（修复了此前永不刷新的缺陷）。
10. 配置 JSON 损坏会保留 `*.corrupt-<ts>` 备份。

## 4 未修 2 条（P2，均写明理由）

| ID | 理由 |
|---|---|
| **F-A4-17** | 122 个 i18n 死键清理：无功能收益，且会与并发批次大面积冲突；建议后续用 i18n lint 一次性处理。（缺键与"未翻译"误报已在本批修正） |
| **F-A4-23** | 提权取消仍进入页面：需要给 `showConfirm` 扩展"自定义按钮文案"能力（涉及 store 模态 API + App.vue 页脚渲染），属交互改造；当前文案已明确"取消可继续在普通权限下查看"。 |

## 5 真机验收清单（代码侧已完成，需人工确认）

| # | 场景 | 期望 |
|---|---|---|
| 1 | 冷启动首开 TUN | 不再弹「缺少 Wintun 驱动」；≤2s 进入"已开启/正在建立" |
| 2 | 设置→组件与库更新→装驱动 | 成功、内核自动恢复；失败有 toast + 日志 |
| 3 | 开系统代理时以管理员重启 | 退出前摘除代理，重启后不残留 |
| 4 | 断网「更新全部订阅」 | 报错而非成功；任务行显示失败 |
| 5 | `data/logs/app-*.jsonl` | 有内容、含 ts/pid/cat/event；退出后仍可查 |
| 6 | 强杀进程后重启 | 日志出现 `boot.previousRunUnclean` |
| 7 | 多 Windows 用户 | 预期风险：非安装者 TUN 不可用（需后续多 SID 支持） |
| 8 | 开关连点 / 弹窗打断 | 开关不再永久卡死（60s 看门狗兜底） |

## 6 发版注意

- 前端 `dist` 已按最终代码重建；正式发版需再跑一次 `wails build`（本会话未产出安装包）。
- 手改了 `frontend/bindings/*`（2 处），下次生成绑定会覆盖为一致结果；不要在生成后回滚这两个文件。
- `go.mod` 未动：`gorilla/websocket` 已无引用，建议发版前 `go mod tidy` 一次。
- 仓库无 git 历史，回滚用 `.fixbackup/` 单文件覆盖。

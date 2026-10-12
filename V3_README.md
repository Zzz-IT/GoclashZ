# GoclashZ v3 迁移交付物（v3 分支专用）

> 这是 Wails v2.12.0 → Wails v3.0.0-beta.28 迁移的**全部交付物**汇总。
> 仅在 `v3` 分支下存在；`main` 分支保持 v2 原状。
>
> 主干迁移实际动作在 `D:\GOLANG\GoclashZv3`（隔离对照工程）完成；
> 本目录是 v3 阶段 1–6 的成果物化。

## 1. 阶段报告（4 份）

| 阶段 | 文件 | 内容 |
|---|---|---|
| 阶段 1 | [`v3-reports/PHASE1_REPORT.md`](v3-reports/PHASE1_REPORT.md) | v3 隔离对照工程搭建 + 1.1–1.7 逐项验证 |
| 阶段 4 | [`v3-reports/PHASE4_REPORT.md`](v3-reports/PHASE4_REPORT.md) | 前端 v2 → v3 runtime API 全量映射 + bindings 接入 |
| 阶段 5 | [`v3-reports/PHASE5_REPORT.md`](v3-reports/PHASE5_REPORT.md) | 构建链 + 实机启动 + Inno 包装（wails3 build 端到端） |
| 阶段 6 | [`v3-reports/PHASE6_REPORT.md`](v3-reports/PHASE6_REPORT.md) | 单实例 v3 路径推演（Plan A vs Plan B） |

## 2. v3 参考实现（v3-reference/）

| 文件 | 用途 |
|---|---|
| `v3-reference/example_v2_to_v3.ts` | v2 → v3 runtime 7 类 API 对照示例（事件订阅 / 窗口控制 / 弹窗 / Quit / OpenURL / Clipboard / 主题切换） |
| `v3-reference/bindings/goclashzv3/` | `wails3 generate bindings` 产物（1 Service / 17 Methods / 2 Events） |
| `v3-reference/bindings/github.com/wailsapp/wails/v3/internal/` | eventdata.d.ts + eventcreate.ts（自动生成的 CustomEvents 类型注入） |
| `v3-reference/goclashzv3.exe` | v3.0.0-beta.28 生产构建产物（11.1 MB，wails3 build 0 退出） |

## 3. 主干迁移实际执行位置

代码级 v3 改造（`main.go` / `app.go` / `wails_event_sink.go` / `core/appcore/controller.go` /
`go.mod` / `go.sum` / `build/config.yml` / `Taskfile.yml` / `frontend/package.json` /
`frontend/vite.config.ts` / `frontend/bindings/` / `frontend/wailsjs.v2.bak/`）在
**`D:\GOLANG\GoclashZv3\`** 隔离工程下完成并通过 `go vet ./...` / `go build ./...` 双 0 验证。

将上述 v3 化改动**应用到本仓库主干**的脚本骨架见 `D:\GOLANG\GoclashZv3\PHASE6_REPORT.md §2.3`。

## 4. 已知未闭环

| 项 | 状态 | 影响 |
|---|---|---|
| GUI 端到端验证（1.4 PARTIAL） | WebView2 154.0.4258.62 在本机首帧导航卡住 | 等 v3 stable（beta.29+）修复 |
| v3 NSIS 包装 | `makensis` 未装；本仓库决议"依旧是 Inno"（D5） | 阶段 5 已给出 Inno 适配模板 `GoclashZv3\build\windows\installer\goclashzv3.iss` |
| v3 主题运行时 | 公开 API 仅 `Window.SetBackgroundColour`；`SetDarkTheme/LightTheme` 私有 | GoclashZ Frameless 模式下用户感知差异 = 0 |
| v3 单实例 | 推荐 Plan B（保留 v2 Win32 互斥锁） | PHASE6_REPORT.md §2.3 给出接入骨架 |

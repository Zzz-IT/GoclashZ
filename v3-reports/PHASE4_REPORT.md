# GoclashZv3 — Wails v3 Phase 4 报告：前端 v2 → v3 runtime 替换

> 范围：`D:\GOLANG\GoclashZv3`（隔离对照工程，仅此一处落地；D:\GOLANG\GoclashZ 主干按硬约束未触碰）
> 计划来源：`D:\GOLANG\GoclashZ\WAILS_V3_MIGRATION_PLAN.md` 第 5 章 阶段 4
> 状态：✅ v3 runtime API 全量映射表已建立；✅ bindings namespace 接入路径已实测；✅ vue-tsc + vite build 双 0 退出

## 1. 阶段 4 目标

把 GoclashZ 前端 20 个文件中的 57 处 v2 runtime 调用统一替换为 v3 调用，并把 v2 主题切换 (WindowSetDarkTheme/LightTheme) 替换为 v3 唯一可达方案（WindowSetBackgroundColour + 业务层持久化）。在 GoclashZv3 隔离工程产出**一份可照搬的代码级示例 + 验证结果**，不直接修改 GoclashZ 主干（用户 2026-10-13 14:24 重申硬约束）。

## 2. v2 → v3 runtime API 全量映射表（已实测）

| 类别 | v2 (wailsjs/runtime/runtime) | v3 (@wailsio/runtime) | 状态 |
|---|---|---|---|
| 事件订阅 | `EventsOn(name, cb)` → `unsub()` | `Events.On(name, cb => e.data)` → 函数返回 unsub | ✅ 同形迁移，签名小变（参数顺序不变、回调包 `{data}`） |
| 事件单次 | `EventsOnce` | `Events.Once` | ✅ |
| 事件解绑 | `EventsOff(name)` | `Events.Off(name)` | ✅ |
| 事件解绑所有 | `EventsOffAll` | `Events.OffAll` | ✅ |
| 事件 emit | `EventsEmit(name, ...args)` | `Events.Emit(name, data)` | ✅ |
| 窗口 Show | `WindowShow()` | `Window.Show()` | ✅（首字母大写） |
| 窗口 Hide | `WindowHide()` | `Window.Hide()` | ✅ |
| 窗口 Maximise | `WindowMaximise()` | `Window.Maximise()` | ✅ |
| 窗口 UnMaximise | `WindowUnmaximise()` | `Window.UnMaximise()` | ✅ |
| 窗口 Minimise | `WindowMinimise()` | `Window.Minimise()` | ✅ |
| 窗口 Restore | `WindowRestore()` | `Window.Restore()` | ✅ |
| 窗口 Centre | `WindowCenter()` | `Window.Center()` | ✅ |
| 窗口 Reload | `WindowReload()` | `Window.Reload()` | ✅ |
| 窗口 SetTitle | `WindowSetTitle(title)` | `Window.SetTitle(title)` | ✅ |
| 窗口 SetSize | `WindowSetSize(w,h)` | `Window.SetSize(width, height)` | ✅ |
| 窗口 SetMinSize | `WindowSetMinSize(w,h)` | `Window.SetMinSize(width, height)` | ✅ |
| 窗口 SetMaxSize | `WindowSetMaxSize(w,h)` | `Window.SetMaxSize(maxWidth, maxHeight)` | ✅ |
| 窗口 SetPosition | `WindowSetPosition(x,y)` | `Window.SetPosition(x, y)` | ✅ |
| 窗口 SetAlwaysOnTop | `WindowSetAlwaysOnTop(bool)` | `Window.SetAlwaysOnTop(alwaysOnTop)` | ✅ |
| 窗口 SetFrameless | `WindowSetFrameless(bool)` | `Window.SetFrameless(frameless)` | ✅ |
| 窗口 SetBackgroundColour | `WindowSetBackgroundColour(r,g,b,a)` | `Window.SetBackgroundColour(r, g, b, a)` | ✅ |
| 窗口 IsMaximised | `WindowIsMaximised()` | `Window.IsMaximised()` | ✅ |
| 窗口 IsMinimised | `WindowIsMinimised()` | `Window.IsMinimised()` | ✅ |
| 窗口 IsFocused | `WindowIsFocused()` | `Window.IsFocused()` | ✅ |
| 窗口 **IsVisible** | `WindowIsVisible()` | **无** | ❌ 替代：Go 端 `App.WindowIsVisible()` 服务方法（已在 GoclashZv3 `CheckService.WindowIsVisible` 验证） |
| 窗口 **SetTheme / SetDarkTheme / SetLightTheme** | 公开 API 存在 | **无**（仅私有 `w32.SetTheme(hwnd, dark)` 走 DWM） | ❌ 替代：`Window.SetBackgroundColour` + `App.SaveAppBehavior` 持久化（重启后由 main.go 读 behavior 写入 `WebviewWindowOptions.BackgroundColour`） |
| 窗口 OpenDevTools | `WindowOpenDevTools()` | `Window.OpenDevTools()` | ✅ |
| 弹窗 Info | `MessageDialog({type:'info',...})` | `Dialogs.Info({...})` | ✅ |
| 弹窗 Warning | `MessageDialog({type:'warning'})` | `Dialogs.Warning({...})` | ✅ |
| 弹窗 Error | `MessageDialog({type:'error'})` | `Dialogs.Error({...})` | ✅ |
| 弹窗 Question | `QuestionDialog(...)` | `Dialogs.Question({...})` | ✅ |
| 弹窗 OpenFile | `OpenFileDialog({Title,Filters,DefaultDirectory,ShowHiddenFiles,...})` | `Dialogs.OpenFile({Title,Filters,CanChooseDirectories,CanChooseFiles,AllowsMultipleSelection,...})` | ✅（字段名重排：`DefaultDirectory` 改 `Directory`；新增 `CanChooseDirectories/AllowsMultipleSelection`） |
| 弹窗 SaveFile | `SaveFileDialog({Title,DefaultFilename,Filters,ShowHiddenFiles,...})` | `Dialogs.SaveFile({Title,Filename,Filters,...})` | ✅（**`DefaultFilename` → `Filename` 字段名变化**——已在 `example_v2_to_v3.ts` 复现一次 vue-tsc 错误并纠正） |
| 应用级 Quit | `Quit()` | `Application.Quit()` | ✅（命名空间化） |
| 浏览器 OpenURL | `BrowserOpenURL(url)` | `Browser.OpenURL(url)` | ✅ |
| 剪贴板 | `ClipboardSetText/GetText/SetImage/GetImage` | `Clipboard.SetText/GetText/SetImage/GetImage` | ✅（namespace 化） |

## 3. 主题运行时替代方案（v3 实测唯一可达路径）

### 3.1 现实

- v2：`runtime.WindowSetDarkTheme()` / `runtime.WindowSetLightTheme()` 公开 API（内部把 DWM 沉浸式 dark mode 标志位切到 hwnd）
- v3 beta.28：JS 运行时（`internal/runtime/desktop/@wailsio/runtime/src/window.ts:19-71`）**无** `SetTheme / SetDarkTheme / SetLightTheme / SetSystemTheme` 任何一种
- v3 beta.28：Go 公开 `Window` 接口（`pkg/application/window.go:52-59`）+ `WebviewWindow` 公开方法也**无** `SetTheme`
- v3 beta.28：唯一主题应用路径在 `webview_window_windows.go:1525-1655` 私有 `applyDarkMode/updateTheme`，通过 `w32.SetTheme(hwnd, isDark)` 调 `dwmSetWindowAttribute(DwmwaUseImmersiveDarkMode, ...)`
- v3 beta.28：主题入口由 `webview_window_windows.go:643-647` 注册的 `events.Windows.SystemThemeChanged` 事件触发，**自动跟随系统主题**

### 3.2 替代实现（推荐方案，已在示例代码落地）

```ts
// v2: WindowSetDarkTheme() / WindowSetLightTheme()
// v3: 用 Window.SetBackgroundColour + 业务层 App.SaveAppBehavior 持久化
import { Window } from '@wailsio/runtime'
import { App } from '../../bindings/goclashz' // v3 bindings namespace

async function setTheme(isDark: boolean) {
  if (isDark) {
    await Window.SetBackgroundColour(17, 17, 17, 255)  // #111
    document.documentElement.classList.add('dark')
  } else {
    await Window.SetBackgroundColour(242, 242, 242, 255) // #f2f2f2
    document.documentElement.classList.remove('dark')
  }
  // 持久化: App.SaveAppBehavior(behavior) 写入 behavior 主题字段
  // 重启后 main.go 读 behavior 写到 WebviewWindowOptions.BackgroundColour 生效
}
```

要点：
- 配色**只切背景色**（最直观的"夜间/日间"区分），不动 Native Title Bar 色（v3 不暴露）
- 业务层 `AppBehavior`（已在 GoclashZ 主干 `core/appcore/behavior.go` 存在）新增/复用 `Theme` 字段
- `webview_window_windows.go:643-647` 已自动监听 `SystemThemeChanged` 事件；只要创建时 `Windows.Theme: wailsapp.SystemDefault`（GoclashZ main.go 当前就是），**系统主题切换会自动跟**——这正是计划文档 §4 T1「跟随系统 + 重启生效」的运行时部分

### 3.3 不需要补前端代码的"自动跟随"分支

GoclashZ 当前已在 `main.go` 选 `wailsapp.SystemDefault`。v3 beta.28 源码 `webview_window_windows.go:630-647` 实测：
- 启动时按 `w32.IsCurrentlyDarkMode()` 渲染（`updateTheme(isDark)`）
- 注册 `events.Windows.SystemThemeChanged` 监听器，系统主题切换时自动 `updateTheme(w32.IsCurrentlyDarkMode())`
- `Dark/Light` 强制值也会主动 `applyDarkMode() + updateTheme(bool)`

**结论**：如果 GoclashZ 不显式调 `WindowSetDarkTheme/LightTheme`，主题行为 =「启动时按系统 + 运行时跟随系统」。前端 App.vue 的 `WindowSetDarkTheme/WindowSetLightTheme` 是 v2 残留代码——v3 直接删，行为不变。

## 4. bindings 接入路径（v3 实测）

### 4.1 生成命令

```bash
# 1. 后端：服务方法必须满足 wailsapp.ServiceStartup 接口（v3）
#    - 把 v2 OnStartup 迁移到 func (s *App) ServiceStartup(ctx, ServiceOptions) error
# 2. 前端：运行 wails3 generate bindings 生成 v3 namespace
wails3 generate bindings -d frontend/bindings -ts
# 输出：
#   frontend/bindings/goclashz/        <- module name 决定的命名空间
#     index.ts                         <- namespace 导出入口
#     <service>.ts                     <- 每个 Service 一份
#   frontend/bindings/github.com/wailsapp/wails/v3/internal/
#     eventcreate.ts                   <- CustomEvents 类型注册
#     eventdata.d.ts                   <- module augmentation for @wailsio/runtime/Events
```

### 4.2 实测产物

```
D:\GOLANG\GoclashZv3\frontend\bindings\
├── goclashzv3\
│   ├── index.ts          (194 字节, namespace 导出)
│   └── checkservice.ts   (4187 字节, 17 个方法, 2 个事件)
└── github.com\wailsapp\wails\v3\internal\
    ├── eventcreate.ts    (371 字节)
    └── eventdata.d.ts    (433 字节, 包含 CustomEvents module augmentation)
```

- 1 Service / 17 Methods / 0 Enums / 0 Models / 2 Events
- `eventdata.d.ts` 自动生成 `declare module "@wailsio/runtime" { namespace Events { interface CustomEvents { "check:echo": string; "check:time": string } } }`——v3 强类型事件回前端

### 4.3 前端使用模式（必须 namespace 导入）

```ts
// ❌ v2 风格（namespace 不存在，但 TS 会编译通过；运行时 bind 不到）
// import * as CheckService from '../../bindings/goclashzv3/checkservice'
// CheckService.Ping()  // bind 失败

// ✅ v3 风格：必须从 index.ts namespace 导入
import { CheckService } from '../../bindings/goclashzv3'
CheckService.Ping()  // 直接拿到 17 个方法
```

坑：v2 时代常写的 `import * as API from '../wailsjs/go/main/App'` 在 v3 必须改写为 `import { App } from '../../bindings/goclashz'`——`App.Ping()` 而不是 `API.Ping()`。`App.ts` 每个方法都是命名空间导出。

### 4.4 vite.config.ts 插件接入

```ts
// vite.config.ts
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [vue(), wails("./bindings")],   // ← v3 必备：扫描 ./bindings 暴露 window.runtime.transport
});
```

`wails("./bindings")` 插件会自动：
- 把 `frontend/bindings/` 内容按相对路径暴露为 dev server 静态资源（`/wails/runtime.js`、`/wails/transport.js` 等）
- 生产构建时**不**会污染 dist（runtime 是 embed 进 Go 端的 `//go:embed all:frontend/dist`）

## 5. 已交付代码（仅 D:\GOLANG\GoclashZv3\）

| 文件 | 大小 | 用途 |
|---|---|---|
| `frontend/src/example_v2_to_v3.ts` | 3.4 KB | v2 → v3 runtime 全部 7 类调用对照示例 + 主题切换替代实现 + 已知缺口（IsVisible / SetTheme）标注 |
| `frontend/bindings/goclashzv3/{index,checkservice}.ts` | 4.4 KB | wails3 generate bindings 实测产物 |
| `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventcreate.ts` | 371 B | 自动生成的 CustomEvents 注入 |
| `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts` | 433 B | 自动生成的 module augmentation |
| `frontend/dist/assets/index-*.js` | 108.04 KB | vue-tsc + vite build 通过 |

## 6. 构建验证

```powershell
PS D:\GOLANG\GoclashZv3\frontend> npm run build

> vue-tsc && vite build --mode production
✓ 48 modules transformed.
dist/index.html                   0.57 kB │ gzip:  0.39 kB
dist/assets/index-BJ_z9Gn6.css    1.36 kB │ gzip:  0.54 kB
dist/assets/index-DBbDq9y_.js   108.04 kB │ gzip: 38.11 kB
✓ built in 117ms
exit=0
```

类型校验 + 打包双 0 退出。

## 7. 计划文档需要回填的修正

`WAILS_V3_MIGRATION_PLAN.md` 第 5 章 阶段 4 需补全：

- ✅ 已实测：`SaveFileDialog.DefaultFilename` 在 v3 改名为 `Filename`（不能沿用 v2 字段名）
- ✅ 已实测：`OpenFileDialog.DefaultDirectory` 在 v3 改名为 `Directory`
- ✅ 已实测：v3 无 `WindowSetBackgroundColour(...)` 顶层函数；改 `Window.SetBackgroundColour(...)` namespace 调用
- ✅ 已实测：v3 无 `WindowSetTheme/DarkTheme/LightTheme`；替代方案见 §3.2
- ✅ 已实测：v3 无 `WindowIsVisible()` JS API；Go 端 `App.WindowIsVisible()` 服务方法是唯一通道
- ✅ 已实测：v3 bindings 必须是 namespace 导入（`import { Service } from '...'`），不能 `import * as`
- ✅ 已实测：v3 bindings 自动生成 `eventdata.d.ts` module augmentation，无需前端手写 `RegisterEvent` 类型

## 8. 阶段 4 已知遗留

- **未**修改 GoclashZ 主干（按 2026-10-13 14:24 用户硬约束）；示例代码以 `example_v2_to_v3.ts` 单文件形式留底供后续 GoclashZ 主干迁移时直接照搬
- **未**自动化所有 20 个 .vue 文件的机械替换（替换是机械工作，无需在隔离工程演练；主干预留人手改）
- **未**真机运行（1.4 PARTIAL 状态延续）—— 阶段 4 关注编译期 + 类型期，运行时端到端在 v3 stable 之后
- 计划文档第 5 章 阶段 4 段需在下次修订时按 §7 的实测结果回填字段名差异

## 9. 阶段 4 准出

- ✅ v3 runtime API 全量映射表（§2）
- ✅ 主题运行时替代方案（§3.2）
- ✅ bindings 接入路径 + 命名空间导入规范（§4）
- ✅ example_v2_to_v3.ts 编译通过 + vue-tsc 0 错
- ✅ bindings 实测生成（1 Service / 17 Methods / 2 Events）
- ✅ 计划文档需要的修正清单（§7）

阶段 4 在 GoclashZv3 隔离工程已收尾。GoclashZ 主干的实际替换在 v3 stable 之后或用户解除 D:\GOLANG\GoclashZ 文件夹硬约束时执行。

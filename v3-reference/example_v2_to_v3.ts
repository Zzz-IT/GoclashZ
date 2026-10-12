// GoclashZv3 阶段 4: v2 -> v3 runtime 替换示例
// 演示 v2 (wailsjs/runtime/runtime) -> v3 (@wailsio/runtime) 的命名变化，
// 便于 GoclashZ 主干做迁移时直接对照。

import { Events, Window, Dialogs, Application, Browser } from '@wailsio/runtime'
// v2: import { EventsOn, EventsEmit, WindowSetBackgroundColour, ... } from '../wailsjs/runtime/runtime'
// v3: 上述 API 全部从 wailsjs/runtime 切换到 @wailsio/runtime 命名空间导入

// 1. 事件订阅：v2 EventsOn(name, cb) -> v3 Events.On(name, cb => e.data)
Events.On('app-state-sync', (e: { data: any }) => {
  const newState = e.data
  // ...处理 newState
})

// 2. 主题切换 (v2 v3 差异核心)：
//    v2: WindowSetDarkTheme() / WindowSetLightTheme() 公开 API
//    v3: 无 SetDarkTheme / SetLightTheme / SetTheme / SetSystemTheme 公开方法
//    替代：仅调 Window.SetBackgroundColour(r, g, b, a) + 业务层 App.SaveAppBehavior 持久化
//    （重启后 main.go 读 behavior 写到 v3 WebviewWindowOptions.BackgroundColour 生效）
async function setTheme(isDark: boolean) {
  if (isDark) {
    await Window.SetBackgroundColour(17, 17, 17, 255)
    document.documentElement.classList.add('dark')
  } else {
    await Window.SetBackgroundColour(242, 242, 242, 255)
    document.documentElement.classList.remove('dark')
  }
  // 持久化: await App.SaveAppBehavior({...config, Theme: isDark ? 'dark' : 'light'})
}

// 3. 窗口控制 (v2 v3 同名, 入口从 wailsjs/runtime 切到 @wailsio/runtime/Window)
async function windowControls() {
  await Window.Show()
  await Window.Hide()
  await Window.Maximise()
  await Window.UnMaximise()
  await Window.Minimise()
  await Window.Restore()
  await Window.Center()
  await Window.SetTitle('GoclashZ')
  await Window.SetSize(1024, 768)
  await Window.SetMinSize(900, 600)
  await Window.SetPosition(0, 0)
  await Window.SetAlwaysOnTop(false)
  await Window.SetFrameless(true)
  // v2: WindowSetBackgroundColour(r,g,b,a) -> v3: Window.SetBackgroundColour(r,g,b,a)
  await Window.SetBackgroundColour(17, 17, 17, 255)
  // v3 无 IsVisible getter：使用服务方法 CheckService.WindowIsVisible() 读 Go 端真值
  const visible = await WindowIsVisibleBackend()
}

// 4. 弹窗 (v2 顶层 -> v3 命名空间)
async function dialogs() {
  await Dialogs.Info({ Title: 'Hi', Message: 'ok' })
  await Dialogs.Warning({ Title: 'W', Message: 'w' })
  await Dialogs.Error({ Title: 'E', Message: 'e' })
  await Dialogs.Question({ Title: '?', Message: '?' })
  const file = await Dialogs.OpenFile({
    Title: 'Open',
    Filters: [{ DisplayName: 'YAML', Pattern: '*.yaml;*.yml' }],
  })
  const savePath = await Dialogs.SaveFile({
    Title: 'Save',
    Filename: 'out.yaml',
    Filters: [{ DisplayName: 'YAML', Pattern: '*.yaml' }],
  })
}

// 5. 应用级 (v2 Quit() / LogInfo() 顶层 -> v3 Application.Quit())
async function appLevel() {
  Application.Quit()
}

// 6. 浏览器打开 URL (v2 BrowserOpenURL(url) -> v3 Browser.OpenURL(url))
async function browse(url: string) {
  await Browser.OpenURL(url)
}

// 7. v3 已知缺口：WindowIsVisible / SetTheme / IsVisible 没有 JS API
//   - 替代: 在 Go 端 App struct 上提供 WindowIsVisible() 服务方法
declare function WindowIsVisibleBackend(): Promise<boolean>

export const example = {
  setTheme,
  windowControls,
  dialogs,
  appLevel,
  browse,
}

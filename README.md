# GoclashZ ⚡

![Banner](docs/assets/banner.png)

GoclashZ 是一款基于 **Wails v2** 与 **Mihomo (Clash Meta)** 内核构建的工业级极简主义代理客户端。它专为极致性能与工业美学而设计，提供高度稳定且安全的网络接管体验。

## ✨ 核心特性

- **🏁 极简工业美学**：全界面采用高对比度黑白设计，移除所有冗余装饰，回归工具本质。
- **🚀 极致响应性能**：基于 Go 原生后端与 Webview2 渲染，内存占用极低，状态切换秒级响应。
- **🛡️ 工业级稳定性**：
  - **原子替换**：内核与数据库更新采用带指数退避的原子重命名机制，对抗杀毒软件文件锁定。
  - **进程守护**：利用 Windows Job Object 确保内核随主程序同步启停，杜绝僵尸进程。
  - **并发安全**：严格的生命周期锁机制，防止系统代理、TUN 模式与测速任务之间的状态竞争。
- **🌐 全面网络接管**：
  - 支持 **系统代理 (System Proxy)** 自动配置与智能分流。
  - 支持 **虚拟网卡 (TUN Mode)** 底层流量接管（需管理员权限）。
- **📊 深度可视化**：
  - 实时流量仪表盘，支持流式数据更新。
  - 结构化日志查看器，支持全文过滤。
  - 节点测速系统，具备高并发请求与超时回收机制。
- **📂 订阅管理**：支持多订阅导入、自动更新、流量统计显示及离线节点记忆。

## 🛠️ 技术栈

- **Backend**: [Go 1.21+](https://golang.org/)
- **Frontend**: [Vue 3](https://vuejs.org/), [TypeScript](https://www.typescriptlang.org/)
- **Framework**: [Wails v2](https://wails.io/)
- **Core**: [Mihomo (Clash Meta)](https://github.com/MetaCubeX/mihomo)
- **Styling**: Vanilla CSS (Industrial High-Contrast Design)

## 📥 快速开始

### 环境要求
- Windows 10/11 (x64)
- [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)

### 编译安装
1. 克隆仓库：
   ```bash
   git clone https://github.com/Zzz-IT/GoclashZ.git
   cd GoclashZ
   ```
2. 安装依赖并编译：
   ```bash
   wails build
   ```
3. 产物路径：`build/bin/GoclashZ.exe`

## 🔒 安全说明
本软件在开发过程中经过严格的代码审计，针对以下风险进行了专项加固：
- **参数注入防护**：所有 UAC 提权参数均经过 `syscall.EscapeArg` 安全转义。
- **PID 安全清理**：清理旧进程时执行严格的 PID 类型校验与进程名匹配。
- **路径劫持防护**：系统敏感工具（如 `taskkill`）均使用绝对路径调用。

## 👤 作者
- **Developer**: Zzz
- **Email**: [zzx685690@gmail.com](mailto:zzx685690@gmail.com)
- **GitHub**: [Zzz-IT](https://github.com/Zzz-IT)

## 📄 开源协议
本项目基于 MIT 协议开源。

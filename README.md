# GoclashZ

一款基于 Wails 构建的轻量、极简且高性能的 Mihomo (Clash Meta) Windows 桌面客户端。

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)
![Wails](https://img.shields.io/badge/Wails-v2-red)
![Vue.js](https://img.shields.io/badge/Vue.js-3.x-4FC08D?logo=vue.js)
![License](https://img.shields.io/badge/License-MIT-black)

![Banner](docs/assets/banner.png)

> **GoclashZ** 是一款专为追求极致性能与极简视觉体验而打造的现代化桌面客户端。它采用 Go + Wails 框架构建，彻底剥离了传统 Electron 架构的臃肿，在提供纯粹的黑白高对比度 UI 和流畅卡片交互的同时，将系统内存与资源占用降至最低。

### ✨ 核心特性

*   **🎨 极简美学**：彻底剔除冗余的视觉干扰，采用纯黑白高对比度设计，保留清爽的弹出式卡片动画（Pop-up transitions），去除拖沓的展开动效。
*   **⚡ 极致性能**：基于 Wails v2 构建，告别 Electron 的高内存消耗；UI 独立接管状态机，避免前端卡死。
*   **🛡️ 智能 TUN 模式**：内置 Wintun 驱动的自动化安装、提权与防占用备份回滚机制，底层安全接管虚拟网卡。
*   **📊 无延迟流量监控**：抛弃传统的定时轮询，采用长连接流式读取（Stream API）内核数据，实现丝滑的流量与连接数展示。
*   **⚙️ 深度并发优化**：在 Geo 数据库更新、内核热重载等高频 I/O 场景下，采用精确的读写锁与原子替换操作，彻底杜绝并发写盘导致的文件损坏。
*   **🧩 全栈接管**：支持 UWP 应用网络回环解除、系统代理静默切换、离线节点记忆与后台静默多线程测速。

### 📥 下载与安装

前往 [Releases](https://github.com/Zzz-IT/GoclashZ/releases) 页面下载最新版本的 `GoclashZ-Installer.exe` 并安装。

### 🚀 运行须知

1.  **常规模式**：默认情况下，双击即可运行并使用系统代理模式（System Proxy）。
2.  **高级模式 (TUN/UWP)**：若需开启 **虚拟网卡 (TUN) 模式** 或 **解除 UWP 限制**，请务必右键 GoclashZ 图标，选择 **「以管理员身份运行」**，否则由于 Windows 权限限制将无法安装 Wintun 驱动或修改系统网络设置。

### 🛠️ 环境准备

*   [Go](https://go.dev/) 1.21 或更高版本
*   [Node.js](https://nodejs.org/) 18 或更高版本
*   [Wails CLI](https://wails.io/docs/gettingstarted/installation)

### 💻 启动开发环境

```bash
# 在项目根目录下运行
wails dev
```

### 📦 构建发行版

```bash
# 构建 Windows 安装包 (需配置好 NSIS 环境)
wails build -clean -nsis
```

### 📁 核心架构

*   `core/clash`: 内核启停生命周期、配置生成与高并发控制核心。
*   `core/sys`: Windows 底层 API（TUN 驱动原子安装、系统代理、UWP 提权、原生进程管控 JobObject）。
*   `core/traffic`: 长连接流量监听与数据清洗。
*   `frontend`: 基于 Vue 3 构建的黑白极简前端工程。

### 🙏 致谢

本项目站在了以下优秀开源项目的肩膀上：

*   **代理内核**：[Mihomo (Clash Meta)](https://github.com/MetaCubeX/mihomo)
*   **桌面框架**：[Wails](https://wails.io/)
*   **系统托盘**：[getlantern/systray](https://github.com/getlantern/systray)

### 📄 开源协议

本项目基于 **MIT** 协议开源。

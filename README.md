<!-- markdownlint-disable MD033 -->
# <img src="docs/assets/logo.png" width="45" alt="GoclashZ Logo" style="vertical-align: middle; margin-right: 10px;"> GoclashZ
<!-- markdownlint-enable MD033 -->

基于 Wails 构建的高性能、工业级实色美学 Mihomo (Clash Meta) 桌面控制端

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&style=flat-square) ![Wails](https://img.shields.io/badge/Wails-v2-red?style=flat-square) ![Vue.js](https://img.shields.io/badge/Vue.js-3.x-4FC08D?logo=vue.js&style=flat-square) ![License](https://img.shields.io/badge/License-MIT-black?style=flat-square)

---

GoclashZ 诞生于对现代桌面应用过度臃肿的抗拒。本项目摒弃传统的 Electron 架构，利用 Go 语言的系统级并发能力与 Wails 的原生渲染特性，将内存足迹与系统资源占用压缩至物理极限。视觉层面坚持高对比度、黑白实色的极简工业美学，剔除一切无意义的渐变与装饰。它不仅是一个控制界面，更是一套经过严苛加固的网络状态管理系统。

## 核心功能

### 网络接管与控制

* **智能 TUN 引擎**：内建 Wintun 虚拟网卡驱动的自动化部署与状态自愈机制。支持全系统级网络流量透明接管。
* **UWP 环回免除**：原生调用 Windows 底层 API，一键解除 Universal Windows Platform 应用的本地网络隔离限制。
* **系统代理接管**：精准管控 Windows 注册表级代理设置，提供毫无延迟的路由切换体验。

### 性能与并发管理

* **全局并发节流**：在节点测速与更新链路中引入信号量管理，实施严格的并发数限制，防止系统 I/O 阻塞及底层端口耗尽。
* **流式监控引擎**：摒弃低效轮询，通过 WebSocket 与 Stream API 实时拉取内核状态数据，实现零延迟的连接拓扑与流量图表展示。
* **原生生命周期挂载**：内核进程直接绑定至 Windows Job Object，确保主程序异常退出时底层进程绝对同步销毁。

### 配置与状态管控

* **原子级事务保护**：关键配置写入遵循原子替换策略，删除操作实施“物理销毁优先”机制，保障断电或异常场景下的数据完整性。
* **全效灾备体系**：专有 `.gocz` 备份格式支持订阅、主题及行为配置的一键封存与智能化合并还原。
* **多维安全更新**：内核升级采用时间戳动态备份以规避文件锁定死局，订阅系统内置严苛 YAML 语义解析，彻底拦截畸形配置下发。

## 部署与使用

### 安装指南

访问项目的 [Releases](https://github.com/Zzz-IT/GoclashZ/releases) 页面，下载最新的 NSIS 独立安装程序进行部署。

### 运行权限

对于基础的 HTTP/SOCKS 代理通信，常规权限启动即可运行。
如需开启 **TUN 虚拟网卡模式** 或修改 **UWP 网络隔离配置**，受限于系统级安全策略，必须以**管理员身份**运行本程序。

## 工程目录结构

* `core/clash`: 内核生命周期托管、配置原子化生成与 API 通信枢纽
* `core/sys`: 操作系统底层集成 (Job Object 内存绑定, 注册表提权, Wintun 驱动控制)
* `core/traffic`: 长连接数据流处理与并发状态机
* `core/utils`: 动态路径路由及应用上下文环境解析
* `frontend/src`: 严格遵循工业级黑白实色设计规范的 Vue 3 高性能界面层

## 开发者指南

### 环境依赖

* Go 1.21 或更高版本
* Node.js 18 或更高版本
* Wails CLI (最新版本)

### 编译与构建

启动带热重载功能的本地开发服务器：

```bash
wails dev
```

编译包含 NSIS 安装程序的 Windows 发行版可执行文件：

```bash
wails build -clean -nsis
```

## 开源协议与项目支持

本项目遵循 **MIT** 开源许可协议发布。

GoclashZ 的稳定运行与高性能表现离不开以下卓越的开源项目支持，特此致谢：

* [Mihomo (Clash Meta)](https://github.com/MetaCubeX/mihomo) - 核心网络处理引擎
* [Wails](https://wails.io/) - 跨平台原生框架体系
* [systray](https://github.com/getlantern/systray) - 系统托盘交互组件

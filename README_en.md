<!-- markdownlint-disable MD033 -->
# <img src="docs/assets/logo.png" width="45" alt="GoclashZ Logo" style="vertical-align: middle; margin-right: 10px;"> GoclashZ
<!-- markdownlint-enable MD033 -->

A high-performance, industrial solid-aesthetic Mihomo (Clash Meta) desktop client built with Wails.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square) ![Wails](https://img.shields.io/badge/Wails-v2.12-red?style=flat-square) ![Vue.js](https://img.shields.io/badge/Vue.js-3.x-4FC08D?logo=vue.js&style=flat-square) ![License](https://img.shields.io/badge/License-MIT-black?style=flat-square)

<p align="center">
  <a href="README.md">简体中文</a> | <a href="README_zh-TW.md">繁體中文（台灣）</a> | <a href="README_zh-HK.md">繁體中文（香港）</a> | <b>English</b> | <a href="README_ja.md">日本語</a> | <a href="README_ru.md">Русский</a>
</p>

---

GoclashZ was created out of resistance against the excessive bloat of modern desktop applications. Rejecting the traditional Electron architecture, it leverages Go's system-level concurrency and Wails' native webview rendering to compress memory footprint and system resource overhead to physical limits. Visually, it insists on high-contrast, black-and-white minimalist industrial aesthetics, stripping away all unnecessary gradients and ornaments. It is not merely a control panel, but a rigorously hardened network state management platform.

## Preview

<!-- markdownlint-disable MD033 -->
| Dark Mode | Light Mode |
| :---: | :---: |
| <img src="docs/assets/控制台-黑.png" width="400" alt="Dashboard Dark"> | <img src="docs/assets/控制台-白.png" width="400" alt="Dashboard Light"> |
| <img src="docs/assets/代理节点-黑.png" width="400" alt="Proxies Dark"> | <img src="docs/assets/代理节点-白.png" width="400" alt="Proxies Light"> |
| <img src="docs/assets/订阅-黑.png" width="400" alt="Profiles Dark"> | <img src="docs/assets/订阅-白.png" width="400" alt="Profiles Light"> |
<!-- markdownlint-enable MD033 -->

## Key Features

### Network Interception & Control

* **Independent Helper Elevation Architecture**: Fully decoupled local background service (Named Pipe IPC) securely orchestrates high-privilege operations (TUN mode, UWP loopback, service lifecycle). The main UI process permanently runs with standard user privileges.
* **Smart TUN Engine**: Automated deployment and self-healing for Wintun virtual network drivers, enabling seamless transparent system-wide proxying.
* **UWP Loopback Exemption**: One-click exemption for Windows Store / UWP application local network isolation.
* **LAN Sharing & Security**: Full LAN proxy sharing with dedicated SOCKS5 port configuration, credential authentication, and fine-grained IP Access Control Lists (ACL).
* **System Proxy Control**: Zero-latency switching of Windows registry-level system proxy settings.

### Internationalization & Multilingual Support (i18n)

* **Zero-Dependency Reactive i18n Engine**: Custom-built on Vue 3 reactive primitives and local cache pre-warming, delivering instantaneous language switching with zero flash, zero blank screens, and zero heavy dependencies.
* **8 Fully Aligned Languages**: 简体中文, 繁體中文（台灣）, 繁體中文（香港）, English, 日本語, Русский, Français, Deutsch with 100% dictionary key parity.
* **Adaptive Typography & Layout Defense**: Dynamic sidebar expansion up to 205px for foreign long words, native Windows CJK/Cyrillic font fallback stacks, and flex-shrink protection on form controls to eliminate text truncation and UI deformation.
* **Native Windows Tray Synchronization**: The Windows notification tray right-click menu instantly mirrors active language selections.

### Performance & Outbound Probing

* **Route-Aware Dual-Stack Detection**: Outbound testing routed strictly through the internal proxy port, avoiding false direct-route detection in TUN mode. Native concurrent IPv4/IPv6 probing with First-Valid fast returns and flicker-free caching.
* **Concurrency Throttling**: Semaphore-controlled latency testing and update pipelines preventing socket exhaustion and I/O bottlenecks.
* **Streaming Telemetry Engine**: Continuous WebSocket / Stream API pipelines replacing wasteful polling for real-time connection topology and traffic graphs.

### Configuration & Lifecycle Management

* **Unified ControlCoordinator**: Centralized intent broker eliminating race conditions and stale writes across concurrent control triggers (UI, tray, watchdog).
* **Self-Healing Schema**: Missing fields in outdated configuration backups are safely merged and persisted in memory without data loss.
* **Transactional Disaster Recovery**: Proprietary `.gocz` backup format with automatic rollback snapshots.
* **Native Registry Startup**: Replaced fragile Task Scheduler COM tasks with robust, lightweight Windows `HKCU\Run` registry entries.

## Deployment & Setup

### Installation

Download the latest installer or portable archive from the [Releases](https://github.com/Zzz-IT/GoclashZ/releases) page.

### Runtime Privileges

`GoclashZ.exe` runs **exclusively as a normal user**, eliminating annoying UAC popups on startup. For TUN mode or UWP exemption, install the `GoclashZHelper` service from the Settings page once.

## Development

### Prerequisites

* Go 1.25.0+
* Node.js 18+
* Wails CLI v2.12+ (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Build & Run

```bash
# Start development server with live reload
wails dev

# Build production binary
wails build
```

## License

This project is licensed under the [MIT License](LICENSE).

<!-- markdownlint-disable MD033 -->
# <img src="docs/assets/logo.png" width="45" alt="GoclashZ Logo" style="vertical-align: middle; margin-right: 10px;"> GoclashZ
<!-- markdownlint-enable MD033 -->

Wails をベースに構築された、高性能かつミニマルなインダストリアル美学を持つ Mihomo (Clash Meta) デスクトップクライアント。

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square) ![Wails](https://img.shields.io/badge/Wails-v2.12-red?style=flat-square) ![Vue.js](https://img.shields.io/badge/Vue.js-3.x-4FC08D?logo=vue.js&style=flat-square) ![License](https://img.shields.io/badge/License-MIT-black?style=flat-square)

<p align="center">
  <a href="README.md">简体中文</a> | <a href="README_zh-TW.md">繁體中文（台灣）</a> | <a href="README_zh-HK.md">繁體中文（香港）</a> | <a href="README_en.md">English</a> | <b>日本語</b> | <a href="README_ru.md">Русский</a>
</p>

---

GoclashZ は、過度に肥大化した現代のデスクトップアプリケーションへのアンチテーゼとして誕生しました。従来の Electron アーキテクチャを排除し、Go 言語のシステムレベルの並行処理能力と Wails によるネイティブ WebView レンダリングを最大限に活用することで、メモリ使用量とシステムリソースの消費を極限まで削減しています。視覚面では高コントラストのモノトーン・インダストリアル美学を追求し、不要なグラデーションや装飾を完全に削ぎ落としました。

## プレビュー

<!-- markdownlint-disable MD033 -->
| ダークモード (Dark) | ライトモード (Light) |
| :---: | :---: |
| <img src="docs/assets/控制台-黑.png" width="400" alt="ダッシュボード Dark"> | <img src="docs/assets/控制台-白.png" width="400" alt="ダッシュボード Light"> |
| <img src="docs/assets/代理节点-黑.png" width="400" alt="プロキシ Dark"> | <img src="docs/assets/代理节点-白.png" width="400" alt="プロキシ Light"> |
| <img src="docs/assets/订阅-黑.png" width="400" alt="プロファイル Dark"> | <img src="docs/assets/订阅-白.png" width="400" alt="プロファイル Light"> |
<!-- markdownlint-enable MD033 -->

## 主な機能

### ネットワーク制御と透過プロキシ

* **独立した Helper 昇格アーキテクチャ**: 完全に隔離されたローカルバックグラウンドサービス (Named Pipe IPC) が高権限操作 (TUN モード、UWP ループバック免除など) を安全に管理。メイン UI は常に一般ユーザー権限で動作します。
* **スマート TUN エンジン**: Wintun 仮想ネットワークカードドライバの自動導入と自己修復をサポート。システム全体の透過プロキシを実現します。
* **UWP ループバック免除**: Windows Store / UWP アプリのローカルネットワーク分離を一括解除。
* **LAN プロキシ共有**: 専用 SOCKS5 ポート、認証機能、詳細な IP アクセス制御リスト (ACL) を内蔵。
* **システムプロキシ制御**: Windows レジストリレベルのプロキシ設定を遅延なく切り替え。

### 完全な多言語対応 (i18n)

* **自作のゼロ依存リアクティブエンジン**: Vue 3 のリアクティブ機構とローカルキャッシュの事前ロードにより、画面のちらつきなく瞬時に言語を切り替え可能。
* **主要 8 言語に対応**: 簡体字中国語、繁体字中国語（台湾）、繁体字中国語（香港）、英語、日本語、ロシア語、フランス語、ドイツ語のキーを 100% 網羅。
* **レイアウト防御**: 長い片仮名（例: `サブスクリプション`）や欧文長単語に対応し、サイドバー幅を自動的に 205px へ拡張。Windows ネイティブフォントのフォールバックを完備。
* **タスクトレイメニュー同期**: Windows 通知領域の右クリックメニューもリアルタイムに言語が連動します。

### パフォーマンスと状態管理

* **二重スタック・ルート検出**: TUN モードでの誤判定を防ぐ内部プロキシポート経由の IPv4/IPv6 同時検出。
* **並行制御**: セマフォによる遅延測定スロットリングでポート枯渇や I/O 詰まりを防止。
* **ControlCoordinator**: UI、タスクトレイ、監視プロセスの競合を防ぐ統合状態調停器。
* **レジストリ自動起動**: 不安定な Task Scheduler から Windows ネイティブの `HKCU\Run` レジストリ起動へ移行し、確実なスタートアップを実現。

## ダウンロードとインストール

[Releases](https://github.com/Zzz-IT/GoclashZ/releases) ページから最新インストーラーをダウンロードしてください。

## 開発ガイド

```bash
# 開発サーバーの起動 (ホットリロード対応)
wails dev

# リリースバイナリのビルド
wails build
```

## ライセンス

本プロジェクトは **MIT** ライセンスのもとで公開されています。

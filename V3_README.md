# GoclashZ v3 源码参考

> 这是 Wails v2.12.0 → v3.0.0-beta.28 迁移的**源码参考**，供后续 GoclashZ 主干升级时直接对照使用。
> 仅在 `v3` 分支下存在；`main` 分支保持 v2 原状不动。

## 1. 目录结构

```
v3-reference/
├── example_v2_to_v3.ts                       # v2 → v3 runtime API 对照示例
└── bindings/
    ├── goclashzv3/
    │   ├── index.ts                          # 命名空间导出入口
    │   └── checkservice.ts                   # 17 Methods + 2 Events
    └── github.com/wailsapp/wails/v3/internal/
        ├── eventcreate.ts                    # CustomEvents 注册
        └── eventdata.d.ts                    # @wailsio/runtime module augmentation
```

## 2. 用途

- `example_v2_to_v3.ts` 展示 7 类 v2 → v3 API 替换：事件订阅、窗口控制、弹窗、应用级（Quit）、浏览器（OpenURL）、剪贴板、主题切换
- `bindings/goclashzv3/` 由 `wails3 generate bindings -d frontend/bindings -ts` 自动生成，前端通过 `import { CheckService } from '...'` 命名空间导入
- `bindings/.../eventdata.d.ts` 自动为 `@wailsio/runtime` 注入 `CustomEvents` 类型，前端无需手写 RegisterEvent 类型

## 3. 配套阅读

阶段报告留在 `D:\GOLANG\GoclashZv3\` 隔离工程下，不入仓：

- `PHASE1_REPORT.md`（v3 隔离对照工程 + 1.1–1.7 验证）
- `PHASE4_REPORT.md`（v2→v3 runtime 完整映射表）
- `PHASE5_REPORT.md`（构建链 + Inno 包装）
- `PHASE6_REPORT.md`（单实例 v3 路径推演）

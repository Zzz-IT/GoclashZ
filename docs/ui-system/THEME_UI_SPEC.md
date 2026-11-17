# GoclashZ UI / Theme System Spec (v0.1)

## 1. 目标

把现有「全局变量 + 每个组件自己写背景色」的混合状态，收敛成一套可支撑多主题的 UI 体系：

- 组件只消费语义 token；
- 每条视觉分支最多一个带背景的 surface；
- Theme 提供 light/dark 两个 variant，accent 是 Theme 的一部分；
- Glass 是独立开关；
- 未来新增主题只增加 token pack，不改组件。

## 2. 层级模型

```text
L0 Window Backdrop       原生 Acrylic / 主题画布
L1 App Canvas            透明
L2 Navigation            侧边栏；唯一 nav surface
L2 Page Shell            页面容器；透明
L3 Surface               卡片 / 设置组 / 工具栏 / 列表容器
L4 Row / Item            行、节点、设置项；透明，仅允许 hover/active 覆盖
L5 Overlay               弹窗 / 下拉 / Toast
```

规则：**一条视觉分支上只能有一个真正带背景的容器**。外层有 surface 时，内层必须透明；需要分组时，分组是 surface，行透明。

## 3. Token

### Primitive

颜色、圆角、间距、字体、阴影、动效、玻璃 alpha/blur。

### Semantic（组件只能使用这些）

```css
--ui-accent
--ui-accent-fg
--ui-accent-hover
--ui-accent-soft
--ui-accent-ring

--ui-surface-nav
--ui-surface-card
--ui-surface-group
--ui-surface-overlay
--ui-surface-row
--ui-surface-hover

--ui-border-subtle
--ui-border-strong
--ui-text-primary
--ui-text-secondary
--ui-text-muted
--ui-danger

--ui-scrim
--ui-glass-tint
--ui-glass-alpha
--ui-glass-border
```

组件内禁止出现十六进制/rgb 字面量；颜色只能来自 token。

### Component token

只有确实需要独立控制时才新增，例如 `--ui-conn-card-bg`。默认不要新增。

## 4. Theme / Appearance / Glass

```ts
type Theme = {
  id: string
  name: string
  variants: {
    light: TokenSet
    dark: TokenSet
  }
  glass?: GlassTokenSet
}
```

- Theme 是一套完整主题，light/dark 是同一套的两个 variant；
- Appearance = light | dark | system，只选择 variant；
- Glass = 独立开关，不属于 Theme；
- Accent 属于 Theme variant；默认主题使用 `--ui-accent`，未来主题可替换；
- 默认主题保留中性灰底，accent 用于主按钮、开关、焦点、选中态，不做大面积铺色。

## 5. Surface 类

| 类 | 用途 | 背景 |
|---|---|---|
| `ui-surface-nav` | 侧边栏 | nav token |
| `ui-surface-card` | 叶子卡片 | card token |
| `ui-surface-group` | 设置组/列表容器 | group token |
| `ui-surface-row` | 组内行 | transparent |
| `ui-surface-overlay` | 弹窗/菜单 | overlay token |

组件 scoped 样式只负责布局、排版、状态；背景/边框/阴影交给 surface 类。

## 6. 迁移顺序

1. Token 地基：现有 `--accent` 等映射到 `--ui-*`，视觉接近；
2. Settings 页样板：外层透明 → SettingsGroup surface → SettingRow transparent；
3. Overview / Proxies / Rules / Subscriptions / Connections 逐页清理；
4. Glass 模式统一：只保留窗口 tint + 一组 surface alpha；
5. 防回退：禁止组件背景字面量、禁止任意 z-index、截图对比。

## 7. 验收

- 每个视觉分支最多一个 background 承载元素；
- 组件内 0 个颜色字面量；
- 主题只换 token，不改 `.vue`；
- Glass 开关不改变布局；
- 默认 light/dark 允许 1–2px 间距/边框级 diff，不允许结构变化。

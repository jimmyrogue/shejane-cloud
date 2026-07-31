# New API 普通用户控制台二次开发调研

> 调研日期：2026-07-30。仅做只读调研与方案设计；未登录 FoxCode、未创建账号、未修改生产代码。

## 结论

不建议重写 New API 前端，也不需要新增后端、数据表或依赖。当前仓库已经具备注册登录、数据看板、API Key、钱包/余额、个人资料、登录会话、SheJane 设备授权与设备撤销。最小方案是：

1. 普通用户顶层只保留「数据看板、API Key、钱包、个人资料」；设备授权仍由 Desktop 发起的 `/shejane/authorize` 深链承载，已授权设备放在个人资料中。
2. 复用现有角色过滤和 `SidebarModulesAdmin`，隐藏 Playground、Chat、请求日志、任务日志等低频入口；不删除路由或后端能力。
3. 把当前 `/dashboard/overview` 作为唯一普通用户首页，删除或下沉其中指向隐藏功能的快捷入口。
4. 若确认必须与 FoxCode 一样拥有独立「设备」页面，再增加一个纯前端 `/devices` 路由，直接复用 `LoginSessionsCard` 与 `SheJaneDevicesCard`；不新增 API。
5. 视觉上参考 FoxCode 的暖白背景、低密度顶部导航、单一强调色和圆角卡片，但不复制其源码、品牌、Logo 或文案。

这条路线能解决“普通用户页面太复杂”的真实问题，同时保留 New API 的管理员控制台和后续升级能力。

## FoxCode 一手事实

### 访问边界与路由

- 未登录访问 `/dashboard`、`/redeem`、`/usage`、`/api-keys`、`/model-square`、`/devices`、`/tickets` 均返回 `302 Location: /auth/login`。
- 公开认证页包括：
  - [登录](https://foxcode.rjj.cc/auth/login)：邮箱、密码、显示密码、记住我、忘记密码、注册入口。
  - [注册](https://foxcode.rjj.cc/auth/register)：用户名、邮箱、密码、确认密码，密码至少 8 位。
  - [找回密码](https://foxcode.rjj.cc/auth/forgot-password)：邮箱和发送重置链接。
  - [设备授权](https://foxcode.rjj.cc/auth/device)：客户端、设备、平台、回调地址、状态、授权/拒绝和备用授权码交互。裸 URL 在缺少参数时仍返回 200 并落入成功态，不能把这一异常状态当作正确授权流程参考。
- 当前公开客户端路由还包含 `/tickets/:id`、`/auth/reset-password`、`/auth/callback`；没有独立 `/profile`、`/wallet` 或 `/balance` 路由。[FoxCode 路由构建资源](https://foxcode.rjj.cc/_nuxt/K6xtb0z5.js)

### 普通用户导航

FoxCode 当前桌面顶栏为：仪表板、兑换订阅、使用统计、API 密钥、模型广场，以及条件显示的外部购买入口。头像下拉菜单包含 API 密钥管理、模型广场、工单中心和退出登录。移动端使用相同的主要入口。`/devices` 页面存在，但不在已核验的全局顶栏中。[FoxCode 布局资源](https://foxcode.rjj.cc/_nuxt/z30tYC_O.js)

因此，FoxCode 值得参考的不是“只保留五个功能”——它实际功能更多——而是以下信息架构原则：

- 顶层只放高频入口；
- 余额直接进入看板，不单独制造一个余额页面；
- 账户操作放入头像菜单；
- 复杂详情留在具体页面，不堆进全局导航。

### 页面内容

- 仪表板：剩余配额（按量/月卡）、活跃订阅、额度限制、重置周期、到期时间、教程、公告和邀请推广。
- 使用统计：今日模型分布、24 小时花费趋势、近一周消费记录、Token 与费用明细。
- API 密钥：创建、复制、启用、禁用、删除，展示创建时间、最后使用时间和状态。
- 设备管理：当前/全部设备、设备名、平台、IP、位置、首次登录、最后使用、活跃令牌、重命名和撤销。
- 设备授权：展示请求访问的客户端与设备，允许授权或拒绝，成功后可复制备用码。

上述内容来自 FoxCode 当前公开中文客户端资源：[中文界面资源](https://foxcode.rjj.cc/_nuxt/Cw6wppaa.js)、[仪表板资源](https://foxcode.rjj.cc/_nuxt/CjN_vbMq.js)、[API Key 资源](https://foxcode.rjj.cc/_nuxt/Dg567-IO.js)、[设备资源](https://foxcode.rjj.cc/_nuxt/ihVkdVgh.js)。资源文件名带构建哈希，后续部署可能变化。

### 可参考的视觉规则

FoxCode 使用暖白背景 `#faf9f5`、半透明白色粘性顶栏、主色 `#D97757`、最大宽度内容容器、圆角白色卡片和移动端折叠菜单。[FoxCode 布局资源](https://foxcode.rjj.cc/_nuxt/z30tYC_O.js)

这些可以转译为本项目的设计 Token；不应逐行复制 FoxCode 的 Nuxt/Vue 产物。

## 当前仓库可直接复用的能力

本地 `main` 为 `2638e52cb44f43342a7a7e5eacec2620cbaa3343`；其上游基线为 `afe16c64cd73853da1eda3bf236f15d69637b4bf`。2026-07-30 核验到的远端 `upstream/main` 已前进到 `66ee6b8f9889050ffef1f863a4314ce4a0516fb9`，即当前 fork 与上游已经分叉：本地包含 1 个定制提交，上游另有 3 个新提交。以下结论以当前实际工作树为准。

### 路由与权限

- 前端使用 TanStack Router 文件路由；`web/src/main.tsx:96-108` 创建路由器，生产构建由 `web/rsbuild.config.ts:89-98` 自动生成/拆分路由。
- 所有 `_authenticated` 页面统一检查用户与 Access Token，未登录跳转 `/sign-in`：`web/src/routes/_authenticated/route.tsx:24-35`。
- 普通用户页面已经存在：
  - `/dashboard/overview`：`web/src/routes/_authenticated/dashboard/$section.tsx:27-37`
  - `/keys`：`web/src/routes/_authenticated/keys/index.tsx:36-39`
  - `/wallet`：`web/src/routes/_authenticated/wallet/index.tsx`
  - `/profile`：`web/src/routes/_authenticated/profile/index.tsx`
- 管理员组由角色过滤，普通用户不会看到 Admin 导航：`web/src/hooks/use-sidebar-view.ts:47-65`。路由本身仍独立执行管理员权限检查；隐藏菜单不是安全控制。

### 现有导航配置

当前侧栏定义了 Chat、Playground、Overview、Dashboard、API Keys、Usage Logs、Task Logs、Wallet、Profile 和管理员组：`web/src/hooks/use-sidebar-data.ts:48-160`。

项目已有两层侧栏开关：

- 平台级 `SidebarModulesAdmin`；
- 用户级 `sidebar_modules`，只能进一步收窄平台允许项。

具体映射与 AND 规则见 `web/src/hooks/use-sidebar-config.ts:97-119`、`web/src/hooks/use-sidebar-config.ts:163-191`、`web/src/hooks/use-sidebar-config.ts:259-310`。系统设置已有可视化开关并保存 `SidebarModulesAdmin`：`web/src/features/system-settings/maintenance/sidebar-modules-section.tsx:60-179`。

限制也很明确：

- `/dashboard/overview` 与 `/dashboard/models` 都映射到同一个 `console.detail`，仅靠后台开关无法保留 Overview、同时隐藏模型分析；需要一次很小的导航代码调整。
- 侧栏开关只隐藏入口，不阻止已登录用户直接访问 URL；如果未来需要产品权限，必须单独加路由/API 授权，不能把菜单隐藏当权限。

### 数据看板、钱包与 Key

- 当前 Overview 已读取用户请求数、余额、已用额度、API Keys 与可用模型：`web/src/features/dashboard/components/overview/overview-dashboard.tsx:458-499`。
- 它已有「创建 API Key」「充值」引导，以及 Key、Wallet 的快捷操作：`web/src/features/dashboard/components/overview/overview-dashboard.tsx:500-555`。
- 摘要卡已经展示余额、近 24 小时用量、预计可用天数并链接 Wallet：`web/src/features/dashboard/components/overview/summary-cards.tsx:139-227`、`web/src/features/dashboard/components/overview/summary-cards.tsx:288-350`。
- Key 页面已经封装创建按钮、表格和对话框：`web/src/features/keys/index.tsx:28-44`。
- Wallet 已包含余额、充值、订阅、兑换和账单历史，无需再造支付页面：`web/src/features/wallet/index.tsx:62-150`。

### 设备授权与设备管理

当前 fork 已经实现比 FoxCode 公开流程更适合 SheJane 的 Authorization Code + PKCE：

- `/shejane/authorize` 是独立公共事务页：`web/src/routes/shejane/authorize.tsx:19-32`。
- 未登录时保存净化后的同源 continuation，并跳转登录；用户确认页只展示应用、设备、平台、版本和 inference-only 权限：`web/src/features/shejane-authorization/components/authorization-page.tsx:68-128`、`web/src/features/shejane-authorization/components/authorization-page.tsx:197-269`。
- 回调只接受固定 `127.0.0.1` 路径与有效端口：`web/src/features/shejane-authorization/lib/authorization.ts:94-127`。
- Profile 已同时包含登录会话与 SheJane 设备卡：`web/src/features/profile/index.tsx:59-86`。
- 设备卡可以列出设备、显示平台/版本/授权时间并撤销访问：`web/src/features/profile/components/shejane-devices-card.tsx:53-80`、`web/src/features/profile/components/shejane-devices-card.tsx:117-200`。
- 后端 API 已有 `GET /api/shejane/devices` 和 `DELETE /api/shejane/devices/:id`：`router/api-router.go:36-37`。

所以「设备授权」不是一个可以手动打开的普通菜单页：它应由 Desktop 发起。普通用户日常需要的是「已授权设备管理」，当前放在 Profile 已经成立。

### i18n、构建与部署

- UI 使用 `i18next`/`react-i18next`，支持 en、zhCN、fr、ru、ja、vi、zhTW，英语为 fallback：`web/src/i18n/config.ts:19-62`。
- 新文案应继续使用 `useTranslation()` 与英文源键，并同步 `web/src/i18n/locales/*.json`；项目规则见 `AGENTS.md:130-139`。
- 前端脚本为 `bun run dev/build/build:check/typecheck/lint/i18n:sync`：`web/package.json:6-19`。
- `make build-web` 安装依赖并构建 `web/dist`：`Makefile:15-20`。
- Docker 构建先生成 `web/dist`，再编译 Go：`Dockerfile:1-28`。
- Go 二进制通过 `//go:embed web/dist` 嵌入前端，未知非 API 路由回退到 SPA `index.html`：`main.go:42-46`、`router/web-router.go:33-51`。

因此二次开发的发布路径是：修改 `web/` → 前端检查与构建 → 重新编译/构建 Docker 镜像；不能只替换服务器上的散落静态文件。

## 最小改造方案

### 目标信息架构

| 场景 | 页面/入口 | 复用位置 |
|---|---|---|
| 未登录 | 登录、注册、找回密码 | 现有 `/sign-in`、`/sign-up` 与 auth features |
| 登录后首页 | 数据看板 | `/dashboard/overview` |
| 登录后 | API Key | `/keys` |
| 登录后 | 钱包与余额 | `/wallet`，余额同时出现在看板 |
| 登录后 | 个人资料与安全 | `/profile` |
| Desktop 发起 | 设备授权确认 | `/shejane/authorize`，不进入常驻导航 |
| 日常管理 | 登录会话、已授权设备 | Profile 内现有两张卡 |
| 管理员 | 渠道、模型、用户、系统设置等 | 保留现有 Admin 组，不删除 |

### P0：配置预览，无代码

在「系统设置 → Sidebar modules」中关闭 Chat 区、Usage Logs、Drawing Logs、Task Logs，仅保留 Dashboard、Token、Wallet、Profile。用真实普通用户账号检查桌面和移动端。

这一步用于快速确认“少入口”是否已经解决主要问题；它不能合并两个 Dashboard 入口，也不会阻止直接 URL。

### P1：推荐的最小正式版本

预计只修改少量前端文件，不新增依赖、不改 API：

1. `web/src/hooks/use-sidebar-data.ts`
   - 普通用户仅展示「数据看板、API Keys、Wallet、Profile」；
   - 只保留 `/dashboard/overview` 一个看板入口；
   - 管理员继续看到原管理组；
   - 可用现有 `requiredRole` 把调试/日志类入口限定为管理员，避免发明新的权限系统。
2. `web/src/features/dashboard/components/overview/overview-dashboard.tsx`
   - 保留余额、用量、API Key、充值和服务状态；
   - 移除普通用户看板中指向 Playground、Channels、Usage Logs、复杂 Pricing 的引导；
   - 保证导航隐藏后页面不再把用户带回复杂区域。
3. `web/src/features/profile/index.tsx`
   - 把登录会话和 SheJane 设备卡提高到更显眼的位置；
   - 普通用户不再承担自定义侧栏的产品决策，侧栏由平台统一配置。
4. `web/src/i18n/locales/*.json`
   - 同步新增/调整文案；运行 `bun run i18n:sync`。

不做：删除路由、删除管理员能力、重写钱包、复制 FoxCode 源码、创建新后端或数据表。

### P2：确认需要后再做的视觉参考

若用户验证 P1 后仍希望“看起来也像 FoxCode”，再做独立视觉阶段：

- 复用仓库已有响应式 `TopNav`（`web/src/components/layout/components/top-nav.tsx:34-129`），把普通用户侧栏改成少量水平入口；管理员仍可保留侧栏或独立管理入口。
- 在 `web/src/styles/theme.css`/主题 preset 中定义暖白背景与陶土色主色，而不是在组件里散落硬编码颜色。
- 复用现有 Card、Button、Dropdown、移动端菜单与 AuthLayout；不增加 UI 库。
- 若必须有独立 `/devices`，只创建文件路由并组合已有两张设备卡；不要复制 FoxCode 的设备鉴权实现。

## 验收标准

1. 普通用户登录后顶层最多 4 个常驻入口：看板、Key、钱包、个人资料。
2. 看板首屏能直接看到余额、近 24 小时用量、Key 状态和充值入口。
3. SheJane Desktop 发起授权时，登录 continuation、同意/拒绝、PKCE 回调均不受影响。
4. Profile 能列出并撤销登录会话和 SheJane 设备，且不展示任何推理密钥。
5. 普通用户看不到 Admin 组；管理员仍能进入现有管理页面。
6. 隐藏菜单不被描述为权限控制；所有敏感 API 继续由后端鉴权。
7. 桌面与移动端均无空导航组、重复 Dashboard、不可达返回路径。
8. 通过受影响测试、`bun run typecheck`、`bun run lint`、`bun run i18n:sync` 与 `bun run build:check`。

## 风险与许可证边界

- FoxCode 只能作为产品层级与视觉参考。公开构建产物不等于允许复制其实现、商标、Logo、插图或专有文案。
- 本仓库代码头显示 AGPLv3，并提供商业许可联系方式；发布二次开发版本前应由项目方确认网络部署与源码提供义务，本文不构成法律意见。
- 项目治理明确保护 New API 与 QuantumNous 相关身份、元数据和归属，不得删除、替换或改名：`AGENTS.md:141-150`。可以通过现有系统设置调整面向用户的站点名称/Logo，但不能移除受保护的项目归属和许可证信息。
- 设备授权属于安全边界。不要为了“像 FoxCode”弱化当前 S256、固定 loopback、短期一次性 code、同源 continuation、session 校验和设备撤销链路；当前设计约束见 `docs/shejane-native-app-authorization.md:139-209`、`docs/shejane-native-app-authorization.md:416-435`。

## 建议决策

先做 P0 配置预览，再实施 P1。P1 已能满足“普通用户只看到必要内容”；只有在真实用户测试后仍明确要求 FoxCode 的横向顶栏与暖色视觉时，才进入 P2。这样最少改动、最容易跟随 upstream，也不会碰设备授权和计费后端。

# 前端界面代码目录

本文档用于快速定位前端界面代码位置。所有前端代码位于 `web/src/` 目录下。

## 路由配置

主要路由配置文件：
- `web/src/App.jsx` - 主路由配置文件，定义了所有页面路由

## 页面列表

### 公开页面（无需登录）

| 路由路径 | 页面组件 | 文件位置 | 说明 |
|---------|---------|---------|------|
| `/` | Home | `web/src/pages/Home/index.jsx` | 首页 |
| `/about` | About | `web/src/pages/About/index.jsx` | 关于页面 |
| `/pricing` | Pricing | `web/src/pages/Pricing/index.jsx` | 模型广场/定价页面 |
| `/user-agreement` | UserAgreement | `web/src/pages/UserAgreement/index.jsx` | 用户协议 |
| `/privacy-policy` | PrivacyPolicy | `web/src/pages/PrivacyPolicy/index.jsx` | 隐私政策 |
| `/setup` | Setup | `web/src/pages/Setup/index.jsx` | 初始化设置页面 |

### 认证相关页面

| 路由路径 | 页面组件 | 文件位置 | 说明 |
|---------|---------|---------|------|
| `/login` | LoginForm | `web/src/components/auth/LoginForm.jsx` | 登录页面 |
| `/register` | RegisterForm | `web/src/components/auth/RegisterForm.jsx` | 注册页面 |
| `/reset` | PasswordResetForm | `web/src/components/auth/PasswordResetForm.jsx` | 重置密码表单 |
| `/user/reset` | PasswordResetConfirm | `web/src/components/auth/PasswordResetConfirm.jsx` | 密码重置确认 |
| `/oauth/github` | OAuth2Callback | `web/src/components/auth/OAuth2Callback.jsx` | GitHub OAuth回调 |
| `/oauth/oidc` | OAuth2Callback | `web/src/components/auth/OAuth2Callback.jsx` | OIDC OAuth回调 |
| `/oauth/linuxdo` | OAuth2Callback | `web/src/components/auth/OAuth2Callback.jsx` | LinuxDo OAuth回调 |

### 控制台页面（需要登录）

| 路由路径 | 页面组件 | 文件位置 | 说明 | 权限要求 |
|---------|---------|---------|------|---------|
| `/console` | Dashboard | `web/src/pages/Dashboard/index.jsx` | 仪表盘/控制台主页 | 登录用户 |
| `/console/chat/:id?` | Chat | `web/src/pages/Chat/index.jsx` | 聊天界面 | 登录用户 |
| `/chat2link` | Chat2Link | `web/src/pages/Chat2Link/index.jsx` | 聊天链接跳转 | 登录用户 |
| `/console/token` | Token | `web/src/pages/Token/index.jsx` | Token管理 | 登录用户 |
| `/console/log` | Log | `web/src/pages/Log/index.jsx` | 使用日志 | 登录用户 |
| `/console/topup` | TopUp | `web/src/pages/TopUp/index.js` | 充值页面 | 登录用户 |
| `/console/midjourney` | Midjourney | `web/src/pages/Midjourney/index.jsx` | Midjourney任务管理 | 登录用户 |
| `/console/task` | Task | `web/src/pages/Task/index.jsx` | 任务管理 | 登录用户 |
| `/console/playground` | Playground | `web/src/pages/Playground/index.jsx` | API测试平台 | 登录用户 |
| `/console/personal` | PersonalSetting | `web/src/components/settings/PersonalSetting.jsx` | 个人设置 | 登录用户 |

### 管理员页面

| 路由路径 | 页面组件 | 文件位置 | 说明 |
|---------|---------|---------|------|
| `/console/models` | ModelPage | `web/src/pages/Model/index.jsx` | 模型管理 |
| `/console/channel` | Channel | `web/src/pages/Channel/index.jsx` | 渠道管理 |
| `/console/user` | User | `web/src/pages/User/index.jsx` | 用户管理 |
| `/console/redemption` | Redemption | `web/src/pages/Redemption/index.jsx` | 兑换码管理 |
| `/console/setting` | Setting | `web/src/pages/Setting/index.jsx` | 系统设置 |

### 错误页面

| 路由路径 | 页面组件 | 文件位置 | 说明 |
|---------|---------|---------|------|
| `/forbidden` | Forbidden | `web/src/pages/Forbidden/index.jsx` | 403 禁止访问 |
| `*` | NotFound | `web/src/pages/NotFound/index.jsx` | 404 未找到 |

## 设置页面子模块

设置页面 (`/console/setting`) 包含多个子模块，位于 `web/src/pages/Setting/` 目录：

| 子模块 | 组件路径 | 说明 |
|-------|---------|------|
| 运营设置 | `web/src/pages/Setting/Operation/` | 运营相关配置 |
| 仪表盘设置 | `web/src/pages/Setting/Dashboard/` | 仪表盘相关配置 |
| 聊天设置 | `web/src/pages/Setting/Chat/` | 聊天功能配置 |
| 绘图设置 | `web/src/pages/Setting/Drawing/` | 绘图功能配置 |
| 支付设置 | `web/src/pages/Setting/Payment/` | 支付相关配置 |
| 模型设置 | `web/src/pages/Setting/Model/` | 模型相关配置 |
| 费率设置 | `web/src/pages/Setting/Ratio/` | 费率配置 |
| 限流设置 | `web/src/pages/Setting/RateLimit/` | 限流配置 |
| 个人设置 | `web/src/pages/Setting/Personal/` | 个人模块配置 |

## 主要组件目录

### 布局组件 (`web/src/components/layout/`)
- `PageLayout.jsx` - 页面布局容器
- `SiderBar.jsx` - 侧边栏导航
- `headerbar/` - 顶部导航栏相关组件
- `Footer.jsx` - 页脚组件
- `SetupCheck.js` - 初始化检查组件

### 表格组件 (`web/src/components/table/`)
按功能模块组织的表格组件：
- `channels/` - 渠道管理表格
- `tokens/` - Token管理表格
- `users/` - 用户管理表格
- `models/` - 模型管理表格
- `redemptions/` - 兑换码管理表格
- `usage-logs/` - 使用日志表格
- `mj-logs/` - Midjourney日志表格
- `task-logs/` - 任务日志表格
- `model-pricing/` - 模型定价表格

### 设置组件 (`web/src/components/settings/`)
- `SystemSetting.jsx` - 系统设置
- `OperationSetting.jsx` - 运营设置
- `DashboardSetting.jsx` - 仪表盘设置
- `ChatsSetting.jsx` - 聊天设置
- `DrawingSetting.jsx` - 绘图设置
- `PaymentSetting.jsx` - 支付设置
- `ModelSetting.jsx` - 模型设置
- `RatioSetting.jsx` - 费率设置
- `RateLimitSetting.jsx` - 限流设置
- `PersonalSetting.jsx` - 个人设置

### 仪表盘组件 (`web/src/components/dashboard/`)
- `index.jsx` - 仪表盘主组件
- `ChartsPanel.jsx` - 图表面板
- `StatsCards.jsx` - 统计卡片
- `ApiInfoPanel.jsx` - API信息面板
- `AnnouncementsPanel.jsx` - 公告面板
- `FaqPanel.jsx` - FAQ面板
- `UptimePanel.jsx` - 在线状态面板

### Playground组件 (`web/src/components/playground/`)
- `ChatArea.jsx` - 聊天区域
- `SettingsPanel.jsx` - 设置面板
- `DebugPanel.jsx` - 调试面板
- `CustomRequestEditor.jsx` - 自定义请求编辑器

### 通用组件 (`web/src/components/common/`)
- `ui/` - UI组件（Loading, CardPro, JSONEditor等）
- `modals/` - 通用模态框
- `markdown/` - Markdown渲染器

### 认证组件 (`web/src/components/auth/`)
- `LoginForm.jsx` - 登录表单
- `RegisterForm.jsx` - 注册表单
- `PasswordResetForm.jsx` - 密码重置表单
- `PasswordResetConfirm.jsx` - 密码重置确认
- `OAuth2Callback.jsx` - OAuth回调处理
- `TwoFAVerification.jsx` - 双因素认证

## 其他重要文件

### 入口文件
- `web/src/index.jsx` - 应用入口文件
- `web/src/App.jsx` - 主应用组件（包含路由配置）

### Context和状态管理
- `web/src/context/` - React Context定义
- `web/src/hooks/` - 自定义Hooks

### 工具和帮助函数
- `web/src/helpers/` - 辅助函数
- `web/src/services/` - API服务调用
- `web/src/constants/` - 常量定义
- `web/src/i18n/` - 国际化配置

## 快速定位指南

### 当遇到问题时：

1. **页面相关问题** → 查看 `web/src/pages/` 对应页面目录
2. **表格相关问题** → 查看 `web/src/components/table/` 对应模块
3. **设置相关问题** → 查看 `web/src/components/settings/` 或 `web/src/pages/Setting/`
4. **认证相关问题** → 查看 `web/src/components/auth/`
5. **布局相关问题** → 查看 `web/src/components/layout/`
6. **API调用问题** → 查看 `web/src/services/`
7. **路由问题** → 查看 `web/src/App.jsx`


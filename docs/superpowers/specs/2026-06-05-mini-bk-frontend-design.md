# Mini-BK Console 前端设计

> **面向 Linux/容器资源的任务调度与运维管控平台 — 前端控制台**
>
> 版本: v1.0 | 日期: 2026-06-05 | 状态: 已确认

---

## 1. 总纲

### 1.1 定位

Mini-BK Console：纯前端 SPA，消费 Go 后端全部 15 个 REST API 端点。同仓 `web/` 目录，Vite 构建，React 全家桶。

### 1.2 技术栈

| 层 | 选择 | 用途 |
|----|------|------|
| 构建 | Vite 6 | 开发服务器 + 生产构建 |
| 框架 | React 19 | UI 组件 |
| 语言 | TypeScript | 类型安全 |
| 路由 | React Router v7 | SPA 页面路由 |
| 组件库 | Ant Design 5 | 表格/表单/布局/图表 |
| 服务端状态 | TanStack React Query | API 缓存/轮询/乐观更新 |
| HTTP 请求 | axios | API 调用 |
| 实时日志 | EventSource (原生 SSE) | 任务执行日志实时流 |

### 1.3 项目结构

```
web/
├── public/
├── src/
│   ├── api/                  # API 请求层
│   │   ├── client.ts         # axios 实例（baseURL, 拦截器）
│   │   ├── tasks.ts          # 任务相关 API
│   │   ├── nodes.ts          # 节点相关 API
│   │   └── stats.ts          # 统计相关 API
│   ├── hooks/                # 自定义 hooks (React Query)
│   │   ├── useTasks.ts
│   │   ├── useNodes.ts
│   │   ├── useStats.ts
│   │   └── useSSE.ts         # SSE 实时日志 hook
│   ├── pages/                # 页面组件
│   │   ├── Dashboard/        # 仪表盘首页
│   │   ├── Tasks/            # 任务管理
│   │   │   ├── TaskList.tsx
│   │   │   ├── TaskDetail.tsx
│   │   │   └── TaskCreate.tsx
│   │   └── Nodes/            # 节点管理
│   │       ├── NodeList.tsx
│   │       └── NodeDetail.tsx
│   ├── components/           # 共享组件
│   │   ├── Layout/           # 全局布局
│   │   ├── TaskStatusTag.tsx
│   │   ├── ResourceBar.tsx
│   │   └── LogStream.tsx     # SSE 日志流组件
│   ├── types/                # TypeScript 类型定义
│   │   └── index.ts
│   ├── router.tsx            # 路由配置
│   ├── App.tsx
│   └── main.tsx
├── index.html
├── package.json
├── tsconfig.json
├── vite.config.ts
└── .env.development          # VITE_API_BASE_URL=/api/v1
```

### 1.4 路由设计

| 路径 | 页面 | 说明 |
|------|------|------|
| `/` | Dashboard | 仪表盘首页，今日统计+资源概览+最近任务 |
| `/tasks` | TaskList | 任务列表（分页+状态筛选+搜索） |
| `/tasks/new` | TaskCreate | 创建新任务表单 |
| `/tasks/:taskUid` | TaskDetail | 任务详情 + 实时日志流 |
| `/nodes` | NodeList | 节点列表 + 资源用量可视化 |
| `/nodes/:nodeId` | NodeDetail | 节点详情 + drain/uncordon 操作 |

### 1.5 布局结构

```
┌──────────────────────────────────────────────┐
│  Logo  Mini-BK Console                       │  ← Header (48px)
├────────┬─────────────────────────────────────┤
│ 仪表盘  │                                     │
│ 任务管理 │         <Outlet />                  │
│  - 列表  │         (页面内容)                   │
│  - 新建  │                                     │
│ 节点管理 │                                     │
├────────┴─────────────────────────────────────┤
│  Mini-BK ResourceOps                         │  ← Footer (24px)
└──────────────────────────────────────────────┘
    Sider (200px)          Content
```

---

## 2. 页面设计

### 2.1 Dashboard（仪表盘）

- **统计卡片**（3 个）：今日提交数、成功率（环状进度条）、失败数
- **最近任务**（Table，10 条）：名称/状态(Tag)/耗时/节点，支持点击跳转
- **节点资源概览**（卡片列表）：每节点 CPU/内存进度条

**数据来源：**
- `useStats()` → `GET /api/v1/stats` + `GET /api/v1/stats/daily`
- `useTasks({ page: 1, size: 10 })` → `GET /api/v1/tasks`
- `useNodes()` → `GET /api/v1/nodes`

### 2.2 TaskList（任务列表）

- **搜索区**：状态下拉筛选 + 任务名称搜索
- **表格列**：task_uid、名称、命令(截断)、状态(Tag 颜色标记)、优先级、超时、创建时间、操作(查看/取消/重跑)
- **分页**：Ant Design Pagination，page_size 可切换(10/20/50)
- **自动刷新**：React Query `refetchInterval: 5000`

### 2.3 TaskCreate（创建任务）

- **表单**：Ant Design Form，两栏布局
- **必填**：name, command(TextArea), workdir
- **可选**：cpu_limit, memory_limit, timeout_sec, priority, node_selector(键值对, Dynamic Form.List)
- **交互**：提交后跳转到 TaskDetail 页面

### 2.4 TaskDetail（任务详情）

- **基本信息**：Descriptions（task_uid, name, command, status, exit_code, 起止时间）
- **执行输出**：两个 Tab（stdout / stderr），`<pre>` 等宽字体
- **实时日志流**：EventSource 连接 `GET /api/v1/tasks/:task_uid/log/stream`（SSE），逐行追加，自动滚底
- **操作栏**：取消按钮（非终态可见）、重跑按钮（终态可见）

### 2.5 NodeList（节点列表）

- **表格列**：hostname、IP、状态(Badge)、CPU 使用率(进度条)、内存使用率(进度条)、运行中任务数、标签(Tags)、心跳时间
- **操作**：Drain / Uncordon 按钮
- **自动刷新**：`refetchInterval: 10000`

### 2.6 NodeDetail（节点详情）

- **资源卡片**：CPU 进度条、内存进度条、磁盘进度条、Load Average
- **运行中任务**：嵌套 Table
- **操作**：Drain / Uncordon

---

## 3. 数据流

```
React Query → axios → Go API Server (:8080)
     ↑                    ↑
  cache/refetch       Gin router
     ↓                    ↓
  UI Components    PostgreSQL/Redis/etcd
```

- **查询类**（useQuery）：统计、任务列表、节点列表、任务详情
- **变更类**（useMutation）：创建任务、取消任务、重跑任务、Drain/Uncordon 节点
- **实时流**（useSSE）：EventSource 读取 log/stream 端点

### Vite 开发代理

```ts
// vite.config.ts
export default defineConfig({
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: true } },
  },
})
```

### 错误处理策略

- axios 拦截器统一处理：500→消息提示、网络错误→重试提示
- React Query retry: 3 次（指数退避）
- 页面级 ErrorBoundary：组件崩溃时显示回退 UI
- 空状态：表格无数据时显示 Empty 占位符

---

## 4. 非目标

- 不做用户登录/鉴权（后端尚无认证系统，Phase 11 才做）
- 不做 i18n 国际化
- 不做响应式移动端适配（面向桌面运维后台）
- 不做 E2E 测试（先用手动验证）
- 不做暗色主题

---

## 5. 实现任务

| # | 任务 | 产出 | 复杂度 |
|---|------|------|--------|
| 1 | 项目初始化 | Vite + deps + 脚手架 + axios 实例 + 类型定义 | 中 |
| 2 | 全局布局 | Header + Sider + Content + React Router | 中 |
| 3 | Dashboard 页 | 统计卡片 + 最近任务 + 节点概览 + hooks | 中 |
| 4 | TaskList 页 | 分页表格 + 状态筛选 + 取消/重跑操作 | 中 |
| 5 | TaskCreate 页 | 创建表单 + 字段校验 + node_selector 动态表单 | 中 |
| 6 | TaskDetail 页 | 任务信息 + 执行输出 + SSE 实时日志流组件 | 大 |
| 7 | NodeList + NodeDetail 页 | 节点表格 + 资源进度条 + Drain/Uncordon | 中 |
| 8 | 错误处理 + 空状态 | axios 拦截器 + ErrorBoundary + Empty + Loading | 小 |
| 9 | 最终集成验证 | `npm run build` + CORS 确认 + Dockerfile 集成 | 小 |

---

## 6. 决策记录

| 决策 | 结论 | 日期 |
|------|------|------|
| 前端框架 | React 19 + TypeScript | 2026-06-05 |
| 构建工具 | Vite 6 | 2026-06-05 |
| 组件库 | Ant Design 5 | 2026-06-05 |
| 状态管理 | TanStack React Query + Context | 2026-06-05 |
| 路由 | React Router v7 | 2026-06-05 |
| 项目结构 | 同仓 `web/` 目录 | 2026-06-05 |
| 页面范围 | 全功能面板（6 页面） | 2026-06-05 |

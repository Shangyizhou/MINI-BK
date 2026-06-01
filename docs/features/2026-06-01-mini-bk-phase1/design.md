# Mini-BK Phase 1 Design

## 概述

一期是渐进式单体 Go 应用。分层架构：Gin HTTP → Service → Scheduler + Executor + Store(PostgreSQL)。核心并发模型：buffered channel 信号量控制并发数，goroutine 异步执行，ticker 轮询调度。

## 目标与非目标

**目标**: 单机任务调度最小闭环。**非目标**: 分布式、前端、权限、脚本库。

## 总体方案

> - **目标架构**: 单体 Go 应用，Gin 对外提供 REST API，内部三层分离
> - **调度策略**: ticker 轮询 + 资源感知（LeastAllocated）
> - **持久化**: PostgreSQL 作为历史事实库，所有任务状态变更必须落库

## 模块职责

### 配置管理 (config)

- 职责：加载 YAML 配置 + 环境变量覆盖
- 约束：Viper 的 AutomaticEnv + Unmarshal 有已知限制，关键字段手动覆盖

### 任务模型 (model)

- 职责：Task 结构体、6 状态状态机、流转合法性校验
- 约束：终态不可逆，取消仅限非终态

### 持久化 (store)

- 职责：PostgreSQL CRUD + 按状态查询 + 分页
- 约束：env 字段为 JSONB，使用 json.Marshal/Unmarshal

### 执行器 (executor)

- 职责：os/exec 进程管理，超时/取消，stdout/stderr 捕获
- 约束：sh -c 执行命令，继承系统环境变量

### 调度器 (scheduler)

- 职责：ticker 轮询，资源感知调度，超时检测
- 约束：只有 Leader 执行调度（单机模式下天然单 Scheduler）

### 业务逻辑 (service)

- 职责：CreateTask 参数校验+默认值，CancelTask 状态校验，RerunTask 复制
- 约束：纯业务逻辑，不直接操作 DB

### HTTP API (api)

- 职责：Gin router + 7 个 handler，JSON 序列化
- 约束：统一错误格式 `{"error": "..."}`

## 数据模型

**tasks 表:**

- **task_uid**: UUID v4，外部唯一标识
- **name**: 任务名称（必填）
- **command**: 执行的命令（必填）
- **workdir**: 工作目录（默认 /tmp）
- **env**: 环境变量 JSONB（默认 {}）
- **cpu_limit**: CPU 核数限制（0=不限）
- **memory_limit**: 内存 MB 限制（0=不限）
- **timeout_sec**: 超时秒数（默认 300）
- **priority**: 优先级，越大越优先（默认 0）
- **status**: created/pending/running/success/failed/canceled
- **exit_code**: 进程退出码（可空）
- **stdout/stderr**: 标准输出/错误
- **error_message**: 系统级错误信息
- **pid**: OS 进程 ID（可空）
- **started_at/finished_at**: 执行起止时间（可空）

## 状态机流转

```
Created ──→ Pending ──→ Running ──→ Success
  │           │            │
  └──→ Canceled ←─────────┘
                         Failed (含超时)
```

## API 设计

```
POST   /api/v1/tasks               创建任务
GET    /api/v1/tasks               任务列表 (?status=&page=&size=)
GET    /api/v1/tasks/:task_uid     任务详情
POST   /api/v1/tasks/:task_uid/cancel  取消任务
POST   /api/v1/tasks/:task_uid/rerun   重跑任务
GET    /api/v1/tasks/:task_uid/log     获取输出
GET    /api/v1/resources           本机资源余量
GET    /api/v1/stats               统计
```

## 并发模型

```go
execSlots := make(chan struct{}, maxConcurrent)
// 执行前: execSlots <- struct{}{}
// 执行后: <-execSlots
```

## 测试策略

- 单元测试：Task 状态机、Scheduler 调度逻辑（mock store）
- 集成测试：全链路创建→等待→验证，真实 PostgreSQL
- 不做：前端测试、性能测试、压力测试

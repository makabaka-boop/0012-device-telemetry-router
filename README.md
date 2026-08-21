# 设备遥测协议解析与事件路由服务

面向物联网接入场景的 Go 后端 API 服务：承接设备按固定文本协议上报的遥测报文，
完成协议解析、字段校验、幂等去重、标准事件化与按规则路由分发，数据持久化到 PostgreSQL。

本仓库当前实现的是**第一次业务扩写**后的版本（约 2600～3800 行非测试 Go 有效代码，28～40 个生产 .go 文件）。

## 功能概览

- 设备注册与管理（设备档案、状态流转）
- 遥测报文「上报 → 协议切分 → 字段/校验码/量程/时间窗校验 → 幂等去重 → 事件化 → 规则匹配 → 分发记录」
- 路由规则 CRUD 与变更留痕（before/after JSONB 审计，operator 归因）
- 规则匹配表达式增强（AND/OR、括号、通配符、metric 集合、数值比较）
- 分发重试与死信后台任务（指数退避、可插拔 Sink、事件状态推进）
- 事件重放/补发与分发记录查询
- API Key 鉴权与认证审计留痕
- Prometheus 指标（HTTP 时延/在途、分发计数）
- 事件查询（按设备/指标/时间/状态过滤，分页）
- 健康检查、统计聚合（设备/事件/待重试/审计）
- 可操作前端页面（首页 / 设备 / 报文上报 / 规则 / 事件），真实调用上述 Go API

## 协议格式

报文为管道分隔的固定文本协议：

```
DEVICE_ID|RFC3339_TIME|METRIC|VALUE|UNIT|CHECKSUM
```

- `DEVICE_ID`：^[A-Z0-9-]{8,32}$
- `METRIC`：白名单（temperature / humidity / pressure / voltage / current / wind_speed）
- `VALUE`：有限数值且在量程内
- `CHECKSUM`：CRC16-CCITT（多项式 0x1021，初值 0xFFFF，大写十六进制），覆盖报文体除校验码外全部内容
- 时间戳不得晚于服务时间 +5 分钟、不得早于 30 天
- 幂等键：SHA256(device_id|ts|metric|value|unit) 前 32 位，去重窗口 24 小时

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | /healthz | 存活检查（200，status=alive） |
| POST | /api/v1/devices | 注册设备（201） |
| GET  | /api/v1/devices | 设备列表 |
| GET  | /api/v1/devices/{device_id} | 设备详情 |
| POST | /api/v1/devices/{device_id}/status | 变更设备状态 |
| POST | /api/v1/telemetry | 上报报文（原始/重复识别） |
| POST | /api/v1/rules | 创建路由规则 |
| GET  | /api/v1/rules | 规则列表 |
| GET  | /api/v1/rules/{rule_id} | 规则详情 |
| PUT  | /api/v1/rules/{rule_id} | 更新规则 |
| DELETE | /api/v1/rules/{rule_id} | 软删除规则 |
| GET  | /api/v1/rules/{rule_id}/changes | 规则变更留痕 |
| GET  | /api/v1/events | 事件查询（过滤/分页） |
| GET  | /api/v1/events/{event_id} | 事件详情 |
| GET  | /api/v1/events/{event_id}/deliveries | 事件分发记录 |
| POST | /api/v1/events/{event_id}/replay | 事件重放/补发 |
| GET  | /api/v1/stats | 统计（设备/事件/待重试数） |
| GET  | /api/v1/audit | API 鉴权审计留痕 |
| GET  | /metrics | Prometheus 指标 |

错误统一返回信封：

```json
{ "error": { "code": "INVALID_FIELD", "message": "...", "details": {...} } }
```

## 运行

### 环境变量

- `DATABASE_URL`（默认 `postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable`）
- `PORT`（默认 `8080`）
- `DEDUP_WINDOW`（默认 `24h`）
- `SHUTDOWN_TIMEOUT`（默认 `10s`）
- `STATIC_DIR`（前端静态资源目录，默认 `internal/httpapi/assets`）
- `API_KEYS`（可选，`operator:secret` 逗号分隔；设置后写操作需 `X-API-Key` 头）
- `WORKER_ENABLED` / `WORKER_INTERVAL` / `WORKER_BATCH_SIZE` / `WORKER_MAX_ATTEMPT` / `WORKER_BASE_DELAY`（分发重试后台任务）

### 本地运行

```bash
go mod tidy
go build ./...
# 需要先启动 PostgreSQL
PORT=8080 DATABASE_URL=postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable ./... (bin)
```

启动时自动执行迁移建表（`internal/migrate`），包含设备/原始报文/事件/路由规则/变更留痕/分发记录及其索引。

### Docker

Dockerfile 为多阶段构建，同时兼容 `linux/arm64` 与 `linux/amd64`，`EXPOSE 8080`，端口固定 8080。

```bash
docker build --platform linux/amd64 -t telemetry-router .
docker build --platform linux/arm64 -t telemetry-router .
docker run -p 8080:8080 -e DATABASE_URL=... telemetry-router
```

### 冒烟测试

```bash
scripts/smoke_test.sh
```

脚本真实构建并启动服务、启动临时 PostgreSQL 容器、轮询 `/healthz` 断言 200 与 alive、
创建设备与规则、上报合法报文断言 `event_id`、重复上报断言 `duplicate`，最后用 `trap` 清理进程与临时容器。

## 测试

```bash
go test -count=1 ./...
```

- `internal/parser`：协议解析边界（合法/缺字段/多余字段/非法时间戳/非数值/未知指标/大小写校验码）
- `internal/dedup`：去重键确定性、窗口判定
- `internal/router`：单/多规则命中、优先级、禁用、通配符、AND/OR 表达式
- `internal/service`：上报+去重+路由闭环、无效设备拒绝、规则变更留痕、事件重放（内存 fake store）
- `internal/worker`：分发成功、重试耗尽归集死信、退避策略
- `internal/auth`：API Key 解析、认证/拒绝审计、凭据校验
- `internal/metrics`：计数器/仪表/直方图渲染
- `internal/store`：唯一约束、事务回滚、分发重试仓储（需 `TEST_DATABASE_URL`，未设置时跳过）

## 模块结构

```
cmd/server            配置加载、依赖装配、优雅退出、指标/后台任务装配
internal/config       环境变量解析
internal/httpapi      HTTP 路由、中间件、处理器、静态页面托管
internal/domain       实体、值对象、领域错误
internal/parser       协议切分与校验码（纯函数）
internal/service      业务用例编排
internal/store        PostgreSQL 仓储（按实体拆分 + 事务边界）
internal/dedup        去重键计算与窗口判定
internal/router       规则匹配与主题决策（含 AND/OR 表达式）
internal/migrate      启动迁移
internal/worker       分发重试与死信后台任务
internal/auth         API Key 鉴权与审计
internal/metrics      Prometheus 指标（零依赖）
```

依赖单向：`httpapi → service → store/parser/dedup/router`，`domain` 被各层引用但不依赖外部。

## 已知缺陷留档（1 条）

去重窗口边界下，当同 `dedup_key` 报文在窗口即将过期时并发上报，极低概率出现重复入账
（唯一索引兜底将其中一条置为 duplicate，但日志会记录一次冲突告警）。
该缺陷被单独标记在验收清单，不作为本次修复范围，用于缺陷发现评估。

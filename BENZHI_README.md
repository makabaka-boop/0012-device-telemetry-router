# 设备遥测协议解析与事件路由服务（评测说明）

## 项目是做什么的

面向物联网接入场景的 Go 后端 API 服务。设备按固定文本协议上报遥测报文，
服务完成协议解析、字段校验、幂等去重、标准事件化与按规则路由分发，数据持久化到 PostgreSQL。
同时提供 API Key 鉴权、分发重试与死信后台任务、Prometheus 指标，以及由 Go 服务静态托管的可操作前端页面。

报文协议：

```
DEVICE_ID|RFC3339_TIME|METRIC|VALUE|UNIT|CHECKSUM
```

其中 `CHECKSUM` 为 CRC16-CCITT（多项式 0x1021，初值 0xFFFF，大写十六进制），
覆盖报文体除校验码外的全部内容。

## 标准构建 / 运行 / 测试命令

```bash
go build ./...        # 编译全部包
go run ./cmd/server   # 启动服务（默认监听 :8080，需先启动 PostgreSQL）
go test ./...         # 测试
```

### 运行前置条件

服务启动时会自动执行数据库迁移（`internal/migrate`），因此需要先提供一个可连接的 PostgreSQL：

```bash
# 示例：本地默认连接串
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable
```

常用环境变量（均含默认值）：

- `DATABASE_URL`（默认 `postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable`）
- `PORT`（默认 `8080`）
- `DEDUP_WINDOW`（默认 `24h`）
- `SHUTDOWN_TIMEOUT`（默认 `10s`）
- `STATIC_DIR`（前端静态资源目录，默认 `internal/httpapi/assets`）
- `API_KEYS`（可选，`operator:secret` 逗号分隔；设置后写操作需 `X-API-Key` 头）
- `WORKER_ENABLED` / `WORKER_INTERVAL` / `WORKER_BATCH_SIZE` / `WORKER_MAX_ATTEMPT` / `WORKER_BASE_DELAY`

### 冒烟测试

```bash
scripts/smoke_test.sh
```

脚本会真实构建并启动服务、启动临时 PostgreSQL 容器、轮询 `/healthz`、创建设备与规则、
上报合法报文断言 `event_id`、重复上报断言 `duplicate`，最后用 `trap` 清理进程与临时容器。

## 评测镜像构建与验证

镜像基于官方 `golang:1.22`（单阶段，保留完整 Go 工具链），构建时预下载依赖并执行 `go build ./...`，启动后进入 bash。

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh my-go-task            # 默认 linux/amd64
./build_benzhi_docker.sh my-go-task linux/amd64
./build_benzhi_docker.sh my-go-task linux/arm64
```

进入容器验证（离线编译不应出现 `downloading ...` 字样）：

```bash
docker run -it my-go-task:latest
go build ./...
go version
```

## 主要 API（摘要）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /healthz | 存活检查（200，status=alive） |
| POST | /api/v1/devices | 注册设备（201） |
| POST | /api/v1/telemetry | 上报报文（原始/重复识别） |
| POST | /api/v1/rules | 创建路由规则 |
| GET/PUT/DELETE | /api/v1/rules/{rule_id} | 规则查询/更新/软删除 |
| GET | /api/v1/rules/{rule_id}/changes | 规则变更留痕 |
| GET | /api/v1/events | 事件查询（过滤/分页） |
| GET | /api/v1/events/{event_id}/deliveries | 分发记录 |
| POST | /api/v1/events/{event_id}/replay | 事件重放/补发 |
| GET | /api/v1/stats | 统计（设备/事件/待重试数） |
| GET | /metrics | Prometheus 指标 |

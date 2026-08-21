# 设备遥测协议解析与事件路由服务 · 项目文档

## 1. 业务目标与真实使用者
本项目面向物联网接入场景，从零构建一套 Go 后端 API 服务，承接设备经固定文本协议上报的遥测报文，完成协议解析、字段校验、幂等去重、标准事件化与按规则路由分发。数据使用 PostgreSQL 持久化，留存设备档案、原始报文、解析后事件、路由规则及其变更留痕。

真实使用者有两类：
- 平台运维：注册/停用设备、配置与变更路由规则、查询事件与分发记录、查看规则变更留痕，并通过健康检查监控服务存活。
- 设备集成开发者：对接上报协议、联调报文格式与校验码、通过可操作页面模拟上报并核对解析与路由结果。

服务同时提供可操作前端页面（本项目计入前端配额，前端代码不计入 Go 规模），页面真实调用下列 Go API 完成交互闭环。

## 2. 核心业务闭环
设备注册生成 device_id 档案 → 设备按固定文本协议上报报文 → API 接收原始报文 → 协议切分与字段校验（格式、类型、量程、时间窗、校验码）→ 幂等去重（命中 dedup_key 则返回原事件，不重复入账）→ 生成标准事件入库 → 匹配启用中的路由规则 → 确定下行主题并记录分发记录 → 设备/规则/事件均可查询，规则变更写留痕。后台任务对失败分发执行重试与死信归集。前端页面覆盖设备管理、报文上报、规则配置、事件查询四个交互闭环。

## 3. 实体字段与关系
- Device 设备档案：device_id（主键，唯一，格式 ^[A-Z0-9-]{8,32}$）、name、protocol_version、status、metadata（JSONB）、last_seen_at、created_at、updated_at。
- RawMessage 原始报文：id（主键）、device_id（外键）、raw_text、checksum_received、checksum_expected、received_at、dedup_key、status、parse_error、event_id（可空）。
- Event 解析后事件：id（主键）、event_id（唯一）、device_id（外键）、ts、metric、value（数值）、unit、dedup_key、route_key、status、parsed_at。
- RouteRule 路由规则：id（主键）、rule_id（唯一）、name、matcher（JSONB，含设备模式/指标集合/表达式）、topic、priority、enabled、deleted_at、created_at、updated_at。
- RouteRuleChangeLog 规则变更留痕：id（主键）、rule_id（外键）、action、before_json、after_json、operator、changed_at。
- DeliveryRecord 分发记录：id（主键）、event_id（外键）、rule_id（外键）、topic、status、attempts、last_error、next_retry_at、dispatched_at。

关系：Device 1—N RawMessage、Device 1—N Event、Event N—1 Device、RouteRule 1—N DeliveryRecord、Event 1—N DeliveryRecord、RouteRule 1—N RouteRuleChangeLog。

## 4. 状态流转与约束
- Device：pending → active；active → suspended；suspended → active；active → deleted（终态）。suspended/deleted 设备的上报被拒绝。
- RawMessage：received → parsed（校验通过）；received → rejected（字段/校验码非法，终态）；received → duplicate（幂等命中，终态）。
- Event：created → routed → dispatched；routed → dead_letter（重试耗尽后归集）。
- RouteRule：enabled → disabled → enabled；删除为软删除（置 deleted_at 并停用），不物理删除以保留留痕。

约束：时间戳 ts 不得晚于服务时间 +5 分钟、不得早于 30 天；metric 必须属于白名单集合；value 必须为有限数值且在量程内；校验码采用 CRC16-CCITT（大写十六进制），覆盖报文体除校验码外全部内容；dedup_key = SHA256(device_id|ts|metric|value|unit) 前 32 位，去重窗口 24 小时。

## 5. API 输入输出与错误语义
统一响应：成功返回 HTTP 2xx 与 JSON 数据；失败返回统一错误信封，含 error.code、error.message、error.details 三段。状态码语义：400 参数/字段非法、401 未认证（扩展阶段启用）、404 不存在、409 冲突、422 校验失败、500 内部错误。错误码示例：INVALID_FIELD、CHECKSUM_MISMATCH、DEVICE_NOT_FOUND、DUPLICATE_MESSAGE、RULE_CONFLICT、UNKNOWN_METRIC、TS_OUT_OF_RANGE、RULE_NOT_FOUND。

- GET /healthz → 200，响应体含 status=alive。
- POST /api/v1/devices 入参 name、protocol_version、metadata → 201 返回设备档案；重复 device_id 返回 409。
- GET /api/v1/devices、GET /api/v1/devices/{device_id}。
- POST /api/v1/telemetry 入参 raw_text（或分段字段）→ 200/201 返回 event_id、duplicate 标记、matched_rules、topics；校验失败 422；未知设备 404；重复报文返回原事件并标记 duplicate。
- POST /api/v1/rules、PUT /api/v1/rules/{rule_id}、DELETE /api/v1/rules/{rule_id}、GET /api/v1/rules、GET /api/v1/rules/{rule_id}/changes。
- GET /api/v1/events 支持 device_id、metric、from、to、status、page、size 过滤；GET /api/v1/events/{event_id}。
- GET /api/v1/stats 返回设备数、事件数、待重试分发数，供前端首页。

## 6. 持久化与变更留存
PostgreSQL 单库多表，启动时执行迁移（internal/migrate）建表与索引：device.device_id 唯一索引；raw_message.dedup_key 唯一索引（结合 received_at 维护 24 小时去重窗口）；event(device_id, ts) 与 event(metric, ts) 复合索引；route_rule(enabled) 与 delivery_record(next_retry_at) 索引。所有写操作在同一事务内写入业务表与 RouteRuleChangeLog（或对应留痕），before/after 以 JSONB 保留，保证可审计。分发记录持久化，支持后台任务幂等重试。

## 7. 模块边界
- cmd/server：配置加载、依赖装配、优雅退出。
- internal/config：环境变量解析（DATABASE_URL、PORT=8080、去重窗口等）。
- internal/httpapi：HTTP 路由、中间件（请求日志、恢复、统一错误）、各资源 handler、静态页面托管（扩展阶段）。
- internal/domain：实体、值对象、领域错误，不依赖存储。
- internal/parser：协议切分与校验码，纯函数，不触碰数据库。
- internal/service：业务用例编排（注册、上报解析+去重+路由、规则 CRUD、事件查询、统计）。
- internal/store：PostgreSQL 仓储接口与实现，SQL 与事务边界。
- internal/dedup：去重键计算与窗口判定。
- internal/router：规则匹配与主题决策（入站事件 → 匹配规则 → 输出主题列表）。
- internal/worker：分发重试与死信后台任务（扩展阶段）。

模块依赖单向：httpapi → service → store/parser/dedup/router；domain 被各层引用但不依赖外部。

## 8. 前端可操作页面（前端配额，不计 Go 规模）
- 首页 /：存活状态、设备数/事件数/待重试数统计（GET /healthz、GET /api/v1/stats），最近事件表格。
- /devices：设备注册表单（POST /api/v1/devices）、设备列表与停用操作。
- /telemetry：报文输入框与提交按钮（POST /api/v1/telemetry），实时展示解析结果、去重命中与命中的下行主题。
- /rules：规则创建/编辑/禁用表单（POST/PUT/DELETE /api/v1/rules）、规则列表与变更留痕查看（GET /api/v1/rules/{rule_id}/changes）。
- /events：按设备/指标/时间/状态过滤查询（GET /api/v1/events）。

页面为纯静态 HTML+CSS+JS，由 Go 服务静态托管（/static 与页面路由），前端代码不计入 Go 规模。

## 9. 关键测试
- 协议解析边界：合法报文、缺字段、多余字段、非法时间戳、非数值 value、未知 metric、大小写校验码。
- 非法字段校验：device_id 格式、量程越界、空 raw_text、超长报文。
- 事件路由命中：单规则命中、多规则命中、优先级排序、无规则、禁用规则不命中。
- 重复报文幂等去重：同 dedup_key 二次上报返回原事件不新增；窗口边界行为。
- 规则变更留痕：创建/更新/禁用均产生 before/after 记录。
- 错误语义：404/409/422 与统一错误信封。
- 存储集成测试：事务回滚、唯一约束、索引命中。

## 10. 启动冒烟与验收场景
Dockerfile：多阶段构建，支持 linux/arm64 与 linux/amd64，EXPOSE 8080，ENTRYPOINT 启动服务，监听端口固定 8080。scripts/smoke_test.sh：真实启动服务（go build 或 docker run）→ 轮询 GET /healthz 断言 200 与 alive → 创建测试设备 → 上报一条合法报文断言 event_id → 重复上报断言 duplicate → 使用 trap 清理进程与临时数据（含测试库清理）。

验收场景：A）设备注册后上报合法报文，事件入库且命中规则产生分发记录；B）同报文重复上报幂等不重复入账；C）非法校验码返回 422 且不产生事件；D）规则变更可查询留痕；E）前端四页面均能真实调用 API 完成闭环；F）healthz 恒为 200；G）arm64/amd64 镜像均能启动。

## 11. 两阶段实施计划
### 初始核心版本（约 1600～2200 行非测试 Go 有效代码，18～24 个生产 .go 文件）
目标为完整可运行的最小闭环，不凑代码。包含：配置、HTTP 服务、健康检查、设备注册、报文解析+校验、幂等去重、规则 CRUD、事件查询、规则留痕、PostgreSQL 迁移与仓储。文件约 22 个（对应模块边界实现，暂不拆仓储、不启用 worker/鉴权/前端托管）。冒烟与关键测试全绿，交付 Dockerfile 与 smoke_test.sh。
### 第一次业务扩写（约 2600～3800 行非测试 Go 有效代码，28～40 个生产 .go 文件）
新增真实业务能力，非凑行数：①分发重试与死信后台任务（internal/worker、delivery_repo）；②规则匹配表达式增强（AND/OR、通配符、metric 集合）；③API Key 鉴权与审计；④前端操作页面与静态托管、stats 聚合；⑤事件重放/补发接口；⑥Prometheus 指标。仓储按实体拆分、handler 按资源拆分，生产文件数增至 28～40 个。所有新增代码均被 API、后台任务、页面或启动路径真实调用。

## 12. 规模与代码治理约束
规模仅统计非测试生产 .go 文件中的有效代码（不含测试、vendor、前端 HTML/CSS/JS/TS、空行与纯注释）。初始核心版本 1600～2200 行、18～24 文件；第一次扩写后 2600～3800 行、28～40 文件；最终验收严格满足 >2000 且 <5000 行、>20 且 <50 文件。禁止复制粘贴、空实现、无调用模块、无意义包装与死代码；所有生产代码必须被 API、业务服务、后台任务或启动路径真实调用。

## 13. 已知缺陷留档（1 条 Bug 数据）
本项目按计划保留 1 条已知 Bug 数据：去重窗口边界下，当同 dedup_key 报文在窗口即将过期时并发上报，极低概率出现重复入账（唯一索引兜底将其中一条置为 duplicate，但日志会记录一次冲突告警）。该缺陷被单独标记在验收清单，不作为本次修复范围，用于缺陷发现评估；不影响其余闭环与规模约束。

代码质量与规模约束：不得用重复代码、死代码、空实现、无调用模块或无意义包装凑行数；每个生产文件、类型和函数都必须服务于文档中的真实业务路径，并由 API、业务服务、后台任务或启动流程实际使用。

## 14. 实施进度（已交付）
本仓库当前实现的是**第一次业务扩写**后的版本（约 2600～3800 行非测试 Go 有效代码、28～40 个生产 .go 文件）。此阶段在初始核心版本之上补齐了真实业务能力：

- 分发重试与死信后台任务（`internal/worker`）：周期性扫描到期分发记录，指数退避重试，超过最大次数归集死信并推进事件状态；下发走可插拔 `Sink`（默认 `LogSink`），生产路径由 `cmd/server` 装配并接入 Prometheus 指标。
- 规则匹配表达式增强（`internal/router/expr.go`）：在原有设备通配符与指标集合之上支持 `AND`/`OR`、括号、`metric/device/unit` 字符串比较（`==`/`!=`/`~` 通配）与 `value` 数值比较（`==`/`!=`/`>`/`<`/`>=`/`<=`/`~`），非法表达式安全地不命中。
- API Key 鉴权与审计（`internal/auth`）：`API_KEYS` 环境变量（`operator:secret` 逗号分隔）按 SHA-256 哈希驻留内存，仅对写操作（POST/PUT/DELETE 的 `/api/v1/*`）强制 `X-API-Key`，认证/拒绝均写 `api_key_audit` 留痕；变更留痕中的 `operator` 由请求上下文归因。
- 事件重放/补发（`GET /events/{id}/deliveries`、`POST /events/{id}/replay`）：重放现有事件经当前启用规则重新生成待分发记录，跳过已 pending/failed 的同规则记录。
- Prometheus 指标（`internal/metrics`，零依赖手写 text 格式）：HTTP 在途/时延、分发结果计数，`GET /metrics` 暴露。
- 仓储按实体拆分（`internal/store/pg_audit.rs`、`pg_delivery_retry.go` 等），handler 按资源拆分，所有新增代码均被 API、后台任务或启动路径真实调用。

验收场景 A–G 全部闭环；1 条已知缺陷（去重窗口并发边界，见第 13 节）仍按计划保留。

## 15. 已知缺陷留档（1 条 Bug 数据）
去重窗口边界下，当同 `dedup_key` 报文在窗口即将过期时并发上报，极低概率出现重复入账（唯一索引兜底将其中一条置为 duplicate，但日志会记录一次冲突告警）。该缺陷被单独标记在验收清单，不作为本次修复范围，用于缺陷发现评估；不影响其余闭环与规模约束。

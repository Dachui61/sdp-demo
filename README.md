# 1 总体架构设计（High-Level Architecture）

## 1.1 目标回顾（简短）

* 支持 L3/L4 隧道与 7 层代理（外部应用）两种接入方式。
* 支持 34+ 种认证适配器与可插拔自定义脚本/适配器。
* 本地部署优先，需支持本地集群和分布式单元（单元内高可用、跨单元扩展）。
* 要求可观测、可扩展、易维护与高可用。

## 1.2 推荐总体拓扑（文字版）

```
Clients (Windows/Mac/Linux/Android/iOS)
   ├─ Client Agent (虚拟网卡, 隧道模块, 上线认证)
   ├─ Browser (外部/网页应用访问)
   ↓
Local Network / Internet
   ↓
Edge Load Balancer / Reverse Proxy (可选)
   ↓
aTrust Gateway / Net-Proxy 集群 (多实例, svc)
   ├─ Auth Service (认证 & MFA)
   ├─ Policy Engine (准入/应用策略)
   ├─ Tunnel Proxy (L3/L4)
   ├─ Application Reverse Proxy (7层)
   ├─ Connector Manager (内网连接器/Connector pool)
   ├─ Session Service (会话管理)
   └─ Audit / Telemetry pipeline
   ↓
Backend Resources (intranet services / apps)
```

## 1.3 关键组件清单（职责 & 推荐实现语言/技术）

> 下列为建议，便于性能与维护权衡：

1. **Gateway / Net Proxy (L4 / TCP short/long tunnel)**

   * 责任：接收客户端隧道连接（短/长隧道），执行包/流转发、虚拟 IP 映射、打包上报终端信息给网关。
   * 推荐技术：Go（高性能网络库，goroutine 友好）或 Rust（更高安全性）；Linux 下使用 TUN/TAP、Windows 使用 WinTUN / NDIS 驱动。
2. **Reverse Proxy / App Proxy (7-layer)**

   * 责任：处理无客户端场景的 HTTP(S)/Web 应用鉴权、Header 注入、URL 路由、Host-based 转发。
   * 推荐技术：基于 Envoy 或 Nginx + Lua 的扩展，控制层与策略交互使用 gRPC/REST。
3. **Auth Service（认证适配器层）**

   * 责任：集中管理认证流程、MFA、自适应认证、对接第三方认证（OAuth/RADIUS/AD/钉钉等），并暴露插件接口。
   * 推荐技术：Kotlin/Java 或 Node.js/Go；提供 gRPC/REST API。
4. **Policy Engine（准入/应用策略）**

   * 责任：条件决策（用户/端点/行为），支持 AND/OR 组合与时间段、地理、进程/软件检测等条件。
   * 推荐技术：使用 OPA（Rego） 或 自研规则服务；规则下发至网关与客户端。
5. **Connector（内网连接器）**

   * 责任：部署在客户内网，和云/中心网关做持久双向连接，作为内网资源的反向代理桥接。
   * 推荐技术：Go 小型单二进制，长连接 + TLS，自动注册到 Gateway。
6. **Session / State Store**

   * 责任：保存会话、虚拟 IP 分配、登录态。
   * 推荐技术：Redis（会话），Postgres（关系数据），Etcd（配置/发现）。
7. **Data & Config（DBs）**

   * MySQL/Postgres：用户 / 授权 / 审批 等结构化数据。
   * Etcd：全局配置、服务发现。
   * Redis：短期会话、速率限制、临时缓存。
8. **Audit / Logging / Telemetry**

   * Prometheus + Grafana（指标），Jaeger（分布式追踪），ELK/ClickHouse（日志/审计）。
9. **管理控制台（Web UI）**

   * React + TypeScript 前端；后端 Admin API（Node.js / Spring Boot / Go）。
10. **CI/CD 与 运维工具**

* Docker + Kubernetes (建议生产用 K8s)，Helm charts，Argo CD 或 Flux。

## 1.4 组件之间通信机制

* **内部服务间**：gRPC（高效、强类型） + mTLS，REST 作为管理/兼容层。
* **网关 <-> 客户端**：双向 TLS（mTLS），携带终端元数据（30Id、设备信息、policy tags）在协议层（封装在隧道握手包/HTTP header）。
* **事件 & 异步**：使用 Kafka 或 NATS 进行异步事件（审计、告警、配置下发、connector 通知）。
* **配置下发**：中心通过 Etcd/Config API 将策略/资源下发到网关/客户端，支持推送与轮询双模式。
* **日志/指标**：应用写入 stdout -> Fluentd/Vector -> Log backend；Prometheus 抓取 /metrics。

## 1.5 安全通信细节

* 网关和客户端之间必须使用 **mTLS** + 双向证书认证；证书可由本地 CA 管理或企业 PKI。
* 对外 HTTP(S) 请使用 HSTS、强 ciphersuite、OCSP stapling。
* 关键数据（私钥、证书）建议使用 HSM 或 Vault（HashiCorp Vault）管理。

---

# 2 项目结构设计（代码仓库与模块化组织）

目标：模块化、低耦合、易扩展，支持团队并行开发与独立部署。

## 2.1 推荐仓库策略

* **单体仓库（mono-repo）或多 repo？**

  * 建议：**mono-repo**（按服务拆子目录），便于版本控制、联动变更、统一 CI。若规模非常大、团队独立度高可考虑多 repo。
* 根目录示例（mono-repo）：

```
/atrust/
  /infra/                     # k8s helm charts, terraform infra templates
  /docs/                      # 设计文档、运维手册、API 文档
  /services/
    /gateway/                 # L4/L3 隧道代理服务 (go)
    /proxy/                   # 7-layer app proxy (envoy + control plane)
    /auth/                    # 认证服务 (java/go)
    /policy/                  # Policy engine (OPA + wrapper)
    /connector/               # 内网连接器 agent (go)
    /session/                 # Session service (go)
    /admin-api/               # 管理后台 API (node/java)
    /console/                 # 前端 (react/typescript)
  /common/                    # 公共库：proto，shared utils，sdk
  /clients/
    /agent-win/               # 客户端 agent windows-specific (c#/cpp/go)
    /agent-linux/             # 客户端 linux (go + tuntap)
    /agent-mac/               # macOS (swift/rust)
  /scripts/                   # release / packaging / drivers build scripts
  /tools/                     # dev tools (test harness, simulators)
  /qa/                        # test cases, load test scripts
```

## 2.2 每个服务的内部结构（示例：gateway）

```
/services/gateway/
  /cmd/                       # main -> bootstrap
  /internal/
    /net/                     # tunnel / virt NIC handlers
    /authclient/              # call to Auth Service
    /policy/                  # local policy cache + matcher
    /session/                 # session lifecycle
    /metrics/                 # prometheus metrics
    /tls/                     # cert mgmt
  /api/
    /proto/                   # .proto definitions (gRPC)
  /configs/
  /deploy/                    # Dockerfile, k8s manifests
  /pkg/                       # reusable packages for other services
```

## 2.3 接口与协议约定（示例）

* 使用 Protobuf 定义所有服务间 API（`/common/proto`），并生成语言绑定。
* 必要 REST API 保留 JSON-over-HTTPS（用于管理控制台兼容）。
* 隧道握手协议（示例字段）：

  ```proto
  message TunnelHandshake {
    string client_id = 1;
    string device_id = 2;
    bytes client_cert = 3;
    map<string,string> meta = 4; // os, ip, process list hash, 30Id ...
  }
  ```
* Policy 表达式使用 Rego 或 JSON Schema；对外提供 `evaluate(user, device, resource, action)` 接口。

## 2.4 插件 / 适配器机制（必须有）

* **认证适配器插件**：所有认证方式通过统一 **Adapter Interface** 暴露（gRPC or dynamic plugin）。

  * 支持热加载/卸载。
  * 推荐实现方案：基于 gRPC + sidecar adapter service 或 WASM 插件（安全沙箱）。
* **脚本扩展点**：允许用户上载自定义脚本（Lua / WASM）来实现字段提取 / 断言，但必须运行在沙箱中并受资源/时间限制。

---

# 3 关键特性：可扩展性、鲁棒性与可维护性

## 3.1 可扩展性设计

1. **服务拆分 + 无状态化**

   * 网关/Proxy 保持尽量无状态（会话状态存 Redis/Session 服务），便于水平扩容。
2. **配置与策略下发采用中心化 + 缓存**

   * Etcd/Config 服务做强一致性写入，网关本地缓存做高 QPS 读取。
3. **插件化认证 & 策略引擎**

   * 认证适配器、认证流程、策略语句都以插件/规则的形式下发/热加载。
4. **Connector 横向扩展**

   * Connector 作为无状态或轻状态进程，支持多实例，用于内网资源接入。
5. **虚拟 IP 与路由池设计**

   * 虚拟 IP 段分配模块支持 CIDR 扩容、按网关/单元划分，避免冲突。
6. **异步事件总线**

   * 使用 Kafka/NATS 以支持审计、大量并发日志、告警扩展（可接入 SIEM）。

## 3.2 鲁棒性 / 高可用性

1. **多层冗余**

   * 每个核心服务至少 N 个实例（K8s Deployment + PodAntiAffinity）。
   * DB：Postgres 主备 + read replicas，Redis Cluster，多 AZ 部署。
2. **熔断 / 限流 / 重试策略**

   * 服务调用链加入 circuit breaker（resilience4j / envoy filters），并在跨单元调用中使用重试与幂等设计。
3. **故障感知与自动切换**

   * Health checks + readiness/liveness probes，自动流量剔除故障实例。
4. **会话迁移 / 单元切换策略**

   * 分布式部署下，若客户端请求打到非其会话单元，使用中心路由器（session router）将流量转发到会话单元，或触发 Logout/重登录。
   * 提供“会话持久化快照”用于快速登录恢复（可选）。
5. **灰度 & 回滚能力**

   * 所有策略、认证 adapter、网关变更支持灰度下发并快速回滚。
6. **灾备**

   * 重要数据做异地备份（RPO/RTO 策略根据合规要求），DB 支持 PITR（point-in-time recovery）。

## 3.3 可维护性 / 运维经验建议

* **统一的 Observability**：Prometheus 指标 + Grafana Dashboards，必备：CPU、Conn Count、Tunnel Conn、Auth Latency、Policy Eval Latency、Errors、Audit QPS 等。
* **集中化日志与审计**：所有审计事件以结构化 JSON 写入（包含 userId、deviceId、resource、action、policyId、result、timestamp 等），并入 ELK/ClickHouse。
* **健康检查与自动化恢复 Playbook**：把常见故障（证书过期、redis 挂、etcd leader 切换）形成自动化 runbook & k8s operator。
* **版本与兼容策略**：服务间 API 使用语义化版本控制；客户端与网关采用向后兼容的握手。

## 3.4 安全性与合规

* **最小权限原则**：服务账户、数据库、运维账号最小权限。
* **审计链完整性**：审计数据带签名/不可篡改（append-only store 或外部 WORM 存储）。
* **敏感信息加密**：DB 中敏感字段做字段级加密，Secrets 存 Vault。
* **合规支持**：提供审计导出、访问日志导出、隐私字段脱敏功能。

---

# 4 详细模块 & 接口举例（供开发直接实现）

## 4.1 认证 Adapter Interface（伪代码/Proto）

```proto
service AuthAdapter {
  rpc Authenticate(AuthenticateRequest) returns (AuthenticateResponse);
  rpc VerifyMFA(VerifyMFARequest) returns (VerifyMFAResponse);
  rpc GetUserInfo(GetUserInfoRequest) returns (UserInfo);
}

message AuthenticateRequest {
  string username = 1;
  map<string,string> credentials = 2; // password, token...
  map<string,string> device_meta = 3;
}
```

* 适配器以独立进程/容器运行，核心 Auth Service 通过 gRPC 调用。适配器可在运行时注册/注销。

## 4.2 Policy Evaluation API

```http
POST /api/v1/policy/evaluate
Body:
{
  "user": { "id":"u123", "roles":["dev"] },
  "device":{"id":"d456","os":"win10","is_trusted":false},
  "resource":{"id":"r1","type":"app","host":"internal.app"},
  "action":"access"
}
```

* 返回：`allow/deny`, `required_steps`（如需要 MFA）和 `policy_id`。

## 4.3 Tunnel Handshake Sequence（文字版）

1. Client -> Gateway : TLS connect + TunnelHandshake (client\_id, device\_id, cert)
2. Gateway 验证证书 -> 调用 Auth Service 验证身份 -> Policy Engine eval -> 下发 session token / virtual IP / routes。
3. Gateway 建立 per-client tunnel session，开始数据转发；会向 Session Service 注册会话元数据以便审计。

---

# 5 测试、质量保障与演练

## 5.1 必须的测试类型

* 单元测试、集成测试（mock adapter）、端到端（E2E）自动化（包含认证流程与隧道流量）。
* 性能测试：并发 Tunnel 连接数、吞吐、Policy eval 延迟、Proxy RPS。
* 混沌测试：随机关闭网关实例、驱逐 Redis 节点、模拟证书过期。
* 安全渗透测试：对外代理、管理控制台、客户端 agent 权限等。

## 5.2 质量门（CI）

* PR 必须通过：静态扫描（gosec/spotbugs）、单元测试覆盖、集成测试（模拟环境）与安全依赖扫描。
* 镜像构建后执行 SCA（软件成分分析），并将镜像签名入制品仓库。

---

# 6 可扩展的设计细节（一些“高级”建议）

1. **使用 WASM 作为安全扩展沙箱**：把自定义策略、认证脚本运行在 WASM 环境（在 gateway 或 policy 层），既安全又高性能。
2. **Policy 预编译与缓存**：Policy 使用 Rego 编译成 AST，网关加载预编译规则实现快速评估。
3. **流量拍照 / 动态审计**：对访问敏感资源的会话进行流量样本抓取（仅在合规允许下），用于事后审计。
4. **渐进式数据平面演进**：初期使用 Envoy + Lua / Go filter 作为数据平面，后期如有需求再替换为更高性能的 Rust 实现。

---

# 7 交付物（建议交付清单）

* 架构设计说明书（本文件 + 架构图）。
* 代码仓库骨架（按照上文目录），包含 scaffold 服务（gateway、auth、policy、connector、console）最小可运行 demo。
* Protobuf / API 定义（common/proto）。
* Helm charts / k8s manifests（dev/staging/production）。
* 配置与 secrets 管理示例（Vault / Kubernetes Secrets + SealedSecrets）。
* Observability dashboards & Prometheus alerts sample。
* 安全/运维 runbooks（证书更新、故障恢复步骤）。
* 自动化测试脚本（basic e2e + load test harness）。

---

# 8 风险 & 缓解建议（短列）

1. **虚拟网卡/驱动跨平台复杂** → 复用成熟库（WinTUN / tuntap / WireGuard primitives）；把驱动与业务分离（驱动最少逻辑）。
2. **会话跨单元一致性问题** → 强化 session ownership 机制（session router），或引入 sticky routing。
3. **Policy 评估性能瓶颈** → 预编译、热缓存、限流与异步审计分离。
4. **认证适配器安全问题（自定义脚本）** → 强制运行在沙箱 / WASM / 限资源容器中。
# AA 分账（aasplit）

生活服务全栈应用：适用于朋友聚餐、合租、旅行等场景的多人费用记录与智能分摊结算工具，支持账单历史查询、智能结算路径优化与数据统计图表。

## 快速启动（Docker Compose，首选）

```bash
# 在项目根目录（支持任意目录名，包括中文目录名）执行
docker compose up -d --build
```

启动完成后：

| 服务 | 访问地址 |
| --- | --- |
| 前端 | http://localhost:18401 |
| 后端 API | http://localhost:19401/api/v1 |
| 健康检查 | http://localhost:19401/healthz |
| 接口文档 | 见下方「API 调用示例」，完整清单见 `backend/api/openapi.yaml` |

默认管理员账号：`admin / admin123`（首次启动自动创建）。

## 项目主要功能

1. **创建分账群组**：创建群组、邀请好友、群组名称与描述编辑、归档。
2. **添加消费记录**：金额、类别（餐饮/交通/住宿/娱乐/其他）、付款人、参与人、分摊方式、小票图片 URL。
3. **多种分摊方式**：均摊 / 按比例 / 按金额，系统自动计算每人应付金额（合计严格等于消费总额）。
4. **智能结算建议（双方确认）**：基于成员净余额贪心匹配最大债权人与债务人，最小化转账次数生成转账清单。每笔转账需**付款方先点「我已转账」**（金额进入待确认，净余额暂不动），**收款方核对是谁转来的钱后点「确认收款」**，转账才完成并更新双方净余额；服务端按 JWT 身份强制校验，任何一方都不能代对方确认。账单新增、修改或退款后，尚未完成双方确认的转账自动作废（voided）并按新账重算，已确认收款的记录保留。
5. **账单历史查询**：按群组、时间范围、消费类别筛选，支持导出账单明细 CSV。
6. **用户中心**：注册登录、头像昵称邮箱管理、我的群组列表、待结算提醒。
7. **数据统计**：月度消费趋势、各类别占比、各成员消费排行（ECharts 图表）。

## 技术栈

| 层 | 技术栈 |
| --- | --- |
| 前端 | Vue 3 + TypeScript + Element Plus，构建工具 Vite |
| 后端 | Go 1.22 + Gin + GORM |
| 数据库 | PostgreSQL 16 |
| 缓存/限流 | Redis 7 |
| 认证 | JWT + RBAC |
| 日志 | `log/slog` 结构化日志 |
| 参数校验 | `github.com/go-playground/validator/v10` |
| 接口文档 | OpenAPI（`backend/api/openapi.yaml`） |

## 项目目录结构

```
cy-381/
├── docker-compose.yml
├── .env
├── .env.example
├── README.md
├── database/
│   └── init.sql
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/config.go
│   │   ├── database/database.go
│   │   ├── model/            # 每个实体一个文件：user/group/group_member/expense/expense_share/settlement/audit_log
│   │   ├── dto/              # 每个实体一个 DTO 文件
│   │   ├── repository/       # 每个实体一个 repository 文件
│   │   ├── service/          # 每个实体一个 service 文件（含事务与状态机）
│   │   ├── handler/          # 每个实体一个 handler 文件
│   │   ├── router/           # 每个实体一个路由注册文件
│   │   ├── middleware/       # auth/rbac/request_id/error_handler/rate_limit/audit/cors/request_logger
│   │   ├── constants/        # enums/error_codes/messages/log_templates
│   │   └── util/             # logger/jwt/password/response/app_error/formatters/pagination/context
│   ├── pkg/splitcalc/        # 分摊计算与结算路径优化（纯算法，可复用）
│   ├── migrations/001_init.sql
│   ├── api/openapi.yaml
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
└── frontend/
    ├── src/
    │   ├── api/              # auth/user/group/expense/settlement/stats/audit
    │   ├── components/       # StatusBadge/EmptyState/DataTable/ConfirmDialog/SplitTypeTag/MoneyText/ExpenseFormDialog
    │   ├── pages/            # 登录/注册/工作台/个人中心/群组/消费/结算/统计/审计
    │   ├── stores/           # auth/group/expense/settlement/audit
    │   ├── hooks/            # useAuth/usePagination
    │   ├── utils/            # request/format
    │   ├── constants/        # 与后端对应枚举
    │   ├── router/
    │   └── layouts/
    ├── Dockerfile
    └── nginx.conf
```

## 环境变量说明

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `COMPOSE_PROJECT_NAME` | `aasplit` | docker compose 项目名（容器/卷名前缀） |
| `DB_NAME` | `aasplit_db` | 数据库名 |
| `DB_USER` | `aasplit_user` | 数据库用户 |
| `DB_PASSWORD` | `aasplit_pwd` | 数据库密码 |
| `JWT_SECRET` | `change_me_to_a_long_random_string` | JWT 签名密钥（生产替换） |
| `JWT_EXPIRE_HOURS` | `72` | JWT 有效期（小时） |
| `RATE_LIMIT_PER_MIN` | `120` | 限流阈值（次/分钟/IP） |
| `CORS_ORIGINS` | `*` | 允许跨域来源 |
| `LOG_LEVEL` | `info` | 日志级别 |
| `FRONTEND_PORT` | `18401` | 前端宿主端口 |
| `BACKEND_PORT` | `19401` | 后端宿主端口 |
| `DB_PORT` | `44012` | 数据库宿主端口 |
| `REDIS_PORT` | `46312` | Redis 宿主端口 |

## 本地开发

后端（Go 1.22+）：

```bash
cd backend
go mod tidy
go run ./cmd/server
# 构建：go build ./...
# 测试：go test ./...
# 静态检查：go vet ./...
```

前端（Node 18+）：

```bash
cd frontend
npm install
npm run dev
```

## API 调用示例（curl）

统一响应格式：`{ "code": 0, "message": "ok", "data": ... }`；认证接口需携带 `Authorization: Bearer <token>`。

```bash
# 1. 注册
curl -sS -X POST http://localhost:19401/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"secret123","nickname":"Alice","email":"alice@test.com"}'

# 2. 登录获取 JWT
TOKEN=$(curl -sS -X POST http://localhost:19401/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r '.data.token')

# 3. 创建群组
curl -sS -X POST http://localhost:19401/api/v1/groups \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"周末聚餐","description":"好友聚餐"}'

# 4. 查询我的群组
curl -sS http://localhost:19401/api/v1/groups \
  -H "Authorization: Bearer $TOKEN"

# 5. 添加消费记录（均摊）
curl -sS -X POST http://localhost:19401/api/v1/groups/1/expenses \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"火锅","amount":300,"category":"dining","payer_id":1,"split_type":"equal","paid_at":"2026-08-01 12:00:00","shares":[{"user_id":1},{"user_id":2},{"user_id":3}]}'

# 6. 按最新账单重算结算转账（新增账单后通常已自动重算，本接口用于手动触发）
curl -sS -X POST http://localhost:19401/api/v1/groups/1/settlements/generate \
  -H "Authorization: Bearer $TOKEN"

# 6.1 付款方（from_user 本人）确认「我已转账」，金额进入待确认，净余额暂不动
curl -sS -X POST http://localhost:19401/api/v1/settlements/transfer \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"settlement_id":10}'

# 6.2 收款方（to_user 本人）确认收款，完成后双方净余额更新；其他人调用返回 40303
curl -sS -X POST http://localhost:19401/api/v1/settlements/confirm \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"settlement_id":10}'

# 6.3 查询等待当前用户操作的转账（待我转账 / 待我确认收款）
curl -sS http://localhost:19401/api/v1/settlements/pending \
  -H "Authorization: Bearer $TOKEN"

# 7. 查询群组统计
curl -sS http://localhost:19401/api/v1/groups/1/stats \
  -H "Authorization: Bearer $TOKEN"

# 8. 导出账单 CSV
curl -sS http://localhost:19401/api/v1/groups/1/expenses/export \
  -H "Authorization: Bearer $TOKEN" -o expenses.csv

# 9. 审计日志（管理员）
curl -sS http://localhost:19401/api/v1/audit-logs \
  -H "Authorization: Bearer $TOKEN"
```

## Docker 部署说明

- 端口映射：前端 `${FRONTEND_PORT:-18401}:80`，后端 `${BACKEND_PORT:-19401}:8080`，数据库 `${DB_PORT:-44012}:5432`，Redis `${REDIS_PORT:-46312}:6379`。
- 数据卷：`db_data`（PostgreSQL 数据）、`redis_data`（Redis 数据），命名卷持久化，不依赖宿主机路径，任意目录名可启动。
- 健康检查：数据库 `pg_isready`、Redis `redis-cli ping`、后端 `/healthz`；后端等待 db/redis healthy 后才启动。
- 常见问题：
  - 端口被占用：修改 `.env` 中对应端口后重新 `docker compose up -d`。
  - 重置数据：`docker compose down -v --remove-orphans` 后重新启动。
  - 换 JWT 密钥：修改 `JWT_SECRET` 后重启后端，旧 token 全部失效。

## 核心实体与接口复用

核心实体（≥4）：**用户 User**、**分账群组 Group**、**消费记录 Expense**、**结算转账 Settlement**（双方确认状态机）、**审计日志 AuditLog**（辅助实体：群组成员 GroupMember、分摊明细 ExpenseShare）。

接口复用说明：
- `GET /groups/:id/expenses` 与 `GET /groups/:id/expenses/export` 复用 `ExpenseService.List`（导出在 List 基础上转换 CSV）。
- 账单新增/修改/退款（`ExpenseService.Create/Update/Delete`）与 `POST /groups/:id/settlements/generate` 复用 `SettlementService.RebuildOnExpenseChange/rebuildInTx`：同一事务内「作废未确认转账 → 扣除已确认收款 → 重算剩余转账」。
- `GET /groups/:id/settlements`、`GET /groups/:id/balances` 与 `GET /settlements/pending` 复用成员校验逻辑（`memberRepo.Exists`）；余额计算同时复用 `shareRepo.SumPaidByGroup/SumOwedByGroup` 与 `settleRepo.SumSettledByGroup`（仅已确认收款的转账计入净余额）。
- `POST /settlements/transfer` 与 `POST /settlements/confirm` 复用同一「行锁 + 当事人校验 + 条件状态更新」流程，分别限定 `from_user` / `to_user` 本人操作。
- `GET /groups/:id/stats` 与 `GET /groups/:id/balances` 复用 `ExpenseShareRepository.SumPaidByGroup/SumOwedByGroup`。

## 横切关注点

1. **JWT 认证 + RBAC 权限**：数据库角色字段（`users.role`）→ `backend/internal/middleware/auth.go`、`backend/internal/middleware/rbac.go`、`backend/internal/util/jwt.go` → 前端 `src/router/index.ts` 路由守卫、`src/stores/auth.ts`、菜单与按钮显隐（`MainLayout.vue`、`GroupMembers.vue`）。
2. **操作审计日志**：数据库 `audit_logs` 表 → `backend/internal/middleware/audit.go`（HTTP 写操作通用审计）→ `backend/internal/service/audit_service.go`（service 业务埋点）→ 前端 `src/pages/audit/AuditLogs.vue`、`src/stores/audit.ts`。
3. **全局错误处理与请求追踪**：`backend/internal/middleware/request_id.go`、`backend/internal/middleware/error_handler.go`、`backend/internal/util/app_error.go`、`backend/internal/constants/error_codes.go` → 前端 `src/utils/request.ts` 拦截器（JWT 注入、401 跳转、统一错误提示）。

## 后端中间件清单

- `internal/middleware/auth.go`：JWT 认证
- `internal/middleware/rbac.go`：RBAC 角色校验
- `internal/middleware/request_id.go`：请求 ID 注入与追踪
- `internal/middleware/error_handler.go`：panic 恢复与统一错误响应
- `internal/middleware/rate_limit.go`：Redis 限流（回退进程内计数）
- `internal/middleware/audit.go`：HTTP 写操作审计
- `internal/middleware/cors.go`：跨域
- `internal/middleware/request_logger.go`：结构化请求日志

## 枚举出现位置清单

### 1. 用户角色 UserRole（user / admin）

后端出现位置：
- `internal/constants/enums.go`：定义 `RoleUser` / `RoleAdmin`
- `internal/model/user.go`：`User.Role` 字段类型与默认值
- `internal/dto/user_dto.go`：`UpdateRoleReq.Role` 的 `oneof=user admin` 校验
- `internal/service/user_service.go`：注册默认角色、`ChangeRole` 校验、日志 `LogUserRoleChanged`
- `internal/service/group_service.go`：`canManage` 管理员判断
- `internal/middleware/rbac.go`：`RequireRoles("admin")` 权限控制
- `internal/router/user.go`：`users.GET` / `users.PUT/:id/role` 挂载 RBAC
- `internal/util/formatters.go`：`RoleText` 中文文案
- `internal/constants/log_templates.go`：`LogUserRegistered` 等日志模板
- `internal/database/database.go`：默认管理员 `RoleAdmin`

前端出现位置：
- `src/constants/index.ts`：`UserRole`
- `src/stores/auth.ts`：`isAdmin` getter
- `src/router/index.ts`：`meta.admin` 路由守卫
- `src/layouts/MainLayout.vue`：审计日志菜单显隐
- `src/pages/group/GroupMembers.vue`：成员角色展示
- `src/utils/format.ts`：`roleText`

### 2. 消费类别 ExpenseCategory（dining / transport / lodging / entertain / other）

后端出现位置：
- `internal/constants/enums.go`：定义五类枚举与 `IsValidExpenseCategory`
- `internal/model/expense.go`：`Expense.Category` 字段
- `internal/dto/expense_dto.go`：`Create/UpdateExpenseReq.Category` 的 `oneof` 校验
- `internal/service/expense_service.go`：创建/更新赋值、日志 `LogExpenseCreated`
- `internal/repository/expense_repository.go`：`List` 按类别筛选
- `internal/repository/stats_repository.go`：`SumByCategory` 分组统计
- `internal/handler/expense_handler.go`：`ExpenseQuery.Category` 筛选透传
- `internal/util/formatters.go`：`CategoryText` 中文文案
- `internal/constants/log_templates.go`：`LogExpenseCreated` 模板

前端出现位置：
- `src/constants/index.ts`：`ExpenseCategory` 与 `CategoryOptions`
- `src/components/StatusBadge.vue`：类别徽标
- `src/components/ExpenseFormDialog.vue`：类别选择下拉
- `src/pages/expense/ExpenseList.vue`：类别筛选
- `src/pages/stats/GroupStats.vue`：类别占比图
- `src/utils/format.ts`：`categoryText`

### 3. 分摊方式 SplitType（equal / ratio / amount）

后端出现位置：
- `internal/constants/enums.go`：定义枚举与 `IsValidSplitType`
- `internal/model/expense.go`：`Expense.SplitType` 字段
- `internal/dto/expense_dto.go`：`oneof=equal ratio amount` 校验
- `internal/service/expense_service.go`：`calcShares` 调用 splitcalc
- `internal/service/expense_service.go`：日志 `LogExpenseCreated/LogExpenseUpdated`
- `pkg/splitcalc/split.go`：三种分摊算法
- `internal/util/formatters.go`：`SplitTypeText`
- `internal/constants/error_codes.go`：`CodeExpenseInvalidSplit` / `CodeExpenseShareMismatch`

前端出现位置：
- `src/constants/index.ts`：`SplitType` 与 `SplitTypeOptions`
- `src/components/SplitTypeTag.vue`、`StatusBadge.vue`
- `src/components/ExpenseFormDialog.vue`：分摊方式单选与参与人输入联动
- `src/pages/expense/ExpenseList.vue`：分摊方式标签
- `src/utils/format.ts`：`splitTypeText`

### 4. 结算状态 SettlementStatus（pending 待付款 / transferred 待收款确认 / settled 已确认收款 / voided 已作废）

状态机：`pending`（付款方点「我已转账」）→ `transferred`（收款方点「确认收款」）→ `settled`；账单新增/修改/退款时，`pending`、`transferred` 统一变为 `voided` 并按新账重算，`settled` 永久保留。

后端出现位置：
- `internal/constants/enums.go`：定义四个枚举值、`IsValidSettlementStatus`、审计动作 `ActionSettlementTransfer/Confirm/VoidRebuild`
- `internal/model/settlement.go`：`Settlement.Status/TransferredAt/SettledAt` 字段、`IsPending/IsTransferred/IsSettled/IsVoid/AwaitingAction`
- `internal/dto/settlement_dto.go`：`SettlementResp.Status/AwaitingUserID/TransferredAt`、`SettlementActionReq`
- `internal/service/settlement_service.go`：`MarkTransferred`（pending→transferred，限付款方）、`ConfirmReceived`（transferred→settled，限收款方）、`rebuildInTx`（作废重算）、`Balances`（扣除已确认收款）
- `internal/service/expense_service.go`：Create/Update/Delete 事务内调用 `RebuildOnExpenseChange`
- `internal/repository/settlement_repository.go`：`MarkTransferred/MarkSettled/VoidUnsettledByGroup/SumSettledByGroupOn/ListPendingByUser/FindByIDForUpdate` 状态筛选与行锁
- `internal/util/formatters.go`：`SettlementStatusText`（待付款/待收款确认/已确认收款/已作废）
- `internal/constants/log_templates.go`：`LogSettlementTransferred/LogSettlementConfirmed/LogSettlementVoidRebuild`
- `internal/constants/error_codes.go`：`CodeSettlementWrongParty`（禁止代确认）/ `CodeSettlementWrongState`
- `internal/handler/settlement_handler.go`、`internal/router/settlement.go`：`POST /settlements/transfer`、`POST /settlements/confirm`

前端出现位置：
- `src/constants/index.ts`：`SettlementStatus` 四值与 `SettlementStatusOptions`
- `src/components/StatusBadge.vue`：结算状态徽标颜色（warning/primary/success/info）
- `src/pages/settlement/SettlementList.vue`：状态、待谁操作、操作时间展示；「我已转账」仅付款方可见、「确认收款」仅收款方可见
- `src/pages/Dashboard.vue`、`src/layouts/MainLayout.vue`：待我转账 / 待我确认提醒与角标
- `src/api/settlement.ts`、`src/stores/settlement.ts`：`markTransferred` / `confirmReceived`
- `src/pages/audit/AuditLogs.vue`：结算审计动作筛选项
- `src/utils/format.ts`：`settlementStatusText`

### 5. 群组状态 GroupStatus（active / archived）

后端出现位置：
- `internal/constants/enums.go`：定义枚举与 `IsValidGroupStatus`
- `internal/model/group.go`：`Group.Status`、`IsActive`
- `internal/service/group_service.go`：`Archive` 状态流转、归档后禁止操作
- `internal/service/expense_service.go` / `settlement_service.go`：归档群组禁止记账/结算
- `internal/repository/group_repository.go`：成员数统计按 `active` 过滤
- `internal/util/formatters.go`：`GroupStatusText`

前端出现位置：
- `src/constants/index.ts`：`GroupStatus` / `GroupStatusOptions`
- `src/components/StatusBadge.vue`：群组状态徽标
- `src/pages/group/GroupDetail.vue`：状态展示与归档操作
- `src/utils/format.ts`：`groupStatusText`

## 测试

```bash
cd backend
go test ./...
```

覆盖：分摊算法（`pkg/splitcalc/split_test.go`，表驱动）、用户仓储 CRUD、群组仓储、消费仓储（表驱动）、用户服务注册/登录（表驱动）、消费服务分摊/退款、结算双方确认流程（付款方转账/收款方确认/禁止代确认/净余额更新）与账单变更后未确认转账作废重算、已确认记录保留。

## License

MIT License

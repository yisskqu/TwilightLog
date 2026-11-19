# TwilightLog

TwilightLog 是一个以文章与摄影作品为核心的个人图文博客，包含公开浏览站、编辑工作区和审核管理后台。

## 当前状态

- 阶段 0：本机开发环境已就绪；OSS、函数计算、DNS、服务器等云资源待补齐。
- 阶段 1：本机 monorepo 骨架已可运行，包含 Go API、PostgreSQL、Caddy、前端占位应用和 CI。
- 开发顺序、任务拆分和验收标准见 [开发文档](docs/开发文档.md)。
- 产品规则以 [PRD](docs/article-photography-prd.md) 为准。

## 技术栈

- 公开站：Astro + React islands + Tailwind CSS
- 管理后台：Vite + React + TypeScript + Tailwind CSS
- API：Go + chi + pgx + goose
- 数据库：PostgreSQL 16
- 媒体存储：本地存储适配器开发，生产接入阿里云 OSS + 函数计算
- 部署：Docker Compose + Caddy

## 目录结构

```text
.
├─ apps/
│  ├─ web/                       # Astro 公开站
│  └─ admin/                     # Vite + React 管理后台
├─ services/
│  └─ api/                       # Go API
│     ├─ cmd/api/
│     ├─ cmd/migrate/
│     ├─ internal/platform/
│     ├─ internal/migrations/
│     └─ api/openapi.yaml
├─ packages/
│  ├─ api-client/                # OpenAPI 生成的 TS 客户端
│  └─ design-tokens/             # 共享设计 token
├─ infra/
│  ├─ caddy/                     # Caddyfile 与占位静态页
│  └─ compose/
│     └─ docker-compose.dev.yml  # caddy + api + postgres
├─ docs/                         # 产品、技术、开发和设计文档
├─ testdata/                     # 测试图片与生成脚本
├─ var/uploads/                  # 本地存储适配器目录（不提交）
├─ .env.example
├─ Makefile
└─ README.md
```

## 本地开发

### 前置条件

- Go 1.27（本机已安装于 `E:\Software\Go\go`）
- Node.js 24
- pnpm 11
- Docker Desktop
- Playwright 1.65.1（浏览器已安装）

### 启动全部服务

```powershell
Copy-Item .env.example .env
docker compose -f infra/compose/docker-compose.dev.yml --env-file .env up -d --build
docker compose -f infra/compose/docker-compose.dev.yml --env-file .env ps
```

本机入口：

| 服务 | 地址 |
|---|---|
| Caddy 占位页 | http://localhost:8080 |
| API 健康检查 | http://localhost:8080/healthz |
| API 就绪检查 | http://localhost:8080/readyz |
| API 能力接口 | http://localhost:8080/api/v1/capabilities |
| PostgreSQL | `localhost:5432` |

停止服务：

```powershell
docker compose -f infra/compose/docker-compose.dev.yml --env-file .env down
```

### 迁移

API 启动时默认自动执行迁移。也可以手动执行：

```powershell
docker exec twilightlog-api /app/migrate status
docker exec twilightlog-api /app/migrate up
docker exec twilightlog-api /app/migrate down
```

### 前端开发

```powershell
pnpm install
pnpm dev:web     # http://localhost:4321，/api 代理到 8080
pnpm dev:admin   # http://localhost:5173，/api 代理到 8080
pnpm -r typecheck
pnpm -r build
```

### Go 检查

```powershell
cd services/api
go test ./...
go vet ./...
staticcheck ./...
```

### 环境变量

`.env` 只用于本地开发，已被 Git 忽略。不要提交 AK、SK、JWT 私钥或真实密码。

如果本机无法访问 Docker Hub，可以在 `.env` 中使用国内镜像变量：

```text
GO_IMAGE=docker.m.daocloud.io/library/golang:1.27-alpine
RUNTIME_IMAGE=docker.m.daocloud.io/library/alpine:3.22
CADDY_IMAGE=docker.m.daocloud.io/library/caddy:2-alpine
POSTGRES_IMAGE=postgres:16-alpine
```

### Playwright

```powershell
npx playwright screenshot "data:text/html,<h1>OK</h1>" .screenshots/check.png
```

## 文档

- [产品需求文档](docs/article-photography-prd.md)
- [技术选型](docs/article-photography-tech-stack.md)
- [完整网站开发方案](docs/完整网站开发方案.md)
- [开发文档](docs/开发文档.md)
- [设计原型](docs/prototypes/prototype.html)
- [首页归档原型](docs/prototypes/index-v4-rain-archive.html)

## CI

`.github/workflows/ci.yml` 包含三个任务：

- `api`：gofmt、go vet、go test、staticcheck
- `frontend`：pnpm install、typecheck、build
- `docker`：构建 API 镜像

`main` 已开启分支保护，需要功能分支 + PR；当前不要求额外审批。

## Git 约定

- `main` 为稳定分支，功能开发使用功能分支。
- 分支命名：`feat/...`、`fix/...`、`docs/...`、`chore/...`。
- 提交信息使用 Conventional Commits，例如 `feat: 增加文章列表接口`。
- 提交前确认没有 `.env`、密钥、上传文件或大体积压缩包。
- PR 使用 `.github/pull_request_template.md` 中的检查项。

## 云资源待办

以下资源不阻塞本机开发，但需在对应阶段前补齐：

- 阿里云 OSS 私有/公开 Bucket、CORS、生命周期
- 函数计算 FC 图片处理函数
- `www`、`api`、`admin`、`cdn` DNS 解析
- 服务器 SSH、防火墙、swap、Docker 部署

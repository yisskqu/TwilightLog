# TwilightLog

TwilightLog 是一个以文章与摄影作品为核心的个人图文博客，包含公开浏览站、编辑工作区和审核管理后台。

## 当前状态

- 阶段 0：本机开发环境已就绪；OSS、函数计算、DNS、服务器等云资源待补齐。
- 开发顺序、任务拆分和验收标准见 [开发文档](docs/开发文档.md)。
- 产品规则以 [PRD](docs/article-photography-prd.md) 为准。

## 技术栈

- 公开站：Astro + React islands + Tailwind CSS
- 管理后台：Vite + React + TypeScript + Tailwind CSS
- API：Go + PostgreSQL
- 数据库：PostgreSQL 16
- 媒体存储：本地存储适配器开发，生产接入阿里云 OSS + 函数计算
- 部署：Docker Compose + Caddy

## 目录结构

```text
.
├─ docs/                         # 产品、技术、开发和设计文档
│  └─ prototypes/                # 设计原型 HTML
├─ infra/
│  └─ compose/
│     └─ docker-compose.dev.yml  # 本地 PostgreSQL
├─ var/
│  └─ uploads/                   # 本地存储适配器目录（不提交）
├─ .env.example                  # 环境变量模板
├─ .gitignore
└─ README.md
```

## 本地开发

### 前置条件

- Go 1.27（本机已安装于 `E:\Software\Go`）
- Node.js 24
- pnpm 11
- Docker Desktop
- Playwright 1.65.1（浏览器已安装）

### 启动数据库

```powershell
Copy-Item .env.example .env
docker compose -f infra/compose/docker-compose.dev.yml --env-file .env up -d
docker compose -f infra/compose/docker-compose.dev.yml --env-file .env ps
```

进入数据库：

```powershell
docker exec -it twilightlog-postgres psql -U twilightlog -d twilightlog
```

停止数据库：

```powershell
docker compose -f infra/compose/docker-compose.dev.yml --env-file .env down
```

### 验证本地工具

```powershell
go version
node -v
pnpm -v
docker compose version
npx playwright --version
```

### 环境变量

`.env` 只用于本地开发，已被 Git 忽略。不要提交 AK、SK、JWT 私钥或真实密码。

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

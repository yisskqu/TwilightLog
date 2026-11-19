# TwilightLog

TwilightLog 是一个以文章与摄影作品为核心的个人图文博客，包含公开浏览站、编辑工作区和审核管理后台。

## 当前状态

- 阶段 0：本机开发环境已就绪；OSS、函数计算、DNS、服务器等云资源待补齐。
- 阶段 1：本机 monorepo 骨架已可运行，包含 Go API、PostgreSQL、Caddy、前端占位应用和 CI。
- 阶段 2：账号、角色、权限、会话与审计已接通；后台提供登录、路由守卫、账号管理和审计页面。
- 阶段 3：媒体上传与图片管线已接通；支持预签名直传、分片上传、格式校验、EXIF 白名单、方向纠正、BlurHash、WebP 衍生图与水印，原图保持私有。
- 阶段 4：内容模型与公开只读 API 已接通；包含五类内容的多态版本、标签作用域隔离、筛选/排序/分页、搜索、ETag 与可重复导入的种子数据。
- 阶段 5：公开站静态 MVP 已可用；首页推荐与补位、文章目录/代码复制/公式/图示、摄影瀑布流与照片查看器、搜索筛选、深浅色与响应式、SEO 与 sitemap 均已接通。
- 阶段 6：管理后台与编辑工作区已接通；包含五类内容的工作版本、提交/撤回、审批对照与标签移除、首页推荐 N₁/N₂、回收站恢复/永久删除、站内通知、标签词库、只读系统信息与设备分级。首页推荐改由后台配置，未配置时按最新公开内容自动补位。
- 阶段 7：联调与验收完成；Playwright 覆盖登录、内容审批全链路、照片全链路、权限与设备分级、回收站、推荐、响应式/无障碍、SEO 与性能；Go 测试接入真实 PostgreSQL；备份恢复与可重复构建完成演练。详见 `docs/阶段7-测试报告.md`。
- 开发顺序、任务拆分和验收标准见 [开发文档](docs/开发文档.md)。
- 产品规则以 [PRD](docs/article-photography-prd.md) 为准。

## 技术栈

- 公开站：Astro 静态生成 + React islands + 原生 CSS 设计 token
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
│     ├─ cmd/admin/              # 仅用于在服务器上创建管理员
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
| 公开衍生图 | `http://localhost:8080/assets/...` |
| 管理后台（开发服务器） | http://localhost:5173 |
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

### 创建管理员

管理员账号只能通过 CLI 创建，后台不提供创建入口。先确保数据库已启动并完成迁移：

```powershell
cd services/api
go run ./cmd/admin create --username admin --email admin@twilightlog.com --password "至少8位密码" --role super_admin
```

密码也可以放在 `ADMIN_PASSWORD` 环境变量中，避免出现在命令历史里。普通账号和编辑账号由管理员在后台“账号管理”中创建。

### 种子内容

阶段 4 的公开接口需要数据才能浏览。种子命令会创建 3 个文章专栏、10 篇公开文章、3 个摄影专栏、6 个有照片的相册与 30 张照片，并生成真实可访问的 WebP 衍生图；重复执行不会产生重复数据。

```powershell
cd services/api
go run ./cmd/seed                 # 默认归属第一个管理员
go run ./cmd/seed --owner admin   # 指定归属账号
```

使用 Docker 运行时，API 容器内同样提供该命令：

```powershell
docker exec twilightlog-api /app/seed --owner admin
```

种子覆盖空相册、草稿、待审、下线、回收站等场景，公开接口只会返回已审批版本。

### 内容只读接口

公开接口不需要登录，统一返回分页 `{items,page,pageSize,total,totalPages}`，支持 `ETag` / `If-None-Match`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/home/recommendations` | 首页推荐文章与相册 |
| GET | `/api/v1/article-columns` | 文章专栏列表 |
| GET | `/api/v1/article-columns/{slug}/articles` | 专栏内文章列表 |
| GET | `/api/v1/articles/{slug}` | 文章详情（含 Markdown 源） |
| GET | `/api/v1/photo-columns` | 摄影专栏列表 |
| GET | `/api/v1/photo-columns/{slug}/albums` | 专栏内相册列表 |
| GET | `/api/v1/albums/{slug}` | 相册详情 |
| GET | `/api/v1/albums/{slug}/photos` | 相册照片列表 |
| GET | `/api/v1/photos` | 照片列表 |
| GET | `/api/v1/photos/{id}` | 照片详情与 EXIF 白名单 |
| GET | `/api/v1/search?q=` | 跨类型搜索 |
| GET | `/api/v1/tags?scope=article\|photo` | 公开标签词库 |

列表通用参数：`page`、`pageSize`（默认 12，最大 50）、`q`、`tags`（逗号分隔，逻辑 AND）、`dateFrom`、`dateTo`、`sort=latest|oldest`。文章与标签按作用域隔离；未公开、待审、下线、回收站内容以及空相册不会出现在任何公开响应中。

搜索使用 PostgreSQL `pg_trgm` 与去代码块的正文索引；当前标准 PostgreSQL 镜像不含 `pg_jieba`/`zhparser`，后续可在不改接口的前提下替换为中文分词。

### 公开站

公开站由 Astro 静态生成，开发期按请求读取 API，构建期生成纯静态 HTML。

| 页面 | 路径 |
|---|---|
| 首页（推荐与入口） | `/` |
| 文章专栏列表 / 专栏文章 | `/articles`、`/articles/{column}` |
| 文章详情 | `/articles/{column}/{slug}` |
| 摄影专栏列表 / 专栏相册 | `/photos`、`/photos/{column}` |
| 相册详情（瀑布流与查看器） | `/photos/{column}/{album}` |
| 全部照片 | `/photos/all` |
| 搜索与筛选 | `/search` |

```powershell
pnpm --filter @twilightlog/web dev     # http://127.0.0.1:4321
pnpm --filter @twilightlog/web build   # 输出 apps/web/dist
```

构建默认从 `http://localhost:8080` 拉取数据，可用 `API_BASE_URL` 指向其他 API，用 `SITE_URL` 指定 canonical 域名。开发服务器会缓存预渲染页面，修改种子或 API 数据后需重启 `pnpm dev:web` 才能看到新内容。

### 认证接口

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/auth/login` | 用户名、密码、记住登录 |
| POST | `/api/v1/auth/refresh` | 用 HttpOnly Cookie 刷新 access token |
| POST | `/api/v1/auth/logout` | 只撤销当前设备会话 |
| GET | `/api/v1/auth/me` | 当前用户与权限 |
| GET | `/api/v1/admin/*` | 管理员接口，编辑访问返回 403 |
| GET | `/api/v1/editor/ping` | 编辑权限探针 |

### 编辑工作区与审批接口

编辑只能管理本人内容；管理员传 `all=1` 可查看全站。提交后公开站继续显示已通过版本，只有审批通过才替换。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/workspace/summary` | 五类内容的状态统计 |
| GET | `/api/v1/workspace/content?type=` | 我的内容列表，支持状态、搜索、分页 |
| POST | `/api/v1/workspace/content` | 新建内容并生成草稿版本 |
| GET/PUT | `/api/v1/workspace/content/{type}/{id}` | 读取或保存工作版本 |
| POST | `/api/v1/workspace/content/{type}/{id}/submit` | 提交审核 |
| POST | `/api/v1/workspace/content/{type}/{id}/withdraw` | 撤回待审版本 |
| POST | `/api/v1/workspace/content/{type}/{id}/trash` | 移入回收站，非空上级会阻止 |
| POST | `/api/v1/workspace/content/{type}/{id}/restore` | 从回收站恢复 |
| GET | `/api/v1/workspace/recycle?type=` | 回收站列表 |
| GET | `/api/v1/admin/reviews` | 待审列表 |
| GET | `/api/v1/admin/reviews/{id}` | 待审详情、公开版对照与标签 |
| POST | `/api/v1/admin/reviews/{id}/approve` | 通过并替换公开版本，可移除本轮标签 |
| POST | `/api/v1/admin/reviews/{id}/reject` | 驳回，理由必填并通知编辑 |
| DELETE | `/api/v1/admin/content/{type}/{id}` | 永久删除回收站内容并清理在线文件 |
| GET/PUT | `/api/v1/admin/recommendations` | N₁/N₂、手工推荐与全自动补位 |
| GET | `/api/v1/admin/recommendations/candidates?type=` | 可选公开文章或相册 |
| GET | `/api/v1/admin/tags?scope=` | 标签词库与使用次数 |
| PATCH | `/api/v1/admin/tags/{id}` | 重命名或停用标签 |
| POST | `/api/v1/admin/tags/{id}/merge` | 合并同作用域标签 |
| GET | `/api/v1/admin/system` | 只读系统信息 |
| GET | `/api/v1/notifications` | 站内通知与未读数 |
| POST | `/api/v1/notifications/{id}/read` | 标记单条已读 |
| POST | `/api/v1/notifications/read-all` | 全部已读 |

高风险操作要求请求头 `X-Client-Device: desktop`；后台会按视口自动发送。手机端可编辑、提交、撤回、恢复回收站和首次发布审批通过，但替换公开版本、永久删除、账号权限与标签维护会被 API 拒绝。

### 媒体接口

除上传地址外，媒体接口均要求编辑或管理员登录。上传地址本身带短时签名，浏览器可直接 PUT。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/media/presign` | 申请上传；返回单文件或分片地址，写操作支持 `Idempotency-Key` |
| PUT | `/api/v1/media/upload/{id}` | 单文件直传 |
| PUT | `/api/v1/media/upload/{id}/parts/{part}` | 分片直传 |
| POST | `/api/v1/media/complete` | 完成上传并开始处理；重复调用不会重复创建 |
| GET | `/api/v1/media/{id}` | 查询上传/处理状态、宽高、BlurHash、EXIF 白名单与衍生图 |
| POST | `/api/v1/media/{id}/retry` | 重试失败的处理 |
| GET | `/api/v1/media/{id}/original` | 获取原图短时签名地址 |
| GET | `/assets/{key}` | 公开 WebP 衍生图，仅限 `public/` 目录 |

规则：

- 文章图片接受 JPG/PNG/WebP，单文件小于 5 MB；摄影照片接受 JPG/PNG/HEIC，单文件小于 50 MB。
- 同一 `batchId` 最多 20 张，第 21 张直接拒绝。
- 格式以文件头为准，扩展名与真实格式不一致会被处理管线拒绝。
- 原图只存 `private/`，匿名访问返回 404；公开端只访问 `public/` 下的 WebP。
- 本地适配器用纯 Go 依赖完成 HEIC 解码、WebP 编码与水印；生产环境必须使用云函数处理，配置为 `MEDIA_PROCESSOR=local` 时生产启动会失败。
- 本地衍生图为预生成的固定 key（`public/media/<id>/w960.webp`），迁移 OSS/CDN 只需替换存储适配器与 `ASSET_BASE_URL`。
- 前端统一使用 `assetUrl(key, variant)` / `assetSrcSet(key)`，禁止写死图片域名。

### 前端开发

```powershell
pnpm install
pnpm dev:web     # http://localhost:4321，/api 代理到 8080
pnpm dev:admin   # http://localhost:5173，/api 代理到 8080
pnpm -r typecheck
pnpm -r build
```

后台页面：

| 角色 | 路径 | 说明 |
|---|---|---|
| 管理员 | `/admin` | 概览与模块入口 |
| 管理员 | `/admin/workspace/{type}` | 全站内容工作区 |
| 管理员 | `/admin/reviews` | 审批中心与公开版差异 |
| 管理员 | `/admin/recommendations` | N₁/N₂、排序与全自动补位 |
| 管理员 | `/admin/recycle` | 全站回收站、永久删除 |
| 管理员 | `/admin/tags` | 标签词库维护 |
| 管理员 | `/admin/system` | 只读系统信息 |
| 编辑 | `/editor/workspace/{type}` | 本人内容工作区与编辑器 |
| 编辑 | `/editor/recycle` | 本人回收站 |
| 编辑/管理员 | `/notifications` 对应后台通知页 | 未读数、已读与跳转 |

### Go 检查

```powershell
cd services/api
go test ./...
go vet ./...
staticcheck ./...
```

内容筛选、分页、标签 AND、搜索与 ETag 的集成测试会读取 `TEST_DATABASE_URL`，未设置时自动跳过。建议使用独立数据库：

```powershell
$env:TEST_DATABASE_URL="postgres://twilightlog:twilightlog_dev@localhost:5432/twilightlog_test?sslmode=disable"
go test ./...
```

### 环境变量

`.env` 只用于本地开发，已被 Git 忽略。不要提交 AK、SK、JWT 密钥或真实密码。

认证相关变量：

```text
JWT_SECRET=至少32位随机字符串
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=720h
REFRESH_TOKEN_TTL_SHORT=24h
COOKIE_SECURE=false          # 生产 HTTPS 必须改为 true
COOKIE_DOMAIN=
CORS_ORIGINS=http://localhost:4321,http://localhost:5173
```

媒体相关变量：

```text
STORAGE_DRIVER=local
LOCAL_STORAGE_ROOT=var/uploads
ASSET_BASE_URL=http://localhost:8080/assets
API_BASE_URL=http://localhost:8080
MEDIA_SIGNING_SECRET=          # 留空时回退到 JWT_SECRET
MEDIA_PROCESSOR=local          # 仅本地开发；生产必须用云函数
MEDIA_PROCESS_ASYNC=true
MEDIA_BATCH_LIMIT=20
MEDIA_MAX_ARTICLE_BYTES=5242880
MEDIA_MAX_PHOTO_BYTES=52428800
```

本地文件结构：

```text
var/uploads/
  private/      # 原图，只允许签名 URL 读取
  public/       # WebP 衍生图，通过 /assets 公开
  tmp/          # 分片上传临时文件，超过 24 小时自动清理
```

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

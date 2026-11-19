# 文章与摄影作品网站 · 基础技术选型方案

- 版本：V1.2
- 日期：2026-11-19
- 状态：基础技术选型已确认，进入实施准备
- V1.1 变化：修复文档审计口径——数据模型拆分为文章/摄影实体表并补版本化映射、评论表移入后期、账号补邮箱与状态字段、通知移除定时发布、修正 CDN 域名与 `assetUrl` 变体、水印参数转为已定稿
- V1.2 变化：文章专栏/摄影专栏加入版本化；展示变体明确为 OSS 图片处理参数、迁移腾讯云前预生成固定 key；补只读 `system_info`；统一变体尺寸与水印口径
- 关联文档：`article-photography-prd.md`（以 PRD 当前版本为准）

---

## 0. 文档目的

本文只确定“用什么技术、为什么、边界在哪”，不展开具体页面实现和接口字段。

当前已确认的约束：

1. 前后端分离，API 优先，未来可能接入微信小程序、Android App。
2. 自有云服务器：当前 2 核 2G、40G ESSD，带宽以实际套餐为准。
3. 开发者本人非 Web 行业，主要借助 AI 完成开发。
4. 长期维护，功能会逐步增加，模块要能独立上下线。
5. 当前不购买 CDN，但代码必须预留 CDN 接入能力。
6. 图片是主要流量来源，不能由应用服务器直接承载。
7. 域名已存在并完成备案：`twilightlog.com`、`www.twilightlog.com`。

### 0.1 当前定稿项

- 公开站采用纯静态生成，不做每请求 SSR。
- 管理后台采用 Ant Design，不使用 shadcn/ui 作为后台主组件库。
- 云厂商先用阿里云，后期可迁移腾讯云；存储、CDN、域名做解耦。
- 图片展示变体用 OSS 图片处理；上传后由云函数做校验、EXIF 提取与清洗。
- 搜索采用 PostgreSQL 中文分词，优先 `pg_jieba`，编译不顺利则用 `zhparser`。
- 公开站 canonical 使用 `https://www.twilightlog.com`，主域 301 跳转。
- 首版不做邮件，只预留通知接口。
- 发布延迟目标：正常 5 分钟内上线，最差不超过 10 分钟。

---

## 1. 结论摘要

推荐基线：

```text
后端：Go + PostgreSQL
公开站：Astro 静态生成
管理后台：React + Vite + Ant Design
图片：OSS/COS 对象存储，CDN 后期接入
部署：Caddy + Docker Compose
```

核心判断：

- 选 Go 是为了长期 CPU/内存效率和单二进制部署，不是为了追求极限吞吐。
- 公开站静态化，避免每请求 SSR；API 只返回 JSON。
- 图片全部外置到对象存储，服务器不存图、不转发图、不处理大图。
- CDN 通过统一的 `AssetResolver` 和 `ASSET_BASE_URL` 预留，后期只改配置和 DNS。
- 云厂商先用阿里云，资源域名统一用 `cdn.twilightlog.com`，迁腾讯云只换 CNAME。
- 先做模块化单体，不做微服务；功能“插拔”用模块注册 + 运行时开关实现。

明确不做：

- 不上微服务、K8s。
- 不在 2C2G 服务器上跑前端构建。
- 不在应用服务器上做图片压缩和衍生图。
- 不把图片、上传文件存服务器本地磁盘。
- 不在首版上 Redis、MinIO、Elasticsearch。
- 不公开原图，不提供图片下载；公开端只给压缩衍生图。
- 首版不做评论、分享功能，也不开 Referer 防盗链。
- 不为了多端共享 UI，只共享 API 契约和类型。

---

## 2. 总体架构

```text
                    ┌──────────────────────────┐
                    │        CDN（后期接入）      │
                    └────────────┬─────────────┘
                                 │
                    ┌────────────▼─────────────┐
                    │   OSS/COS 对象存储        │
                    │   原图私有 / 衍生图公开     │
                    └────────────▲─────────────┘
                                 │ 直传 / 读取
┌──────────────┐    HTTPS   ┌────┴─────────────┐
│ 浏览器        │──────────▶│      Caddy       │
└──────────────┘            └────┬─────────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
       ┌──────▼──────┐   ┌───────▼───────┐  ┌───────▼───────┐
       │ 公开站静态文件 │   │ 管理后台静态文件 │  │   Go API      │
       │ Astro build  │   │ React build   │  │  /api/v1      │
       └──────────────┘   └───────────────┘  └───────┬───────┘
                                                     │
                                            ┌────────▼────────┐
                                            │  PostgreSQL     │
                                            │  内容/权限/审批   │
                                            └─────────────────┘
```

小程序 / App 不加载公开站静态文件，只调用 `api` 域名下的 JSON 接口；后续接入时在 Caddy 增加独立 `api` 站点，不复用公开站的静态路径。

运行时只保留：

- Caddy：HTTPS、静态文件、反向代理。
- Go API：业务接口。
- PostgreSQL：唯一主数据库。
- 可选 worker：与 API 同一代码库的第二个入口，首版可不部署。

前端构建产物是纯静态文件，由 Caddy 直接返回；不需要 Node 常驻进程。

---

## 3. 技术栈清单

| 层 | 选择 | 说明 |
|---|---|---|
| 后端语言 | Go 1.23+ | 单二进制、内存低、部署简单 |
| HTTP 路由 | chi 或 echo | 轻量，贴近标准库 |
| 数据库 | PostgreSQL 16 | JSONB、事务、全文/模糊搜索 |
| 数据访问 | pgx + sqlc | SQL 可控，生成类型安全代码 |
| 迁移 | goose | 只允许 migration 改表 |
| 参数校验 | go-playground/validator 或自定义 | 边界校验 |
| API 契约 | OpenAPI 3.1 + oapi-codegen | 前后端、多端共享契约 |
| 认证 | JWT access + refresh，argon2id | 不自己发明密码学 |
| 队列 | River 或 PostgreSQL `SKIP LOCKED` | 不引入 Redis |
| 日志 | log/slog | 结构化日志 |
| 公开站 | Astro + React islands + Tailwind | 静态生成，JS 少，SEO 好 |
| 管理后台 | React + Vite + Ant Design | 表格/表单/弹窗现成 |
| 后台数据请求 | TanStack Query | 缓存、重试、加载态 |
| 后台表单 | React Hook Form + Zod | 前端校验 |
| 内容渲染 | goldmark + Chroma + bluemonday | Markdown → 安全 HTML 快照 |
| 公式/图表 | KaTeX + Mermaid（客户端增强） | 服务端只出占位和源码 |
| 图片存储 | 阿里云 OSS / 腾讯云 COS | 原图私有，衍生图公开 |
| 图片处理 | OSS 图片处理 + 云函数 | 展示变体走 OSS；上传后清洗走云函数 |
| 中文搜索 | PostgreSQL + pg_jieba/zhparser + pg_trgm | 中文分词 + 模糊/代码词兜底 |
| 邮件通知 | 首版不做，预留 `Notifier` | 后期接阿里云邮件推送或腾讯云 SES |
| 访问统计 | GoAccess（当前）/ Umami（4G 后） | 先用日志，升级后接轻量分析 |
| CDN | 后期接入 | 通过统一 URL 解析层切换 |
| 部署 | Docker Compose + Caddy | 三个左右容器 |
| CI | GitHub Actions / Gitee Go | 构建不在生产服务器执行 |
| 监控 | Sentry + 外部 uptime + 日志轮转 | 轻量优先 |

---

## 4. 前端选型

### 4.1 公开站为什么用 Astro

- 文章和相册以内容为主，静态生成最合适。
- 默认输出 HTML，JS 很少，首屏和 SEO 好。
- 服务器不需要 Node SSR 进程，2C2G 压力小。
- 图片、代码块、公式、Mermaid 都可以按需增强。
- 需要交互的局部（筛选、搜索、图片查看器）用 React island。

构建与发布流程：

```text
后台发布内容
  → 触发 CI 构建（webhook）
  → Astro 从 API 拉取已发布内容
  → 生成静态文件
  → 上传到服务器静态目录或 OSS
  → 清理 CDN 缓存（接入后）
```

代价：发布到公开可见有构建延迟。目标定为**正常 5 分钟内、最差 10 分钟内**；后台显示构建状态，构建失败时旧版本继续服务。如果后期要求秒级可见，再评估“文章详情按需 SSR / 预渲染兜底”，不改变 API。

### 4.2 管理后台为什么独立

- 后台不需要 SEO，独立 SPA 更简单。
- 与公开站分开构建，避免后台代码进入公开包。
- 权限仍由 API 校验，前端隐藏只是体验。

Ant Design 与 shadcn/ui 的区别：

| 维度 | Ant Design | shadcn/ui |
|---|---|---|
| 定位 | 完整组件库 | 复制到项目里的组件代码 |
| 复杂后台 | 表格、表单、上传、日期、弹窗开箱即用 | 需要自己组合 Data Table、表单、上传 |
| 视觉 | 企业后台风格，Token 可调 | Tailwind 自由定制，更现代 |
| 代码量 | 少，配置为主 | 多，组件归项目维护 |
| 升级 | 跟库升级 | 代码归你，升级需手动合并 |
| 适合 | 管理后台、审批、CRUD | 高度定制的产品界面 |

结论：

- **管理后台用 Ant Design**。你的后台大量是表格、筛选、审批、上传、弹窗，Ant Design 能显著减少工作量。
- **公开站不用 Ant Design**，用 Astro + Tailwind 做自己的视觉，避免“后台风”。
- shadcn/ui 更适合愿意自己维护组件、追求独特视觉的团队；对当前阶段不划算。

### 4.3 备选方案

- 如果希望只维护一个前端框架，公开站可用 Next.js 静态导出，后台仍可用同一套 React。
- 如果团队更熟 Vue，公开站可用 Nuxt SSG，后台用 Vue + Ant Design Vue；架构不变。

首版建议 Astro + React，原因是内容站静态化最直接、运行时最轻。

---

## 5. 后端模块化单体

### 5.1 目录结构

```text
services/api/
  cmd/
    api/main.go
    worker/main.go          # 可选，首版可不部署
  internal/
    platform/               # 配置、日志、数据库、HTTP、鉴权
    modules/
      auth/
      articles/
      photography/
      media/
      taxonomy/
      review/
      recommendation/
      audit/
    migrations/
  api/openapi.yaml
```

### 5.2 模块契约

每个模块实现统一接口，启动时注册：

```go
type Module interface {
    ID() string
    Routes() []Route
    Permissions() []Permission
    Migrations() []Migration
    Menus() []MenuItem
    Events() []Event
    Enabled(ctx context.Context) bool
}
```

规则：

- 模块只访问自己的表。
- 跨模块通过 service 接口或事件，不直接写对方 SQL。
- `Enabled=false` 时不挂路由、不进菜单、不跑任务。
- 下线不删数据；真正卸载必须走 migration 并备份。
- 新增功能 = 新增模块目录，不改核心路由。

### 5.3 功能开关

- 配置来源：数据库 `feature_flags` + 环境变量覆盖。
- 对外提供 `GET /api/v1/capabilities`，返回启用的模块、菜单、权限。
- 前端导航和路由根据 capabilities 生成。
- API 中间件拒绝对已关闭模块的请求。

### 5.4 为什么不做微服务

- 2C2G 资源不够。
- 当前只有一个团队、一个数据库，拆分只会增加运维和一致性问题。
- 模块化单体已经能保证边界；将来真需要拆，再按模块拆服务。

---

## 6. API 约定

- 统一前缀：`/api/v1`。
- 公开接口与后台接口分域：`/api/v1`、`/api/v1/admin`。
- 分页：`page` + `pageSize`，默认 12；排序必须带稳定次级排序字段。
- 错误格式统一：`code`、`message`、`details`、`requestId`。
- 写操作支持 `Idempotency-Key`，审批、发布、删除必须幂等。
- 列表和详情支持 `ETag` / `If-None-Match`。
- OpenAPI 为唯一契约来源，前端类型由契约生成。
- 版本升级不破坏旧客户端；废弃字段先标记再移除。

认证与权限：

- access token 短期有效，refresh token 轮换。
- Web 端 refresh 放 HttpOnly/Secure/SameSite Cookie；小程序/App 放安全存储。
- 角色：访客匿名，编辑/管理员登录；不开放公开注册。
- 对象级权限必须在 API 校验：归属、公开状态、审批状态、是否被禁用。
- 高风险操作要求二次验证；不能只靠 User-Agent 判断 PC。

### 6.1 登录范围与站内通知

首版登录只用于分权：

- 只有编辑、管理员账号需要登录；访客匿名浏览。
- 不开放公开注册，不做收藏、关注、个人中心；账号由管理员创建。
- 后期若做评论，再单独评估访客账号和匿名评论策略，不与当前登录混在一起。

站内通知范围：

| 类型 | 触发 | 接收人 |
|---|---|---|
| 审批 | 编辑提交待审 | 管理员 |
| 审批结果 | 通过、驳回（必须带理由） | 提交编辑 |
| 发布 | 发布成功、构建失败 | 编辑、管理员 |
| 内容状态 | 下架、删除、恢复、推荐位变更 | 内容归属编辑 |
| 账号 | 角色变更、禁用/启用、密码重置 | 本人 |
| 系统 | 备份失败、磁盘不足、证书到期、任务积压 | 管理员 |
| 评论（后期） | 新评论、审核结果、回复 | 相关内容作者/管理员 |

实现约定：

- 表：`notifications(id, recipient_id, type, severity, title, body, link, read_at, dedupe_key, created_at)`。
- 支持未读数量、全部已读、按类型筛选、点击跳转。
- 由领域事件异步写入，使用 outbox，避免业务事务里直接发通知。
- 同一事件用 `dedupe_key` 去重；不给自己发通知；正文不放敏感信息。
- 保留 90 天，之后归档或清理。
- 首版只做站内通知；邮件后期复用同一批通知记录，通过 `Notifier` 发送。

邮件后期再加时的难度：

- **事务邮件**（审批结果、密码重置、异常告警）：中等难度。接阿里云邮件推送或腾讯云 SES，验证域名 SPF/DKIM/DMARC，做模板、队列和重试。
- **批量/营销邮件**：更麻烦，涉及退订、频率、合规和送达率，不建议早期做。

不要自己搭 SMTP 服务器。

---

## 7. 数据与内容模型

核心表：

```text
users                 # username、email(唯一)、password_hash、role_id、status = active | disabled、created_at
roles
permissions
article_columns
articles
photo_columns
albums
photos
content_versions      # 多态：(entity_type, entity_id)
tags                  # scope = article | photo
content_tags
media_assets
reviews
recommendations
audit_logs
feature_flags
notifications
system_info           # 只读系统信息：版本、构建状态、存储用量、任务状态
jobs                  # 使用 River 时可省略
```

后期表：

```text
comments              # 后期再做，首版不建路由/菜单/表
guest_identities      # 后期再做
```

**PRD 对象 → 数据表 → 版本化方式：**

| PRD 对象 | 数据表 | 版本化方式 |
|---|---|---|
| 文章专栏 | `article_columns` | `content_versions`（`entity_type='article_column'`）保存名称、简介、封面等快照；`article_columns.published_version_id` 指向公开版本 |
| 文章 | `articles` | `content_versions`（`entity_type='article'`）；`articles.published_version_id` 指向公开版本，工作版本以最新待审或驳回记录为准 |
| 摄影专栏 | `photo_columns` | `content_versions`（`entity_type='photo_column'`）保存名称、简介、封面、题材等快照；`photo_columns.published_version_id` 指向公开版本 |
| 相册 | `albums` | `content_versions`（`entity_type='album'`）；`albums.published_version_id` 指向公开版本 |
| 照片 | `photos` + `media_assets` | `content_versions`（`entity_type='photo'`）保存标题、描述、标签、位置等文字版本；图像文件由 `media_assets` 记录，替换图像即新增一条资产并由版本快照引用 |
| 首页推荐 | `recommendations` | 不版本化，保存即生效 |
| 审核记录 | `reviews` | 不版本化，通过 `entity_type + entity_id + version_id` 指向具体提交 |
| 标签词库 | `tags` / `content_tags` | 标签词库本身不版本化，重命名/合并/停用立即影响已公开内容和筛选；内容与标签的关联随 `content_versions` 快照保存，审批时移除标签只作用于提交版本 |

关键设计：

- `article_columns.published_version_id`、`photo_columns.published_version_id`、`articles.published_version_id`、`albums.published_version_id` 和 `photos.published_version_id` 分别指向已通过审批的版本。
- `content_versions` 保存实体类型的快照字段、提交人、审核状态和渲染结果；文章额外保存 Markdown 源、AST 和 HTML 快照。
- 文章专栏、摄影专栏、文章、相册、照片都遵循同一版本规则：编辑提交后公开版保持不变，只有管理员审批通过后才替换公开版本。
- 审批针对具体版本；预览和发布使用同一份渲染快照。
- 标签用 `scope` 隔离文章与摄影，唯一约束 `(scope, name)`。
- 照片位置、拍摄时间等需要筛选的字段单独建列；其余元数据放 JSONB。
- 主键用 UUIDv7 或 ULID，分页稳定。
- 已确认搜索采用 PostgreSQL 中文分词：`pg_jieba` 或 `zhparser`，配合 `pg_trgm` 做模糊和代码词兜底。
- 搜索字段加权：标题 A、摘要/标签 B、正文 C；正文索引前去掉 Markdown 标记和代码块。
- 中文分词需要自定义词典，补充人名、地名、摄影术语和技术名词。
- 先用 GIN 索引 + `ts_rank` 排序；效果不够再评估 Meilisearch，不首版引入。
- 删除走软删除 + 回收站，异步清理对象存储。
- `system_info` 由部署脚本或 CI 写入版本、构建状态、存储用量和任务状态，后台只读展示。

内容渲染链路：

```text
Markdown 源
  → goldmark 解析
  → Chroma 代码高亮
  → bluemonday 白名单清洗
  → 渲染 HTML 快照入库
  → 前端插入 HTML
  → 客户端增强 KaTeX / Mermaid
```

### 7.1 EXIF 白名单与缩略图

只保留以下字段，其余全部剥离：

| 字段 | 用途 |
|---|---|
| Make / Model | 设备机型 |
| LensModel | 镜头 |
| FocalLength / FocalLengthIn35mmFilm | 焦距、等效焦距 |
| FNumber | 光圈 |
| ExposureTime | 快门速度 |
| ISO | 感光度 |
| DateTimeOriginal + OffsetTimeOriginal | 原始拍摄时间与时区 |
| ExposureBiasValue | 曝光补偿 |
| WhiteBalance | 白平衡 |
| Orientation | 仅用于纠正方向，不公开展示 |
| PixelXDimension / PixelYDimension | 原图尺寸 |

不解析、不保留 GPS、测光模式、闪光灯等其余字段。照片位置按 PRD 的独立位置字段由上传者或管理员手动填写，不读取、不存储、不暴露 EXIF GPS。

EXIF 只在**上传时读取一次**：云函数或处理任务解析原图，提取白名单字段写入数据库；之后详情页展示的是数据库字段，不再从图片里读 EXIF。

存储约定：

- 展示用归一化值，例如 `f/2.8`、`1/250s`、`35mm`、`ISO 100`、`+0.3EV`、`自动白平衡`。
- 拍摄时间优先 `DateTimeOriginal`，没有再用 `DateTime`；保留时区偏移。
- 数据库存白名单字段，不存整块原始 EXIF；原图私有。
- 公开衍生图重新编码并剥离全部元数据，包括 MakerNotes、XMP、IPTC、ICC。

关于 EXIF 缩略图：

- **是真的。** EXIF 的 IFD1 可以内嵌一张 JPEG 缩略图。常见尺寸约 160×120 或 320×240，大小通常几 KB 到几十 KB。
- 现代手机不一定只用标准 EXIF 缩略图，可能把更大的预览图放在 MPF 或厂商私有段，大小从几百 KB 到数 MB 不等。
- **不建议直接拿来当相册预览图**：可能缺失、分辨率低、比例不对、方向错误、内容过时（原图编辑后未更新），质量也不适合瀑布流。
- 可以作为上传后的临时占位图或 BlurHash 的参考，但必须严格解析、重新编码、剥离元数据，不能直接把原始缩略图公开。
- 正式预览仍由原图生成 w480/w960/w1440 等衍生图；缩略图只当辅助，不作为主方案。

### 7.2 匿名评论与游客身份（后期，首版不做）

首版不实现评论，以下仅作为后期设计记录，暂不建路由、菜单和评论表。

后期方案：

- 访客首次评论时，服务端创建一个游客身份，显示为“游客 + 8 位随机数字”，例如“游客 48291375”。
- 游客号在数据库有唯一约束，冲突时重新生成；不要用自增 ID，避免被推测。
- 用签名的 HttpOnly Cookie 记住游客身份；清 Cookie 或换浏览器会变成新游客，这是可接受的。
- 同一游客可连续评论；是否允许修改/删除自己的评论，可用短时签名令牌实现，首版可不做。

审核与防滥用：

- 所有评论默认 `pending`，管理员审核后 `approved` 才公开；支持驳回、标记垃圾。
- 管理员收到站内通知；审核动作写审计日志。
- 纯文本评论，禁止 HTML；限制长度、链接数量和连续提交频率。
- 记录 IP 哈希和 UA 哈希用于反滥用，不公开展示。
- 蜜罐字段 + 达到阈值后加验证码；不要一上来就强制验证码，影响体验。
- 提交后显示“审核中”；没有邮件通知，审核结果只能在页面查看。

未来若开放登录评论，可以把游客评论认领到账号，不推翻现有结构。

---

## 8. 媒体与 CDN 预留

### 8.1 原则

- 数据库只存 `storage_key`，不存完整 URL。
- 内容里只写 `media://<id>`，渲染时解析。
- 所有图片 URL 由统一函数生成。
- 原图私有，公开端只访问衍生图。

### 8.2 后端抽象

```go
type AssetResolver interface {
    PublicURL(key, variant string) string
    SignedURL(key string, ttl time.Duration) (string, error)
    PresignPut(key, contentType string) (string, map[string]string, error)
    Purge(key string) error
}
```

配置：

```text
ASSET_BASE_URL=https://bucket.oss-cn-xxx.aliyuncs.com   # 当前
# 接入 CDN 后
ASSET_BASE_URL=https://cdn.twilightlog.com
```

`Purge` 当前为空实现；接 CDN 后实现刷新逻辑。

### 8.3 前端约定

```ts
// variant: 'w480' | 'w960' | 'w1440' | 'w2048' | 'w2560'
// 当前 OSS：variant 转为 OSS 图片处理参数
assetUrl(key, variant) => `${ASSET_BASE_URL}/${key}?x-oss-process=${variantProcess(variant)}`
```

- 第二个参数决定取哪个尺寸的衍生图，所有 `srcset` 由同一函数按 variant 组合生成。
- 当前展示变体由 OSS 图片处理参数实时生成；接入 CDN 后 URL 形态不变，CDN 缓存带参数的完整 URL。
- 迁移腾讯云前改为预生成固定变体 key，只调整 `assetUrl`/`AssetResolver` 实现，不改调用方。
- 所有 `<img>`、`srcset`、相册封面、文章图片必须走这个函数。
- 禁止在 Markdown、数据库、组件里写死域名。
- 衍生图使用内容哈希命名，便于长期缓存。

### 8.4 分阶段

| 阶段 | 方案 |
|---|---|
| 本地开发 | 本地存储适配器，仅开发使用 |
| 上线初期 | OSS 公共读衍生图；原图私有 + 签名 URL |
| 后期 | OSS 前加 CDN，改 `ASSET_BASE_URL` + DNS + 刷缓存 |
| 再后期 | 需要时增加 CDN 签名、图片处理参数、边缘规则 |

CDN 接入不会改数据库、不改历史文章、不改前端组件。

### 8.5 上传链路

```text
客户端 → API 申请预签名
       → 直传 OSS（分片）
       → API 校验文件头/大小/归属
       → 写 media_assets
       → 触发衍生图任务（OSS 或云函数）
```

应用服务器不代理上传流量。

### 8.6 图片处理：OSS 与云函数怎么分工

| 维度 | OSS 图片处理 | 云函数 |
|---|---|---|
| 使用方式 | URL 参数实时处理 | 上传事件触发，代码处理 |
| 适合 | 缩放、裁剪、格式、质量 | 剥 EXIF、校验、BlurHash、固定变体 |
| 开发量 | 几乎为零 | 需要写和部署函数 |
| 厂商绑定 | URL 参数绑定阿里云 | 代码可迁移，但依赖运行环境 |
| 成本 | 按处理请求计费 | 按调用和时长计费 |

已确认组合：

1. **展示变体**（w480/w960/w1440/w2048/w2560、WebP）用 OSS 图片处理，CDN 缓存。
2. **上传后一次性处理**用云函数：校验图片、提取白名单 EXIF、剥离 EXIF/GPS、纠正方向、生成无元数据的清洗 master 和 BlurHash。
3. **不在应用服务器上跑 sharp/libvips**，避免 2C2G 的 CPU 和内存尖峰。
4. 迁移腾讯云前，把常用尺寸预生成成固定 key，OSS 和 COS 存同样的 key，避免 URL 处理参数绑定厂商；这一步只改 `assetUrl`/`AssetResolver` 实现，不改前端调用。

推荐处理链路：

```text
原图上传到私有桶
  → 云函数校验、提取 EXIF、纠正方向、剥离元数据
  → 生成无 EXIF 的清洗 master
  → OSS 图片处理从 master 生成展示变体
  → CDN 缓存公开变体
```

这样既保留 OSS 图片处理的便利，又保证公开图不带 EXIF。

如果暂时不想上云函数，至少要先验证 OSS 图片处理输出是否已剥离 EXIF；不能确认时，原图只进私有桶，公开端不暴露原图。

### 8.7 图片交付与防盗

公开端只交付压缩衍生图，不交付原图：

| 用途 | 建议尺寸 | 格式 |
|---|---|---|
| 瀑布流缩略图 | 长边 480 / 960 | WebP，质量 75–80 |
| 相册网格 | 长边 960 | WebP |
| 详情查看 | 长边 1440 / 2048 | WebP，质量 80 |
| 大屏查看 | 长边 2560 | WebP，质量 80–85 |

约定：

- 详情页和放大查看都只到 2560，不提供原图；放大超过该尺寸用 CSS 放大，不再给更高分辨率。
- 原图只进私有桶，管理后台需要时用短时效签名 URL。
- 公开衍生图重新编码，统一转 sRGB，剥离 EXIF/XMP/IPTC/MakerNotes；不要直接转存原图。
- 文件名用内容哈希，便于 CDN 长期缓存，也避免暴露原始文件名。

清洗 EXIF 麻烦吗：

- 不麻烦，关键是**不要原图直出**。重新编码一次就会丢掉大部分元数据。
- libvips 的 `strip`、ImageMagick 的 `-strip`、sharp 的 `withMetadata({})`、Pillow 保存时不传 exif，都能做到。
- 推荐在上传后的云函数里做：读原图 → 提取白名单 EXIF → 纠正方向 → 转 sRGB → 生成多尺寸 → 压缩 → 剥离元数据 → 写公开桶。
- 如果只用 OSS 图片处理，必须先验证输出是否保留 EXIF；不能确认就加云函数。

关于防盗图：

- **浏览器保存无法彻底阻止。** 右键禁用、拖拽禁用、Canvas 渲染都拦不住截图和开发者工具，只会伤害正常体验。
- 真正有效的是：不公开原图、只给压缩图、隐藏原图地址、限制尺寸、可见水印、短时签名 URL、访问频率限制。
- 不提供下载按钮，响应使用 `Content-Disposition: inline`；但这只是减少误下载，不是安全边界。
- 不开启 Referer 防盗链，也不做分享功能；分享时直接给域名。
- 需要更强溯源时，再考虑 C2PA 或隐形水印。

### 8.8 可见水印

公开衍生图统一加一个小小的 TwilightLog 标志，放在右下角：

- 水印在生成衍生图时写入，不修改原图；原图永远私有。
- 使用透明背景的 SVG/PNG；白色或单色标志，配一层很淡的阴影，避免在亮色照片上消失。
- 尺寸按长边比例自适应，宽度 5–7%，最小 24px、最大 120px；右边距和下边距 2–3%。
- 不透明度 55–70%，既要能看见，又不能压过照片。
- 480 缩略图使用简化标志或更小尺寸，避免糊成一团；960 以上用完整标志。
- 水印资源带版本号，例如 `watermark-v1.png`；换标志后需要批量重新生成衍生图并清 CDN 缓存。
- 水印只能做来源标识，不能阻止裁剪和截图，核心仍然是“不公开原图”。

Logo 设计待办：

- 设计一个一笔成型的 TwilightLog 标志，后续单独做，不在本轮技术选型中展开。
- 在 Logo 定稿前，水印先用临时文字标或占位标志，接口和尺寸规则不变。

---

## 9. 部署与运维

### 9.1 当前服务器

2 核 2G / 40G ESSD 可以用于开发和初期运行，但要注意：

- 不在服务器上执行前端构建。
- 图片不落本地磁盘。
- PostgreSQL 调低内存参数。
- 增加 2G swap 防 OOM。

内存预算：

```text
系统            250–350MB
Caddy            30–50MB
Go API           50–100MB
PostgreSQL      300–450MB
余量            700MB–1GB
```

### 9.2 Docker Compose 服务

```text
caddy      反向代理 + 静态文件 + HTTPS
api        Go API
postgres   数据库
worker     可选，首版不部署
```

不部署：Redis、MinIO、Node 常驻、独立搜索服务。

### 9.3 续费配置建议

| 档位 | 配置 | 适用 |
|---|---|---|
| 最低可用 | 2C4G / 80G ESSD / 3–5M 或流量包 | Go + PG + 静态站，低流量 |
| 推荐 | 4C8G / 100G ESSD / 5–10M 或流量包 | 留余量，后续加任务/统计/搜索 |
| 不建议 | 2C2G / 40G / 3M | 只够当前，后续容易紧 |

优先加内存：PostgreSQL 的缓存和连接更吃内存；Go API 本身很省。CPU 4 核主要是给未来后台任务和数据库留余量。

带宽说明：

- 图片外置后，API 和后台 3–5M 足够。
- 如果现有套餐的峰值带宽充足且续费不贵，可以保留；不要为了峰值数字牺牲内存。
- 若按月流量计费，重点评估 OSS/CDN 流量成本。

### 9.4 备份与监控

- 每日 `pg_dump` 到对象存储，保留 30 天。
- 永久删除只清除在线原图、公开衍生图、云端副本、处理临时文件和缓存；历史数据库备份中的副本无法定点删除，随 30 天轮转自然失效，不承诺即时清除。
- 对象存储开启版本控制或生命周期规则。
- 日志轮转，避免 40G 磁盘被日志占满。
- Sentry 捕获前后端异常；外部 uptime 监控 API 和首页。
- 备份必须定期演练恢复。

### 9.5 换服务器

1. 新机器启动同一套 Compose。
2. 恢复 PostgreSQL 备份。
3. DNS 或 CDN 回源指向新 IP。
4. 清理 CDN 缓存。
5. 验证登录、列表、详情、上传、审批。
6. 旧机器观察后下线。

域名保持不变；图片在 OSS，不参与迁移。国内跨云厂商迁移需提前处理备案接入变更。

### 9.6 域名与入口规划

已备案域名：`twilightlog.com`、`www.twilightlog.com`。

已确认入口：

| 域名 | 用途 | 解析方式 |
|---|---|---|
| `www.twilightlog.com` | 公开站主域 | CNAME 到 CDN 或服务器 |
| `twilightlog.com` | 主域跳转 | 301 到 `www` |
| `api.twilightlog.com` | Go API | A 记录到服务器 |
| `admin.twilightlog.com` | 管理后台静态站 | CNAME 到 CDN 或服务器 |
| `cdn.twilightlog.com` | 图片/静态资源 | CNAME 到 CDN，后期换厂商只改 CNAME |

约定：

- 公开站 canonical 统一用 `https://www.twilightlog.com`，主域 301，避免重复收录。
- Cookie 使用 `Domain=.twilightlog.com`，API 的 CORS 只允许 `www` 和 `admin` 两个来源。
- 所有 HTTPS 证书覆盖上述子域；可用通配符证书，也可分别签发。
- 现有静态 HTML 由新站替换；如有旧链接，保留 301 到对应新页面。
- 小程序后期只把 `api.twilightlog.com` 加入 request 合法域名，域名不变就不需要反复改。

### 9.7 访问统计与数据分析

目标分开看：

- **访问统计**：PV/UV、热门文章/相册/照片、来源、设备、地区、404。
- **内容分析**：搜索词、阅读完成、图片查看、外链点击。
- **审计日志**：谁在后台改了什么，属于安全审计，不是访问统计。

推荐路线：

| 阶段 | 方案 | 说明 |
|---|---|---|
| 当前 2C2G | Caddy JSON 访问日志 + GoAccess | 服务端分析，几乎不占额外内存；看 PV/UV/路径/状态码/来源 |
| 续费 4G 后 | 自建 Umami + 独立数据库 | 轻量、隐私友好、支持自定义事件；Node 常驻约 100–200MB |
| SEO 数据 | Google Search Console + 百度搜索资源平台 | 不占服务器，补关键词和收录数据 |

不选 Matomo：PHP + MySQL，偏重；不选 Plausible CE：依赖 ClickHouse，2C2G 不合适。

埋点事件建议：

```text
article_view
photo_view
album_view
search
outbound_link
```

约定：

- 事件属性记录 `visitor_type=guest|editor|admin`，用于区分登录用户和匿名访客；不记录用户 ID。
- 后台统计默认排除编辑/管理员流量，避免自己访问污染内容数据；可提供开关查看全部。
- 当前 GoAccess 只能按 `admin` 域名/路径粗略区分员工；升级 Umami 后再记录 `visitor_type` 属性。
- 不存原始 IP，按天加盐哈希；不采集敏感个人信息。
- 统计脚本异步加载，不阻塞页面。
- 数据保留 90–180 天，后台只展示聚合结果。
- 统计服务与业务 API 分开数据库或至少分开 schema，统计写入不影响业务。
- 后台首页可嵌入 Umami 看板，或只展示核心聚合数字。

---

## 10. 多端策略

- 现在：Web 公开站 + 管理后台 + API。
- 下一步：PWA，成本最低。
- 小程序：Taro（React）接同一套 API；后台和高危操作不进入小程序。
- Android：先 PWA；确需上架再评估 React Native。
- 多端只共享 API 契约、类型、设计 token，不共享页面组件。
- 小程序对公式、Mermaid、长代码的渲染能力有限，需要降级方案或 `web-view`。

---

## 11. AI 开发约束

1. 前端 TypeScript 严格模式；Go 开启 `go vet`、`staticcheck`。
2. 接口边界必须有 OpenAPI/Zod/validator 校验。
3. 数据库只允许 migration 改表。
4. 每个功能必须通过：类型检查、lint、单元测试、构建。
5. 关键链路用 Playwright 冒烟：登录、发布、审批、上传、照片弹窗。
6. 一次只做一个模块，完成后提交 Git，便于回滚。
7. 权限只在 API 校验，前端隐藏不算安全。
8. 密钥、数据库密码、OSS AK 只放环境变量。
9. AI 生成的部署脚本必须人工核对后再执行。
10. 不允许 AI 直接在生产数据库执行 SQL。

---

## 12. 主要风险与取舍

| 风险 | 影响 | 应对 |
|---|---|---|
| Go 开发 CRUD 比 TS 慢 | 功能迭代速度 | 用 sqlc/OpenAPI 减少重复代码；AI 辅助 |
| 静态站发布有构建延迟 | 新内容不能秒级可见 | 接受延迟；必要时加详情页 SSR 兜底 |
| 两套前端（Astro + React） | 维护成本 | 共用类型和设计 token；后台独立 |
| 2C2G 内存紧 | OOM | swap、PG 调参、构建放 CI |
| 依赖 OSS | 成本和网络 | 统一 AssetResolver，保留本地开发适配器 |
| CDN 后接 | 迁移风险 | 域名稳定、URL 统一生成、缓存可清理 |
| 国内备案 | 上线时间 | 提前办理；服务器/OSS/CDN 尽量同一厂商 |
| 小程序渲染限制 | 内容不一致 | 简化渲染或 web-view，提前设计降级 |

---

## 13. 决策状态

已确认（V1.2 定稿）：

1. 云厂商先用阿里云，后期迁移腾讯云。
2. 公开站采用纯静态，发布延迟目标 5–10 分钟。
3. 域名 `twilightlog.com` / `www.twilightlog.com` 已备案。
4. 需要中文分词搜索；优先 `pg_jieba`，编译不顺利则用 `zhparser`。
5. 首版不做邮件；站内通知要做，范围按 6.1。
6. 需要访问统计和数据分析，并区分登录用户与匿名访客；当前先用 GoAccess，升级 4G 后接 Umami。
7. EXIF 只保留白名单字段，额外保留曝光补偿和白平衡；其余不保留；不使用 EXIF 缩略图作为相册主预览。
8. 登录只用于编辑/管理员分权；不做收藏、关注、个人中心。
9. 评论后期再做，首版不做；后期采用匿名 + 审核的游客身份方案。
10. 不允许公开原图和下载；缩略图与详情图都用压缩衍生图，并清洗 EXIF。
11. 公开图加右下角小 Logo 水印；Logo 一笔成型设计后续单独做。
12. 不开启 Referer 防盗链，不做分享功能。
13. 详情图最大长边 2560。
14. 小程序和 Android 暂不排期，保留 API 扩展能力。
15. 管理后台采用 Ant Design。
16. 图片展示变体用 OSS 图片处理；上传后由云函数清洗，云函数默认阿里云函数计算 FC。
17. 公开站 canonical 用 `https://www.twilightlog.com`，主域 301 跳转。
18. 水印尺寸按长边 5–7% 自适应（最小 24px、最大 120px），不透明度 55–70%；480 缩略图使用简化标志，960 以上用完整标志。
19. 文章专栏、摄影专栏与文章、相册、照片一样版本化；编辑提交后公开版保持不变，管理员审批通过后才更新。
20. 展示变体当前由 OSS 图片处理参数生成；迁移腾讯云前改为预生成固定变体 key，只调整 `assetUrl`/`AssetResolver` 实现。

尚未定稿的设计项：

1. TwilightLog 一笔成型 Logo 的设计与定稿。
2. 评论功能启动时的验证码阈值和评论长度上限。
3. 清洗 master 的最大长边和是否保留长期归档原图。

---

## 14. 下一步

1. 初始化仓库结构：`apps/web`、`apps/admin`、`services/api`、`infra`。
2. 定稿 OpenAPI 初始契约：认证、文章只读、相册只读、媒体上传、站内通知。
3. 实现认证、媒体上传、EXIF 白名单提取、OSS 图片处理、阿里云 FC 清洗、Logo 水印、文章/相册只读接口。
4. 再实现管理后台、审批流和站内通知。
5. 接入访问日志统计；升级 4G 后接 Umami。
6. 配置 `www` canonical、主域 301、`api`/`admin`/`cdn` 子域和 HTTPS。
7. 接公开站静态构建和 CDN。
8. Logo 设计定稿后，替换水印资源并批量重生成历史图。

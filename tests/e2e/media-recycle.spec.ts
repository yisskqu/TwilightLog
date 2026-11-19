import { readFileSync } from "node:fs";
import path from "node:path";
import { expect, test } from "@playwright/test";
import {
  absoluteUrl,
  ADMIN_PASSWORD,
  ADMIN_USER,
  buildWeb,
  call,
  createContent,
  ensureEditor,
  loginToken,
  loginUI,
  logoutUI,
  publish,
  purgeContent,
  readDistPage,
  ROOT,
  trashContent,
  uniqueSuffix,
  uploadMedia,
  variantUrl,
} from "./helpers";

const suffix = uniqueSuffix();
const columnSlug = `e2e-photo-${suffix}`;
const columnName = `E2E 摄影专栏 ${suffix}`;
const albumSlug = `e2e-album-${suffix}`;
const albumName = `E2E 相册 ${suffix}`;
const photoTitle = `E2E 照片 ${suffix}`;
const articleColumnName = `E2E 回收专栏 ${suffix}`;
const articleTitle = `E2E 回收文章 ${suffix}`;

let adminToken = "";
let editorToken = "";
let photoColumnId = "";
let albumId = "";
let photoId = "";
let mediaId = "";
let publishedVariantUrl = "";
let articleColumnId = "";
let articleId = "";

test.describe.configure({ mode: "serial" });

test.beforeAll(async ({ request }) => {
  adminToken = await loginToken(request, ADMIN_USER, ADMIN_PASSWORD);
  const editor = await ensureEditor(request, adminToken);
  editorToken = editor.token;
});

test("图片上传：衍生图、尺寸上限与元数据清理", async ({ request }) => {
  const photo = await uploadMedia(request, editorToken, "testdata/images/normal.jpg", "photo");
  mediaId = photo.id;
  expect(photo.status).toBe("ready");
  const names = photo.variants.map((item) => item.name);
  expect(names).toContain("w480");
  expect(names).toContain("w960");
  for (const variant of photo.variants) {
    expect(Math.max(variant.width, variant.height)).toBeLessThanOrEqual(2560);
  }

  // 6MB 图片走分片上传，长边仍被压到 2560
  const large = await uploadMedia(request, editorToken, "testdata/images/oversized-6mb.jpg", "photo");
  expect(large.status).toBe("ready");
  const longest = Math.max(...large.variants.map((variant) => Math.max(variant.width, variant.height)));
  expect(longest).toBeLessThanOrEqual(2560);

  // 匿名访问原图接口被拒绝，公开资源路径不存在原始文件
  const anonymousOriginal = await call(request, "GET", `/api/v1/media/${photo.id}/original`);
  expect(anonymousOriginal.status).toBe(401);
  const publicOriginal = await request.get(`${new URL(variantUrl(photo, "w960")).origin}/assets/media/${photo.id}/original.jpg`, {
    failOnStatusCode: false,
  });
  expect(publicOriginal.status()).toBe(404);

  // 带 GPS 的样张：公开变体不含 EXIF/GPS 标记
  const gps = await uploadMedia(request, editorToken, "testdata/images/gps.jpg", "photo");
  const variant = await request.get(variantUrl(gps, "w960"));
  expect(variant.status()).toBe(200);
  const bytes = await variant.body();
  const text = bytes.toString("latin1");
  expect(text.includes("GPS")).toBe(false);
  expect(text.includes("Exif\u0000\u0000")).toBe(false);
  expect(gps.variants.length).toBeGreaterThan(0);

  // 重复完成是幂等的
  const again = await call(request, "POST", "/api/v1/media/complete", {
    token: editorToken,
    data: { mediaId: photo.id },
  });
  expect([200, 202]).toContain(again.status);

  // HEIC 与 EXIF 方向
  const heic = await uploadMedia(request, editorToken, "testdata/images/sample.heic", "photo");
  expect(heic.status).toBe("ready");
  const heicBytes = await (await request.get(variantUrl(heic, "w480"))).body();
  expect(heicBytes.subarray(0, 4).toString("latin1")).toBe("RIFF");
  expect(heicBytes.subarray(8, 12).toString("latin1")).toBe("WEBP");

  const rotated = await uploadMedia(request, editorToken, "testdata/images/orientation-6.jpg", "photo");
  expect(rotated.status).toBe("ready");
  expect(rotated.width).toBeLessThan(rotated.height);
  expect(rotated.exif.orientation).toBe(6);

  // 不支持的 MIME 在预签名阶段被拒绝
  const rejected = await call(request, "POST", "/api/v1/media/presign", {
    token: editorToken,
    data: {
      purpose: "photo",
      filename: "notes.txt",
      contentType: "text/plain",
      size: 1024,
      batchId: crypto.randomUUID(),
    },
  });
  expect(rejected.status).toBe(415);
  expect(rejected.data.code).toBe("unsupported_format");

  // 伪装扩展名：处理失败并可重试
  const fakeBuffer = readFileSync(path.join(ROOT, "testdata/images/fake-extension.jpg"));
  const fakePresign = await call(request, "POST", "/api/v1/media/presign", {
    token: editorToken,
    data: {
      purpose: "photo",
      filename: "fake.jpg",
      contentType: "image/jpeg",
      size: fakeBuffer.length,
      batchId: crypto.randomUUID(),
    },
  });
  expect(fakePresign.status).toBe(201);
  const fakePut = await request.put(absoluteUrl(fakePresign.data.uploadUrl), {
    headers: { "Content-Type": "image/jpeg" },
    data: fakeBuffer,
  });
  expect(fakePut.status()).toBeLessThan(300);
  await call(request, "POST", "/api/v1/media/complete", { token: editorToken, data: { mediaId: fakePresign.data.mediaId } });
  await expect
    .poll(async () => {
      const current = await call(request, "GET", `/api/v1/media/${fakePresign.data.mediaId}`, {
        token: editorToken,
      });
      return current.data?.status;
    }, { timeout: 30_000 })
    .toBe("failed");
  const duplicateFailed = await call(request, "POST", "/api/v1/media/complete", {
    token: editorToken,
    data: { mediaId: fakePresign.data.mediaId },
  });
  expect(duplicateFailed.status).toBe(409);
  const retried = await call(request, "POST", `/api/v1/media/${fakePresign.data.mediaId}/retry`, {
    token: editorToken,
  });
  expect([200, 202]).toContain(retried.status);
});

test("照片上传 → 审批 → 相册瀑布流与查看器", async ({ page, request }) => {
  test.setTimeout(240_000);

  const column = await createContent(request, editorToken, {
    type: "photo_column",
    slug: columnSlug,
    payload: { name: columnName, description: "阶段 7 摄影联调", theme: "城市" },
  });
  photoColumnId = column.id;
  await publish(request, editorToken, adminToken, "photo_column", photoColumnId, columnName);

  const album = await createContent(request, editorToken, {
    type: "album",
    parentId: photoColumnId,
    slug: albumSlug,
    payload: { title: albumName, description: "联调相册", location: "上海", shotAt: "2026-11-01" },
  });
  albumId = album.id;
  await publish(request, editorToken, adminToken, "album", albumId, albumName);

  const photo = await createContent(request, editorToken, {
    type: "photo",
    parentId: albumId,
    payload: { mediaId, title: photoTitle, description: "联调照片", location: "上海", takenAt: "2026-11-01" },
  });
  photoId = photo.id;
  await publish(request, editorToken, adminToken, "photo", photoId, photoTitle);

  const photos = await call(request, "GET", `/api/v1/albums/${albumSlug}/photos?pageSize=10`);
  expect(photos.status).toBe(200);
  expect(photos.data.items.map((item: any) => item.title)).toContain(photoTitle);
  const published = photos.data.items.find((item: any) => item.title === photoTitle);
  expect(published.media.variants.length).toBeGreaterThan(0);
  publishedVariantUrl = new URL(published.media.variants[0].url, "http://localhost:8080").toString();

  const detail = await call(request, "GET", `/api/v1/photos/${published.id}`);
  expect(detail.status).toBe(200);
  expect(JSON.stringify(detail.data)).not.toMatch(/latitude|longitude|GPS/i);

  // 构建后相册页面包含照片瓦片
  const dist = buildWeb();
  const html = readDistPage(dist, `/photos/${columnSlug}/${albumSlug}`);
  expect(html, "构建产物缺少相册页面").not.toBeNull();
  expect(html!).toContain(photoTitle);
  expect(html!).toContain("data-photo-index");

  // 已有种子相册（dev 服务已生成）验证瀑布流与查看器交互
  await page.goto(`${process.env.E2E_WEB_URL ?? "http://localhost:4321"}/photos/street/neon-rain`);
  await page.waitForFunction(() => !document.querySelector("astro-island[ssr]"), null, { timeout: 15_000 });
  const tiles = page.locator(".photo-tile");
  await expect(tiles.first()).toBeVisible();
  expect(await tiles.count()).toBeGreaterThan(0);
  await tiles.first().click();
  const viewer = page.getByRole("dialog");
  await expect(viewer).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(viewer).toBeHidden();

  // 永久删除照片时同时清理在线文件
  await trashContent(request, editorToken, "photo", photoId);
  const purged = await call(request, "DELETE", `/api/v1/admin/content/photo/${photoId}`, {
    token: adminToken,
  });
  expect(purged.status, JSON.stringify(purged.data)).toBe(200);
  expect(purged.data.mediaPurged).toBeGreaterThanOrEqual(1);
  const media = await call(request, "GET", `/api/v1/media/${mediaId}`, { token: adminToken });
  expect(media.status).toBe(404);
  const variantAfterPurge = await request.get(publishedVariantUrl, { failOnStatusCode: false });
  expect([403, 404]).toContain(variantAfterPurge.status());
});

test("回收站：恢复与永久删除（含媒体清理）", async ({ page, request }) => {
  test.setTimeout(120_000);

  const column = await createContent(request, editorToken, {
    type: "article_column",
    slug: `e2e-recycle-${suffix}`,
    payload: { name: articleColumnName },
  });
  articleColumnId = column.id;
  await publish(request, editorToken, adminToken, "article_column", articleColumnId, articleColumnName);

  const article = await createContent(request, editorToken, {
    type: "article",
    parentId: articleColumnId,
    slug: `e2e-recycle-article-${suffix}`,
    payload: { title: articleTitle, summary: "回收站联调", bodyMarkdown: "正文" },
  });
  articleId = article.id;
  await publish(request, editorToken, adminToken, "article", articleId, articleTitle);
  expect((await call(request, "GET", `/api/v1/articles/${article.slug}`)).status).toBe(200);

  // 编辑在工作区把文章移入回收站
  await loginUI(page, process.env.E2E_EDITOR_USER ?? "e2e_editor", process.env.E2E_EDITOR_PASSWORD ?? "E2Eeditor2026!");
  await page.goto("/editor/workspace/article");
  page.on("dialog", (dialog) => void dialog.accept());
  const row = page.locator("tr", { hasText: articleTitle });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "移入回收站" }).click();

  await expect
    .poll(async () => (await call(request, "GET", `/api/v1/articles/${article.slug}`)).status)
    .toBe(404);

  // 回收站恢复
  await page.goto("/editor/recycle");
  await page.getByRole("button", { name: "文章", exact: true }).click();
  const trashRow = page.locator("tr", { hasText: articleTitle });
  await expect(trashRow).toBeVisible();
  await trashRow.getByRole("button", { name: "恢复" }).click();
  await expect(page.getByText(`已恢复「${articleTitle}」`)).toBeVisible();
  expect((await call(request, "GET", `/api/v1/articles/${article.slug}`)).status).toBe(200);

  // 管理员永久删除
  await trashContent(request, editorToken, "article", articleId);
  await logoutUI(page);
  await loginUI(page, ADMIN_USER, ADMIN_PASSWORD);
  await page.goto("/admin/recycle");
  // 管理员默认只看自己的内容，勾选「查看全站」才能看到编辑的回收站条目
  await page.getByRole("checkbox", { name: "查看全站" }).check();
  await page.getByRole("button", { name: "文章", exact: true }).click();
  const purgeRow = page.locator("tr", { hasText: articleTitle });
  await expect(purgeRow).toBeVisible();
  await purgeRow.getByRole("button", { name: "永久删除" }).click();
  await expect(page.getByText(/已永久删除/)).toBeVisible();
  await expect
    .poll(async () => (await call(request, "GET", `/api/v1/articles/${article.slug}`)).status)
    .toBe(404);
});

test.afterAll(async ({ request }) => {
  const cleanup = async (type: string, id: string) => {
    if (!id) return;
    await trashContent(request, adminToken, type, id).catch(() => undefined);
    await purgeContent(request, adminToken, type, id).catch(() => undefined);
  };
  await cleanup("album", albumId);
  await cleanup("photo_column", photoColumnId);
  await cleanup("article_column", articleColumnId);
});

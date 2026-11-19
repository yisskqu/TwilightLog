import { expect, test } from "@playwright/test";
import {
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
  readDistFile,
  readDistPage,
  ROOT,
  trashContent,
  uniqueSuffix,
} from "./helpers";
import path from "node:path";

const suffix = uniqueSuffix();
const columnSlug = `e2e-col-${suffix}`;
const columnName = `E2E 专栏 ${suffix}`;
const articleSlug = `e2e-article-${suffix}`;
const v1Title = `E2E 联调文章 ${suffix}`;
const v2Title = `E2E 修订文章 ${suffix}`;
const articlePath = `/articles/${columnSlug}/${articleSlug}`;

let adminToken = "";
let editorToken = "";
let columnId = "";
let articleId = "";

test.describe.configure({ mode: "serial" });

test.beforeAll(async ({ request }) => {
  adminToken = await loginToken(request, ADMIN_USER, ADMIN_PASSWORD);
  const editor = await ensureEditor(request, adminToken);
  editorToken = editor.token;

  const column = await createContent(request, editorToken, {
    type: "article_column",
    slug: columnSlug,
    payload: { name: columnName, description: "阶段 7 联调自动创建" },
  });
  columnId = column.id;
  await publish(request, editorToken, adminToken, "article_column", columnId, columnName);
});

test.afterAll(async ({ request }) => {
  try {
    if (articleId) {
      await trashContent(request, adminToken, "article", articleId).catch(() => undefined);
      await purgeContent(request, adminToken, "article", articleId).catch(() => undefined);
    }
    if (columnId) {
      await trashContent(request, adminToken, "article_column", columnId).catch(() => undefined);
      await purgeContent(request, adminToken, "article_column", columnId).catch(() => undefined);
    }
  } catch {
    // 清理失败不影响测试结论，回收站里会留下痕迹。
  }
});

test("编辑在后台创建文章 → 提交 → 管理员审批 → 公开可见", async ({ page, request }) => {
  test.setTimeout(240_000);

  await loginUI(page, process.env.E2E_EDITOR_USER ?? "e2e_editor", process.env.E2E_EDITOR_PASSWORD ?? "E2Eeditor2026!");
  await page.goto(`/editor/content/new?type=article`);

  await expect(page.getByRole("heading", { name: "新建文章" })).toBeVisible();
  await page.getByLabel("所属文章专栏").selectOption({ label: columnName });
  await page.getByLabel("链接标识（可选）").fill(articleSlug);
  await page.getByLabel("标题", { exact: true }).fill(v1Title);
  await page.getByLabel("摘要").fill("联调摘要：审批前后公开版本应保持一致。");
  await page.getByLabel("正文（Markdown）").fill("# 联调正文\n\n这是一篇用于阶段 7 验收的文章。");
  await page.getByLabel("标签").fill(`联调, ${suffix}`);

  await page.getByRole("button", { name: "保存草稿" }).click();
  await expect(page.getByText("草稿已保存")).toBeVisible();
  await page.waitForURL(/\/editor\/content\/article\/[0-9a-f-]+$/);
  articleId = page.url().split("/").pop() ?? "";

  await page.getByRole("button", { name: "提交审核" }).click();
  await expect(page.getByText("已提交审核，等待管理员审批")).toBeVisible();

  // 审批前公开接口不可见
  const beforeApproval = await call(request, "GET", `/api/v1/articles/${articleSlug}`);
  expect(beforeApproval.status).toBe(404);

  // 管理员在审批中心通过
  const admin = await page.context().browser()!.newContext();
  const adminPage = await admin.newPage();
  await loginUI(adminPage, ADMIN_USER, ADMIN_PASSWORD);
  await adminPage.goto("/admin/reviews");
  const reviewItem = adminPage.locator(".review-items button", { hasText: v1Title });
  await expect(reviewItem).toBeVisible();
  await reviewItem.click();
  await expect(adminPage.locator(".review-detail h2")).toHaveText(v1Title);
  await adminPage.getByRole("button", { name: "通过并替换公开版本" }).click();
  await expect(adminPage.getByText(/已通过/)).toBeVisible();
  await admin.close();

  const afterApproval = await call(request, "GET", `/api/v1/articles/${articleSlug}`);
  expect(afterApproval.status).toBe(200);
  expect(afterApproval.data.title).toBe(v1Title);

  // 生产构建产物中出现新页面、canonical 与 sitemap 条目
  const dist = buildWeb();
  const html = readDistPage(dist, articlePath);
  expect(html, `构建产物缺少 ${articlePath}`).not.toBeNull();
  expect(html!).toContain(v1Title);
  expect(html!).toContain(`<link rel="canonical" href="https://www.twilightlog.com${articlePath}">`);
  expect(html!).toContain('property="og:title"');

  const sitemapIndex = readDistFile(dist, "sitemap-index.xml");
  expect(sitemapIndex, "缺少 sitemap-index.xml").not.toBeNull();
  expect(sitemapIndex!).toContain("sitemap-0.xml");
  const sitemap = readDistFile(dist, "sitemap-0.xml");
  expect(sitemap, "缺少 sitemap-0.xml").not.toBeNull();
  expect(sitemap!).toContain(articlePath);
});

test("编辑修改已公开文章：审批前旧版、审批后新版", async ({ page, request }) => {
  test.setTimeout(240_000);
  expect(articleId, "依赖上一条用例创建的文章").toBeTruthy();

  await loginUI(page, process.env.E2E_EDITOR_USER ?? "e2e_editor", process.env.E2E_EDITOR_PASSWORD ?? "E2Eeditor2026!");
  await page.goto(`/editor/content/article/${articleId}`);
  await expect(page.getByRole("heading", { name: "编辑文章" })).toBeVisible();

  await page.getByLabel("标题", { exact: true }).fill(v2Title);
  await page.getByRole("button", { name: "保存草稿" }).click();
  await expect(page.getByText("草稿已保存")).toBeVisible();
  await page.getByRole("button", { name: "提交审核" }).click();
  await expect(page.getByText("已提交审核，等待管理员审批")).toBeVisible();

  // 审批前：公开接口与已构建页面仍是上一版
  const pending = await call(request, "GET", `/api/v1/articles/${articleSlug}`);
  expect(pending.status).toBe(200);
  expect(pending.data.title).toBe(v1Title);
  const beforeHTML = readDistPage(path.join(ROOT, "apps", "web", "dist"), articlePath);
  expect(beforeHTML).toContain(v1Title);
  expect(beforeHTML).not.toContain(v2Title);

  // 管理员审批
  const admin = await page.context().browser()!.newContext();
  const adminPage = await admin.newPage();
  await loginUI(adminPage, ADMIN_USER, ADMIN_PASSWORD);
  await adminPage.goto("/admin/reviews");
  const reviewItem = adminPage.locator(".review-items button", { hasText: v2Title });
  await expect(reviewItem).toBeVisible();
  await reviewItem.click();
  await expect(adminPage.locator(".review-detail h2")).toHaveText(v2Title);
  await adminPage.getByRole("button", { name: "通过并替换公开版本" }).click();
  await expect(adminPage.getByText(/已通过/)).toBeVisible();
  await admin.close();

  const published = await call(request, "GET", `/api/v1/articles/${articleSlug}`);
  expect(published.status).toBe(200);
  expect(published.data.title).toBe(v2Title);

  const dist = buildWeb();
  const html = readDistPage(dist, articlePath);
  expect(html).not.toBeNull();
  expect(html!).toContain(v2Title);
  expect(html!).not.toContain(v1Title);
});

test.describe("登录、退出与直接 URL 权限", () => {
  test("编辑登录后被拦截在管理后台之外，退出后回到登录页", async ({ page }) => {
    await loginUI(page, process.env.E2E_EDITOR_USER ?? "e2e_editor", process.env.E2E_EDITOR_PASSWORD ?? "E2Eeditor2026!");
    await expect(page.getByRole("heading", { name: "编辑工作区" })).toBeVisible();

    await page.goto("/admin/users");
    await expect(page.getByRole("heading", { name: "无权访问" })).toBeVisible();
    await expect(page.getByText("403")).toBeVisible();

    await page.locator(".sidebar").getByRole("button", { name: "退出登录" }).click();
    await page.waitForURL(/\/login$/);
    await expect(page.getByRole("heading", { name: "登录后台" })).toBeVisible();
  });

  test("管理员登录、刷新后保持会话、正常退出", async ({ page }) => {
    await loginUI(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.waitForURL(/\/admin$/);
    await expect(page.getByRole("heading", { name: "概览" })).toBeVisible();

    await page.reload();
    await expect(page.getByRole("heading", { name: "概览" })).toBeVisible();

    await logoutUI(page);
    await expect(page.getByRole("heading", { name: "登录后台" })).toBeVisible();
  });

  test("未登录用户访问后台会被送回登录页", async ({ page }) => {
    await page.goto("/admin/reviews");
    await page.waitForURL(/\/login$/);
    await expect(page.getByRole("heading", { name: "登录后台" })).toBeVisible();
  });
});

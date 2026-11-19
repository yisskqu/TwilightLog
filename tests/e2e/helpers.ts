import { execSync } from "node:child_process";
import { readFileSync, existsSync } from "node:fs";
import path from "node:path";
import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const ROOT = process.cwd();
export const API_URL = process.env.E2E_API_URL ?? "http://localhost:8080";
export const ADMIN_URL = process.env.E2E_ADMIN_URL ?? "http://localhost:5173";
export const WEB_URL = process.env.E2E_WEB_URL ?? "http://localhost:4321";

export const ADMIN_USER = process.env.E2E_ADMIN_USER ?? "admin";
export const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? "Twilight2026!";
export const EDITOR_USER = process.env.E2E_EDITOR_USER ?? "e2e_editor";
export const EDITOR_PASSWORD = process.env.E2E_EDITOR_PASSWORD ?? "E2Eeditor2026!";

export type Device = "desktop" | "mobile";

export interface ApiResult {
  status: number;
  data: any;
  headers: Record<string, string>;
}

export async function call(
  request: APIRequestContext,
  method: string,
  urlPath: string,
  options: { token?: string; data?: unknown; device?: Device; headers?: Record<string, string> } = {},
): Promise<ApiResult> {
  const headers: Record<string, string> = {
    "X-Client-Device": options.device ?? "desktop",
    ...options.headers,
  };
  if (options.token) {
    headers.Authorization = `Bearer ${options.token}`;
  }
  const response = await request.fetch(`${API_URL}${urlPath}`, {
    method,
    headers,
    data: options.data,
    failOnStatusCode: false,
  });
  const text = await response.text();
  let data: any = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }
  return { status: response.status(), data, headers: response.headers() as Record<string, string> };
}

/** Logs in through the real API and returns the access token. */
export async function loginToken(
  request: APIRequestContext,
  username: string,
  password: string,
): Promise<string> {
  const result = await call(request, "POST", "/api/v1/auth/login", {
    data: { username, password, remember: false },
  });
  expect(result.status, `登录失败：${JSON.stringify(result.data)}`).toBe(200);
  return result.data.accessToken as string;
}

/** UI login used by browser tests (sets the in-memory token and refresh cookie). */
export async function loginUI(page: Page, username: string, password: string): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill(password);
  await page.getByRole("button", { name: "登录" }).click();
  await page.waitForURL((url) => !url.pathname.startsWith("/login"));
}

export async function logoutUI(page: Page): Promise<void> {
  // 侧边栏与 403 页都提供退出按钮，固定用侧边栏那个避免歧义。
  await page.locator(".sidebar").getByRole("button", { name: "退出登录" }).click();
  await page.waitForURL(/\/login$/);
}

/**
 * Ensures the shared editor account exists and is an active editor with a known
 * password, then returns a token for it.
 */
export async function ensureEditor(
  request: APIRequestContext,
  adminToken: string,
): Promise<{ id: string; token: string }> {
  const list = await call(request, "GET", `/api/v1/admin/users?search=${EDITOR_USER}&pageSize=50`, {
    token: adminToken,
  });
  expect(list.status, JSON.stringify(list.data)).toBe(200);
  const existing = (list.data.items as Array<{ id: string; username: string }>).find(
    (item) => item.username === EDITOR_USER,
  );

  let id = existing?.id;
  if (!id) {
    const created = await call(request, "POST", "/api/v1/admin/users", {
      token: adminToken,
      data: {
        username: EDITOR_USER,
        email: `${EDITOR_USER}@test.local`,
        password: EDITOR_PASSWORD,
        role: "editor",
      },
    });
    expect(created.status, JSON.stringify(created.data)).toBe(201);
    id = created.data.id;
  } else {
    const reset = await call(request, "POST", `/api/v1/admin/users/${id}/reset-password`, {
      token: adminToken,
      data: { password: EDITOR_PASSWORD },
    });
    expect(reset.status, JSON.stringify(reset.data)).toBe(200);
    const role = await call(request, "PATCH", `/api/v1/admin/users/${id}/role`, {
      token: adminToken,
      data: { role: "editor" },
    });
    expect(role.status, JSON.stringify(role.data)).toBe(200);
    const status = await call(request, "PATCH", `/api/v1/admin/users/${id}/status`, {
      token: adminToken,
      data: { status: "active" },
    });
    expect(status.status, JSON.stringify(status.data)).toBe(200);
  }

  return { id: id!, token: await loginToken(request, EDITOR_USER, EDITOR_PASSWORD) };
}

export interface ContentInput {
  type: string;
  slug?: string;
  parentId?: string;
  payload: Record<string, unknown>;
}

export async function createContent(
  request: APIRequestContext,
  token: string,
  input: ContentInput,
): Promise<any> {
  const result = await call(request, "POST", "/api/v1/workspace/content", {
    token,
    data: { type: input.type, slug: input.slug ?? "", parentId: input.parentId ?? "", payload: input.payload },
  });
  expect(result.status, JSON.stringify(result.data)).toBe(201);
  return result.data;
}

export async function saveContent(
  request: APIRequestContext,
  token: string,
  type: string,
  id: string,
  payload: Record<string, unknown>,
  slug = "",
): Promise<any> {
  const result = await call(request, "PUT", `/api/v1/workspace/content/${type}/${id}`, {
    token,
    data: { slug, payload },
  });
  expect(result.status, JSON.stringify(result.data)).toBe(200);
  return result.data;
}

export async function submitContent(
  request: APIRequestContext,
  token: string,
  type: string,
  id: string,
): Promise<any> {
  const result = await call(request, "POST", `/api/v1/workspace/content/${type}/${id}/submit`, { token });
  expect(result.status, JSON.stringify(result.data)).toBe(200);
  return result.data;
}

export async function pendings(request: APIRequestContext, adminToken: string): Promise<any[]> {
  const result = await call(request, "GET", "/api/v1/admin/reviews", { token: adminToken });
  expect(result.status, JSON.stringify(result.data)).toBe(200);
  return result.data.items as any[];
}

/** Polls the review queue until the given title appears, then approves it. */
export async function approveByTitle(
  request: APIRequestContext,
  adminToken: string,
  title: string,
): Promise<any> {
  let target: any = null;
  for (let attempt = 0; attempt < 30 && !target; attempt += 1) {
    target = (await pendings(request, adminToken)).find((item) => item.title === title);
    if (!target) {
      await new Promise((resolve) => setTimeout(resolve, 300));
    }
  }
  expect(target, `审批队列中找不到「${title}」`).toBeTruthy();
  const result = await call(request, "POST", `/api/v1/admin/reviews/${target.versionId}/approve`, {
    token: adminToken,
    data: { comment: "" },
  });
  expect(result.status, JSON.stringify(result.data)).toBe(200);
  return result.data;
}

/** Publishes a content type created by the given token end to end. */
export async function publish(
  request: APIRequestContext,
  editorToken: string,
  adminToken: string,
  type: string,
  id: string,
  title: string,
): Promise<void> {
  await submitContent(request, editorToken, type, id);
  await approveByTitle(request, adminToken, title);
}

export async function trashContent(
  request: APIRequestContext,
  token: string,
  type: string,
  id: string,
): Promise<void> {
  const result = await call(request, "POST", `/api/v1/workspace/content/${type}/${id}/trash`, { token });
  expect(result.status, JSON.stringify(result.data)).toBe(200);
}

export async function purgeContent(
  request: APIRequestContext,
  adminToken: string,
  type: string,
  id: string,
): Promise<number> {
  const result = await call(request, "DELETE", `/api/v1/admin/content/${type}/${id}`, { token: adminToken });
  expect(result.status, JSON.stringify(result.data)).toBe(200);
  return result.data.mediaPurged as number;
}

export interface MediaAsset {
  id: string;
  status: string;
  width: number;
  height: number;
  variants: Array<{ name: string; url: string; width: number; height: number; size: number }>;
}

const MIME_BY_EXT: Record<string, string> = {
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  webp: "image/webp",
  heic: "image/heic",
  heif: "image/heic",
};

/** Uploads a fixture image through presign → PUT → complete and waits for ready. */
export async function uploadMedia(
  request: APIRequestContext,
  token: string,
  filePath: string,
  purpose: "article" | "photo",
): Promise<MediaAsset> {
  const absolute = path.isAbsolute(filePath) ? filePath : path.join(ROOT, filePath);
  const buffer = readFileSync(absolute);
  const filename = path.basename(absolute);
  const ext = filename.split(".").pop()?.toLowerCase() ?? "";
  const contentType = MIME_BY_EXT[ext] ?? "application/octet-stream";

  const presign = await call(request, "POST", "/api/v1/media/presign", {
    token,
    data: {
      purpose,
      filename,
      contentType,
      size: buffer.length,
      batchId: crypto.randomUUID(),
    },
  });
  expect(presign.status, JSON.stringify(presign.data)).toBe(201);
  const presigned = presign.data;
  const mediaId = presigned.mediaId as string;

  if (presigned.uploadMode === "multipart") {
    const urls = (presigned.partUrls ?? []) as string[];
    const partSize = (presigned.partSize ?? 5 * 1024 * 1024) as number;
    expect(urls.length).toBeGreaterThan(0);
    for (let index = 0; index < urls.length; index += 1) {
      const chunk = buffer.subarray(index * partSize, Math.min((index + 1) * partSize, buffer.length));
      const put = await request.put(absoluteUrl(urls[index]), {
        headers: { "Content-Type": "application/octet-stream" },
        data: chunk,
      });
      expect(put.status(), `分片 ${index + 1} 上传失败`).toBeLessThan(300);
    }
  } else {
    expect(presigned.uploadUrl, "服务端未返回上传地址").toBeTruthy();
    const put = await request.put(absoluteUrl(presigned.uploadUrl), {
      headers: { "Content-Type": presigned.headers?.["Content-Type"] ?? contentType },
      data: buffer,
    });
    expect(put.status(), "图片上传失败").toBeLessThan(300);
  }

  const complete = await call(request, "POST", "/api/v1/media/complete", { token, data: { mediaId } });
  expect([200, 202], JSON.stringify(complete.data)).toContain(complete.status);

  const deadline = Date.now() + 60_000;
  let asset = complete.data as MediaAsset;
  while (asset.status === "processing" || asset.status === "uploading") {
    expect(Date.now(), "图片处理超时").toBeLessThan(deadline);
    await new Promise((resolve) => setTimeout(resolve, 500));
    const current = await call(request, "GET", `/api/v1/media/${mediaId}`, { token });
    expect(current.status, JSON.stringify(current.data)).toBe(200);
    asset = current.data as MediaAsset;
  }
  expect(asset.status, JSON.stringify(asset)).toBe("ready");
  return asset;
}

export function absoluteUrl(url: string): string {
  return url.startsWith("http") ? url : `${API_URL}${url}`;
}

export function variantUrl(asset: MediaAsset, name: string): string {
  const variant = asset.variants.find((item) => item.name === name) ?? asset.variants[0];
  expect(variant, `图片变体 ${name} 不存在`).toBeTruthy();
  return absoluteUrl(variant.url);
}

/** Runs a production build of the public site and returns the dist directory. */
export function buildWeb(): string {
  execSync("pnpm --filter @twilightlog/web build", { cwd: ROOT, stdio: "pipe", encoding: "utf8", timeout: 180_000 });
  const dist = path.join(ROOT, "apps", "web", "dist");
  expect(existsSync(path.join(dist, "index.html")), "构建产物缺少 index.html").toBe(true);
  return dist;
}

/** Reads an exact file from dist; returns null when it was not generated. */
export function readDistFile(dist: string, relative: string): string | null {
  const target = path.join(dist, relative.replace(/^\/+/, ""));
  return existsSync(target) ? readFileSync(target, "utf8") : null;
}

/** Reads a built static page from dist; returns null when the page was not generated. */
export function readDistPage(dist: string, urlPath: string): string | null {
  const cleaned = urlPath.replace(/^\/+|\/+$/g, "");
  const candidates = [
    path.join(dist, cleaned, "index.html"),
    path.join(dist, `${cleaned}.html`),
  ];
  for (const candidate of candidates) {
    if (existsSync(candidate)) {
      return readFileSync(candidate, "utf8");
    }
  }
  return null;
}

export function uniqueSuffix(): string {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

import { expect, test } from "@playwright/test";
import {
  ADMIN_PASSWORD,
  ADMIN_USER,
  API_URL,
  call,
  createContent,
  ensureEditor,
  loginToken,
  purgeContent,
  trashContent,
  uniqueSuffix,
} from "./helpers";

let adminToken = "";
let editorToken = "";

test.beforeAll(async ({ request }) => {
  adminToken = await loginToken(request, ADMIN_USER, ADMIN_PASSWORD);
  const editor = await ensureEditor(request, adminToken);
  editorToken = editor.token;
});

test.describe("公开 API 契约", () => {
  test("分页、排序与筛选参数生效且非法参数被拒绝", async ({ request }) => {
    const pageOne = await call(request, "GET", "/api/v1/photo-columns?page=1&pageSize=2&sort=latest");
    expect(pageOne.status).toBe(200);
    expect(pageOne.data.page).toBe(1);
    expect(pageOne.data.pageSize).toBe(2);
    expect(pageOne.data.items.length).toBeLessThanOrEqual(2);
    expect(pageOne.data.totalPages).toBe(Math.ceil(pageOne.data.total / 2));

    const pageTwo = await call(request, "GET", "/api/v1/photo-columns?page=2&pageSize=2&sort=latest");
    expect(pageTwo.status).toBe(200);
    expect(pageTwo.data.page).toBe(2);
    const firstIDs = pageOne.data.items.map((item: any) => item.id);
    const secondIDs = pageTwo.data.items.map((item: any) => item.id);
    expect(firstIDs.filter((id: string) => secondIDs.includes(id))).toHaveLength(0);

    const latest = await call(request, "GET", "/api/v1/photo-columns?pageSize=50&sort=latest");
    const oldest = await call(request, "GET", "/api/v1/photo-columns?pageSize=50&sort=oldest");
    const latestIDs = latest.data.items.map((item: any) => item.id);
    const oldestIDs = oldest.data.items.map((item: any) => item.id);
    expect(latestIDs).toEqual([...oldestIDs].reverse());

    const oversize = await call(request, "GET", "/api/v1/photo-columns?pageSize=500");
    expect(oversize.status).toBe(200);
    expect(oversize.data.pageSize).toBeLessThanOrEqual(50);

    const invalidSort = await call(request, "GET", "/api/v1/photos?sort=sideways");
    expect(invalidSort.status).toBe(400);
    expect(invalidSort.data.code).toBe("validation_error");

    const invalidDate = await call(request, "GET", "/api/v1/search?q=雨&dateFrom=2026-13-01");
    expect(invalidDate.status).toBe(400);
  });

  test("错误响应统一格式并带 requestId", async ({ request }) => {
    const missing = await call(request, "GET", "/api/v1/articles/definitely-missing-slug");
    expect(missing.status).toBe(404);
    expect(missing.data.code).toBe("not_found");
    expect(typeof missing.data.message).toBe("string");
    expect(missing.data.requestId).toBeTruthy();
    expect(missing.headers["x-request-id"]).toBeTruthy();

    const malformed = await call(request, "GET", "/api/v1/workspace/content/article/not-a-uuid", {
      token: adminToken,
    });
    expect(malformed.status).toBe(404);
    expect(malformed.data.code).toBe("not_found");

    const unknownType = await call(request, "GET", "/api/v1/workspace/content?type=news", {
      token: adminToken,
    });
    expect(unknownType.status).toBe(400);
    expect(unknownType.data.code).toBe("validation_error");
  });

  test("ETag 命中返回 304", async ({ request }) => {
    const first = await call(request, "GET", "/api/v1/articles/go-observability");
    expect(first.status).toBe(200);
    const etag = first.headers["etag"];
    expect(etag).toBeTruthy();

    const second = await request.get(`${API_URL}/api/v1/articles/go-observability`, {
      headers: { "If-None-Match": etag! },
      failOnStatusCode: false,
    });
    expect(second.status()).toBe(304);
  });

  test("越权访问与不存在返回一致的 403", async ({ request }) => {
    // 管理员创建的内容对编辑不可见
    const suffix = uniqueSuffix();
    const column = await createContent(request, adminToken, {
      type: "article_column",
      slug: `e2e-private-${suffix}`,
      payload: { name: `E2E 私有专栏 ${suffix}` },
    });

    const other = await call(request, "GET", `/api/v1/workspace/content/article_column/${column.id}`, {
      token: editorToken,
    });
    expect(other.status).toBe(403);
    expect(other.data.code).toBe("forbidden");

    const ghost = await call(
      request,
      "GET",
      "/api/v1/workspace/content/article_column/00000000-0000-4000-8000-000000000000",
      { token: editorToken },
    );
    expect(ghost.status).toBe(403);
    expect(ghost.data.code).toBe(other.data.code);
    expect(ghost.data.message).toBe(other.data.message);

    const writeAttempt = await call(request, "POST", `/api/v1/workspace/content/article_column/${column.id}/submit`, {
      token: editorToken,
    });
    expect(writeAttempt.status).toBe(403);

    const adminOnly = await call(request, "GET", "/api/v1/admin/reviews", { token: editorToken });
    expect(adminOnly.status).toBe(403);

    await trashContent(request, adminToken, "article_column", column.id);
    await purgeContent(request, adminToken, "article_column", column.id);
  });

  test("首页推荐：N=0 隐藏、自动补位与非法配置", async ({ request }) => {
    const original = await call(request, "GET", "/api/v1/admin/recommendations", { token: adminToken });
    expect(original.status, JSON.stringify(original.data)).toBe(200);
    const restore = {
      items: (original.data.items as any[]).map((item) => ({
        position: item.position,
        entityType: item.entityType,
        entityId: item.entityId,
      })),
      n1: original.data.n1,
      n2: original.data.n2,
      autoFill: original.data.autoFill,
    };

    try {
      const zero = await call(request, "PUT", "/api/v1/admin/recommendations", {
        token: adminToken,
        data: { items: [], n1: 0, n2: 0, autoFill: false },
      });
      expect(zero.status, JSON.stringify(zero.data)).toBe(200);
      const hidden = await call(request, "GET", "/api/v1/home/recommendations");
      expect(hidden.status).toBe(200);
      expect(hidden.data.n1).toBe(0);
      expect(hidden.data.n2).toBe(0);
      expect(hidden.data.items).toHaveLength(0);

      const auto = await call(request, "PUT", "/api/v1/admin/recommendations", {
        token: adminToken,
        data: { items: [], n1: 3, n2: 2, autoFill: true },
      });
      expect(auto.status, JSON.stringify(auto.data)).toBe(200);
      const filled = await call(request, "GET", "/api/v1/home/recommendations");
      expect(filled.data.autoFill).toBe(true);
      expect(filled.data.items.length).toBeGreaterThan(0);
      expect(filled.data.items.length).toBeLessThanOrEqual(5);

      const invalid = await call(request, "PUT", "/api/v1/admin/recommendations", {
        token: adminToken,
        data: { items: [], n1: 13, n2: 0, autoFill: false },
      });
      expect(invalid.status).toBe(400);
      expect(invalid.data.code).toBe("validation_error");
    } finally {
      const restored = await call(request, "PUT", "/api/v1/admin/recommendations", {
        token: adminToken,
        data: restore,
      });
      expect(restored.status, JSON.stringify(restored.data)).toBe(200);
    }
  });

  test("审批与回收站操作幂等", async ({ request }) => {
    const suffix = uniqueSuffix();
    const column = await createContent(request, editorToken, {
      type: "article_column",
      slug: `e2e-idem-${suffix}`,
      payload: { name: `E2E 幂等专栏 ${suffix}` },
    });
    const submitted = await call(request, "POST", `/api/v1/workspace/content/article_column/${column.id}/submit`, {
      token: editorToken,
    });
    expect(submitted.status).toBe(200);

    const duplicateSubmit = await call(
      request,
      "POST",
      `/api/v1/workspace/content/article_column/${column.id}/submit`,
      { token: editorToken },
    );
    expect(duplicateSubmit.status).toBe(409);

    const reviews = await call(request, "GET", "/api/v1/admin/reviews", { token: adminToken });
    const item = (reviews.data.items as any[]).find((entry) => entry.title === `E2E 幂等专栏 ${suffix}`);
    expect(item).toBeTruthy();

    const approved = await call(request, "POST", `/api/v1/admin/reviews/${item.versionId}/approve`, {
      token: adminToken,
      data: { comment: "" },
    });
    expect(approved.status).toBe(200);

    const approveAgain = await call(request, "POST", `/api/v1/admin/reviews/${item.versionId}/approve`, {
      token: adminToken,
      data: { comment: "" },
    });
    expect(approveAgain.status).toBe(409);
    expect(approveAgain.data.code).toBe("conflict");

    const trashed = await call(request, "POST", `/api/v1/workspace/content/article_column/${column.id}/trash`, {
      token: editorToken,
    });
    expect(trashed.status).toBe(200);

    const trashAgain = await call(request, "POST", `/api/v1/workspace/content/article_column/${column.id}/trash`, {
      token: editorToken,
    });
    expect(trashAgain.status).toBe(409);

    const purged = await call(request, "DELETE", `/api/v1/admin/content/article_column/${column.id}`, {
      token: adminToken,
    });
    expect(purged.status).toBe(200);

    const purgeAgain = await call(request, "DELETE", `/api/v1/admin/content/article_column/${column.id}`, {
      token: adminToken,
    });
    expect(purgeAgain.status).toBe(404);
  });
});

test.describe("权限与设备分级", () => {
  test("移动端高风险接口被拒绝", async ({ request }) => {
    const status = await call(request, "PATCH", `/api/v1/admin/users/${crypto.randomUUID()}/status`, {
      token: adminToken,
      device: "mobile",
      data: { status: "disabled" },
    });
    expect(status.status).toBe(403);
    expect(status.data.code).toBe("desktop_required");

    const purge = await call(
      request,
      "DELETE",
      "/api/v1/admin/content/article/00000000-0000-4000-8000-000000000000",
      { token: adminToken, device: "mobile" },
    );
    expect(purge.status).toBe(403);
    expect(purge.data.code).toBe("desktop_required");
  });

  test("撤权与禁用后旧 token 立即失效", async ({ request }) => {
    const suffix = uniqueSuffix();
    const username = `e2e_revoke_${suffix}`;
    const password = "E2Erevoke2026!";
    const created = await call(request, "POST", "/api/v1/admin/users", {
      token: adminToken,
      data: { username, email: `${username}@test.local`, password, role: "editor" },
    });
    expect(created.status).toBe(201);
    const userID = created.data.id as string;

    const token = await loginToken(request, username, password);
    const allowed = await call(request, "GET", "/api/v1/workspace/summary", { token });
    expect(allowed.status).toBe(200);

    const revoked = await call(request, "PATCH", `/api/v1/admin/users/${userID}/role`, {
      token: adminToken,
      data: { role: "user" },
    });
    expect(revoked.status).toBe(200);
    const afterRevoke = await call(request, "GET", "/api/v1/workspace/summary", { token });
    expect(afterRevoke.status).toBe(403);
    const adminAfterRevoke = await call(request, "GET", "/api/v1/admin/reviews", { token });
    expect(adminAfterRevoke.status).toBe(403);

    const regranted = await call(request, "PATCH", `/api/v1/admin/users/${userID}/role`, {
      token: adminToken,
      data: { role: "editor" },
    });
    expect(regranted.status).toBe(200);

    const disabled = await call(request, "PATCH", `/api/v1/admin/users/${userID}/status`, {
      token: adminToken,
      data: { status: "disabled" },
    });
    expect(disabled.status).toBe(200);
    const afterDisable = await call(request, "GET", "/api/v1/workspace/summary", { token });
    expect([401, 403]).toContain(afterDisable.status);

    const loginAfterDisable = await call(request, "POST", "/api/v1/auth/login", {
      data: { username, password, remember: false },
    });
    expect(loginAfterDisable.status).toBe(401);
  });
});

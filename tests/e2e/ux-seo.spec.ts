import { expect, test } from "@playwright/test";
import {
  ADMIN_PASSWORD,
  ADMIN_USER,
  buildWeb,
  loginUI,
  readDistFile,
  WEB_URL,
} from "./helpers";

const publicPages = [
  "/",
  "/articles",
  "/articles/tech/go-observability",
  "/photos",
  "/photos/street/neon-rain",
  "/search",
];

async function expectNoHorizontalScroll(page: import("@playwright/test").Page) {
  const overflow = await page.evaluate(() => {
    const root = document.documentElement;
    return {
      scrollWidth: root.scrollWidth,
      clientWidth: root.clientWidth,
    };
  });
  expect(
    overflow.scrollWidth,
    `横向溢出：${overflow.scrollWidth} > ${overflow.clientWidth}`,
  ).toBeLessThanOrEqual(overflow.clientWidth + 1);
}

test.describe("响应式：手机 / 平板 / 桌面", () => {
  for (const viewport of [
    { name: "手机", width: 390, height: 844 },
    { name: "平板", width: 834, height: 1112 },
    { name: "桌面", width: 1440, height: 900 },
  ]) {
    test(`${viewport.name}视口下公开站无横向滚动`, async ({ page }) => {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      for (const url of publicPages) {
        await page.goto(`${WEB_URL}${url}`);
        await expect(page.locator("body")).toBeVisible();
        await expectNoHorizontalScroll(page);
      }
    });
  }

  test("手机端管理后台表格不撑破页面", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await loginUI(page, process.env.E2E_EDITOR_USER ?? "e2e_editor", process.env.E2E_EDITOR_PASSWORD ?? "E2Eeditor2026!");
    await page.goto("/editor/workspace/article");
    await expect(page.getByRole("heading", { name: "编辑工作区" })).toBeVisible();
    await expectNoHorizontalScroll(page);
    await page.goto("/editor/notifications");
    await expectNoHorizontalScroll(page);
  });
});

test.describe("可访问性", () => {
  test("登录表单可用键盘操作且焦点可见", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByRole("heading", { name: "登录后台" })).toBeVisible();
    await page.keyboard.press("Tab");
    const focused = await page.evaluate(() => {
      const element = document.activeElement as HTMLElement | null;
      if (!element) return null;
      const style = getComputedStyle(element);
      return {
        tag: element.tagName,
        autocomplete: element.getAttribute("autocomplete"),
        outline: style.outlineStyle,
        shadow: style.boxShadow,
      };
    });
    expect(focused?.autocomplete).toBe("username");
    expect(focused?.outline !== "none" || focused?.shadow !== "none").toBe(true);

    await page.keyboard.press("Tab");
    const second = await page.evaluate(() => {
      const element = document.activeElement as HTMLElement | null;
      return element?.getAttribute("autocomplete") ?? "";
    });
    expect(second).toBe("current-password");
  });

  test("正文与次要文字的对比度达到 AA", async ({ page }) => {
    await page.goto(`${WEB_URL}/articles/tech/go-observability`);
    const ratios = await page.evaluate(() => {
      const parse = (value: string) => {
        const match = value.match(/\d+(\.\d+)?/g)?.map(Number) ?? [];
        return { r: match[0] ?? 0, g: match[1] ?? 0, b: match[2] ?? 0 };
      };
      const channel = (value: number) => {
        const normalized = value / 255;
        return normalized <= 0.03928 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
      };
      const luminance = (color: { r: number; g: number; b: number }) =>
        0.2126 * channel(color.r) + 0.7152 * channel(color.g) + 0.0722 * channel(color.b);
      const ratio = (foreground: string, background: string) => {
        const a = luminance(parse(foreground));
        const b = luminance(parse(background));
        const lighter = Math.max(a, b);
        const darker = Math.min(a, b);
        return (lighter + 0.05) / (darker + 0.05);
      };
      const bodyStyle = getComputedStyle(document.body);
      const background = bodyStyle.backgroundColor;
      const paragraph = document.querySelector<HTMLElement>(".prose p") ?? document.body;
      const muted = document.querySelector<HTMLElement>(".eyebrow");
      return {
        body: ratio(getComputedStyle(paragraph).color, background),
        muted: muted ? ratio(getComputedStyle(muted).color, background) : 21,
      };
    });
    expect(ratios.body).toBeGreaterThanOrEqual(4.5);
    expect(ratios.muted).toBeGreaterThanOrEqual(4.5);
  });

  test("减少动态效果时揭示元素直接可见且无过渡", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(`${WEB_URL}/`);
    const state = await page.evaluate(() => {
      const nodes = Array.from(document.querySelectorAll<HTMLElement>(".reveal"));
      return nodes.map((node) => {
        const style = getComputedStyle(node);
        return { opacity: style.opacity, duration: style.transitionDuration };
      });
    });
    expect(state.length).toBeGreaterThan(0);
    for (const item of state) {
      expect(item.opacity).toBe("1");
      expect(["0s", "0s, 0s", "0s, 0s, 0s"]).toContain(item.duration);
    }
  });

  test("相册查看器支持键盘关闭与左右切换", async ({ page }) => {
    await page.goto(`${WEB_URL}/photos/street/neon-rain`);
    await page.waitForFunction(() => !document.querySelector("astro-island[ssr]"), null, { timeout: 15_000 });
    await page.locator(".photo-tile").first().click();
    const viewer = page.getByRole("dialog");
    await expect(viewer).toBeVisible();
    await expect(page.getByRole("button", { name: "关闭照片查看" })).toBeFocused();
    await page.keyboard.press("ArrowRight");
    await expect(page.locator(".viewer__count")).toContainText("2 /");
    await page.keyboard.press("Escape");
    await expect(viewer).toBeHidden();
  });
});

test.describe("SEO", () => {
  test("robots 与页面 canonical / 分享卡片", async ({ page }) => {
    const robots = await page.request.get("http://localhost:4321/robots.txt");
    expect(robots.status()).toBe(200);
    const robotsBody = await robots.text();
    expect(robotsBody).toContain("User-agent: *");
    expect(robotsBody).toContain("Sitemap: https://www.twilightlog.com/sitemap-index.xml");

    await page.goto(`${WEB_URL}/articles/tech/go-observability`);
    const canonical = await page.locator('link[rel="canonical"]').getAttribute("href");
    expect(canonical).toBe("https://www.twilightlog.com/articles/tech/go-observability");
    const ogTitle = await page.locator('meta[property="og:title"]').getAttribute("content");
    expect(ogTitle).toContain("Go 服务的可观测性入门");
    const description = await page.locator('meta[name="description"]').getAttribute("content");
    expect(description && description.length).toBeGreaterThan(10);

    const jsonLd = await page.locator('script[type="application/ld+json"]').first().textContent();
    expect(jsonLd).toContain("BlogPosting");
  });

  test("不存在的页面返回 404 且 noindex", async ({ page }) => {
    const response = await page.goto(`${WEB_URL}/articles/tech/definitely-not-here`);
    expect(response?.status()).toBe(404);
    await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);
    await expect(page.getByRole("heading", { level: 1, name: "404" })).toBeVisible();
    await expect(page.getByText("这一页可能已经下线，或者从未存在过。")).toBeVisible();
  });

  test("构建产物包含 sitemap 且覆盖文章与相册", async ({ request }) => {
    test.setTimeout(180_000);
    const dist = buildWeb();
    const index = readDistFile(dist, "sitemap-index.xml");
    expect(index).not.toBeNull();
    expect(index!).toContain("<sitemapindex");
    const sitemap = readDistFile(dist, "sitemap-0.xml");
    expect(sitemap).not.toBeNull();
    expect(sitemap!).toContain("/articles/tech/go-observability");
    expect(sitemap!).toContain("/photos/street/neon-rain");
    expect(sitemap!).not.toContain("/search");

    const home = await request.get("http://localhost:4321/");
    expect(home.status()).toBe(200);
  });
});

test.describe("手机端高风险操作不可用", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("账号页只读、回收站不提供永久删除", async ({ page }) => {
    await loginUI(page, ADMIN_USER, ADMIN_PASSWORD);
    await page.goto("/admin/users");
    await expect(page.getByText("手机端账号页为只读")).toBeVisible();
    await expect(page.getByRole("button", { name: "创建账号" })).toBeDisabled();

    await page.goto("/admin/recycle");
    await expect(page.getByRole("heading", { name: "回收站" })).toBeVisible();
    await expect(page.getByRole("button", { name: "永久删除" })).toHaveCount(0);
  });

  test("手机端无法通过接口规避限制", async ({ request }) => {
    const login = await request.post("http://localhost:8080/api/v1/auth/login", {
      data: { username: ADMIN_USER, password: ADMIN_PASSWORD, remember: false },
    });
    expect(login.status()).toBe(200);
    const token = (await login.json()).accessToken as string;
    const response = await request.patch(
      "http://localhost:8080/api/v1/admin/users/00000000-0000-4000-8000-000000000000/status",
      {
        headers: {
          Authorization: `Bearer ${token}`,
          "X-Client-Device": "mobile",
          "Content-Type": "application/json",
        },
        data: { status: "disabled" },
        failOnStatusCode: false,
      },
    );
    expect(response.status()).toBe(403);
    expect((await response.json()).code).toBe("desktop_required");
  });
});

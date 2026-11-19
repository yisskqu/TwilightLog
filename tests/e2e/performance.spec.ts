import { expect, test } from "@playwright/test";
import { absoluteUrl, API_URL, call, WEB_URL } from "./helpers";

function percentile(values: number[], p: number): number {
  const sorted = [...values].sort((a, b) => a - b);
  const index = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1);
  return sorted[Math.max(0, index)];
}

test("公开 API p95 延迟", async ({ request }) => {
  const endpoints = [
    "/api/v1/home/recommendations",
    "/api/v1/article-columns?pageSize=12",
    "/api/v1/photos?pageSize=12",
    "/api/v1/albums/neon-rain/photos?pageSize=12",
    "/api/v1/articles/go-observability",
    "/api/v1/search?q=雨",
  ];
  const summary: Record<string, { p50: number; p95: number }> = {};

  for (const endpoint of endpoints) {
    const samples: number[] = [];
    // 预热一次，避免冷启动影响分位数
    await call(request, "GET", endpoint);
    for (let index = 0; index < 20; index += 1) {
      const started = Date.now();
      const result = await call(request, "GET", endpoint);
      expect(result.status, endpoint).toBe(200);
      samples.push(Date.now() - started);
    }
    const p50 = percentile(samples, 50);
    const p95 = percentile(samples, 95);
    summary[endpoint] = { p50, p95 };
    console.log(`API ${endpoint} p50=${p50}ms p95=${p95}ms`);
    expect(p95, `${endpoint} p95 过高`).toBeLessThan(500);
  }
  console.log(`API 延迟汇总 ${JSON.stringify(summary)}`);
});

test("首屏加载、图片懒加载与布局稳定性", async ({ page, request }) => {
  test.setTimeout(120_000);

  const pages = ["/", "/articles", "/articles/tech/go-observability", "/photos/street/neon-rain"];
  for (const url of pages) {
    await page.addInitScript(() => {
      (window as any).__cls = 0;
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries() as any[]) {
          if (!entry.hadRecentInput) (window as any).__cls += entry.value;
        }
      }).observe({ type: "layout-shift", buffered: true });
    });
    const started = Date.now();
    await page.goto(`${WEB_URL}${url}`, { waitUntil: "load" });
    const elapsed = Date.now() - started;
    await page.waitForTimeout(800);
    const cls = await page.evaluate(() => (window as any).__cls as number);
    const htmlSize = (await page.content()).length;
    console.log(`页面 ${url} load=${elapsed}ms CLS=${cls.toFixed(4)} HTML=${htmlSize}B`);
    expect(elapsed, `${url} 首屏过慢`).toBeLessThan(5000);
    expect(cls, `${url} 布局抖动`).toBeLessThan(0.1);
    expect(htmlSize, `${url} HTML 过大`).toBeLessThan(400_000);
  }

  await page.goto(`${WEB_URL}/photos/street/neon-rain`);
  const lazy = await page.locator("img[loading='lazy']").count();
  const total = await page.locator(".photo-tile img").count();
  expect(total).toBeGreaterThan(0);
  expect(lazy).toBe(total);
  const sized = await page.locator(".photo-tile img").evaluateAll((nodes) =>
    nodes.every((node) => node.getAttribute("width") && node.getAttribute("height")),
  );
  expect(sized, "图片缺少宽高导致布局抖动").toBe(true);

  // 衍生图体积控制在 960 宽档位
  const photos = await call(request, "GET", "/api/v1/albums/neon-rain/photos?pageSize=1");
  const variant = photos.data.items[0].media.variants.find((item: any) => item.name === "w960");
  expect(variant).toBeTruthy();
  const response = await request.get(absoluteUrl(variant.url));
  expect(response.status()).toBe(200);
  const body = await response.body();
  console.log(`w960 衍生图 ${body.length}B`);
  expect(body.length).toBeLessThan(600 * 1024);
  expect(API_URL).toContain("localhost");
});

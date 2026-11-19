import type { components } from "@twilightlog/api-client";

export type Asset = components["schemas"]["Asset"];
export type AssetVariant = components["schemas"]["AssetVariant"];
export type ArticleColumn = components["schemas"]["ArticleColumn"];
export type ArticleSummary = components["schemas"]["ArticleSummary"];
export type ArticleDetail = components["schemas"]["ArticleDetail"];
export type PhotoColumn = components["schemas"]["PhotoColumn"];
export type Album = components["schemas"]["Album"];
export type PhotoSummary = components["schemas"]["PhotoSummary"];
export type PhotoDetail = components["schemas"]["PhotoDetail"];
export type SearchHit = components["schemas"]["SearchHit"];
export type Recommendation = components["schemas"]["Recommendation"];

export interface Page<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
}

export interface ColumnPage<T> extends Page<T> {
  column: { slug: string; name: string };
}

/**
 * Server-side API origin used while generating static pages.
 * The browser always talks to the same origin through Caddy.
 */
export const API_BASE_URL = (
  import.meta.env.API_BASE_URL ?? "http://localhost:8080"
).replace(/\/+$/, "");

/** Browser-side API prefix. Empty means same-origin `/api/v1`. */
export const PUBLIC_API_BASE = "";

const EMPTY_PAGE: Page<never> = {
  items: [],
  page: 1,
  pageSize: 12,
  total: 0,
  totalPages: 0,
};

async function getJSON<T>(path: string): Promise<T | null> {
  const url = `${API_BASE_URL}${path}`;
  try {
    const response = await fetch(url, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      console.warn(`[content] ${response.status} ${response.statusText} · ${url}`);
      return null;
    }
    return (await response.json()) as T;
  } catch (error) {
    console.warn(`[content] 请求失败，构建将使用空数据 · ${url}`, error);
    return null;
  }
}

function withQuery(path: string, params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") continue;
    search.set(key, String(value));
  }
  const query = search.toString();
  return query ? `${path}?${query}` : path;
}

export async function getRecommendations(limit = 10): Promise<Recommendation[]> {
  const data = await getJSON<{ items: Recommendation[] }>(
    withQuery("/api/v1/home/recommendations", { limit }),
  );
  return data?.items ?? [];
}

export async function listArticleColumns(pageSize = 50): Promise<Page<ArticleColumn>> {
  return (
    (await getJSON<Page<ArticleColumn>>(
      withQuery("/api/v1/article-columns", { pageSize }),
    )) ?? (EMPTY_PAGE as Page<ArticleColumn>)
  );
}

export async function listColumnArticles(
  slug: string,
  pageSize = 12,
  page = 1,
): Promise<ColumnPage<ArticleSummary>> {
  return (
    (await getJSON<ColumnPage<ArticleSummary>>(
      withQuery(`/api/v1/article-columns/${encodeURIComponent(slug)}/articles`, {
        pageSize,
        page,
      }),
    )) ?? { ...(EMPTY_PAGE as Page<ArticleSummary>), column: { slug, name: slug } }
  );
}

export async function getArticle(slug: string): Promise<ArticleDetail | null> {
  return getJSON<ArticleDetail>(`/api/v1/articles/${encodeURIComponent(slug)}`);
}

export async function listPhotoColumns(pageSize = 50): Promise<Page<PhotoColumn>> {
  return (
    (await getJSON<Page<PhotoColumn>>(withQuery("/api/v1/photo-columns", { pageSize }))) ??
    (EMPTY_PAGE as Page<PhotoColumn>)
  );
}

export async function listColumnAlbums(
  slug: string,
  pageSize = 50,
  page = 1,
): Promise<ColumnPage<Album>> {
  return (
    (await getJSON<ColumnPage<Album>>(
      withQuery(`/api/v1/photo-columns/${encodeURIComponent(slug)}/albums`, {
        pageSize,
        page,
      }),
    )) ?? { ...(EMPTY_PAGE as Page<Album>), column: { slug, name: slug } }
  );
}

export async function getAlbum(slug: string): Promise<Album | null> {
  return getJSON<Album>(`/api/v1/albums/${encodeURIComponent(slug)}`);
}

export async function listAlbumPhotos(slug: string, pageSize = 50): Promise<Page<PhotoSummary>> {
  return (
    (await getJSON<Page<PhotoSummary>>(
      withQuery(`/api/v1/albums/${encodeURIComponent(slug)}/photos`, { pageSize }),
    )) ?? (EMPTY_PAGE as Page<PhotoSummary>)
  );
}

export async function listPhotos(pageSize = 12, page = 1): Promise<Page<PhotoSummary>> {
  return (
    (await getJSON<Page<PhotoSummary>>(withQuery("/api/v1/photos", { pageSize, page }))) ??
    (EMPTY_PAGE as Page<PhotoSummary>)
  );
}

export async function searchContent(
  params: {
    q?: string;
    type?: string;
    tags?: string;
    dateFrom?: string;
    dateTo?: string;
    sort?: string;
    page?: number;
    pageSize?: number;
  },
  pageSize = 12,
): Promise<Page<SearchHit>> {
  return (
    (await getJSON<Page<SearchHit>>(
      withQuery("/api/v1/search", { ...params, pageSize, page: params.page ?? 1 }),
    )) ?? (EMPTY_PAGE as Page<SearchHit>)
  );
}

export async function listTags(scope: "article" | "photo"): Promise<string[]> {
  const data = await getJSON<{ items: { name: string }[] }>(
    withQuery("/api/v1/tags", { scope }),
  );
  return data?.items.map((tag) => tag.name) ?? [];
}

/** Newest public articles across every column, used for home page padding. */
export async function getLatestArticles(limit: number): Promise<ArticleSummary[]> {
  const columns = await listArticleColumns();
  const pages = await Promise.all(
    columns.items.map((column) => listColumnArticles(column.slug, limit + 4)),
  );
  const seen = new Set<string>();
  return pages
    .flatMap((page) => page.items)
    .filter((article) => {
      if (seen.has(article.id)) return false;
      seen.add(article.id);
      return true;
    })
    .sort((a, b) => Date.parse(b.publishedAt) - Date.parse(a.publishedAt))
    .slice(0, limit);
}

/** Newest albums across every photo column, used for home page padding. */
export async function getLatestAlbums(limit: number): Promise<Album[]> {
  const columns = await listPhotoColumns();
  const pages = await Promise.all(
    columns.items.map((column) => listColumnAlbums(column.slug, limit + 4)),
  );
  const seen = new Set<string>();
  return pages
    .flatMap((page) => page.items)
    .filter((album) => {
      if (seen.has(album.id)) return false;
      seen.add(album.id);
      return true;
    })
    .sort((a, b) => albumTime(b) - albumTime(a))
    .slice(0, limit);
}

export function albumTime(album: Album): number {
  return Date.parse(album.latestPhotoAt ?? album.publishedAt ?? album.shotAt ?? "");
}

import { useCallback, useEffect, useRef, useState } from "react";
import type {
  Album,
  ArticleSummary,
  Asset,
  PhotoSummary,
  SearchHit,
} from "../lib/api";
import { formatDate, imageSource } from "../lib/format";
import PhotoViewer from "./PhotoViewer";

export type ExplorerItem = ArticleSummary | Album | PhotoSummary | SearchHit;

export interface TagGroup {
  key: string;
  label: string;
  tags: string[];
}

interface ExplorerProps {
  kind: "article" | "album" | "photo" | "search";
  endpoint: string;
  initialItems: ExplorerItem[];
  initialTotal: number;
  initialTotalPages: number;
  tagGroups?: TagGroup[];
  typeFilter?: boolean;
  perPage?: number;
  emptyTitle?: string;
  emptyText?: string;
}

interface Config {
  q: string;
  tags: string[];
  dateFrom: string;
  dateTo: string;
  sort: string;
  type: string;
}

const EMPTY_CONFIG: Config = {
  q: "",
  tags: [],
  dateFrom: "",
  dateTo: "",
  sort: "latest",
  type: "",
};

const TYPE_OPTIONS = [
  { value: "", label: "全部类型" },
  { value: "article", label: "文章" },
  { value: "article_column", label: "文章专栏" },
  { value: "album", label: "相册" },
  { value: "photo", label: "照片" },
  { value: "photo_column", label: "摄影专栏" },
];

const TYPE_LABELS: Record<string, string> = {
  article: "文章",
  article_column: "文章专栏",
  album: "相册",
  photo: "照片",
  photo_column: "摄影专栏",
};

function readUrlConfig(): Config | null {
  const params = new URLSearchParams(window.location.search);
  const hasAny = ["q", "tags", "dateFrom", "dateTo", "sort", "type", "page"].some((key) =>
    params.has(key),
  );
  if (!hasAny) return null;
  return {
    q: params.get("q") ?? "",
    tags: (params.get("tags") ?? "")
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean),
    dateFrom: params.get("dateFrom") ?? "",
    dateTo: params.get("dateTo") ?? "",
    sort: params.get("sort") ?? "latest",
    type: params.get("type") ?? "",
  };
}

function urlPage(): number {
  const page = Number(new URLSearchParams(window.location.search).get("page"));
  return Number.isFinite(page) && page > 0 ? page : 1;
}

function writeUrlConfig(config: Config, page: number): void {
  const params = new URLSearchParams();
  if (config.q.trim()) params.set("q", config.q.trim());
  if (config.tags.length) params.set("tags", config.tags.join(","));
  if (config.dateFrom) params.set("dateFrom", config.dateFrom);
  if (config.dateTo) params.set("dateTo", config.dateTo);
  if (config.sort && config.sort !== "latest") params.set("sort", config.sort);
  if (config.type) params.set("type", config.type);
  if (page > 1) params.set("page", String(page));
  const query = params.toString();
  const next = `${window.location.pathname}${query ? `?${query}` : ""}`;
  window.history.replaceState(null, "", next);
}

function Cover({ asset, sizes }: { asset: Asset | null | undefined; sizes: string }) {
  const image = imageSource(asset ?? null, "w960");
  if (!image) return <div className="card-media no-image" />;
  return (
    <div className="card-media">
      <img
        src={image.src}
        srcSet={image.srcset}
        sizes={sizes}
        width={image.width}
        height={image.height}
        alt=""
        loading="lazy"
        decoding="async"
      />
    </div>
  );
}

function articleHref(article: ArticleSummary): string {
  return `/articles/${article.column.slug}/${article.slug}`;
}

function albumHref(album: Album): string {
  return `/photos/${album.column.slug}/${album.slug}`;
}

function searchHref(hit: SearchHit): string {
  switch (hit.type) {
    case "article":
      return `/articles/${hit.column.slug}/${hit.slug}`;
    case "album":
      return `/photos/${hit.column.slug}/${hit.slug}`;
    case "article_column":
      return `/articles/${hit.slug}`;
    case "photo_column":
      return `/photos/${hit.slug}`;
    case "photo":
      return `/photos/${hit.column.slug}/${hit.album.slug}`;
    default:
      return "/search";
  }
}

export default function Explorer({
  kind,
  endpoint,
  initialItems,
  initialTotal,
  initialTotalPages,
  tagGroups = [],
  typeFilter = false,
  perPage = 12,
  emptyTitle = "没有找到内容",
  emptyText = "换个关键词或清除筛选条件再试一次。",
}: ExplorerProps) {
  const [items, setItems] = useState<ExplorerItem[]>(initialItems);
  const [total, setTotal] = useState(initialTotal);
  const [totalPages, setTotalPages] = useState(initialTotalPages);
  const [page, setPage] = useState(1);
  const [config, setConfig] = useState<Config>(EMPTY_CONFIG);
  const [tagScope, setTagScope] = useState(tagGroups[0]?.key ?? "");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [viewerIndex, setViewerIndex] = useState<number | null>(null);

  const firstRender = useRef(true);
  const qEffectReady = useRef(false);
  const fromUrlQuery = useRef<string | null>(null);
  const requestId = useRef(0);

  const activeTags =
    tagGroups.find((group) => group.key === tagScope)?.tags ?? tagGroups[0]?.tags ?? [];

  const load = useCallback(
    async (nextConfig: Config, targetPage: number) => {
      if (kind === "search" && !nextConfig.q.trim()) {
        requestId.current += 1;
        setItems([]);
        setTotal(0);
        setTotalPages(0);
        setPage(1);
        setLoading(false);
        setError("");
        writeUrlConfig(nextConfig, 1);
        return;
      }
      const id = ++requestId.current;
      setLoading(true);
      setError("");
      const params = new URLSearchParams();
      if (nextConfig.q.trim()) params.set("q", nextConfig.q.trim());
      if (nextConfig.tags.length) params.set("tags", nextConfig.tags.join(","));
      if (nextConfig.dateFrom) params.set("dateFrom", nextConfig.dateFrom);
      if (nextConfig.dateTo) params.set("dateTo", nextConfig.dateTo);
      if (nextConfig.sort) params.set("sort", nextConfig.sort);
      if (nextConfig.type) params.set("type", nextConfig.type);
      params.set("page", String(targetPage));
      params.set("pageSize", String(perPage));
      try {
        const response = await fetch(`${endpoint}?${params.toString()}`, {
          headers: { Accept: "application/json" },
        });
        if (!response.ok) {
          throw new Error(`${response.status} ${response.statusText}`);
        }
        const data = await response.json();
        if (id !== requestId.current) return;
        setItems(Array.isArray(data.items) ? data.items : []);
        setTotal(typeof data.total === "number" ? data.total : 0);
        setTotalPages(typeof data.totalPages === "number" ? data.totalPages : 0);
        setPage(typeof data.page === "number" ? data.page : targetPage);
        writeUrlConfig(nextConfig, typeof data.page === "number" ? data.page : targetPage);
      } catch (loadError) {
        if (id !== requestId.current) return;
        setError(loadError instanceof Error ? loadError.message : "请求失败");
      } finally {
        if (id === requestId.current) setLoading(false);
      }
    },
    [endpoint, kind, perPage],
  );

  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      const fromUrl = readUrlConfig();
      if (fromUrl) {
        fromUrlQuery.current = fromUrl.q;
        setConfig(fromUrl);
        void load(fromUrl, urlPage());
      }
    }
  }, [load]);

  useEffect(() => {
    if (!qEffectReady.current) {
      qEffectReady.current = true;
      return;
    }
    if (fromUrlQuery.current !== null) {
      if (config.q === fromUrlQuery.current) {
        fromUrlQuery.current = null;
        return;
      }
      fromUrlQuery.current = null;
    }
    const timer = window.setTimeout(() => {
      void load(config, 1);
    }, 320);
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config.q]);

  const apply = (patch: Partial<Config>) => {
    const next = { ...config, ...patch };
    setConfig(next);
    if (!("q" in patch)) {
      void load(next, 1);
    }
  };

  const toggleTag = (tag: string) => {
    const tags = config.tags.includes(tag)
      ? config.tags.filter((item) => item !== tag)
      : [...config.tags, tag];
    apply({ tags });
  };

  const reset = () => {
    setConfig(EMPTY_CONFIG);
    setItems(initialItems);
    setTotal(initialTotal);
    setTotalPages(initialTotalPages);
    setPage(1);
    setError("");
    window.history.replaceState(null, "", window.location.pathname);
  };

  const goPage = (target: number) => {
    if (target < 1 || target > totalPages || target === page) return;
    void load(config, target);
    document.querySelector(".explorer")?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  const photos = kind === "photo" ? (items as PhotoSummary[]) : [];
  const activeFilters =
    Boolean(config.q.trim()) ||
    config.tags.length > 0 ||
    Boolean(config.dateFrom) ||
    Boolean(config.dateTo) ||
    config.sort !== "latest" ||
    Boolean(config.type);

  return (
    <div className={`explorer${loading ? " loading" : ""}`}>
      <div className="filter-bar" role="search">
        <div className="filter-row">
          <div className="field grow">
            <label htmlFor="explorer-q">关键词</label>
            <input
              id="explorer-q"
              type="search"
              placeholder="搜索标题、摘要或标签…"
              value={config.q}
              onChange={(event) => setConfig({ ...config, q: event.target.value })}
            />
          </div>
          {typeFilter ? (
            <div className="field">
              <label htmlFor="explorer-type">类型</label>
              <select
                id="explorer-type"
                value={config.type}
                onChange={(event) => apply({ type: event.target.value })}
              >
                {TYPE_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
          ) : null}
          <div className="field">
            <label htmlFor="explorer-from">起始日期</label>
            <input
              id="explorer-from"
              type="date"
              value={config.dateFrom}
              onChange={(event) => apply({ dateFrom: event.target.value })}
            />
          </div>
          <div className="field">
            <label htmlFor="explorer-to">结束日期</label>
            <input
              id="explorer-to"
              type="date"
              value={config.dateTo}
              onChange={(event) => apply({ dateTo: event.target.value })}
            />
          </div>
          <div className="field">
            <label htmlFor="explorer-sort">排序</label>
            <select
              id="explorer-sort"
              value={config.sort}
              onChange={(event) => apply({ sort: event.target.value })}
            >
              <option value="latest">最新优先</option>
              <option value="oldest">最早优先</option>
            </select>
          </div>
          <div className="filter-actions">
            <button type="button" className="btn ghost" onClick={reset} disabled={!activeFilters}>
              重置
            </button>
          </div>
        </div>

        {tagGroups.length ? (
          <div className="filter-row">
            {tagGroups.length > 1 ? (
              <div className="field">
                <label htmlFor="explorer-scope">标签范围</label>
                <select
                  id="explorer-scope"
                  value={tagScope}
                  onChange={(event) => {
                    setTagScope(event.target.value);
                    if (config.tags.length) apply({ tags: [] });
                  }}
                >
                  {tagGroups.map((group) => (
                    <option key={group.key} value={group.key}>
                      {group.label}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
            <div className="field grow">
              <label>标签（同时包含，多选）</label>
              <div className="tag-picker">
                {activeTags.map((tag) => (
                  <button
                    key={tag}
                    type="button"
                    className="tag-toggle"
                    aria-pressed={config.tags.includes(tag)}
                    onClick={() => toggleTag(tag)}
                  >
                    {tag}
                  </button>
                ))}
              </div>
            </div>
          </div>
        ) : null}
      </div>

      <div className="result-meta" aria-live="polite">
        <span>
          {loading ? "正在检索…" : `共 ${total} 项`}
          {activeFilters ? " · 已应用筛选" : ""}
        </span>
        {error ? (
          <span style={{ color: "var(--danger)" }}>加载失败：{error}</span>
        ) : null}
      </div>

      {items.length === 0 && !loading ? (
        <div className="empty-state">
          <strong>{emptyTitle}</strong>
          <span>{emptyText}</span>
          {activeFilters ? (
            <p style={{ marginTop: 16 }}>
              <button type="button" className="btn" onClick={reset}>
                清除筛选
              </button>
            </p>
          ) : null}
        </div>
      ) : null}

      {kind === "article" ? (
        <div className="card-grid result-grid">
          {(items as ArticleSummary[]).map((article) => (
            <a className="card" key={article.id} href={articleHref(article)}>
              <Cover
                asset={article.cover}
                sizes="(max-width: 640px) 100vw, (max-width: 960px) 50vw, 33vw"
              />
              <div className="card-body">
                <p className="eyebrow">{article.column.name}</p>
                <h3>{article.title}</h3>
                <p>{article.summary}</p>
                <p className="meta">
                  {formatDate(article.publishedAt)}
                  {article.tags.length ? ` · ${article.tags.slice(0, 2).join(" / ")}` : ""}
                </p>
              </div>
            </a>
          ))}
        </div>
      ) : null}

      {kind === "album" ? (
        <div className="card-grid result-grid">
          {(items as Album[]).map((album) => (
            <a className="card" key={album.id} href={albumHref(album)}>
              <Cover
                asset={album.cover}
                sizes="(max-width: 640px) 100vw, (max-width: 960px) 50vw, 33vw"
              />
              <div className="card-body">
                <p className="eyebrow">
                  {album.column.name}
                  {album.location ? ` · ${album.location}` : ""}
                </p>
                <h3>{album.title}</h3>
                <p>{album.description}</p>
                <p className="meta">{album.photoCount} 张</p>
              </div>
            </a>
          ))}
        </div>
      ) : null}

      {kind === "photo" ? (
        <div className="photo-masonry result-grid">
          {(items as PhotoSummary[]).map((photo, index) => {
            const image = imageSource(photo.media, "w960");
            return (
              <button
                type="button"
                className="photo-tile"
                key={photo.id}
                onClick={() => setViewerIndex(index)}
              >
                {image ? (
                  <img
                    src={image.src}
                    srcSet={image.srcset}
                    sizes="(max-width: 560px) 100vw, (max-width: 1024px) 50vw, 33vw"
                    width={image.width}
                    height={image.height}
                    alt={photo.title}
                    loading="lazy"
                    decoding="async"
                  />
                ) : null}
                <span className="caption">
                  <strong>{photo.title}</strong>
                  <span>
                    {photo.album.name}
                    {photo.location ? ` · ${photo.location}` : ""}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      ) : null}

      {kind === "search" ? (
        <div className="card-grid result-grid">
          {(items as SearchHit[]).map((hit) => (
            <a className="card" key={`${hit.type}-${hit.id}`} href={searchHref(hit)}>
              <Cover
                asset={hit.cover}
                sizes="(max-width: 640px) 100vw, (max-width: 960px) 50vw, 33vw"
              />
              <div className="card-body">
                <p className="eyebrow">
                  {TYPE_LABELS[hit.type] ?? hit.type}
                  {hit.column?.name ? ` · ${hit.column.name}` : ""}
                </p>
                <h3>{hit.title}</h3>
                <p>{hit.summary}</p>
                <p className="meta">{formatDate(hit.publishedAt)}</p>
              </div>
            </a>
          ))}
        </div>
      ) : null}

      {totalPages > 1 ? (
        <nav className="pagination" aria-label="分页">
          <button type="button" onClick={() => goPage(page - 1)} disabled={page <= 1}>
            上一页
          </button>
          {pageNumbers(page, totalPages).map((item, index) =>
            item === "…" ? (
              <button key={`gap-${index}`} type="button" disabled>
                …
              </button>
            ) : (
              <button
                key={item}
                type="button"
                aria-current={item === page ? "page" : undefined}
                onClick={() => goPage(item)}
              >
                {item}
              </button>
            ),
          )}
          <button type="button" onClick={() => goPage(page + 1)} disabled={page >= totalPages}>
            下一页
          </button>
        </nav>
      ) : null}

      {kind === "photo" ? (
        <PhotoViewer
          photos={photos}
          index={viewerIndex}
          onIndexChange={setViewerIndex}
          onClose={() => setViewerIndex(null)}
        />
      ) : null}
    </div>
  );
}

function pageNumbers(current: number, total: number): Array<number | "…"> {
  if (total <= 7) {
    return Array.from({ length: total }, (_, index) => index + 1);
  }
  const pages: Array<number | "…"> = [1];
  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  if (start > 2) pages.push("…");
  for (let page = start; page <= end; page += 1) pages.push(page);
  if (end < total - 1) pages.push("…");
  pages.push(total);
  return pages;
}

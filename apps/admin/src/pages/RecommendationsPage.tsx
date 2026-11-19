import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  apiRequest,
  type Candidate,
  type RecommendationItem,
  type RecommendationSettings,
} from "../api";

export function RecommendationsPage() {
  const [settings, setSettings] = useState<RecommendationSettings>({ n1: 3, n2: 3, autoFill: true, items: [] });
  const [articles, setArticles] = useState<Candidate[]>([]);
  const [albums, setAlbums] = useState<Candidate[]>([]);
  const [pickArticle, setPickArticle] = useState("");
  const [pickAlbum, setPickAlbum] = useState("");
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [current, articleList, albumList] = await Promise.all([
        apiRequest<RecommendationSettings>("/api/v1/admin/recommendations"),
        apiRequest<{ items: Candidate[] }>("/api/v1/admin/recommendations/candidates?type=article"),
        apiRequest<{ items: Candidate[] }>("/api/v1/admin/recommendations/candidates?type=album"),
      ]);
      setSettings(current);
      setArticles(articleList.items);
      setAlbums(albumList.items);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载推荐配置失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const articleCount = settings.items.filter((item) => item.entityType === "article").length;
  const albumCount = settings.items.filter((item) => item.entityType === "album").length;

  function addItem(kind: "article" | "album") {
    const id = kind === "article" ? pickArticle : pickAlbum;
    if (!id) {
      return;
    }
    if (settings.items.some((item) => item.entityType === kind && item.entityId === id)) {
      setError("该内容已在推荐列表中");
      return;
    }
    setSettings((current) => ({
      ...current,
      items: [...current.items, { position: current.items.length + 1, entityType: kind, entityId: id, title: "" }],
    }));
    if (kind === "article") {
      setPickArticle("");
    } else {
      setPickAlbum("");
    }
  }

  function moveItem(from: number, to: number) {
    if (to < 0 || to >= settings.items.length || from === to) {
      return;
    }
    const next = [...settings.items];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    setSettings((current) => ({ ...current, items: next.map((entry, index) => ({ ...entry, position: index + 1 })) }));
  }

  function titleOf(item: RecommendationItem): string {
    if (item.title) {
      return item.title;
    }
    const pool = item.entityType === "article" ? articles : albums;
    return pool.find((entry) => entry.entityId === item.entityId)?.title ?? item.entityId.slice(0, 8);
  }

  async function save(next?: RecommendationSettings) {
    const payload = next ?? settings;
    if (next) {
      setSettings(next);
    }
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await apiRequest("/api/v1/admin/recommendations", {
        method: "PUT",
        body: JSON.stringify({
          n1: payload.n1,
          n2: payload.n2,
          autoFill: payload.autoFill,
          items: payload.items.map((item, index) => ({
            position: index + 1,
            entityType: item.entityType,
            entityId: item.entityId,
          })),
        }),
      });
      setMessage("推荐配置已保存，公开首页下次构建后生效");
      await load();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "保存失败");
    } finally {
      setBusy(false);
    }
  }

  const overflowArticles = settings.items.filter((item) => item.entityType === "article").slice(settings.n1);
  const overflowAlbums = settings.items.filter((item) => item.entityType === "album").slice(settings.n2);
  const overflow =
    overflowArticles.length > 0 || overflowAlbums.length > 0
      ? `当前已选文章 ${articleCount} 篇、相册 ${albumCount} 个，超过 N₁/N₂ 上限。保存前需要移除：${[
          ...overflowArticles,
          ...overflowAlbums,
        ]
          .map((item) => titleOf(item))
          .join("、")}`
      : "";

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">CURATION</span>
          <h1>首页推荐</h1>
          <p className="subtitle">N₁ 控制推荐文章数量，N₂ 控制推荐相册数量。未配置时可按最新公开内容自动补位。</p>
        </div>
        <div className="head-actions">
          <button
            type="button"
            className="btn"
            disabled={busy}
            onClick={() => {
              if (window.confirm("清空手工推荐并恢复全自动补位？")) {
                void save({ n1: settings.n1, n2: settings.n2, autoFill: true, items: [] });
              }
            }}
          >
            清空并恢复全自动
          </button>
        </div>
      </header>

      {error ? <div className="alert">{error}</div> : null}
      {message ? <div className="notice">{message}</div> : null}
      {overflow ? <div className="alert">{overflow}</div> : null}

      {loading ? (
        <p className="muted">加载中…</p>
      ) : (
        <div className="reco-layout">
          <div className="panel">
            <h2>数量与补位</h2>
            <div className="inline-fields">
              <label className="field">
                <span>N₁ 推荐文章数</span>
                <input
                  type="number"
                  min={0}
                  max={12}
                  value={settings.n1}
                  onChange={(event) =>
                    setSettings((current) => ({ ...current, n1: Number(event.target.value) }))
                  }
                />
              </label>
              <label className="field">
                <span>N₂ 推荐相册数</span>
                <input
                  type="number"
                  min={0}
                  max={12}
                  value={settings.n2}
                  onChange={(event) =>
                    setSettings((current) => ({ ...current, n2: Number(event.target.value) }))
                  }
                />
              </label>
            </div>
            <label className="checkbox">
              <input
                type="checkbox"
                checked={settings.autoFill}
                onChange={(event) => setSettings((current) => ({ ...current, autoFill: event.target.checked }))}
              />
              未配置时自动用最新公开内容补位
            </label>
            <p className="muted small-text">
              已选文章 {articleCount} / {settings.n1}，相册 {albumCount} / {settings.n2}
            </p>

            <h2>添加内容</h2>
            <div className="add-row">
              <select value={pickArticle} onChange={(event) => setPickArticle(event.target.value)}>
                <option value="">选择文章</option>
                {articles.map((item) => (
                  <option key={item.entityId} value={item.entityId}>
                    {item.title || item.entityId.slice(0, 8)}
                  </option>
                ))}
              </select>
              <button type="button" className="btn small" onClick={() => addItem("article")}>
                添加文章
              </button>
            </div>
            <div className="add-row">
              <select value={pickAlbum} onChange={(event) => setPickAlbum(event.target.value)}>
                <option value="">选择相册</option>
                {albums.map((item) => (
                  <option key={item.entityId} value={item.entityId}>
                    {item.title || item.entityId.slice(0, 8)}
                  </option>
                ))}
              </select>
              <button type="button" className="btn small" onClick={() => addItem("album")}>
                添加相册
              </button>
            </div>
            <button type="button" className="btn primary" disabled={busy} onClick={() => void save()}>
              {busy ? "保存中…" : "保存推荐"}
            </button>
          </div>

          <div className="panel">
            <h2>推荐顺序</h2>
            <p className="muted small-text">拖动或使用上下按钮调整顺序，顺序即首页展示顺序。</p>
            <ul className="reco-list">
              {settings.items.map((item, index) => (
                <li
                  key={`${item.entityType}-${item.entityId}`}
                  draggable
                  onDragStart={() => setDragIndex(index)}
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={() => {
                    if (dragIndex !== null) {
                      moveItem(dragIndex, index);
                      setDragIndex(null);
                    }
                  }}
                >
                  <span className="reco-position">{index + 1}</span>
                  <span className={`badge ${item.entityType === "article" ? "status-role-user" : "status-active"}`}>
                    {item.entityType === "article" ? "文章" : "相册"}
                  </span>
                  <strong>{titleOf(item)}</strong>
                  <span className="actions">
                    <button type="button" className="btn tiny" onClick={() => moveItem(index, index - 1)}>
                      ↑
                    </button>
                    <button type="button" className="btn tiny" onClick={() => moveItem(index, index + 1)}>
                      ↓
                    </button>
                    <button
                      type="button"
                      className="btn tiny danger"
                      onClick={() =>
                        setSettings((current) => ({
                          ...current,
                          items: current.items.filter((_, position) => position !== index),
                        }))
                      }
                    >
                      移除
                    </button>
                  </span>
                </li>
              ))}
              {settings.items.length === 0 ? (
                <li className="empty">
                  {settings.autoFill ? "未手工配置，首页将自动补位最新公开内容" : "未配置推荐，首页只显示最新列表"}
                </li>
              ) : null}
            </ul>
          </div>
        </div>
      )}
    </section>
  );
}

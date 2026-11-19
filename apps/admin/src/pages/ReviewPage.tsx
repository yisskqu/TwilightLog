import { useCallback, useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  ApiError,
  apiRequest,
  isMobileViewport,
  type ReviewDetail,
  type ReviewItem,
} from "../api";
import { CONTENT_TYPES, STATUS_LABELS } from "../contentConfig";

const TYPE_FILTERS = ["", "article_column", "article", "photo_column", "album", "photo"] as const;

export function ReviewPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedVersion = searchParams.get("version") ?? "";
  const [items, setItems] = useState<ReviewItem[]>([]);
  const [type, setType] = useState("");
  const [search, setSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [detail, setDetail] = useState<ReviewDetail | null>(null);
  const [removeTags, setRemoveTags] = useState<string[]>([]);
  const [comment, setComment] = useState("");
  const [rejectReason, setRejectReason] = useState("");
  const [mode, setMode] = useState<"approve" | "reject">("approve");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const loadList = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const query = new URLSearchParams();
      if (type) {
        query.set("type", type);
      }
      if (search) {
        query.set("q", search);
      }
      const result = await apiRequest<{ items: ReviewItem[] }>(`/api/v1/admin/reviews?${query.toString()}`);
      setItems(result.items);
      if (!selectedVersion && result.items.length > 0) {
        setSearchParams({ version: result.items[0].versionId }, { replace: true });
      }
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载待审列表失败");
    } finally {
      setLoading(false);
    }
  }, [type, search, selectedVersion, setSearchParams]);

  useEffect(() => {
    void loadList();
  }, [loadList]);

  useEffect(() => {
    if (!selectedVersion) {
      setDetail(null);
      return;
    }
    let cancelled = false;
    void apiRequest<ReviewDetail>(`/api/v1/admin/reviews/${selectedVersion}`)
      .then((result) => {
        if (!cancelled) {
          setDetail(result);
          setRemoveTags([]);
          setComment("");
          setRejectReason("");
        }
      })
      .catch((caught) => {
        if (!cancelled) {
          setError(caught instanceof ApiError ? caught.message : "加载审批详情失败");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [selectedVersion]);

  async function decide() {
    if (!detail) {
      return;
    }
    if (mode === "reject" && !rejectReason.trim()) {
      setError("驳回理由不能为空");
      return;
    }
    if (mode === "approve" && detail.hasPublished && isMobileViewport()) {
      setError("替换公开版本需要在桌面端操作");
      return;
    }
    setBusy(true);
    setError("");
    setMessage("");
    try {
      if (mode === "approve") {
        const result = await apiRequest<{ removedTags: string[] }>(
          `/api/v1/admin/reviews/${detail.versionId}/approve`,
          { method: "POST", body: JSON.stringify({ comment, removeTags }) },
        );
        setMessage(
          result.removedTags.length > 0
            ? `已通过，并在本次公开版本中移除标签：${result.removedTags.join("、")}`
            : "已通过并替换公开版本",
        );
      } else {
        await apiRequest(`/api/v1/admin/reviews/${detail.versionId}/reject`, {
          method: "POST",
          body: JSON.stringify({ comment: rejectReason }),
        });
        setMessage("已驳回，编辑会收到通知与理由");
      }
      setSearchParams({}, { replace: true });
      await loadList();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "操作失败");
    } finally {
      setBusy(false);
    }
  }

  const config = detail ? CONTENT_TYPES[detail.type] : null;
  const keys = detail
    ? Array.from(new Set([...Object.keys(detail.published ?? {}), ...Object.keys(detail.payload ?? {})]))
    : [];

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">REVIEW</span>
          <h1>审批中心</h1>
          <p className="subtitle">左侧是待审队列，右侧可对照公开版本逐项确认。通过后立即替换公开版本。</p>
        </div>
        <Link className="btn" to="/admin/recommendations">
          首页推荐
        </Link>
      </header>

      {error ? <div className="alert">{error}</div> : null}
      {message ? <div className="notice">{message}</div> : null}

      <div className="review-layout">
        <aside className="panel review-list">
          <div className="toolbar compact">
            <form
              className="search"
              onSubmit={(event) => {
                event.preventDefault();
                setSearch(searchInput.trim());
              }}
            >
              <input
                value={searchInput}
                onChange={(event) => setSearchInput(event.target.value)}
                placeholder="搜索待审标题"
              />
              <button type="submit" className="btn small">
                搜索
              </button>
            </form>
            <select value={type} onChange={(event) => setType(event.target.value)}>
              {TYPE_FILTERS.map((value) => (
                <option key={value || "all"} value={value}>
                  {value ? CONTENT_TYPES[value].plural : "全部类型"}
                </option>
              ))}
            </select>
          </div>

          {loading ? <p className="muted">加载中…</p> : null}
          {!loading && items.length === 0 ? <p className="empty">当前没有待审内容</p> : null}
          <ul className="review-items">
            {items.map((item) => (
              <li key={item.versionId}>
                <button
                  type="button"
                  className={`review-item ${item.versionId === selectedVersion ? "active" : ""}`}
                  onClick={() => setSearchParams({ version: item.versionId })}
                >
                  <span className="badge status-role-user">{CONTENT_TYPES[item.type].label}</span>
                  <strong>{item.title || "未命名"}</strong>
                  <span className="muted small-text">
                    {item.authorName} · v{item.versionNo} ·{" "}
                    {item.submittedAt ? new Date(item.submittedAt).toLocaleString("zh-CN") : "刚提交"}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </aside>

        <div className="panel review-detail">
          {!detail || !config ? (
            <p className="empty">选择左侧内容查看详情</p>
          ) : (
            <>
              <div className="review-detail-head">
                <div>
                  <span className="badge status-role-user">{config.label}</span>
                  <h2>{detail.title || "未命名"}</h2>
                  <p className="muted small-text">
                    作者 {detail.authorName} · 提交 v{detail.versionNo} ·{" "}
                    {detail.submittedAt ? new Date(detail.submittedAt).toLocaleString("zh-CN") : "—"}
                  </p>
                </div>
                <div className="segmented">
                  <button
                    type="button"
                    className={mode === "approve" ? "active" : ""}
                    onClick={() => setMode("approve")}
                  >
                    通过
                  </button>
                  <button
                    type="button"
                    className={mode === "reject" ? "active" : ""}
                    onClick={() => setMode("reject")}
                  >
                    驳回
                  </button>
                </div>
              </div>

              <div className="diff-table">
                <div className="diff-head">
                  <span>字段</span>
                  <span>当前公开版本</span>
                  <span>待审版本</span>
                </div>
                {keys.map((key) => {
                  const before = formatValue(detail.published?.[key]);
                  const after = formatValue(detail.payload?.[key]);
                  return (
                    <div className={`diff-row ${before !== after ? "changed" : ""}`} key={key}>
                      <span className="mono">{key}</span>
                      <span>{before}</span>
                      <span>{after}</span>
                    </div>
                  );
                })}
              </div>

              {detail.tags.length > 0 ? (
                <div className="tag-review">
                  <h3>本次版本标签</h3>
                  <p className="muted small-text">勾选的标签将在公开版本中移除，本轮只影响本次版本。</p>
                  <div className="tag-row">
                    {detail.tags.map((tag) => (
                      <label className="checkbox" key={tag}>
                        <input
                          type="checkbox"
                          checked={removeTags.includes(tag)}
                          onChange={(event) =>
                            setRemoveTags((current) =>
                              event.target.checked ? [...current, tag] : current.filter((value) => value !== tag),
                            )
                          }
                        />
                        {tag}
                      </label>
                    ))}
                  </div>
                </div>
              ) : null}

              {mode === "approve" ? (
                <label className="field">
                  <span>审批备注（可选）</span>
                  <textarea rows={2} value={comment} onChange={(event) => setComment(event.target.value)} />
                </label>
              ) : (
                <label className="field">
                  <span>驳回理由（必填，会通知编辑）</span>
                  <textarea rows={3} value={rejectReason} onChange={(event) => setRejectReason(event.target.value)} />
                </label>
              )}

              <div className="editor-actions">
                <button type="button" className="btn primary" disabled={busy} onClick={() => void decide()}>
                  {busy ? "处理中…" : mode === "approve" ? "通过并替换公开版本" : "驳回"}
                </button>
                {detail.hasPublished && isMobileViewport() ? (
                  <span className="muted small-text">手机端只能通过首次发布，替换公开版本请使用桌面端。</span>
                ) : null}
              </div>

              {config.code === "article" && typeof detail.payload.bodyMarkdown === "string" ? (
                <details className="preview">
                  <summary>查看正文</summary>
                  <pre className="code-block">{detail.payload.bodyMarkdown}</pre>
                </details>
              ) : null}

              <p className="muted small-text">
                当前状态：{STATUS_LABELS[detail.status] ?? detail.status}
                {detail.publishedAt ? ` · 公开版本发布 ${new Date(detail.publishedAt).toLocaleString("zh-CN")}` : ""}
              </p>
            </>
          )}
        </div>
      </div>
    </section>
  );
}

function formatValue(value: unknown): string {
  if (value === undefined || value === null || value === "") {
    return "—";
  }
  if (Array.isArray(value)) {
    return value.map((item) => String(item)).join("、");
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  const text = String(value);
  return text.length > 240 ? `${text.slice(0, 240)}…` : text;
}

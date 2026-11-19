import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  ApiError,
  apiRequest,
  isMobileViewport,
  type ContentType,
  type WorkspaceItem,
  type WorkspacePage as WorkspacePageData,
} from "../api";
import { useAuth } from "../auth";
import { CONTENT_TYPES, TYPE_ORDER } from "../contentConfig";

export function RecycleBinPage() {
  const { isAdmin } = useAuth();
  const base = isAdmin ? "/admin" : "/editor";
  const [type, setType] = useState<ContentType>("article_column");
  const [allScope, setAllScope] = useState(false);
  const [items, setItems] = useState<WorkspaceItem[]>([]);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const query = new URLSearchParams({
        type,
        page: String(page),
        pageSize: "12",
      });
      if (isAdmin && allScope) {
        query.set("all", "1");
      }
      const result = await apiRequest<WorkspacePageData>(`/api/v1/workspace/recycle?${query.toString()}`);
      setItems(result.items);
      setTotal(result.total);
      setTotalPages(result.totalPages);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载回收站失败");
    } finally {
      setLoading(false);
    }
  }, [type, page, allScope, isAdmin]);

  useEffect(() => {
    void load();
  }, [load]);

  async function restore(item: WorkspaceItem) {
    setBusyId(item.id);
    setError("");
    try {
      await apiRequest(`/api/v1/workspace/content/${item.type}/${item.id}/restore`, { method: "POST" });
      setMessage(`已恢复「${item.title}」`);
      await load();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "恢复失败");
    } finally {
      setBusyId("");
    }
  }

  async function purge(item: WorkspaceItem) {
    if (
      !window.confirm(
        `永久删除「${item.title}」及其在线文件？将清理 ${item.mediaCount} 个原图/衍生图资源，此操作不可恢复。`,
      )
    ) {
      return;
    }
    setBusyId(item.id);
    setError("");
    try {
      const result = await apiRequest<{ mediaPurged: number }>(
        `/api/v1/admin/content/${item.type}/${item.id}`,
        { method: "DELETE" },
      );
      setMessage(`已永久删除，清理 ${result.mediaPurged} 个在线文件`);
      await load();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "永久删除失败");
    } finally {
      setBusyId("");
    }
  }

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">RECYCLE</span>
          <h1>回收站</h1>
          <p className="subtitle">移入回收站的内容不会出现在公开站，可随时恢复；永久删除仅管理员可在桌面端操作。</p>
        </div>
        <div className="head-actions">
          {isAdmin ? (
            <label className="checkbox">
              <input type="checkbox" checked={allScope} onChange={(event) => setAllScope(event.target.checked)} />
              查看全站
            </label>
          ) : null}
          <Link className="btn" to={`${base}/workspace/${type}`}>
            返回工作区
          </Link>
        </div>
      </header>

      {error ? <div className="alert">{error}</div> : null}
      {message ? <div className="notice">{message}</div> : null}

      <div className="type-tabs">
        {TYPE_ORDER.map((code) => (
          <button
            key={code}
            type="button"
            className={`type-tab ${code === type ? "active" : ""}`}
            onClick={() => {
              setType(code);
              setPage(1);
            }}
          >
            {CONTENT_TYPES[code].plural}
          </button>
        ))}
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>标题</th>
              <th>类型</th>
              <th>上级 / 归属</th>
              <th>删除时间</th>
              <th className="right">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>
                  <span className="strong">{item.title || "未命名"}</span>
                  {item.slug ? <span className="code">/{item.slug}</span> : null}
                </td>
                <td>{CONTENT_TYPES[item.type].label}</td>
                <td className="muted small-text">
                  <div>{item.parentTitle || "—"}</div>
                  {isAdmin && allScope ? <div>{item.ownerName}</div> : null}
                  <div>在线文件 {item.mediaCount}</div>
                </td>
                <td className="muted nowrap">
                  {item.trashedAt ? new Date(item.trashedAt).toLocaleString("zh-CN") : "—"}
                </td>
                <td className="right actions">
                  <button
                    type="button"
                    className="btn small"
                    disabled={busyId === item.id}
                    onClick={() => void restore(item)}
                  >
                    恢复
                  </button>
                  {isAdmin && !isMobileViewport() ? (
                    <button
                      type="button"
                      className="btn small danger"
                      disabled={busyId === item.id}
                      onClick={() => void purge(item)}
                    >
                      永久删除
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
            {!loading && items.length === 0 ? (
              <tr>
                <td colSpan={5} className="empty">
                  回收站为空
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <div className="pagination">
        <span className="muted">
          共 {total} 条，第 {page} / {totalPages} 页
        </span>
        <button type="button" className="btn small" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>
          上一页
        </button>
        <button
          type="button"
          className="btn small"
          disabled={page >= totalPages}
          onClick={() => setPage((value) => value + 1)}
        >
          下一页
        </button>
      </div>
    </section>
  );
}

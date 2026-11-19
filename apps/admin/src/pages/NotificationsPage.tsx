import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { ApiError, apiRequest, type NotificationItem, type NotificationPage } from "../api";
import { useAuth } from "../auth";

export function NotificationsPage() {
  const { isAdmin } = useAuth();
  const [items, setItems] = useState<NotificationItem[]>([]);
  const [unread, setUnread] = useState(0);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await apiRequest<NotificationPage>(`/api/v1/notifications?page=${page}&pageSize=20`);
      setItems(result.items);
      setUnread(result.unread);
      setTotal(result.total);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载通知失败");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    void load();
  }, [load]);

  async function markRead(item: NotificationItem) {
    if (item.readAt) {
      return;
    }
    try {
      await apiRequest(`/api/v1/notifications/${item.id}/read`, { method: "POST" });
      setItems((current) =>
        current.map((entry) => (entry.id === item.id ? { ...entry, readAt: new Date().toISOString() } : entry)),
      );
      setUnread((value) => Math.max(0, value - 1));
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "操作失败");
    }
  }

  async function markAll() {
    try {
      await apiRequest("/api/v1/notifications/read-all", { method: "POST" });
      await load();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "操作失败");
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / 20));

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">INBOX</span>
          <h1>通知</h1>
          <p className="subtitle">内容提交、审批结果、权限与账号变更都会在这里留下记录。</p>
        </div>
        <div className="head-actions">
          <span className="badge status-pending">{unread} 条未读</span>
          <button type="button" className="btn" onClick={() => void markAll()} disabled={unread === 0}>
            全部已读
          </button>
          <Link className="btn" to={isAdmin ? "/admin/workspace/article_column" : "/editor/workspace/article_column"}>
            返回工作区
          </Link>
        </div>
      </header>

      {error ? <div className="alert">{error}</div> : null}

      {loading ? <p className="muted">加载中…</p> : null}
      <ul className="notification-list">
        {items.map((item) => (
          <li key={item.id} className={item.readAt ? "read" : "unread"}>
            <span className={`dot severity-${item.severity}`} />
            <div className="notification-body">
              <div className="notification-head">
                <strong>{item.title}</strong>
                <span className="muted small-text">{new Date(item.createdAt).toLocaleString("zh-CN")}</span>
              </div>
              <p>{item.body}</p>
              <div className="notification-actions">
                {!item.readAt ? (
                  <button type="button" className="btn tiny" onClick={() => void markRead(item)}>
                    标记已读
                  </button>
                ) : null}
                {item.link ? (
                  <Link className="btn tiny" to={item.link}>
                    查看
                  </Link>
                ) : null}
              </div>
            </div>
          </li>
        ))}
      </ul>
      {!loading && items.length === 0 ? <p className="empty">暂无通知</p> : null}

      {totalPages > 1 ? (
        <div className="pagination">
          <button type="button" className="btn small" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>
            上一页
          </button>
          <span className="muted">
            第 {page} / {totalPages} 页
          </span>
          <button
            type="button"
            className="btn small"
            disabled={page >= totalPages}
            onClick={() => setPage((value) => value + 1)}
          >
            下一页
          </button>
        </div>
      ) : null}
    </section>
  );
}

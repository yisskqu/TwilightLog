import { useCallback, useEffect, useState } from "react";
import { ApiError, apiRequest, type AuditEntry, type Page } from "../api";

const actionLabels: Record<string, string> = {
  "auth.login": "登录",
  "auth.logout": "退出",
  "account.create": "创建账号",
  "account.grant_editor": "授予编辑",
  "account.revoke_editor": "撤销编辑",
  "account.reset_password": "重置密码",
  "account.disable": "禁用账号",
  "account.enable": "启用账号",
};

export function AuditPage() {
  const [items, setItems] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const pageSize = 20;

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await apiRequest<Page<AuditEntry>>(
        `/api/v1/admin/audit?page=${page}&pageSize=${pageSize}`,
      );
      setItems(result.items);
      setTotal(result.total);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载审计日志失败");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    void load();
  }, [load]);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <h1>审计日志</h1>
          <p className="subtitle">登录、退出、授权、撤权、禁用、启用和密码重置都会记录，只追加不可修改。</p>
        </div>
        <button type="button" className="btn" onClick={() => void load()}>
          刷新
        </button>
      </header>

      {error ? <div className="alert">{error}</div> : null}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>时间</th>
              <th>操作</th>
              <th>操作人</th>
              <th>对象</th>
              <th>结果</th>
              <th>IP / UA</th>
            </tr>
          </thead>
          <tbody>
            {items.map((entry) => (
              <tr key={entry.id}>
                <td className="muted nowrap">{new Date(entry.createdAt).toLocaleString("zh-CN")}</td>
                <td>
                  <span className="strong">{actionLabels[entry.action] ?? entry.action}</span>
                  <span className="code">{entry.action}</span>
                </td>
                <td>{entry.actorName || "—"}</td>
                <td className="muted">
                  {entry.targetType}
                  {entry.targetId ? ` / ${entry.targetId.slice(0, 8)}…` : ""}
                </td>
                <td>
                  <span className={`badge result-${entry.result}`}>
                    {entry.result === "success" ? "成功" : "失败"}
                  </span>
                </td>
                <td className="muted small-text">
                  <div>{entry.ip || "—"}</div>
                  <div className="truncate">{entry.userAgent || "—"}</div>
                </td>
              </tr>
            ))}
            {!loading && items.length === 0 ? (
              <tr>
                <td colSpan={6} className="empty">
                  暂无审计记录
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

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
    </section>
  );
}

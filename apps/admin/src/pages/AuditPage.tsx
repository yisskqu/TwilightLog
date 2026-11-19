import { Fragment, useCallback, useEffect, useState } from "react";
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
  "content.create": "新建内容",
  "content.save": "保存草稿",
  "content.submit": "提交审核",
  "content.withdraw": "撤回",
  "content.approve": "通过审批",
  "content.reject": "驳回",
  "content.trash": "移入回收站",
  "content.restore": "恢复内容",
  "content.purge": "永久删除",
  "recommendation.save": "保存推荐",
  "tag.rename": "重命名标签",
  "tag.merge": "合并标签",
  "tag.enable": "启用标签",
  "tag.disable": "停用标签",
};

const actions = Object.keys(actionLabels);

export function AuditPage() {
  const [items, setItems] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [action, setAction] = useState("");
  const [actor, setActor] = useState("");
  const [result, setResult] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const pageSize = 20;

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const query = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
      if (action) query.set("action", action);
      if (actor) query.set("actor", actor);
      if (result) query.set("result", result);
      if (from) query.set("from", from);
      if (to) query.set("to", to);
      const response = await apiRequest<Page<AuditEntry>>(`/api/v1/admin/audit?${query.toString()}`);
      setItems(response.items);
      setTotal(response.total);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载审计日志失败");
    } finally {
      setLoading(false);
    }
  }, [page, action, actor, result, from, to]);

  useEffect(() => {
    void load();
  }, [load]);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <h1>审计日志</h1>
          <p className="subtitle">登录、权限、内容与标签操作都会记录，只追加不可修改。</p>
        </div>
        <button type="button" className="btn" onClick={() => void load()}>
          刷新
        </button>
      </header>

      {error ? <div className="alert">{error}</div> : null}

      <div className="toolbar audit-filters">
        <select
          value={action}
          onChange={(event) => {
            setAction(event.target.value);
            setPage(1);
          }}
        >
          <option value="">全部动作</option>
          {actions.map((value) => (
            <option key={value} value={value}>
              {actionLabels[value]}
            </option>
          ))}
        </select>
        <input
          value={actor}
          onChange={(event) => setActor(event.target.value)}
          placeholder="操作人用户名"
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              setPage(1);
              void load();
            }
          }}
        />
        <select
          value={result}
          onChange={(event) => {
            setResult(event.target.value);
            setPage(1);
          }}
        >
          <option value="">全部结果</option>
          <option value="success">成功</option>
          <option value="failure">失败</option>
        </select>
        <label className="inline-date">
          从
          <input type="date" value={from} onChange={(event) => setFrom(event.target.value)} />
        </label>
        <label className="inline-date">
          至
          <input type="date" value={to} onChange={(event) => setTo(event.target.value)} />
        </label>
        <button
          type="button"
          className="btn small"
          onClick={() => {
            setAction("");
            setActor("");
            setResult("");
            setFrom("");
            setTo("");
            setPage(1);
          }}
        >
          重置
        </button>
      </div>

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
              <th className="right">详情</th>
            </tr>
          </thead>
          <tbody>
            {items.map((entry) => (
              <Fragment key={entry.id}>
                <tr>
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
                  <td className="right">
                    <button
                      type="button"
                      className="btn tiny"
                      onClick={() => setExpandedId((current) => (current === entry.id ? null : entry.id))}
                    >
                      {expandedId === entry.id ? "收起" : "查看"}
                    </button>
                  </td>
                </tr>
                {expandedId === entry.id ? (
                  <tr className="audit-detail">
                    <td colSpan={7}>
                      <pre className="code-block small">{JSON.stringify(entry.metadata ?? {}, null, 2)}</pre>
                    </td>
                  </tr>
                ) : null}
              </Fragment>
            ))}
            {!loading && items.length === 0 ? (
              <tr>
                <td colSpan={7} className="empty">
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
          第 {page} / {totalPages} 页，共 {total} 条
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

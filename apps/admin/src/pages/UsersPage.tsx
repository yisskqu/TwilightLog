import { useCallback, useEffect, useState, type FormEvent } from "react";
import { ApiError, apiRequest, type Account, type Page } from "../api";

const roleLabels: Record<string, string> = {
  super_admin: "超级管理员",
  admin: "管理员",
  editor: "编辑",
  user: "普通账号",
};

export function UsersPage() {
  const [items, setItems] = useState<Account[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);

  const pageSize = 12;

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const query = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
      });
      if (search) {
        query.set("search", search);
      }
      const result = await apiRequest<Page<Account>>(`/api/v1/admin/users?${query.toString()}`);
      setItems(result.items);
      setTotal(result.total);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载账号失败");
    } finally {
      setLoading(false);
    }
  }, [page, search]);

  useEffect(() => {
    void load();
  }, [load]);

  async function mutate(action: () => Promise<unknown>) {
    setError(null);
    try {
      await action();
      await load();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "操作失败");
    } finally {
      setBusyId(null);
    }
  }

  function handleSearch(event: FormEvent) {
    event.preventDefault();
    setPage(1);
    setSearch(searchInput.trim());
  }

  async function toggleRole(account: Account) {
    setBusyId(account.id);
    const nextRole = account.role === "editor" ? "user" : "editor";
    await mutate(() =>
      apiRequest(`/api/v1/admin/users/${account.id}/role`, {
        method: "PATCH",
        body: JSON.stringify({ role: nextRole }),
      }),
    );
  }

  async function toggleStatus(account: Account) {
    setBusyId(account.id);
    const nextStatus = account.status === "active" ? "disabled" : "active";
    await mutate(() =>
      apiRequest(`/api/v1/admin/users/${account.id}/status`, {
        method: "PATCH",
        body: JSON.stringify({ status: nextStatus }),
      }),
    );
  }

  async function resetPassword(account: Account) {
    const password = window.prompt(`为 ${account.username} 设置新密码（至少 8 位）`);
    if (!password) {
      return;
    }
    setBusyId(account.id);
    await mutate(() =>
      apiRequest(`/api/v1/admin/users/${account.id}/reset-password`, {
        method: "POST",
        body: JSON.stringify({ password }),
      }),
    );
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <h1>账号管理</h1>
          <p className="subtitle">创建普通账号、授予编辑权限、重置密码或禁用账号。管理员账号只能通过服务器命令操作。</p>
        </div>
        <button type="button" className="btn primary" onClick={() => setShowCreate((current) => !current)}>
          {showCreate ? "收起" : "创建账号"}
        </button>
      </header>

      {showCreate ? <CreateAccountForm onCreated={() => void load()} onError={setError} /> : null}
      {error ? <div className="alert">{error}</div> : null}

      <div className="toolbar">
        <form onSubmit={handleSearch} className="search">
          <input
            value={searchInput}
            onChange={(event) => setSearchInput(event.target.value)}
            placeholder="搜索用户名或邮箱"
          />
          <button type="submit" className="btn">
            搜索
          </button>
        </form>
        <span className="muted">共 {total} 个账号</span>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>用户名</th>
              <th>邮箱</th>
              <th>角色</th>
              <th>状态</th>
              <th>创建时间</th>
              <th className="right">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((account) => (
              <tr key={account.id}>
                <td className="strong">{account.username}</td>
                <td>{account.email}</td>
                <td>
                  <span className={`badge role-${account.role}`}>{roleLabels[account.role] ?? account.role}</span>
                </td>
                <td>
                  <span className={`badge status-${account.status}`}>
                    {account.status === "active" ? "正常" : "已禁用"}
                  </span>
                </td>
                <td className="muted">{new Date(account.createdAt).toLocaleString("zh-CN")}</td>
                <td className="right actions">
                  {account.canManage ? (
                    <>
                      <button
                        type="button"
                        className="btn small"
                        disabled={busyId === account.id}
                        onClick={() => void toggleRole(account)}
                      >
                        {account.role === "editor" ? "撤销编辑" : "授予编辑"}
                      </button>
                      <button
                        type="button"
                        className="btn small"
                        disabled={busyId === account.id}
                        onClick={() => void resetPassword(account)}
                      >
                        重置密码
                      </button>
                      <button
                        type="button"
                        className="btn small danger"
                        disabled={busyId === account.id}
                        onClick={() => void toggleStatus(account)}
                      >
                        {account.status === "active" ? "禁用" : "启用"}
                      </button>
                    </>
                  ) : (
                    <span className="muted">受保护</span>
                  )}
                </td>
              </tr>
            ))}
            {!loading && items.length === 0 ? (
              <tr>
                <td colSpan={6} className="empty">
                  没有匹配的账号
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

function CreateAccountForm({ onCreated, onError }: { onCreated: () => void; onError: (message: string) => void }) {
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"user" | "editor">("user");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    try {
      await apiRequest("/api/v1/admin/users", {
        method: "POST",
        body: JSON.stringify({ username, email, password, role }),
      });
      setUsername("");
      setEmail("");
      setPassword("");
      setRole("user");
      onCreated();
    } catch (caught) {
      onError(caught instanceof ApiError ? caught.message : "创建账号失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="create-panel" onSubmit={(event) => void handleSubmit(event)}>
      <label className="field">
        <span>用户名</span>
        <input
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          placeholder="3-32 位小写字母、数字、点、横线或下划线"
          required
        />
      </label>
      <label className="field">
        <span>邮箱</span>
        <input type="email" value={email} onChange={(event) => setEmail(event.target.value)} required />
      </label>
      <label className="field">
        <span>初始密码</span>
        <input
          type="password"
          minLength={8}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
        />
      </label>
      <label className="field">
        <span>角色</span>
        <select value={role} onChange={(event) => setRole(event.target.value === "editor" ? "editor" : "user")}>
          <option value="user">普通账号</option>
          <option value="editor">编辑</option>
        </select>
      </label>
      <button type="submit" className="btn primary" disabled={submitting}>
        {submitting ? "创建中…" : "创建"}
      </button>
    </form>
  );
}

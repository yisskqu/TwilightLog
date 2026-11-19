import { useCallback, useEffect, useState } from "react";
import { ApiError, apiRequest, isMobileViewport, type TagUsage } from "../api";

export function TagsPage() {
  const [scope, setScope] = useState<"article" | "photo">("article");
  const [items, setItems] = useState<TagUsage[]>([]);
  const [search, setSearch] = useState("");
  const [editingId, setEditingId] = useState("");
  const [editName, setEditName] = useState("");
  const [mergeTargets, setMergeTargets] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await apiRequest<{ items: TagUsage[] }>(`/api/v1/admin/tags?scope=${scope}`);
      setItems(result.items);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载标签失败");
    } finally {
      setLoading(false);
    }
  }, [scope]);

  useEffect(() => {
    void load();
  }, [load]);

  async function run(id: string, task: () => Promise<unknown>, success: string) {
    setBusy(id);
    setError("");
    setMessage("");
    try {
      await task();
      setMessage(success);
      await load();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "操作失败");
    } finally {
      setBusy("");
    }
  }

  async function rename(tag: TagUsage) {
    const name = editName.trim();
    if (!name || name === tag.name) {
      setEditingId("");
      return;
    }
    await run(tag.id, () =>
      apiRequest(`/api/v1/admin/tags/${tag.id}`, { method: "PATCH", body: JSON.stringify({ name }) }),
      `已重命名为「${name}」`,
    );
    setEditingId("");
  }

  function toggle(tag: TagUsage) {
    void run(
      tag.id,
      () =>
        apiRequest(`/api/v1/admin/tags/${tag.id}`, {
          method: "PATCH",
          body: JSON.stringify({ active: !tag.active }),
        }),
      tag.active ? `已停用「${tag.name}」` : `已启用「${tag.name}」`,
    );
  }

  function merge(tag: TagUsage) {
    const targetId = mergeTargets[tag.id];
    if (!targetId) {
      setError("请选择要合并到的目标标签");
      return;
    }
    const target = items.find((item) => item.id === targetId);
    if (!window.confirm(`将「${tag.name}」合并到「${target?.name ?? targetId}」？合并后不可撤销。`)) {
      return;
    }
    void run(
      tag.id,
      () => apiRequest(`/api/v1/admin/tags/${tag.id}/merge`, { method: "POST", body: JSON.stringify({ targetId }) }),
      `已合并到「${target?.name ?? targetId}」`,
    );
  }

  const filtered = items.filter((item) => item.name.toLowerCase().includes(search.trim().toLowerCase()));
  const desktop = !isMobileViewport();

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">TAXONOMY</span>
          <h1>标签词库</h1>
          <p className="subtitle">文章与摄影标签按作用域隔离；重命名、合并、停用属于高风险操作，仅桌面端可执行。</p>
        </div>
        <div className="segmented">
          <button type="button" className={scope === "article" ? "active" : ""} onClick={() => setScope("article")}>
            文章标签
          </button>
          <button type="button" className={scope === "photo" ? "active" : ""} onClick={() => setScope("photo")}>
            摄影标签
          </button>
        </div>
      </header>

      {error ? <div className="alert">{error}</div> : null}
      {message ? <div className="notice">{message}</div> : null}
      {!desktop ? <div className="notice">当前为窄屏/移动端，标签维护按钮已禁用，请在桌面端操作。</div> : null}

      <div className="toolbar">
        <input
          className="search-input"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="筛选标签"
        />
        <span className="muted small-text">共 {items.length} 个标签</span>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>标签</th>
              <th>使用次数</th>
              <th>状态</th>
              <th className="right">维护</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((tag) => (
              <tr key={tag.id}>
                <td>
                  {editingId === tag.id ? (
                    <span className="inline-edit">
                      <input value={editName} onChange={(event) => setEditName(event.target.value)} />
                      <button type="button" className="btn tiny" disabled={!desktop || busy === tag.id} onClick={() => void rename(tag)}>
                        保存
                      </button>
                      <button type="button" className="btn tiny" onClick={() => setEditingId("")}>
                        取消
                      </button>
                    </span>
                  ) : (
                    <span className="strong">{tag.name}</span>
                  )}
                </td>
                <td>{tag.usage}</td>
                <td>
                  <span className={`badge ${tag.active ? "status-active" : "status-disabled"}`}>
                    {tag.active ? "启用" : "停用"}
                  </span>
                </td>
                <td className="right actions">
                  <button
                    type="button"
                    className="btn tiny"
                    disabled={!desktop || busy === tag.id}
                    onClick={() => {
                      setEditingId(tag.id);
                      setEditName(tag.name);
                    }}
                  >
                    重命名
                  </button>
                  <button type="button" className="btn tiny" disabled={!desktop || busy === tag.id} onClick={() => toggle(tag)}>
                    {tag.active ? "停用" : "启用"}
                  </button>
                  <select
                    value={mergeTargets[tag.id] ?? ""}
                    onChange={(event) =>
                      setMergeTargets((current) => ({ ...current, [tag.id]: event.target.value }))
                    }
                    disabled={!desktop}
                  >
                    <option value="">合并到…</option>
                    {items
                      .filter((item) => item.id !== tag.id)
                      .map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.name}
                        </option>
                      ))}
                  </select>
                  <button
                    type="button"
                    className="btn tiny danger"
                    disabled={!desktop || busy === tag.id}
                    onClick={() => merge(tag)}
                  >
                    合并
                  </button>
                </td>
              </tr>
            ))}
            {!loading && filtered.length === 0 ? (
              <tr>
                <td colSpan={4} className="empty">
                  没有匹配的标签
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  );
}

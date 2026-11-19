import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  ApiError,
  apiRequest,
  type ContentType,
  type WorkspaceItem,
  type WorkspacePage as WorkspacePageData,
  type WorkspaceSummary,
} from "../api";
import { useAuth } from "../auth";
import { CONTENT_TYPES, STATUS_LABELS, TYPE_ORDER, isContentType } from "../contentConfig";

const STATUS_FILTERS = ["all", "draft", "pending", "rejected", "published", "trashed"] as const;

export function WorkspacePage() {
  const { user, isAdmin } = useAuth();
  const navigate = useNavigate();
  const params = useParams<{ type?: string }>();
  const base = isAdmin ? "/admin" : "/editor";
  const activeType: ContentType = isContentType(params.type) ? params.type : "article_column";

  const [summary, setSummary] = useState<WorkspaceSummary | null>(null);
  const [items, setItems] = useState<WorkspaceItem[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>("all");
  const [search, setSearch] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [allScope, setAllScope] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const pageSize = 12;

  const loadSummary = useCallback(async () => {
    try {
      const query = isAdmin && allScope ? "?all=1" : "";
      setSummary(await apiRequest<WorkspaceSummary>(`/api/v1/workspace/summary${query}`));
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载统计失败");
    }
  }, [allScope, isAdmin]);

  const loadList = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const query = new URLSearchParams({
        type: activeType,
        status,
        page: String(page),
        pageSize: String(pageSize),
      });
      if (search) {
        query.set("q", search);
      }
      if (isAdmin && allScope) {
        query.set("all", "1");
      }
      const result = await apiRequest<WorkspacePageData>(`/api/v1/workspace/content?${query.toString()}`);
      setItems(result.items);
      setTotal(result.total);
      setTotalPages(result.totalPages);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "加载内容失败");
    } finally {
      setLoading(false);
    }
  }, [activeType, status, page, search, allScope, isAdmin]);

  useEffect(() => {
    void loadSummary();
  }, [loadSummary]);

  useEffect(() => {
    void loadList();
  }, [loadList]);

  function switchType(type: ContentType) {
    setPage(1);
    setStatus("all");
    setSearch("");
    setSearchInput("");
    navigate(`${base}/workspace/${type}`);
  }

  function handleSearch(event: FormEvent) {
    event.preventDefault();
    setPage(1);
    setSearch(searchInput.trim());
  }

  async function act(id: string, action: "submit" | "withdraw" | "trash") {
    if (action === "trash" && !window.confirm("确定移入回收站？")) {
      return;
    }
    setBusyId(id);
    setError("");
    try {
      await apiRequest(`/api/v1/workspace/content/${activeType}/${id}/${action}`, { method: "POST" });
      await Promise.all([loadList(), loadSummary()]);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "操作失败");
    } finally {
      setBusyId("");
    }
  }

  const activeConfig = CONTENT_TYPES[activeType];
  const currentStats = summary?.stats.find((entry) => entry.type === activeType);

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">WORKSPACE</span>
          <h1>编辑工作区</h1>
          <p className="subtitle">
            {user?.username}，这里只显示你有权管理的内容。修改会形成工作版本，提交后由管理员审批。
          </p>
        </div>
        <div className="head-actions">
          {isAdmin ? (
            <label className="checkbox">
              <input type="checkbox" checked={allScope} onChange={(event) => setAllScope(event.target.checked)} />
              查看全站
            </label>
          ) : null}
          <Link className="btn primary" to={`${base}/content/new?type=${activeType}`}>
            新建{activeConfig.label}
          </Link>
        </div>
      </header>

      {error ? <div className="alert">{error}</div> : null}

      <div className="type-tabs">
        {TYPE_ORDER.map((code) => {
          const stats = summary?.stats.find((entry) => entry.type === code);
          return (
            <button
              key={code}
              type="button"
              className={`type-tab ${code === activeType ? "active" : ""}`}
              onClick={() => switchType(code)}
            >
              <span>{CONTENT_TYPES[code].plural}</span>
              <span className="type-count">{stats ? stats.total - stats.trashed : 0}</span>
              {stats && stats.pending > 0 ? <span className="badge status-pending">{stats.pending} 待审</span> : null}
            </button>
          );
        })}
      </div>

      {currentStats ? (
        <div className="stat-row">
          <span>全部 {currentStats.total - currentStats.trashed}</span>
          <span>草稿 {currentStats.draft}</span>
          <span>待审 {currentStats.pending}</span>
          <span>已驳回 {currentStats.rejected}</span>
          <span>已发布 {currentStats.published}</span>
          <span>回收站 {currentStats.trashed}</span>
        </div>
      ) : null}

      <div className="toolbar">
        <form className="search" onSubmit={handleSearch}>
          <input
            value={searchInput}
            onChange={(event) => setSearchInput(event.target.value)}
            placeholder={`搜索${activeConfig.label}标题或标识`}
          />
          <button type="submit" className="btn">
            搜索
          </button>
        </form>
        <div className="filter-row">
          {STATUS_FILTERS.map((value) => (
            <button
              key={value}
              type="button"
              className={`chip ${status === value ? "active" : ""}`}
              onClick={() => {
                setStatus(value);
                setPage(1);
              }}
            >
              {value === "all" ? "全部" : STATUS_LABELS[value]}
            </button>
          ))}
        </div>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>标题</th>
              <th>状态</th>
              <th>上级 / 归属</th>
              <th>更新时间</th>
              <th className="right">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>
                  <Link className="strong" to={`${base}/content/${activeType}/${item.id}`}>
                    {item.title || "未命名"}
                  </Link>
                  {item.slug ? <span className="code">/{item.slug}</span> : null}
                  {item.status === "rejected" ? <span className="badge status-disabled">待修改</span> : null}
                </td>
                <td>
                  <span className={`badge status-${badgeClass(item.status)}`}>{STATUS_LABELS[item.status] ?? item.status}</span>
                  <span className="muted small-text"> v{item.versionNo}</span>
                </td>
                <td className="muted small-text">
                  <div>{item.parentTitle || "—"}</div>
                  {isAdmin && allScope ? <div>{item.ownerName || item.ownerId.slice(0, 8)}</div> : null}
                </td>
                <td className="muted nowrap">{new Date(item.updatedAt).toLocaleString("zh-CN")}</td>
                <td className="right actions">
                  <Link className="btn small" to={`${base}/content/${activeType}/${item.id}`}>
                    编辑
                  </Link>
                  {item.status === "draft" || item.status === "rejected" ? (
                    <button
                      type="button"
                      className="btn small primary"
                      disabled={busyId === item.id}
                      onClick={() => void act(item.id, "submit")}
                    >
                      提交审核
                    </button>
                  ) : null}
                  {item.status === "pending" ? (
                    <button
                      type="button"
                      className="btn small"
                      disabled={busyId === item.id}
                      onClick={() => void act(item.id, "withdraw")}
                    >
                      撤回
                    </button>
                  ) : null}
                  {item.status !== "trashed" ? (
                    <button
                      type="button"
                      className="btn small danger"
                      disabled={busyId === item.id}
                      onClick={() => void act(item.id, "trash")}
                    >
                      移入回收站
                    </button>
                  ) : (
                    <Link className="btn small" to={`${base}/recycle`}>
                      回收站
                    </Link>
                  )}
                </td>
              </tr>
            ))}
            {!loading && items.length === 0 ? (
              <tr>
                <td colSpan={5} className="empty">
                  暂无{activeConfig.label}
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

function badgeClass(status: string): string {
  switch (status) {
    case "published":
      return "active";
    case "pending":
      return "pending";
    case "rejected":
    case "trashed":
      return "disabled";
    default:
      return "role-user";
  }
}

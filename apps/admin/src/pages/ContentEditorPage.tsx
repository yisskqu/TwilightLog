import { useCallback, useEffect, useState, type FormEvent } from "react";
import { marked } from "marked";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  ApiError,
  apiRequest,
  isMobileViewport,
  type ContentType,
  type WorkspaceDetail,
  type WorkspacePage as WorkspacePageData,
} from "../api";
import { useAuth } from "../auth";
import { CONTENT_TYPES, STATUS_LABELS, isContentType, payloadString, payloadTags } from "../contentConfig";
import { MediaField } from "../components/MediaField";

export function ContentEditorPage() {
  const params = useParams<{ type?: string; id?: string }>();
  const [searchParams] = useSearchParams();
  const { isAdmin } = useAuth();
  const navigate = useNavigate();
  const base = isAdmin ? "/admin" : "/editor";

  const typeFromParam = params.type;
  const typeFromQuery = searchParams.get("type") ?? undefined;
  const type: ContentType | null = isContentType(typeFromParam)
    ? typeFromParam
    : isContentType(typeFromQuery)
      ? typeFromQuery
      : null;
  const editingID = params.id ?? "";

  const [detail, setDetail] = useState<WorkspaceDetail | null>(null);
  const [slug, setSlug] = useState("");
  const [parentId, setParentId] = useState(searchParams.get("parentId") ?? "");
  const [form, setForm] = useState<Record<string, string>>({});
  const [parents, setParents] = useState<{ id: string; title: string }[]>([]);
  const [loading, setLoading] = useState(Boolean(editingID));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [showPreview, setShowPreview] = useState(false);

  const config = type ? CONTENT_TYPES[type] : null;

  const applyDetail = useCallback(
    (next: WorkspaceDetail) => {
      setDetail(next);
      setSlug(next.slug ?? "");
      setParentId(next.parentId ?? "");
      const source = next.working?.payload ?? next.published?.payload ?? {};
      const nextForm: Record<string, string> = {};
      if (config) {
        for (const field of config.fields) {
          if (field.kind === "tags") {
            nextForm[field.key] = payloadTags(source).join(", ");
          } else {
            const value = payloadString(source, field.key);
            nextForm[field.key] = field.kind === "date" ? value.slice(0, 10) : value;
          }
        }
      }
      setForm(nextForm);
    },
    [config],
  );

  useEffect(() => {
    if (!editingID || !type) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    void apiRequest<WorkspaceDetail>(`/api/v1/workspace/content/${type}/${editingID}`)
      .then((result) => {
        if (!cancelled) {
          applyDetail(result);
        }
      })
      .catch((caught) => {
        if (!cancelled) {
          setError(caught instanceof ApiError ? caught.message : "加载内容失败");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [editingID, type, applyDetail]);

  useEffect(() => {
    if (!config?.parent) {
      return;
    }
    const parentType = config.parent;
    let cancelled = false;
    const query = new URLSearchParams({ type: parentType, status: "all", pageSize: "50" });
    if (isAdmin) {
      query.set("all", "1");
    }
    void apiRequest<WorkspacePageData>(`/api/v1/workspace/content?${query.toString()}`)
      .then((result) => {
        if (!cancelled) {
          setParents(result.items.map((item) => ({ id: item.id, title: item.title || "未命名" })));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setParents([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [config, isAdmin]);

  if (!type || !config) {
    return (
      <section className="page">
        <div className="alert">未知的内容类型，请从工作区重新进入。</div>
      </section>
    );
  }

  function buildPayload(): Record<string, unknown> {
    const payload: Record<string, unknown> = {};
    for (const field of config!.fields) {
      const raw = (form[field.key] ?? "").trim();
      if (field.kind === "tags") {
        payload[field.key] = raw
          .split(/[,，]/)
          .map((value) => value.trim())
          .filter(Boolean);
      } else {
        payload[field.key] = raw;
      }
    }
    return payload;
  }

  async function save(): Promise<WorkspaceDetail | null> {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const body = JSON.stringify({ type, slug, parentId, payload: buildPayload() });
      const result = editingID
        ? await apiRequest<WorkspaceDetail>(`/api/v1/workspace/content/${type}/${editingID}`, {
            method: "PUT",
            body: JSON.stringify({ slug, payload: buildPayload() }),
          })
        : await apiRequest<WorkspaceDetail>("/api/v1/workspace/content", { method: "POST", body });
      applyDetail(result);
      setMessage("草稿已保存");
      if (!editingID) {
        navigate(`${base}/content/${type}/${result.id}`, { replace: true });
      }
      return result;
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "保存失败");
      return null;
    } finally {
      setBusy(false);
    }
  }

  async function handleSave(event: FormEvent) {
    event.preventDefault();
    await save();
  }

  async function submitForReview() {
    let id = editingID;
    if (!id) {
      const saved = await save();
      if (!saved) {
        return;
      }
      id = saved.id;
    }
    setBusy(true);
    setError("");
    try {
      const result = await apiRequest<WorkspaceDetail>(`/api/v1/workspace/content/${type}/${id}/submit`, {
        method: "POST",
      });
      applyDetail(result);
      setMessage("已提交审核，等待管理员审批");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "提交失败");
    } finally {
      setBusy(false);
    }
  }

  async function withdraw() {
    if (!editingID) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await apiRequest<WorkspaceDetail>(`/api/v1/workspace/content/${type}/${editingID}/withdraw`, {
        method: "POST",
      });
      applyDetail(result);
      setMessage("已撤回为草稿");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "撤回失败");
    } finally {
      setBusy(false);
    }
  }

  async function trash() {
    if (!editingID || !window.confirm("确定移入回收站？")) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await apiRequest(`/api/v1/workspace/content/${type}/${editingID}/trash`, { method: "POST" });
      navigate(`${base}/workspace/${type}`, { replace: true });
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "操作失败");
    } finally {
      setBusy(false);
    }
  }

  const working = detail?.working ?? null;
  const pending = working?.status === "pending";
  const rejected = working?.status === "rejected";
  const previewHTML = marked.parse(form.bodyMarkdown ?? "") as string;
  const mediaPurpose: "article" | "photo" = config.scope === "article" ? "article" : "photo";

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <Link className="muted small-text" to={`${base}/workspace/${type}`}>
            ← 返回{config.plural}
          </Link>
          <h1>
            {editingID ? "编辑" : "新建"}
            {config.label}
          </h1>
          <p className="subtitle">
            {editingID ? "保存会形成工作版本，公开站继续显示已通过的版本。" : "创建后会先生成草稿，提交后进入审批队列。"}
          </p>
        </div>
        {detail ? (
          <span className={`badge status-${detail.status === "published" ? "active" : detail.status === "pending" ? "pending" : "role-user"}`}>
            {STATUS_LABELS[detail.status] ?? detail.status}
          </span>
        ) : null}
      </header>

      {error ? <div className="alert">{error}</div> : null}
      {message ? <div className="notice">{message}</div> : null}
      {rejected && working?.comment ? <div className="alert">驳回理由：{working.comment}</div> : null}
      {pending ? <div className="notice">当前版本正在审批中，撤回后才能继续编辑。</div> : null}

      {loading ? (
        <p className="muted">加载中…</p>
      ) : (
        <form className="editor-layout" onSubmit={(event) => void handleSave(event)}>
          <div className="panel editor-form">
            {config.parent ? (
              <label className="field">
                <span>所属{CONTENT_TYPES[config.parent].label}</span>
                <select value={parentId} onChange={(event) => setParentId(event.target.value)} required disabled={pending}>
                  <option value="">请选择</option>
                  {parents.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.title}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}

            {config.hasSlug ? (
              <label className="field">
                <span>链接标识（可选）</span>
                <input
                  value={slug}
                  onChange={(event) => setSlug(event.target.value)}
                  placeholder="留空时按标题自动生成"
                  disabled={pending}
                />
              </label>
            ) : null}

            {config.fields.map((field) => {
              if (field.kind === "media") {
                return (
                  <div className="field" key={field.key}>
                    <span>
                      {field.label}
                      {field.required ? "（必填）" : ""}
                    </span>
                    <MediaField
                      value={form[field.key] ?? ""}
                      purpose={mediaPurpose}
                      onChange={(mediaId) => setForm((current) => ({ ...current, [field.key]: mediaId }))}
                    />
                  </div>
                );
              }
              if (field.kind === "textarea") {
                return (
                  <label className="field" key={field.key}>
                    <span>{field.label}</span>
                    <textarea
                      rows={field.rows ?? 4}
                      value={form[field.key] ?? ""}
                      placeholder={field.placeholder}
                      onChange={(event) => setForm((current) => ({ ...current, [field.key]: event.target.value }))}
                      disabled={pending}
                    />
                  </label>
                );
              }
              return (
                <label className="field" key={field.key}>
                  <span>{field.label}</span>
                  <input
                    type={field.kind === "date" ? "date" : "text"}
                    value={form[field.key] ?? ""}
                    placeholder={field.placeholder}
                    onChange={(event) => setForm((current) => ({ ...current, [field.key]: event.target.value }))}
                    disabled={pending}
                  />
                </label>
              );
            })}

            <div className="editor-actions">
              <button type="submit" className="btn primary" disabled={busy || pending}>
                {busy ? "保存中…" : "保存草稿"}
              </button>
              {!pending ? (
                <button type="button" className="btn" disabled={busy} onClick={() => void submitForReview()}>
                  提交审核
                </button>
              ) : (
                <button type="button" className="btn" disabled={busy} onClick={() => void withdraw()}>
                  撤回
                </button>
              )}
              {editingID ? (
                <button type="button" className="btn danger" disabled={busy} onClick={() => void trash()}>
                  移入回收站
                </button>
              ) : null}
              {type === "article" ? (
                <button type="button" className="btn" onClick={() => setShowPreview((value) => !value)}>
                  {showPreview ? "收起预览" : "预览正文"}
                </button>
              ) : null}
            </div>
            {!editingID ? <p className="muted small-text">首次提交前需要先保存草稿。</p> : null}
          </div>

          <aside className="panel editor-side">
            <h2>版本信息</h2>
            {detail ? (
              <dl className="facts">
                <div>
                  <dt>工作版本</dt>
                  <dd>
                    {working ? `v${working.versionNo} · ${STATUS_LABELS[working.status] ?? working.status}` : "—"}
                  </dd>
                </div>
                <div>
                  <dt>公开版本</dt>
                  <dd>
                    {detail.published
                      ? `v${detail.published.versionNo} · ${new Date(detail.published.publishedAt ?? "").toLocaleString("zh-CN")}`
                      : "尚未发布"}
                  </dd>
                </div>
                <div>
                  <dt>更新时间</dt>
                  <dd>{new Date(detail.updatedAt).toLocaleString("zh-CN")}</dd>
                </div>
              </dl>
            ) : (
              <p className="muted small-text">保存后显示版本信息。</p>
            )}
            {type === "article" && showPreview ? (
              <div className="preview">
                <h2>正文预览</h2>
                <div className="article-preview" dangerouslySetInnerHTML={{ __html: previewHTML }} />
              </div>
            ) : null}
            {isMobileViewport() ? (
              <p className="muted small-text">手机端可编辑、提交和撤回；永久删除与账号权限操作需在桌面端完成。</p>
            ) : null}
          </aside>
        </form>
      )}
    </section>
  );
}

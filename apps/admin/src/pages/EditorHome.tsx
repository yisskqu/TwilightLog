import { useEffect, useState } from "react";
import { ApiError, apiRequest } from "../api";
import { useAuth } from "../auth";

export function EditorHome() {
  const { user } = useAuth();
  const [status, setStatus] = useState("正在校验编辑权限…");

  useEffect(() => {
    let cancelled = false;
    async function check() {
      try {
        await apiRequest("/api/v1/editor/ping");
        if (!cancelled) {
          setStatus("编辑权限已确认");
        }
      } catch (caught) {
        if (!cancelled) {
          setStatus(caught instanceof ApiError ? caught.message : "权限校验失败");
        }
      }
    }
    void check();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <h1>编辑工作区</h1>
          <p className="subtitle">你好，{user?.username}。文章与摄影内容的编辑、提交和审批将在阶段 4 接入。</p>
        </div>
      </header>

      <div className="panel">
        <h2>当前状态</h2>
        <p className="muted">{status}</p>
        <p className="muted">
          编辑可以访问本工作区，但不能进入管理后台。撤销编辑权限或禁用账号后，下一次请求会立即失效。
        </p>
      </div>
    </section>
  );
}

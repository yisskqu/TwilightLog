import { NavLink, useNavigate } from "react-router-dom";
import { useEffect, useState, type ReactNode } from "react";
import { apiRequest, type NotificationPage } from "../api";
import { useAuth } from "../auth";

const roleLabels: Record<string, string> = {
  super_admin: "超级管理员",
  admin: "管理员",
  editor: "编辑",
  user: "普通账号",
};

export function AppShell({ children }: { children: ReactNode }) {
  const { user, isAdmin, logout } = useAuth();
  const navigate = useNavigate();
  const [unread, setUnread] = useState(0);
  const base = isAdmin ? "/admin" : "/editor";

  useEffect(() => {
    let active = true;
    async function poll() {
      try {
        const result = await apiRequest<NotificationPage>("/api/v1/notifications?pageSize=1");
        if (active) {
          setUnread(result.unread);
        }
      } catch {
        // 通知轮询失败不影响主导航。
      }
    }
    void poll();
    const timer = window.setInterval(() => void poll(), 30_000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);

  async function handleLogout() {
    await logout();
    navigate("/login", { replace: true });
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">TL</span>
          <div>
            <strong>TwilightLog</strong>
            <span className="brand-sub">内容管理后台</span>
          </div>
        </div>

        <nav className="nav">
          {isAdmin ? (
            <>
              <NavLink to="/admin" end>
                概览
              </NavLink>
              <NavLink to="/admin/workspace/article_column">编辑工作区</NavLink>
              <NavLink to="/admin/reviews">
                审批中心
                {unread > 0 ? <span className="nav-badge">{unread}</span> : null}
              </NavLink>
              <NavLink to="/admin/recommendations">首页推荐</NavLink>
              <NavLink to="/admin/recycle">回收站</NavLink>
              <NavLink to="/admin/users">账号管理</NavLink>
              <NavLink to="/admin/tags">标签词库</NavLink>
              <NavLink to="/admin/audit">审计日志</NavLink>
              <NavLink to="/admin/system">系统信息</NavLink>
              <NavLink to="/admin/notifications">
                通知
                {unread > 0 ? <span className="nav-badge">{unread}</span> : null}
              </NavLink>
              <NavLink to="/admin/media">媒体测试</NavLink>
            </>
          ) : (
            <>
              <NavLink to="/editor/workspace/article_column">编辑工作区</NavLink>
              <NavLink to="/editor/recycle">回收站</NavLink>
              <NavLink to="/editor/notifications">
                通知
                {unread > 0 ? <span className="nav-badge">{unread}</span> : null}
              </NavLink>
              <NavLink to="/editor/media">媒体测试</NavLink>
            </>
          )}
        </nav>

        <div className="sidebar-foot">
          <span className="role-chip">{roleLabels[user?.role ?? ""] ?? "访客"}</span>
          <span className="username">{user?.username}</span>
          <button type="button" className="link-button" onClick={() => void handleLogout()}>
            退出登录
          </button>
        </div>
      </aside>

      <main className="content">
        <div className="topbar">
          <NavLink className="btn small" to={`${base}/notifications`}>
            通知{unread > 0 ? ` · ${unread}` : ""}
          </NavLink>
        </div>
        {children}
      </main>
    </div>
  );
}

export function LoadingScreen({ label = "正在恢复会话…" }: { label?: string }) {
  return (
    <div className="splash">
      <div className="splash-mark">TL</div>
      <p>{label}</p>
    </div>
  );
}

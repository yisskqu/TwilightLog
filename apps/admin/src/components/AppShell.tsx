import { NavLink, useNavigate } from "react-router-dom";
import type { ReactNode } from "react";
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
              <NavLink to="/admin/users">账号管理</NavLink>
              <NavLink to="/admin/audit">审计日志</NavLink>
            </>
          ) : (
            <NavLink to="/editor">编辑工作区</NavLink>
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

      <main className="content">{children}</main>
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

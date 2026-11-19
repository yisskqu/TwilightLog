import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

export function ForbiddenPage() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  async function handleLogout() {
    await logout();
    navigate("/login", { replace: true });
  }

  return (
    <div className="forbidden">
      <span className="eyebrow">403</span>
      <h1>无权访问</h1>
      <p className="muted">
        {user ? `当前账号 ${user.username} 没有后台访问权限。` : "请先登录后再访问。"}
        如需开通，请联系管理员。
      </p>
      <div className="forbidden-actions">
        <button type="button" className="btn" onClick={() => navigate(-1)}>
          返回
        </button>
        {user ? (
          <button type="button" className="btn danger" onClick={() => void handleLogout()}>
            退出登录
          </button>
        ) : null}
      </div>
    </div>
  );
}

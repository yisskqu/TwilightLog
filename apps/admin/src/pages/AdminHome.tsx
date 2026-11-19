import { Link } from "react-router-dom";
import { useAuth } from "../auth";

export function AdminHome() {
  const { user } = useAuth();

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <h1>概览</h1>
          <p className="subtitle">阶段 2 已接通账号、角色、会话和审计。内容模块将在后续阶段接入。</p>
        </div>
      </header>

      <div className="cards">
        <Link className="card" to="/admin/users">
          <span className="eyebrow">ACCOUNTS</span>
          <h2>账号管理</h2>
          <p>创建普通账号，授予或撤销编辑权限，重置密码，禁用或启用账号。</p>
        </Link>
        <Link className="card" to="/admin/audit">
          <span className="eyebrow">AUDIT</span>
          <h2>审计日志</h2>
          <p>查看登录、退出和权限变更记录，追踪操作人与来源地址。</p>
        </Link>
      </div>

      <div className="panel">
        <h2>当前会话</h2>
        <dl className="facts">
          <div>
            <dt>用户名</dt>
            <dd>{user?.username}</dd>
          </div>
          <div>
            <dt>邮箱</dt>
            <dd>{user?.email}</dd>
          </div>
          <div>
            <dt>角色</dt>
            <dd>{user?.role}</dd>
          </div>
          <div>
            <dt>权限</dt>
            <dd>{user?.permissions.join("、") || "—"}</dd>
          </div>
        </dl>
      </div>
    </section>
  );
}

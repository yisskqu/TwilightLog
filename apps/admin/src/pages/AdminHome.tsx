import { Link } from "react-router-dom";
import { useAuth } from "../auth";

export function AdminHome() {
  const { user } = useAuth();

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">OVERVIEW</span>
          <h1>概览</h1>
          <p className="subtitle">内容生产、审批、推荐、账号与审计已全部接通。高风险操作仅桌面端可用。</p>
        </div>
      </header>

      <div className="cards">
        <Link className="card" to="/admin/workspace/article_column">
          <span className="eyebrow">WORKSPACE</span>
          <h2>编辑工作区</h2>
          <p>查看全站五类内容，代编辑调整草稿，处理待审、驳回与回收站状态。</p>
        </Link>
        <Link className="card" to="/admin/reviews">
          <span className="eyebrow">REVIEW</span>
          <h2>审批中心</h2>
          <p>对照公开版本与待审版本，逐项确认标签、正文与元数据后通过或驳回。</p>
        </Link>
        <Link className="card" to="/admin/recommendations">
          <span className="eyebrow">CURATION</span>
          <h2>首页推荐</h2>
          <p>配置 N₁/N₂ 数量、手工排序，或清空后恢复全自动补位。</p>
        </Link>
        <Link className="card" to="/admin/tags">
          <span className="eyebrow">TAXONOMY</span>
          <h2>标签词库</h2>
          <p>按文章与摄影作用域管理标签，处理重命名、合并与停用。</p>
        </Link>
        <Link className="card" to="/admin/users">
          <span className="eyebrow">ACCOUNTS</span>
          <h2>账号管理</h2>
          <p>创建普通账号，授予或撤销编辑权限，重置密码，禁用或启用账号。</p>
        </Link>
        <Link className="card" to="/admin/audit">
          <span className="eyebrow">AUDIT</span>
          <h2>审计日志</h2>
          <p>按动作、操作人、结果和日期筛选，追踪内容与权限变更。</p>
        </Link>
        <Link className="card" to="/admin/recycle">
          <span className="eyebrow">RECYCLE</span>
          <h2>回收站</h2>
          <p>恢复误删内容；管理员可在桌面端永久删除并清理在线文件。</p>
        </Link>
        <Link className="card" to="/admin/system">
          <span className="eyebrow">SYSTEM</span>
          <h2>系统信息</h2>
          <p>只读查看版本、存储用量、上传限制、会话策略与水印参数。</p>
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

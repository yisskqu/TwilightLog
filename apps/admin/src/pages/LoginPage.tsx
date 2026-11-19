import { useState, type FormEvent } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { ApiError } from "../api";
import { homePathFor, useAuth } from "../auth";
import { LoadingScreen } from "../components/AppShell";

export function LoginPage() {
  const { user, loading, login } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (loading) {
    return <LoadingScreen label="正在检查登录状态…" />;
  }
  if (user) {
    return <Navigate to={homePathFor(user.role)} replace />;
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const loggedIn = await login(username.trim(), password, remember);
      navigate(homePathFor(loggedIn.role), { replace: true });
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "登录失败，请稍后重试");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="auth-screen">
      <form className="auth-card" onSubmit={(event) => void handleSubmit(event)}>
        <span className="eyebrow">TWILIGHTLOG / ADMIN</span>
        <h1>登录后台</h1>
        <p className="hint">账号由管理员创建，不开放自助注册。</p>

        {error ? <div className="alert">{error}</div> : null}

        <label className="field">
          <span>用户名</span>
          <input
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            autoComplete="username"
            required
          />
        </label>

        <label className="field">
          <span>密码</span>
          <input
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoComplete="current-password"
            required
          />
        </label>

        <label className="checkbox">
          <input
            type="checkbox"
            checked={remember}
            onChange={(event) => setRemember(event.target.checked)}
          />
          <span>记住登录（7 天）</span>
        </label>

        <button type="submit" className="btn primary" disabled={submitting}>
          {submitting ? "登录中…" : "登录"}
        </button>
      </form>
    </div>
  );
}

import { useEffect, useState } from "react";
import { ApiError, apiRequest, type SystemInfo } from "../api";

export function SystemPage() {
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    void apiRequest<SystemInfo>("/api/v1/admin/system")
      .then(setInfo)
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "加载系统信息失败"));
  }, []);

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">SYSTEM</span>
          <h1>系统信息</h1>
          <p className="subtitle">只读展示。系统配置不接受后台修改，避免误操作影响生产环境。</p>
        </div>
      </header>

      {error ? <div className="alert">{error}</div> : null}
      {!info ? <p className="muted">加载中…</p> : null}

      {info ? (
        <div className="system-grid">
          <div className="panel">
            <h2>版本与构建</h2>
            <dl className="facts">
              <div>
                <dt>API 版本</dt>
                <dd>{info.version}</dd>
              </div>
              <div>
                <dt>构建状态</dt>
                <dd>{info.buildStatus}</dd>
              </div>
              <div>
                <dt>运行环境</dt>
                <dd>{info.appEnv}</dd>
              </div>
              <div>
                <dt>图片处理器</dt>
                <dd>{info.processor}</dd>
              </div>
              <div>
                <dt>快照时间</dt>
                <dd>{new Date(info.updatedAt).toLocaleString("zh-CN")}</dd>
              </div>
            </dl>
          </div>

          <div className="panel">
            <h2>存储与任务</h2>
            <dl className="facts">
              <div>
                <dt>媒体占用</dt>
                <dd>{formatBytes(info.storageUsage)}</dd>
              </div>
              <div>
                <dt>任务状态</dt>
                <dd>
                  <pre className="code-block small">{JSON.stringify(info.jobStatus ?? {}, null, 2)}</pre>
                </dd>
              </div>
            </dl>
          </div>

          <div className="panel">
            <h2>上传与展示</h2>
            <dl className="facts">
              <div>
                <dt>文章图片上限</dt>
                <dd>{info.maxArticleMB} MB</dd>
              </div>
              <div>
                <dt>摄影照片上限</dt>
                <dd>{info.maxPhotoMB} MB</dd>
              </div>
              <div>
                <dt>单批上限</dt>
                <dd>{info.batchLimit} 张</dd>
              </div>
              <div>
                <dt>水印</dt>
                <dd>{info.watermark}</dd>
              </div>
            </dl>
          </div>

          <div className="panel">
            <h2>会话与安全</h2>
            <dl className="facts">
              <div>
                <dt>Access Token</dt>
                <dd>{info.accessTokenTTL}</dd>
              </div>
              <div>
                <dt>Refresh Token</dt>
                <dd>{info.refreshTokenTTL}</dd>
              </div>
              <div>
                <dt>Cookie Secure</dt>
                <dd>{info.cookieSecure ? "已开启" : "开发环境关闭"}</dd>
              </div>
              <div>
                <dt>设备分级</dt>
                <dd>永久删除、账号权限、替换公开版本等高风险操作仅桌面端可用</dd>
              </div>
            </dl>
          </div>

          <div className="panel panel-wide">
            <h2>人工操作（不在后台提供）</h2>
            <ul className="plain-list">
              <li>创建管理员：服务器执行 <code className="code">/app/admin create</code></li>
              <li>重置管理员密码：服务器命令，不在后台提供入口</li>
              <li>系统配置修改：部署环境变量与配置文件，后台只读</li>
              <li>审计日志只追加，不提供编辑与删除入口</li>
            </ul>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function formatBytes(value: number): string {
  if (!value) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

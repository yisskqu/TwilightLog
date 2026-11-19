import { useEffect, useState } from "react";
import { colors } from "@twilightlog/design-tokens";

type Capabilities = {
  modules: string[];
  menus: string[];
  permissions: string[];
};

export default function App() {
  const [status, setStatus] = useState("正在检查 API…");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const response = await fetch("/api/v1/capabilities");
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        const data = (await response.json()) as Capabilities;
        if (!cancelled) {
          setStatus(`API 正常，启用模块 ${data.modules.length} 个`);
        }
      } catch (error) {
        if (!cancelled) {
          setStatus(`API 不可用：${error instanceof Error ? error.message : "未知错误"}`);
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <main
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        background: colors.paper,
        color: colors.ink,
        fontFamily: "system-ui, -apple-system, Segoe UI, sans-serif",
      }}
    >
      <section
        style={{
          maxWidth: 640,
          padding: "48px 32px",
          border: `2px solid ${colors.ink}`,
          boxShadow: `8px 8px 0 ${colors.ink}`,
          background: colors.surface,
        }}
      >
        <p style={{ margin: 0, letterSpacing: "0.18em", fontSize: 12 }}>
          TWILIGHTLOG / ADMIN
        </p>
        <h1 style={{ margin: "12px 0", fontSize: "clamp(2rem, 7vw, 4rem)" }}>
          管理后台
        </h1>
        <p>阶段 1 占位页。认证、内容管理和审批将在后续阶段接入。</p>
        <p style={{ color: colors.accent }}>{status}</p>
      </section>
    </main>
  );
}

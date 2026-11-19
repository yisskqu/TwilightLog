import { defineConfig } from "astro/config";
import react from "@astrojs/react";
import sitemap from "@astrojs/sitemap";

const site = process.env.SITE_URL ?? "https://www.twilightlog.com";

export default defineConfig({
  site,
  // Search is deliberately noindex, so it must not be advertised in the sitemap.
  integrations: [react(), sitemap({ filter: (page) => !page.includes("/search") })],
  server: {
    host: "127.0.0.1",
    port: 4321,
  },
  vite: {
    server: {
      proxy: {
        "/api": "http://localhost:8080",
        "/assets": "http://localhost:8080",
        "/healthz": "http://localhost:8080",
        "/readyz": "http://localhost:8080",
      },
    },
  },
});

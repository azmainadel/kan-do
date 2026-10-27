import type { ProxyOptions } from "vite";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The Go server embeds the build from internal/web/dist/app.
// `npm run dev` proxies the API to a board started with `kan-do --no-open`.
const board = process.env.KANDO_URL ?? "http://localhost:4242";

// The board refuses requests from other origins; the dev server is one, so drop the header.
const proxy: ProxyOptions = {
  target: board,
  changeOrigin: true,
  configure: (p) => p.on("proxyReq", (req) => req.removeHeader("origin")),
};

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/web/dist/app",
    emptyOutDir: true,
  },
  server: {
    proxy: { "/api": proxy, "/images": proxy },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
  },
});

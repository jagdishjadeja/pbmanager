import { defineConfig } from "astro/config";

export default defineConfig({
  output: "static",
  server: { port: 4321 },
  vite: {
    server: {
      proxy: { "/api/manager": "http://127.0.0.1:8095" },
    },
  },
});

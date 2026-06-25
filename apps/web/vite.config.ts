import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During dev, proxy the Connect service path to the Go server so the browser
// talks to it same-origin — Connect's POST + server-streaming then need no CORS.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/triage.v1.TriageService": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});

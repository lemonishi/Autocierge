/// <reference types="vitest/config" />
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Component tests run under jsdom. Kept separate from vite.config.ts so the
// production build (`vite build`) carries no test config.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
  },
});

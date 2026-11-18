import { defineConfig, searchForWorkspaceRoot } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
    // The help window shows ../build/appicon.svg, outside the frontend folder.
    fs: { allow: [searchForWorkspaceRoot(process.cwd()), "../build"] },
  },
  plugins: [svelte(), wails("./bindings")],
});

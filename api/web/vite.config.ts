import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';

const outDir = '../internal/webui/dist';

// emptyOutDir removes the committed .gitkeep: put it back after each build, so
// the Go embed directive always finds the folder.
function keepGitkeep(): Plugin {
  return {
    name: 'keep-gitkeep',
    closeBundle() {
      writeFileSync(resolve(import.meta.dirname, outDir, '.gitkeep'), '');
    },
  };
}

// The build is embedded into the Go binary (internal/webui/dist).
// In dev, API calls are proxied to the Go server on :8080.
export default defineConfig({
  base: '/',
  plugins: [react(), keepGitkeep()],
  build: {
    outDir,
    emptyOutDir: true,
  },
  server: {
    host: true,
    port: 5173,
    proxy: { '/api': 'http://localhost:8080', '/healthz': 'http://localhost:8080' },
  },
});

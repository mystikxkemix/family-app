package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"familyapp/internal/webui"
)

// website serves the web app built into the binary (internal/webui). Paths
// that are not files get index.html, so the app's own routes (/lists,
// /admin...) work on reload.
func (s *Server) website() http.Handler {
	files := webui.Files()
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(files, name); err == nil && !info.IsDir() {
				// Vite puts a content hash in every asset name: cache them for good.
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(files, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("The website is not built. Run `npm ci && npm run build` in api/web.\n"))
			return
		}
		// index.html names the current asset files: never cache it.
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

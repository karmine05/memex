package api

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed web/dist/*
var websiteFS embed.FS

// WebsiteFS returns the embedded website filesystem
func WebsiteFS() fs.FS {
	sub, _ := fs.Sub(websiteFS, "web/dist")
	return sub
}

// WebsiteHandler returns an http.Handler that serves the embedded website
func WebsiteHandler() http.Handler {
	fsys := WebsiteFS()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		upath := r.URL.Path
		if !strings.HasPrefix(upath, "/") {
			upath = "/" + upath
		}

		// Remove trailing slash for non-root paths
		if len(upath) > 1 && strings.HasSuffix(upath, "/") {
			upath = strings.TrimSuffix(upath, "/")
		}

		filePath := strings.TrimPrefix(upath, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		// Try to open the file
		f, err := fsys.Open(filePath)
		if err != nil {
			// Check if it's a directory path (e.g., "dashboard" -> try "dashboard/index.html")
			if !strings.Contains(filePath, ".") {
				// Try directory index
				dirIndexPath := filePath + "/index.html"
				f, err = fsys.Open(dirIndexPath)
				if err == nil {
					filePath = dirIndexPath
				}
			}

			// For SPA routing, fall back to index.html for non-asset paths
			if err != nil {
				if !strings.Contains(filePath, ".") || strings.HasPrefix(filePath, "assets/") {
					filePath = "index.html"
					f, err = fsys.Open(filePath)
				}
				if err != nil {
					http.NotFound(w, r)
					return
				}
			}
		} else {
			// Check if it's a directory (doesn't implement ReadSeeker)
			if fi, err := f.Stat(); err == nil && fi.IsDir() {
				f.Close()
				// Try directory index
				dirIndexPath := filePath + "/index.html"
				f, err = fsys.Open(dirIndexPath)
				if err == nil {
					filePath = dirIndexPath
				} else {
					http.NotFound(w, r)
					return
				}
			}
		}
		defer f.Close()

		// Get file info for content length
		fi, _ := f.Stat()

		// Set correct content type
		setContentType(w, filePath)

		// Set cache headers for static assets
		if strings.HasPrefix(filePath, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if filePath == "index.html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}

		// Serve the file directly - ensure it implements ReadSeeker
		rs, ok := f.(io.ReadSeeker)
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, filePath, fi.ModTime(), rs)
	})
}

func setContentType(w http.ResponseWriter, filePath string) {
	ext := path.Ext(filePath)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	case ".txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
}
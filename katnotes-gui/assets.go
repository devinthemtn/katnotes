package main

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/devinthemtn/katnotes/internal/notes"
)

// assetRoute is where the webview loads files from the notes' assets folder.
// The preview rewrites "../assets/x.png" to "/kn-assets/x.png".
const assetRoute = "/kn-assets/"

// SaveAsset stores an image (base64 data) in the assets folder and returns
// the Markdown to insert into a note. name is the original file name; an
// empty one gets a "pasted-<time>" name.
func (s *NotesService) SaveAsset(name, dataBase64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", err
	}
	st := s.lock()
	defer s.mu.Unlock()
	rel, err := st.SaveAsset(name, data)
	if err != nil {
		return "", err
	}
	return notes.ImageMarkdown("", rel), nil
}

// ImportAsset copies the image file at path into the assets folder (for
// ":image <path>") and returns the Markdown to insert. A leading ~ is the
// home folder.
func (s *NotesService) ImportAsset(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	st := s.lock()
	defer s.mu.Unlock()
	rel, err := st.ImportAsset(path)
	if err != nil {
		return "", err
	}
	return notes.ImageMarkdown("", rel), nil
}

// assetMiddleware serves GET /kn-assets/<file> from the current notes
// folder's assets/ and passes every other request on.
func (s *NotesService) assetMiddleware() application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, assetRoute) {
				next.ServeHTTP(w, r)
				return
			}
			s.serveAsset(w, r)
		})
	}
}

func (s *NotesService) serveAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, err := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), assetRoute))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	path, ok := s.store.AssetPath(name)
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	// SVGs can carry scripts; this stops them running if one is opened as
	// a document rather than through <img>.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

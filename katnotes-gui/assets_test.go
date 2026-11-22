package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceSaveAndServeAsset(t *testing.T) {
	s := newService(t)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	md, err := s.SaveAsset("Shot 1.png", base64.StdEncoding.EncodeToString(png))
	if err != nil {
		t.Fatal(err)
	}
	if md != "![](../assets/shot-1.png)" {
		t.Errorf("markdown = %q", md)
	}
	if _, err := s.SaveAsset("x.png", "not base64!"); err == nil {
		t.Error("bad base64 accepted")
	}
	// A file next to the notes folder that must never be served.
	os.WriteFile(filepath.Join(s.store.Root, "secret.png"), []byte("no"), 0o644)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := s.assetMiddleware()(next)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://wails.localhost"+path, nil))
		return rec
	}
	if rec := get("/kn-assets/shot-1.png"); rec.Code != http.StatusOK || rec.Body.String() != string(png) {
		t.Errorf("asset: %d %q", rec.Code, rec.Body.String())
	}
	for _, bad := range []string{"/kn-assets/../secret.png", "/kn-assets/%2e%2e%2fsecret.png", "/kn-assets/missing.png", "/kn-assets/"} {
		if rec := get(bad); rec.Code == http.StatusOK {
			t.Errorf("%s served: %q", bad, rec.Body.String())
		}
	}
	if rec := get("/index.html"); rec.Code != http.StatusTeapot {
		t.Errorf("other paths not passed on: %d", rec.Code)
	}
}

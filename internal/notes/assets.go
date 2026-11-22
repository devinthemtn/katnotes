package notes

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// AssetsDir is the folder under the notes root that holds images and other
// files notes refer to. Notes link to them as "../assets/<file>".
const AssetsDir = "assets"

// MaxAssetSize is the largest file SaveAsset accepts.
const MaxAssetSize = 25 << 20

// ErrAsset is returned for a file SaveAsset won't store.
var ErrAsset = errors.New("unsupported asset")

// imageExts are the file types SaveAsset accepts.
var imageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".avif", ".bmp"}

// sniffExt maps a sniffed content type to an extension, for data without a
// usable file name (a pasted image).
var sniffExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/bmp":  ".bmp",
	"image/avif": ".avif",
}

// AssetsPath is the absolute path of the assets folder.
func (s *Store) AssetsPath() string { return filepath.Join(s.Root, AssetsDir) }

// AssetPath returns the file path for an asset name, or false if the name
// isn't a plain file name inside the assets folder.
func (s *Store) AssetPath(name string) (string, bool) {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) ||
		strings.HasPrefix(name, ".") {
		return "", false
	}
	return filepath.Join(s.AssetsPath(), name), true
}

// SaveAsset stores data in the assets folder and returns its path relative
// to the notes root, e.g. "assets/screenshot.png". name is the original file
// name; only its base is used, slugified. An empty name (or one without an
// extension) gets a "pasted-<time>" name and an extension from the content.
// A file with the same bytes already in the folder is reused.
func (s *Store) SaveAsset(name string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("%w: empty file", ErrAsset)
	}
	if len(data) > MaxAssetSize {
		return "", fmt.Errorf("%w: %d MB is over the %d MB limit", ErrAsset, len(data)>>20, MaxAssetSize>>20)
	}
	base := ""
	if n := strings.Trim(strings.ReplaceAll(name, `\`, "/"), "/ "); n != "" {
		base = filepath.Base(n)
	}
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if ext == "" {
		ext = sniffExt[http.DetectContentType(data)]
		if ext == "" {
			return "", fmt.Errorf("%w: can't tell the image type of %q", ErrAsset, name)
		}
	}
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	if !slices.Contains(imageExts, ext) {
		return "", fmt.Errorf("%w: %s files (want %s)", ErrAsset, ext, strings.Join(imageExts, " "))
	}
	slug := Slugify(stem)
	if slug == "untitled" {
		slug = "pasted-" + s.now().Format("20060102-150405")
	}

	dir := s.AssetsPath()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if same, err := findSame(dir, data); err != nil {
		return "", err
	} else if same != "" {
		return AssetsDir + "/" + same, nil
	}

	tmp, err := os.CreateTemp(dir, ".katnotes-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	for i := 1; i < 10000; i++ {
		file := slug + ext
		if i > 1 {
			file = fmt.Sprintf("%s-%d%s", slug, i, ext)
		}
		err := placeNew(tmp.Name(), filepath.Join(dir, file))
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return AssetsDir + "/" + file, nil
	}
	return "", fmt.Errorf("%w: too many files named %s%s", ErrAsset, slug, ext)
}

// findSame returns the name of a file in dir with exactly data's bytes.
func findSame(dir string, data []byte) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var sum [32]byte
	summed := false
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fi, err := e.Info()
		if err != nil || fi.Size() != int64(len(data)) {
			continue
		}
		other, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if !summed {
			sum, summed = sha256.Sum256(data), true
		}
		if sha256.Sum256(other) == sum && bytes.Equal(other, data) {
			return e.Name(), nil
		}
	}
	return "", nil
}

// ImportAsset copies the file at src into the assets folder (see SaveAsset).
func (s *Store) ImportAsset(src string) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("%w: %s is a folder", ErrAsset, src)
	}
	if fi.Size() > MaxAssetSize {
		return "", fmt.Errorf("%w: %d MB is over the %d MB limit", ErrAsset, fi.Size()>>20, MaxAssetSize>>20)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	return s.SaveAsset(filepath.Base(src), data)
}

// ImageMarkdown is the Markdown for an image in the assets folder, as seen
// from a note: rel is what SaveAsset returned.
func ImageMarkdown(alt, rel string) string {
	alt = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(alt)
	return "![" + alt + "](../" + rel + ")"
}

// Assets lists the files in the assets folder, sorted by name.
func (s *Store) Assets() ([]string, error) {
	entries, err := os.ReadDir(s.AssetsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// OrphanAssets lists the files in the assets folder that no note mentions
// (as "assets/<file>" anywhere in its body).
func (s *Store) OrphanAssets() ([]string, error) {
	files, err := s.Assets()
	if err != nil || len(files) == 0 {
		return nil, err
	}
	all, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool, len(files))
	for i := range all {
		body := all[i].Body
		if !strings.Contains(body, AssetsDir+"/") {
			continue
		}
		for _, f := range files {
			if !used[f] && (strings.Contains(body, AssetsDir+"/"+f) ||
				strings.Contains(body, AssetsDir+"/"+strings.ReplaceAll(f, " ", "%20"))) {
				used[f] = true
			}
		}
	}
	var out []string
	for _, f := range files {
		if !used[f] {
			out = append(out, f)
		}
	}
	return out, nil
}

// IsImageFile reports whether name has an extension SaveAsset accepts.
func IsImageFile(name string) bool {
	return slices.Contains(imageExts, strings.ToLower(filepath.Ext(name)))
}

// AttachImage copies the image at src into the assets folder and adds its
// Markdown to the end of n, on a line of its own, then saves n. It returns
// the asset's path relative to the notes root.
func (s *Store) AttachImage(n *Note, src, alt string) (string, error) {
	rel, err := s.ImportAsset(src)
	if err != nil {
		return "", err
	}
	body := strings.TrimRight(n.Body, "\n")
	if body != "" {
		body += "\n\n"
	}
	n.Body = body + ImageMarkdown(alt, rel) + "\n"
	if err := s.Save(n); err != nil {
		return "", err
	}
	return rel, nil
}

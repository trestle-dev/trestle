package files

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

type objectStorage interface {
	Put(context.Context, string, string, string) error
	Serve(http.ResponseWriter, *http.Request, string, Metadata) error
	Delete(context.Context, string) error
	Cleanup(context.Context, map[string]bool) (int, error)
}
type localStorage struct{ root string }

func (s *localStorage) Put(_ context.Context, key, staged, _ string) error {
	return os.Rename(staged, filepath.Join(s.root, key))
}
func (s *localStorage) Serve(w http.ResponseWriter, r *http.Request, key string, m Metadata) error {
	path := filepath.Join(s.root, key)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="download"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, m.Name, info.ModTime(), file)
	return nil
}
func (s *localStorage) Delete(_ context.Context, key string) error {
	err := os.Remove(filepath.Join(s.root, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (s *localStorage) Cleanup(_ context.Context, known map[string]bool) (int, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || known[entry.Name()] {
			continue
		}
		if os.Remove(filepath.Join(s.root, entry.Name())) == nil {
			removed++
		}
	}
	return removed, nil
}

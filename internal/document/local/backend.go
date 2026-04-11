package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/document"
)

// Config holds the local filesystem backend configuration.
type Config struct {
	BasePath string `json:"base_path"`
}

type Backend struct {
	basePath string
}

// NewFromConfig creates a local Backend from the JSONB config stored in the DB.
func NewFromConfig(raw json.RawMessage) (document.Backend, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("local: invalid config: %w", err)
	}

	if cfg.BasePath == "" {
		return nil, fmt.Errorf("local: base_path is required")
	}

	return &Backend{basePath: cfg.BasePath}, nil
}

func (b *Backend) Type() string { return "local" }

// resolveKey joins the key with basePath and rejects any path that escapes basePath.
func (b *Backend) resolveKey(key string) (string, error) {
	path := filepath.Join(b.basePath, filepath.Clean(key))
	base := filepath.Clean(b.basePath) + string(os.PathSeparator)
	if !strings.HasPrefix(path+string(os.PathSeparator), base) {
		return "", fmt.Errorf("local: key escapes base path: %s", key)
	}
	return path, nil
}

// Upload writes the content to <basePath>/<uuid>_<filename> and returns the
// relative path as the storage key.
func (b *Backend) Upload(_ context.Context, filename string, content io.Reader) (string, error) {
	if err := os.MkdirAll(b.basePath, 0o750); err != nil {
		return "", fmt.Errorf("local: creating base path: %w", err)
	}

	key := uuid.New().String() + "_" + filepath.Base(filename)
	dst := filepath.Join(b.basePath, key)

	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("local: creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, content); err != nil {
		_ = os.Remove(dst)
		return "", fmt.Errorf("local: writing file: %w", err)
	}

	return key, nil
}

// Download opens the file at <basePath>/<key> and returns a ReadCloser.
func (b *Backend) Download(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := b.resolveKey(key)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local: file not found: %s", key)
		}
		return nil, fmt.Errorf("local: opening file: %w", err)
	}

	return f, nil
}

// Delete removes the file at <basePath>/<key>.
func (b *Backend) Delete(_ context.Context, key string) error {
	path, err := b.resolveKey(key)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("local: removing file: %w", err)
	}

	return nil
}

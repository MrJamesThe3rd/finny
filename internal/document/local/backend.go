package local

import (
	"context"
	"encoding/json"
	"errors"
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

// Backends constructs local backends. Root confines base_path, which arrives
// from user-supplied config and would otherwise name any path the process can
// write to. See knowledge-base.md §4 for the storage trust model.
type Backends struct {
	Root string
}

func (b Backends) New(raw json.RawMessage) (document.Backend, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("local: invalid config: %w", err)
	}

	basePath, err := resolveUnderRoot(b.Root, cfg.BasePath)
	if err != nil {
		return nil, fmt.Errorf("local: %w", err)
	}

	if err := os.MkdirAll(basePath, 0o750); err != nil {
		return nil, fmt.Errorf("local: creating base path: %w", err)
	}

	return &Backend{basePath: basePath}, nil
}

// resolveUnderRoot interprets basePath as relative to root, returning the
// absolute directory it names.
func resolveUnderRoot(root, basePath string) (string, error) {
	if root == "" {
		return "", ErrRootNotConfigured
	}

	if basePath == "" {
		return "", ErrBasePathRequired
	}

	if filepath.IsAbs(basePath) {
		return "", ErrBasePathNotRelative
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving storage root: %w", err)
	}

	path := filepath.Join(absRoot, basePath)
	if path != absRoot && !strings.HasPrefix(path, absRoot+string(os.PathSeparator)) {
		return "", ErrBasePathEscapesRoot
	}

	return path, nil
}

func (b *Backend) Type() string { return "local" }

// open returns an os.Root confined to basePath. Every file operation goes
// through one: unlike a lexical prefix check, it is enforced by the OS per
// path component, so a symlink planted under basePath cannot escape it.
// The caller must close it.
func (b *Backend) open() (*os.Root, error) {
	root, err := os.OpenRoot(b.basePath)
	if err != nil {
		return nil, fmt.Errorf("local: opening base path: %w", err)
	}

	return root, nil
}

// Upload writes the content to <basePath>/<uuid>_<filename> and returns the
// relative path as the storage key.
func (b *Backend) Upload(_ context.Context, filename string, content io.Reader) (string, error) {
	root, err := b.open()
	if err != nil {
		return "", err
	}

	defer root.Close() //nolint:errcheck

	key := uuid.New().String() + "_" + filepath.Base(filename)

	f, err := root.Create(key)
	if err != nil {
		return "", fmt.Errorf("local: creating file: %w", err)
	}

	defer f.Close() //nolint:errcheck

	if _, err := io.Copy(f, content); err != nil {
		_ = root.Remove(key)

		return "", fmt.Errorf("local: writing file: %w", err)
	}

	return key, nil
}

func (b *Backend) Download(_ context.Context, key string) (io.ReadCloser, error) {
	root, err := b.open()
	if err != nil {
		return nil, err
	}

	defer root.Close() //nolint:errcheck

	f, err := root.Open(key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("local: %w: %s", ErrFileNotFound, key)
		}

		return nil, fmt.Errorf("local: opening file: %w", err)
	}

	return f, nil
}

func (b *Backend) Delete(_ context.Context, key string) error {
	root, err := b.open()
	if err != nil {
		return err
	}

	defer root.Close() //nolint:errcheck

	if err := root.Remove(key); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("local: removing file: %w", err)
	}

	return nil
}

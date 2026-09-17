package local

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// base_path is user-supplied config, so these cases are what Root exists for.
// Each positive case pins the resolved path: asserting only "no error" passes
// even if base_path is ignored entirely.
func TestBackends_New_ConfinesBasePathToRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		useRoot bool
		give    string
		wantRel string
		wantErr error
	}{
		{name: "relative path", useRoot: true, give: "invoices", wantRel: "invoices"},
		{name: "nested path", useRoot: true, give: "vibrantgarden/invoices", wantRel: "vibrantgarden/invoices"},
		{name: "traversal collapses inside root", useRoot: true, give: "a/../b", wantRel: "b"},
		{name: "root itself", useRoot: true, give: ".", wantRel: "."},
		{name: "traversal escapes", useRoot: true, give: "../../etc/cron.d", wantErr: ErrBasePathEscapesRoot},
		{name: "absolute path", useRoot: true, give: "/etc/cron.d", wantErr: ErrBasePathNotRelative},
		{name: "empty base_path", useRoot: true, give: "", wantErr: ErrBasePathRequired},
		{name: "unconfigured root", give: "invoices", wantErr: ErrRootNotConfigured},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var root string
			if tt.useRoot {
				root = t.TempDir()
			}

			backend, err := Backends{Root: root}.New(configFor(t, tt.give))

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			absRoot, err := filepath.Abs(root)
			require.NoError(t, err)

			want := filepath.Join(absRoot, tt.wantRel)
			assert.Equal(t, want, backend.(*Backend).basePath)
		})
	}
}

func TestBackends_New_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := Backends{Root: t.TempDir()}.New(json.RawMessage(`{`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config")
}

func TestBackend_UploadDownloadDelete(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	backend, err := Backends{Root: root}.New(configFor(t, "invoices"))
	require.NoError(t, err)

	key, err := backend.Upload(context.Background(), "fatura.pdf", strings.NewReader("pdf bytes"))
	require.NoError(t, err)
	assert.Contains(t, key, "fatura.pdf")

	written := filepath.Join(root, "invoices", key)
	require.FileExists(t, written, "upload must land under the configured root")

	rc, err := backend.Download(context.Background(), key)
	require.NoError(t, err)

	defer rc.Close() //nolint:errcheck

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "pdf bytes", string(content))

	require.NoError(t, backend.Delete(context.Background(), key))
	assert.NoFileExists(t, written)
}

func TestBackend_Download_NotFound(t *testing.T) {
	t.Parallel()

	backend, err := Backends{Root: t.TempDir()}.New(configFor(t, "invoices"))
	require.NoError(t, err)

	_, err = backend.Download(context.Background(), "missing-key")
	require.ErrorIs(t, err, ErrFileNotFound)
}

// Delete is the destructive one, so it gets the same traversal coverage as Download.
func TestBackend_RejectsKeysEscapingBasePath(t *testing.T) {
	t.Parallel()

	backend, err := Backends{Root: t.TempDir()}.New(configFor(t, "invoices"))
	require.NoError(t, err)

	const escape = "../../../etc/passwd"

	_, err = backend.Download(context.Background(), escape)
	require.Error(t, err)

	require.Error(t, backend.Delete(context.Background(), escape))
}

// A lexical prefix check cannot see symlinks; os.Root is enforced per path
// component by the OS, so a link planted inside base_path still cannot escape.
func TestBackend_RejectsSymlinkEscapingBasePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(root, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))

	backend, err := Backends{Root: root}.New(configFor(t, "invoices"))
	require.NoError(t, err)

	link := filepath.Join(root, "invoices", "escape")
	require.NoError(t, os.Symlink(outside, link))

	_, err = backend.Download(context.Background(), "escape")
	require.Error(t, err, "a symlink out of base_path must not resolve")
}

// Whatever resolveUnderRoot accepts must sit inside the root.
func FuzzResolveUnderRoot(f *testing.F) {
	for _, seed := range []string{"invoices", "a/../b", "../etc", "/abs", "", ".", "a/./b"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, basePath string) {
		const root = "/srv/finny/documents"

		got, err := resolveUnderRoot(root, basePath)
		if err != nil {
			return
		}

		assert.True(t, got == root || strings.HasPrefix(got, root+string(os.PathSeparator)),
			"accepted base_path %q resolved outside the root: %q", basePath, got)
	})
}

func configFor(t *testing.T, basePath string) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(Config{BasePath: basePath})
	require.NoError(t, err)

	return raw
}

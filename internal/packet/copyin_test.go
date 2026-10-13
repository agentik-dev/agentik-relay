package packet

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// copyIn copies each rel under root (a file, or a directory walked; a
// symlink skipped) to the same rel under a fresh t.TempDir() and returns
// that directory: a test reads the real tree only through a copy, and a
// `"../..` literal outside a testdata directory sits on a copyIn line
// (AR17D act 4; TestAR10_0_NoTestOpensRealCheckout scans for it). The
// same helper lives in each package that reads the tree.
func copyIn(t *testing.T, root string, rels ...string) string {
	t.Helper()
	dst := t.TempDir()
	for _, rel := range rels {
		err := filepath.WalkDir(filepath.Join(root, rel), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			r, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			to := filepath.Join(dst, r)
			if d.IsDir() {
				return os.MkdirAll(to, 0o755)
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return err
			}
			return os.WriteFile(to, b, info.Mode().Perm())
		})
		if err != nil {
			t.Fatalf("copyIn %s: %v", rel, err)
		}
	}
	return dst
}

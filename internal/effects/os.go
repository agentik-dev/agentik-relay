package effects

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// OSFS is the machine's file system.
type OSFS struct{}

func (OSFS) ReadFile(p string) ([]byte, error) { return os.ReadFile(p) }
func (OSFS) WriteFile(p string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(p, data, perm)
}
func (OSFS) Rename(from, to string) error              { return os.Rename(from, to) }
func (OSFS) MkdirAll(p string, perm fs.FileMode) error { return os.MkdirAll(p, perm) }
func (OSFS) Remove(p string) error                     { return os.Remove(p) }

// WriteTemp is os.CreateTemp in dir, written and closed; a failed write
// leaves no file.
func (OSFS) WriteTemp(dir, pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Lock is flock(LOCK_EX) on path, its directory made first; the lock ends
// with the release or the process (the kernel drops it with the fd).
func (OSFS) Lock(p string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

func (OSFS) Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (OSFS) List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

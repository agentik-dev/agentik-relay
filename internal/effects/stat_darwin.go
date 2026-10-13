package effects

import (
	"io/fs"
	"syscall"
)

// statKey is the stat a tree entry's sum is bound to (AR24 act 0): the
// account can write new bytes with the size and mtime restored, never
// with the ctime set back.
type statKey struct {
	dev, ino, size, ctime int64
	mode                  uint32
}

// statOf is fi's statKey; the zero key when fi carries no Stat_t.
func statOf(fi fs.FileInfo) statKey {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return statKey{}
	}
	return statKey{int64(st.Dev), int64(st.Ino), st.Size, st.Ctimespec.Nano(), uint32(st.Mode)}
}

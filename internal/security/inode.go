package security

import (
	"os"
	"syscall"
)

// inodeOf extracts the inode number so log rotation (file replaced at the
// same path) can be detected even if the new file happens to be larger than
// the old read offset.
func inodeOf(info os.FileInfo) uint64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return st.Ino
}

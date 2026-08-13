package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const defaultMinFreeBytes = int64(5) * 1024 * 1024 * 1024

var lookupFree = filesystemFree

func filesystemFree(path string) (int64, error) {
	st, err := statfsNearest(path)
	if err != nil {
		return 0, err
	}
	bsize := int64(st.Bsize)
	if st.Frsize > 0 {
		bsize = int64(st.Frsize)
	}
	return int64(st.Bavail) * bsize, nil
}

func statfsNearest(path string) (syscall.Statfs_t, error) {
	var st syscall.Statfs_t
	p := path
	for {
		err := syscall.Statfs(p, &st)
		if err == nil {
			return st, nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return st, err
		}
		p = parent
	}
}

func sameFilesystem(a, b string) bool {
	sa, errA := statfsNearest(a)
	sb, errB := statfsNearest(b)
	if errA != nil || errB != nil {
		return false
	}
	return sa.Fsid == sb.Fsid
}

func tempSpaceNeeded(srcSize, minFree int64) int64 {
	if srcSize < 0 {
		srcSize = 0
	}
	if minFree < 0 {
		minFree = 0
	}
	return srcSize + srcSize + minFree
}

func outputSpaceNeeded(fileSize, minFree int64) int64 {
	if fileSize < 0 {
		fileSize = 0
	}
	if minFree < 0 {
		minFree = 0
	}
	return fileSize + minFree
}

func (m *Manager) guardEncodeSpace(srcSize int64, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}
	if err := ensureDiskSpace(m.TempDir, tempSpaceNeeded(srcSize, m.minFree()), "temp"); err != nil {
		return err
	}
	outNeed := m.minFree()
	if !sameFilesystem(m.TempDir, outDir) {
		outNeed = outputSpaceNeeded(srcSize, m.minFree())
	}
	return ensureDiskSpace(outDir, outNeed, "output")
}

func ensureDiskSpace(path string, need int64, label string) error {
	if need <= 0 {
		return nil
	}
	free, err := lookupFree(path)
	if err != nil {
		return fmt.Errorf("could not check %s filesystem space: %w", label, err)
	}
	if free < need {
		return fmt.Errorf("%s filesystem has %s free, need %s", label, formatBytes(free), formatBytes(need))
	}
	return nil
}

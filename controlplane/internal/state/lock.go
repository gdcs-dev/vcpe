package state

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type WriterLock struct {
	f *os.File
}

func AcquireWriterLock(stateRoot string) (*WriterLock, error) {
	lockPath := filepath.Join(stateRoot, ".writer.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open writer lock: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire writer lock: %w", err)
	}

	return &WriterLock{f: f}, nil
}

func (l *WriterLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	if err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.f.Close()
		return fmt.Errorf("release writer lock: %w", err)
	}
	if err := l.f.Close(); err != nil {
		return fmt.Errorf("close writer lock: %w", err)
	}
	l.f = nil
	return nil
}

package instance

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

const lockRegionLength = 1

// Windows byte locks are mandatory: lock beyond metadata so another process can read the activation address.
func lockFile(file *os.File) (bool, error) {
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, lockRegionLength, 0, &windows.Overlapped{Offset: uint32(metadataLimit)})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

//go:build !linux && !darwin

package sys

import "os"

func datasync(file *os.File) error {
	// fallback to sync since it is supposed to be a superset of datasync
	return file.Sync()
}

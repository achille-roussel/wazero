//go:build !linux

package sys

const rootfsOpenFileFlags = O_NOFOLLOW | O_RDONLY

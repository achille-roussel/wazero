package sys

const (
	rootfsOpenFileFlags = O_NOFOLLOW | O_PATH | O_RDONLY
	rootfsReadlinkFlags = O_NOFOLLOW | O_PATH | O_RDONLY
)

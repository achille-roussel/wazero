package sys_test

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
)

func TestLayerFS_ReadOnly(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		return sys.ReadOnlyFS(
			sys.LayerFS(
				sys.NewFS(baseFS),
				sys.ErrFS(sys.ErrNotExist),
			),
		)
	})
}

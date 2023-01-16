package sys_test

import (
	"math/rand"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func TestDevice(t *testing.T) {
	maj := int(rand.Uint32())
	min := int(rand.Uint32())

	if runtime.GOOS == "darwin" {
		maj &= 0xFFFFFF
		min &= 0xFF
	}

	device := sys.Dev(maj, min)
	major := device.Major()
	minor := device.Minor()

	if major != maj {
		t.Errorf("major device number mismatch:\nwant = %08X\ngot  = %08X", maj, major)
	}
	if minor != min {
		t.Errorf("minor device number mismatch:\nwant = %08X\ngot  = %08X", min, minor)
	}
}

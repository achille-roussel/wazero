package wasi

import (
	"testing"

	"github.com/tetratelabs/wazero/wasi/syscall"
)

func TestTable(t *testing.T) {
	table := new(fileTable)

	if n := table.len(); n != 0 {
		t.Errorf("new table is not empty: length=%d", n)
	}

	v0 := new(file)
	v1 := new(file)
	v2 := new(file)

	k0 := table.insert(v0)
	k1 := table.insert(v1)
	k2 := table.insert(v2)

	for _, lookup := range []struct {
		key syscall.Fd
		val *file
	}{
		{key: k0, val: v0},
		{key: k1, val: v1},
		{key: k2, val: v2},
	} {
		if v := table.lookup(lookup.key); v == nil {
			t.Errorf("value not found for key '%v'", lookup.key)
		} else if v != lookup.val {
			t.Errorf("wrong value returned for key '%v': want=%q got=%q", lookup.key, lookup.val, v)
		}
	}

	if n := table.len(); n != 3 {
		t.Errorf("wrong table length: want=3 got=%d", n)
	}

	k0Found := false
	k1Found := false
	k2Found := false
	table.scan(func(k syscall.Fd, v *file) bool {
		var want *file
		switch k {
		case k0:
			k0Found, want = true, v0
		case k1:
			k1Found, want = true, v1
		case k2:
			k2Found, want = true, v2
		}
		if v != want {
			t.Errorf("wrong value found ranging over '%v': want=%q got=%q", k, want, v)
		}
		return true
	})

	for _, found := range []struct {
		key syscall.Fd
		ok  bool
	}{
		{key: k0, ok: k0Found},
		{key: k1, ok: k1Found},
		{key: k2, ok: k2Found},
	} {
		if !found.ok {
			t.Errorf("key not found while ranging over table: %v", found.key)
		}
	}

	for i, deletion := range []struct {
		key syscall.Fd
		val *file
	}{
		{key: k1, val: v1},
		{key: k0, val: v0},
		{key: k2, val: v2},
	} {
		if v := table.delete(deletion.key); v == nil {
			t.Errorf("no values were deleted for key '%v'", deletion.key)
		} else if v != deletion.val {
			t.Errorf("wrong value returned when deleting '%v': want=%q got=%q", deletion.key, deletion.val, v)
		}
		if n, want := table.len(), 3-(i+1); n != want {
			t.Errorf("wrong table length after deletion: want=%d got=%d", want, n)
		}
	}
}

func BenchmarkTableInsert(b *testing.B) {
	table := new(fileTable)
	file := new(file)

	for i := 0; i < b.N; i++ {
		table.insert(file)

		if (i % 65536) == 0 {
			table.reset() // to avoid running out of memory
		}
	}
}

func BenchmarkTableLookup(b *testing.B) {
	const N = 65536
	table := new(fileTable)
	keys := make([]syscall.Fd, N)
	sentinel := new(file)

	for i := range keys {
		keys[i] = table.insert(sentinel)
	}

	var f *file
	for i := 0; i < b.N; i++ {
		f = table.lookup(keys[i%N])
	}
	if f != sentinel {
		b.Error("wrong file returned by lookup")
	}
}

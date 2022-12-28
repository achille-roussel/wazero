package wasi_snapshot_preview1

import "testing"

func TestFileTable(t *testing.T) {
	table := new(fileTable)

	if n := table.len(); n != 0 {
		t.Errorf("new table is not empty: length=%d", n)
	}

	// The dircookie field is used as a sentinel value.
	v0 := file{dircookie: 1}
	v1 := file{dircookie: 2}
	v2 := file{dircookie: 3}

	k0 := table.insert(v0)
	k1 := table.insert(v1)
	k2 := table.insert(v2)

	for _, lookup := range []struct {
		key Fd
		val file
	}{
		{key: k0, val: v0},
		{key: k1, val: v1},
		{key: k2, val: v2},
	} {
		if v := table.lookup(lookup.key); v == nil {
			t.Errorf("value not found for key '%v'", lookup.key)
		} else if v.dircookie != lookup.val.dircookie {
			t.Errorf("wrong value returned for key '%v': want=%v got=%v", lookup.key, lookup.val.dircookie, v.dircookie)
		}
	}

	if n := table.len(); n != 3 {
		t.Errorf("wrong table length: want=3 got=%d", n)
	}

	k0Found := false
	k1Found := false
	k2Found := false
	table.scan(func(k Fd, v *file) bool {
		var want file
		switch k {
		case k0:
			k0Found, want = true, v0
		case k1:
			k1Found, want = true, v1
		case k2:
			k2Found, want = true, v2
		}
		if v.dircookie != want.dircookie {
			t.Errorf("wrong value found ranging over '%v': want=%v got=%v", k, want.dircookie, v.dircookie)
		}
		return true
	})

	for _, found := range []struct {
		key Fd
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
		key Fd
	}{
		{key: k1},
		{key: k0},
		{key: k2},
	} {
		table.delete(deletion.key)
		if table.lookup(deletion.key) != nil {
			t.Errorf("item found after deletion of '%v'", deletion.key)
		}
		if n, want := table.len(), 3-(i+1); n != want {
			t.Errorf("wrong table length after deletion: want=%d got=%d", want, n)
		}
	}
}

func BenchmarkFileTableInsert(b *testing.B) {
	table := new(fileTable)

	for i := 0; i < b.N; i++ {
		table.insert(file{})

		if (i % 65536) == 0 {
			table.reset() // to avoid running out of memory
		}
	}
}

func BenchmarkFileTableLookup(b *testing.B) {
	const sentinel = 42
	const numFiles = 65536
	table := new(fileTable)
	files := make([]Fd, numFiles)

	for i := range files {
		files[i] = table.insert(file{dircookie: sentinel})
	}

	var f *file
	for i := 0; i < b.N; i++ {
		f = table.lookup(files[i%numFiles])
	}
	if f.dircookie != sentinel {
		b.Error("wrong file returned by lookup")
	}
}
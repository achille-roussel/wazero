package wasm

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/sys"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/wasi"
)

func TestCallContext_WithMemory(t *testing.T) {
	tests := []struct {
		name       string
		mod        *CallContext
		mem        *MemoryInstance
		expectSame bool
	}{
		{
			name:       "nil->nil: same",
			mod:        &CallContext{},
			mem:        nil,
			expectSame: true,
		},
		{
			name:       "nil->mem: not same",
			mod:        &CallContext{},
			mem:        &MemoryInstance{},
			expectSame: false,
		},
		{
			name:       "mem->nil: same",
			mod:        &CallContext{memory: &MemoryInstance{}},
			mem:        nil,
			expectSame: true,
		},
		{
			name:       "mem1->mem2: not same",
			mod:        &CallContext{memory: &MemoryInstance{}},
			mem:        &MemoryInstance{},
			expectSame: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod2 := tc.mod.WithMemory(tc.mem)
			if tc.expectSame {
				require.Same(t, tc.mod, mod2)
			} else {
				require.NotSame(t, tc.mod, mod2)
				require.Equal(t, tc.mem, mod2.memory)
			}
		})
	}
}

func TestCallContext_String(t *testing.T) {
	s, ns := newStore()

	tests := []struct {
		name, moduleName, expected string
	}{
		{
			name:       "empty",
			moduleName: "",
			expected:   "Module[]",
		},
		{
			name:       "not empty",
			moduleName: "math",
			expected:   "Module[math]",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			// Ensure paths that can create the host module can see the name.
			m, err := s.Instantiate(context.Background(), ns, &Module{}, tc.moduleName, nil)
			defer m.Close(testCtx) //nolint

			require.NoError(t, err)
			require.Equal(t, tc.expected, m.String())
			require.Equal(t, tc.expected, ns.Module(m.Name()).String())
		})
	}
}

func TestCallContext_Close(t *testing.T) {
	s, ns := newStore()

	tests := []struct {
		name           string
		closer         func(context.Context, *CallContext) error
		expectedClosed uint64
	}{
		{
			name: "Close()",
			closer: func(ctx context.Context, callContext *CallContext) error {
				return callContext.Close(ctx)
			},
			expectedClosed: uint64(1),
		},
		{
			name: "CloseWithExitCode(255)",
			closer: func(ctx context.Context, callContext *CallContext) error {
				return callContext.CloseWithExitCode(ctx, 255)
			},
			expectedClosed: uint64(255)<<32 + 1,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("%s calls ns.CloseWithExitCode(module.name))", tc.name), func(t *testing.T) {
			for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
				moduleName := t.Name()
				m, err := s.Instantiate(ctx, ns, &Module{}, moduleName, nil)
				require.NoError(t, err)

				// We use side effects to see if Close called ns.CloseWithExitCode (without repeating store_test.go).
				// One side effect of ns.CloseWithExitCode is that the moduleName can no longer be looked up.
				require.Equal(t, ns.Module(moduleName), m)

				// Closing should not err.
				require.NoError(t, tc.closer(ctx, m))

				require.Equal(t, tc.expectedClosed, *m.closed)

				// Verify our intended side-effect
				require.Nil(t, ns.Module(moduleName))

				// Verify no error closing again.
				require.NoError(t, tc.closer(ctx, m))
			}
		})
	}

	t.Run("calls Context.Close()", func(t *testing.T) {
		sysCtx := sys.DefaultContext(wasi.NewFS(testfs.FS{"foo": &testfs.File{}}))

		_, err := pathOpen(sysCtx, "/foo")
		require.NoError(t, err)

		m, err := s.Instantiate(context.Background(), ns, &Module{}, t.Name(), sysCtx)
		require.NoError(t, err)

		// We use side effects to determine if Close in fact called Context.Close (without repeating sys_test.go).
		// One side effect of Context.Close is that it closes all open files. Verify our base case.
		numFilesBefore := sysCtx.NumFiles()
		require.True(t, numFilesBefore != 0, "sysCtx has opened files")

		// Closing should not err.
		require.NoError(t, m.Close(testCtx))

		// Verify our intended side-effect
		numFilesAfter := sysCtx.NumFiles()
		require.False(t, numFilesAfter != 0, "expected no opened files")

		// Verify no error closing again.
		require.NoError(t, m.Close(testCtx))
	})

	t.Run("error closing", func(t *testing.T) {
		// Right now, the only way to err closing the sys context is if a File.Close erred.
		testFS := testfs.FS{"foo": &testfs.File{CloseErr: errors.New("error closing")}}
		sysCtx := sys.DefaultContext(wasi.NewFS(testFS))

		_, err := pathOpen(sysCtx, "/foo")
		require.NoError(t, err)

		m, err := s.Instantiate(context.Background(), ns, &Module{}, t.Name(), sysCtx)
		require.NoError(t, err)

		require.EqualError(t, m.Close(testCtx), "error closing")

		// Verify our intended side-effect
		numFiles := sysCtx.NumFiles()
		require.False(t, numFiles != 0, "expected no opened files")
	})
}

func TestCallContext_CallDynamic(t *testing.T) {
	s, ns := newStore()

	tests := []struct {
		name           string
		closer         func(context.Context, *CallContext) error
		expectedClosed uint64
	}{
		{
			name: "Close()",
			closer: func(ctx context.Context, callContext *CallContext) error {
				return callContext.Close(ctx)
			},
			expectedClosed: uint64(1),
		},
		{
			name: "CloseWithExitCode(255)",
			closer: func(ctx context.Context, callContext *CallContext) error {
				return callContext.CloseWithExitCode(ctx, 255)
			},
			expectedClosed: uint64(255)<<32 + 1,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("%s calls ns.CloseWithExitCode(module.name))", tc.name), func(t *testing.T) {
			for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
				moduleName := t.Name()
				m, err := s.Instantiate(ctx, ns, &Module{}, moduleName, nil)
				require.NoError(t, err)

				// We use side effects to see if Close called ns.CloseWithExitCode (without repeating store_test.go).
				// One side effect of ns.CloseWithExitCode is that the moduleName can no longer be looked up.
				require.Equal(t, ns.Module(moduleName), m)

				// Closing should not err.
				require.NoError(t, tc.closer(ctx, m))

				require.Equal(t, tc.expectedClosed, *m.closed)

				// Verify our intended side-effect
				require.Nil(t, ns.Module(moduleName))

				// Verify no error closing again.
				require.NoError(t, tc.closer(ctx, m))
			}
		})
	}

	t.Run("calls Context.Close()", func(t *testing.T) {
		sysCtx := sys.DefaultContext(wasi.NewFS(testfs.FS{"foo": &testfs.File{}}))

		_, err := pathOpen(sysCtx, "/foo")
		require.NoError(t, err)

		m, err := s.Instantiate(context.Background(), ns, &Module{}, t.Name(), sysCtx)
		require.NoError(t, err)

		// We use side effects to determine if Close in fact called Context.Close (without repeating sys_test.go).
		// One side effect of Context.Close is that it closes all open files. Verify our base case.
		numFilesBefore := sysCtx.NumFiles()
		require.True(t, numFilesBefore != 0, "sysCtx has opened files")

		// Closing should not err.
		require.NoError(t, m.Close(testCtx))

		// Verify our intended side-effect
		numFilesAfter := sysCtx.NumFiles()
		require.False(t, numFilesAfter != 0, "expected no opened files")

		// Verify no error closing again.
		require.NoError(t, m.Close(testCtx))
	})

	t.Run("error closing", func(t *testing.T) {
		// Right now, the only way to err closing the sys context is if a File.Close erred.
		testFS := testfs.FS{"foo": &testfs.File{CloseErr: errors.New("error closing")}}
		sysCtx := sys.DefaultContext(wasi.NewFS(testFS))

		_, err := pathOpen(sysCtx, "/foo")
		require.NoError(t, err)

		m, err := s.Instantiate(context.Background(), ns, &Module{}, t.Name(), sysCtx)
		require.NoError(t, err)

		require.EqualError(t, m.Close(testCtx), "error closing")

		// Verify our intended side-effect
		numFiles := sysCtx.NumFiles()
		require.False(t, numFiles != 0, "expected no opened files")
	})
}

func pathOpen(sysCtx *sys.Context, path string) (wasi_snapshot_preview1.Fd, error) {
	fd, errno := sysCtx.PathOpen(
		wasi_snapshot_preview1.None,
		0, // lookupflags
		path,
		0,                        // oflags
		wasi_snapshot_preview1.R, // fsRightsBase
		wasi_snapshot_preview1.R, // fsRightsInheriting
		0,                        // fdflags
	)
	if errno != wasi_snapshot_preview1.ESUCCESS {
		return wasi_snapshot_preview1.None, errno
	}
	return fd, nil
}

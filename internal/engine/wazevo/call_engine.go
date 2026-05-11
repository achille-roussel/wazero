package wazevo

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/expctxkeys"
	"github.com/tetratelabs/wazero/internal/internalapi"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

type (
	// callEngine implements api.Function.
	callEngine struct {
		internalapi.WazeroOnly
		stack []byte
		// stackTop is the pointer to the *aligned* top of the stack. This must be updated
		// whenever the stack is changed. This is passed to the assembly function
		// at the very beginning of api.Function Call/CallWithStack.
		stackTop uintptr
		// executable is the pointer to the executable code for this function.
		executable         *byte
		preambleExecutable *byte
		// parent is the *moduleEngine from which this callEngine is created.
		parent *moduleEngine
		// indexInModule is the index of the function in the module.
		indexInModule wasm.Index
		// sizeOfParamResultSlice is the size of the parameter/result slice.
		sizeOfParamResultSlice int
		requiredParams         int
		// execCtx holds various information to be read/written by assembly functions.
		execCtx executionContext
		// execCtxPtr holds the pointer to the executionContext which doesn't change after callEngine is created.
		execCtxPtr        uintptr
		numberOfResults   int
		stackIteratorImpl stackIterator
		// tryHandlers is the stack of active try_table exception handlers,
		// used to match catch clauses when a throw exits to the dispatch loop.
		tryHandlers []tryHandler
		// pendingException holds the most recently caught exception, so handler
		// code can read its params after re-entry.
		pendingException *wasm.Exception
		// gcKeepAlive holds *wasm.WasmStruct / *wasm.WasmArray (and any
		// other wasm-gc heap objects) allocated during this call. Refs
		// on the operand stack are stored as raw uintptrs which Go's GC
		// cannot trace; this slice keeps them reachable for the lifetime
		// of the call. Append-only; cleared at end-of-call.
		gcKeepAlive []any
	}

	// tryHandler records the state at a try_table entry for exception handling.
	// On match, we restore the stack to the checkpoint state and re-enter at returnAddress.
	tryHandler struct {
		// Cloned stack and state from the try_table entry checkpoint,
		// using the same approach as experimental.Snapshot.
		sp, fp, top    uintptr
		returnAddress  *byte
		savedRegisters [64][2]uint64
		stack          []byte // cloned stack
		// catchClauses describes what exceptions this handler catches.
		catchClauses []wazevoapi.CatchClauseInstance
		// moduleInstance is the module that set up this try handler.
		// Used for tag matching in doHandleException (the tag index in
		// catch clauses is relative to this module's tag index space).
		moduleInstance *wasm.ModuleInstance
	}

	// executionContext is the struct to be read/written by assembly functions.
	executionContext struct {
		// exitCode holds the wazevoapi.ExitCode describing the state of the function execution.
		exitCode wazevoapi.ExitCode
		// callerModuleContextPtr holds the moduleContextOpaque for Go function calls.
		callerModuleContextPtr *byte
		// originalFramePointer holds the original frame pointer of the caller of the assembly function.
		originalFramePointer uintptr
		// originalStackPointer holds the original stack pointer of the caller of the assembly function.
		originalStackPointer uintptr
		// goReturnAddress holds the return address to go back to the caller of the assembly function.
		goReturnAddress uintptr
		// stackBottomPtr holds the pointer to the bottom of the stack.
		stackBottomPtr *byte
		// goCallReturnAddress holds the return address to go back to the caller of the Go function.
		goCallReturnAddress *byte
		// stackPointerBeforeGoCall holds the stack pointer before calling a Go function.
		stackPointerBeforeGoCall *uint64
		// stackGrowRequiredSize holds the required size of stack grow.
		stackGrowRequiredSize uintptr
		// memoryGrowTrampolineAddress holds the address of memory grow trampoline function.
		memoryGrowTrampolineAddress *byte
		// stackGrowCallTrampolineAddress holds the address of stack grow trampoline function.
		stackGrowCallTrampolineAddress *byte
		// checkModuleExitCodeTrampolineAddress holds the address of check-module-exit-code function.
		checkModuleExitCodeTrampolineAddress *byte
		// savedRegisters is the opaque spaces for save/restore registers.
		// We want to align 16 bytes for each register, so we use [64][2]uint64.
		savedRegisters [64][2]uint64
		// goFunctionCallCalleeModuleContextOpaque is the pointer to the target Go function's moduleContextOpaque.
		goFunctionCallCalleeModuleContextOpaque uintptr
		// tableGrowTrampolineAddress holds the address of table grow trampoline function.
		tableGrowTrampolineAddress *byte
		// refFuncTrampolineAddress holds the address of ref-func trampoline function.
		refFuncTrampolineAddress *byte
		// memmoveAddress holds the address of memmove function implemented by Go runtime. See memmove.go.
		memmoveAddress uintptr
		// framePointerBeforeGoCall holds the frame pointer before calling a Go function. Note: only used in amd64.
		framePointerBeforeGoCall uintptr
		// memoryWait32TrampolineAddress holds the address of memory_wait32 trampoline function.
		memoryWait32TrampolineAddress *byte
		// memoryWait32TrampolineAddress holds the address of memory_wait64 trampoline function.
		memoryWait64TrampolineAddress *byte
		// memoryNotifyTrampolineAddress holds the address of the memory_notify trampoline function.
		memoryNotifyTrampolineAddress *byte
		// throwAllocTrampolineAddress holds the address of the throw-alloc trampoline:
		// phase 1 of throw, which allocates the Exception heap object.
		throwAllocTrampolineAddress *byte
		// throwTrampolineAddress holds the address of the throw/throw_ref trampoline function.
		throwTrampolineAddress *byte
		// tryTableEnterTrampolineAddress holds the address of the try_table enter trampoline function.
		tryTableEnterTrampolineAddress *byte
		// tryTableLeaveTrampolineAddress holds the address of the try_table leave trampoline function.
		tryTableLeaveTrampolineAddress *byte
		// exceptionPtr holds the pointer to the Exception struct,
		// used on the throw side (throwAlloc stores the new Exception)
		// and on the catch side (catch_ref/catch_all_ref retrieve the exnref).
		exceptionPtr uintptr
		// exceptionParamsPtr points into exceptionPtr's Params slice
		// backing array. On the throw side, throwAlloc sets it so compiled
		// code can store params at [ptr + i*8]. On the catch side, compiled
		// handler blocks load params from the same pointer.
		exceptionParamsPtr uintptr
		// caughtExceptionClauseIdx is set by the dispatch loop to -1 on
		// TryTableEnter (normal path) or to the matched catch clause index
		// when an exception is caught. Compiled code loads this from execCtx
		// after the trampoline call to decide which handler to dispatch to.
		caughtExceptionClauseIdx int64
		// callIndirectSubtypeCheckTrampolineAddress holds the address of the
		// wasm-gc subtype-aware call_indirect / call_ref check trampoline.
		// The trampoline writes (actualTypeID, expectedTypeID) onto the
		// goCallStack and exits with ExitCodeCallIndirectSubtypeCheck.
		callIndirectSubtypeCheckTrampolineAddress *byte
		// allocStructTrampolineAddress holds the address of the wasm-gc
		// struct allocator trampoline.
		allocStructTrampolineAddress *byte
		// allocArrayTrampolineAddress holds the address of the wasm-gc
		// array allocator trampoline.
		allocArrayTrampolineAddress *byte
		// gcScratchBuffer is a fixed-size buffer used by wasm-gc operations
		// to pass variable-length argument vectors (e.g. struct.new field
		// values) between native code and the Go-side allocator handlers.
		// Sized to handle typical wasm-gc producers; allocations exceeding
		// this size trap with a clear error.
		gcScratchBuffer [256]uint64
		// gcAccessTrampolineAddress is the address of the wasm-gc unified
		// heap-access trampoline. See GCAccessMode for the modes dispatch.
		gcAccessTrampolineAddress *byte
	}
)

func (c *callEngine) requiredInitialStackSize() int {
	const initialStackSizeDefault = 10240
	stackSize := initialStackSizeDefault
	paramResultInBytes := c.sizeOfParamResultSlice * 8 * 2 // * 8 because uint64 is 8 bytes, and *2 because we need both separated param/result slots.
	required := paramResultInBytes + 32 + 16               // 32 is enough to accommodate the call frame info, and 16 exists just in case when []byte is not aligned to 16 bytes.
	if required > stackSize {
		stackSize = required
	}
	return stackSize
}

func (c *callEngine) init() {
	stackSize := c.requiredInitialStackSize()
	if wazevoapi.StackGuardCheckEnabled {
		stackSize += wazevoapi.StackGuardCheckGuardPageSize
	}
	c.stack = make([]byte, stackSize)
	c.stackTop = alignedStackTop(c.stack)
	if wazevoapi.StackGuardCheckEnabled {
		c.execCtx.stackBottomPtr = &c.stack[wazevoapi.StackGuardCheckGuardPageSize]
	} else {
		c.execCtx.stackBottomPtr = &c.stack[0]
	}
	c.execCtxPtr = uintptr(unsafe.Pointer(&c.execCtx))
}

// alignedStackTop returns 16-bytes aligned stack top of given stack.
// 16 bytes should be good for all platform (arm64/amd64).
func alignedStackTop(s []byte) uintptr {
	stackAddr := uintptr(unsafe.Pointer(&s[len(s)-1]))
	return stackAddr - (stackAddr & (16 - 1))
}

// Definition implements api.Function.
func (c *callEngine) Definition() api.FunctionDefinition {
	return c.parent.module.Source.FunctionDefinition(c.indexInModule)
}

// Call implements api.Function.
func (c *callEngine) Call(ctx context.Context, params ...uint64) ([]uint64, error) {
	if c.requiredParams != len(params) {
		return nil, fmt.Errorf("expected %d params, but passed %d", c.requiredParams, len(params))
	}
	paramResultSlice := make([]uint64, c.sizeOfParamResultSlice)
	copy(paramResultSlice, params)
	if err := c.callWithStack(ctx, paramResultSlice); err != nil {
		return nil, err
	}
	return paramResultSlice[:c.numberOfResults], nil
}

func (c *callEngine) addFrame(builder wasmdebug.ErrorBuilder, addr uintptr) (def api.FunctionDefinition, listener experimental.FunctionListener) {
	eng := c.parent.parent.parent
	cm := eng.compiledModuleOfAddr(addr)
	if cm == nil {
		// This case, the module might have been closed and deleted from the engine.
		// We fall back to searching the imported modules that can be referenced from this callEngine.

		// First, we check itself.
		if checkAddrInBytes(addr, c.parent.parent.executable) {
			cm = c.parent.parent
		} else {
			// Otherwise, search all imported modules. TODO: maybe recursive, but not sure it's useful in practice.
			p := c.parent
			for i := range p.importedFunctions {
				candidate := p.importedFunctions[i].me.parent
				if checkAddrInBytes(addr, candidate.executable) {
					cm = candidate
					break
				}
			}
		}
	}

	if cm != nil {
		index := cm.functionIndexOf(addr)
		def = cm.module.FunctionDefinition(cm.module.ImportFunctionCount + index)
		var sources []string
		if dw := cm.module.DWARFLines; dw != nil {
			sourceOffset := cm.getSourceOffset(addr)
			sources = dw.Line(sourceOffset)
		}
		builder.AddFrame(def.DebugName(), def.ParamTypes(), def.ResultTypes(), sources)
		if len(cm.listeners) > 0 {
			listener = cm.listeners[index]
		}
	}
	return
}

// CallWithStack implements api.Function.
func (c *callEngine) CallWithStack(ctx context.Context, paramResultStack []uint64) (err error) {
	if c.sizeOfParamResultSlice > len(paramResultStack) {
		return fmt.Errorf("need %d params, but stack size is %d", c.sizeOfParamResultSlice, len(paramResultStack))
	}
	return c.callWithStack(ctx, paramResultStack)
}

// CallWithStack implements api.Function.
func (c *callEngine) callWithStack(ctx context.Context, paramResultStack []uint64) (err error) {
	snapshotEnabled := ctx.Value(expctxkeys.EnableSnapshotterKey{}) != nil
	if snapshotEnabled {
		ctx = context.WithValue(ctx, expctxkeys.SnapshotterKey{}, c)
	}

	if wazevoapi.StackGuardCheckEnabled {
		defer func() {
			wazevoapi.CheckStackGuardPage(c.stack)
		}()
	}

	p := c.parent
	ensureTermination := p.parent.ensureTermination
	m := p.module
	if ensureTermination {
		select {
		case <-ctx.Done():
			// If the provided context is already done, close the module and return the error.
			m.CloseWithCtxErr(ctx)
			return m.FailIfClosed()
		default:
		}
	}

	// Clear any stale try_table handlers from a previous call.
	c.tryHandlers = c.tryHandlers[:0]

	var paramResultPtr *uint64
	if len(paramResultStack) > 0 {
		paramResultPtr = &paramResultStack[0]
	}
	defer func() {
		r := recover()
		if s, ok := r.(*snapshot); ok {
			// A snapshot that wasn't handled was created by a different call engine possibly from a nested wasm invocation,
			// let it propagate up to be handled by the caller.
			panic(s)
		}
		if r != nil {
			type listenerForAbort struct {
				def api.FunctionDefinition
				lsn experimental.FunctionListener
			}

			var listeners []listenerForAbort
			builder := wasmdebug.NewErrorBuilder()
			if c.execCtx.stackPointerBeforeGoCall != nil {
				def, lsn := c.addFrame(builder, uintptr(unsafe.Pointer(c.execCtx.goCallReturnAddress)))
				if lsn != nil {
					listeners = append(listeners, listenerForAbort{def, lsn})
				}
				returnAddrs := unwindStack(
					uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)),
					c.execCtx.framePointerBeforeGoCall,
					c.stackTop,
					nil,
				)
				if len(returnAddrs) > 1 {
					for _, retAddr := range returnAddrs[:len(returnAddrs)-1] { // the last return addr is the trampoline, so we skip it.
						def, lsn = c.addFrame(builder, retAddr)
						if lsn != nil {
							listeners = append(listeners, listenerForAbort{def, lsn})
						}
					}
				}
			}
			err = builder.FromRecovered(r)

			for _, lsn := range listeners {
				lsn.lsn.Abort(ctx, m, lsn.def, err)
			}
		} else {
			if err != wasmruntime.ErrRuntimeStackOverflow { // Stackoverflow case shouldn't be panic (to avoid extreme stack unwinding).
				err = c.parent.module.FailIfClosed()
			}
		}

		if err != nil {
			// Ensures that we can reuse this callEngine even after an error.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			c.tryHandlers = c.tryHandlers[:0]
		}
	}()

	if ensureTermination {
		done := m.CloseModuleOnCanceledOrTimeout(ctx)
		defer done()
	}

	if c.stackTop&(16-1) != 0 {
		panic("BUG: stack must be aligned to 16 bytes")
	}
	entrypoint(c.preambleExecutable, c.executable, c.execCtxPtr, c.parent.opaquePtr, paramResultPtr, c.stackTop)
	for {
		switch ec := c.execCtx.exitCode; ec & wazevoapi.ExitCodeMask {
		case wazevoapi.ExitCodeOK:
			return nil
		case wazevoapi.ExitCodeGrowStack:
			oldsp := uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
			oldTop := c.stackTop
			oldStack := c.stack
			var newsp, newfp uintptr
			if wazevoapi.StackGuardCheckEnabled {
				newsp, newfp, err = c.growStackWithGuarded()
			} else {
				newsp, newfp, err = c.growStack()
			}
			if err != nil {
				return err
			}
			adjustClonedStack(oldsp, oldTop, newsp, newfp, c.stackTop)
			// Old stack must be alive until the new stack is adjusted.
			runtime.KeepAlive(oldStack)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, newsp, newfp)
		case wazevoapi.ExitCodeGrowMemory:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			argRes := &s[0]
			if res, ok := mem.Grow(uint32(*argRes)); !ok {
				*argRes = uint64(0xffffffff) // = -1 in signed 32-bit integer.
			} else {
				*argRes = uint64(res)
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeTableGrow:
			mod := c.callerModuleInstance()
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			tableIndex, num, ref := uint32(s[0]), uint32(s[1]), uintptr(s[2])
			table := mod.Tables[tableIndex]
			s[0] = uint64(uint32(int32(table.Grow(num, ref))))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoFunction:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, goCallStackView(c.execCtx.stackPointerBeforeGoCall))
			}()
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoFunctionWithListener:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			listeners := hostModuleListenersSliceFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			// Call Listener.Before.
			callerModule := c.callerModuleInstance()
			listener := listeners[index]
			hostModule := hostModuleFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			def := hostModule.FunctionDefinition(wasm.Index(index))
			listener.Before(ctx, callerModule, def, s, c.stackIterator(true))
			// Call into the Go function.
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, s)
			}()
			// Call Listener.After.
			listener.After(ctx, callerModule, def, s)
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoModuleFunction:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoModuleFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			mod := c.callerModuleInstance()
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, mod, goCallStackView(c.execCtx.stackPointerBeforeGoCall))
			}()
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoModuleFunctionWithListener:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoModuleFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			listeners := hostModuleListenersSliceFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			// Call Listener.Before.
			callerModule := c.callerModuleInstance()
			listener := listeners[index]
			hostModule := hostModuleFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			def := hostModule.FunctionDefinition(wasm.Index(index))
			listener.Before(ctx, callerModule, def, s, c.stackIterator(true))
			// Call into the Go function.
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, callerModule, s)
			}()
			// Call Listener.After.
			listener.After(ctx, callerModule, def, s)
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallListenerBefore:
			stack := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			index := wasm.Index(stack[0])
			mod := c.callerModuleInstance()
			listener := mod.Engine.(*moduleEngine).listeners[index]
			def := mod.Source.FunctionDefinition(index + mod.Source.ImportFunctionCount)
			listener.Before(ctx, mod, def, stack[1:], c.stackIterator(false))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallListenerAfter:
			stack := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			index := wasm.Index(stack[0])
			mod := c.callerModuleInstance()
			listener := mod.Engine.(*moduleEngine).listeners[index]
			def := mod.Source.FunctionDefinition(index + mod.Source.ImportFunctionCount)
			listener.After(ctx, mod, def, stack[1:])
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCheckModuleExitCode:
			// Note: this operation must be done in Go, not native code. The reason is that
			// native code cannot be preempted and that means it can block forever if there are not
			// enough OS threads (which we don't have control over).
			if err := m.FailIfClosed(); err != nil {
				panic(err)
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeRefFunc:
			mod := c.callerModuleInstance()
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			funcIndex := wasm.Index(s[0])
			ref := mod.Engine.FunctionInstanceReference(funcIndex)
			s[0] = uint64(ref)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeMemoryWait32:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance
			if !mem.Shared {
				panic(wasmruntime.ErrRuntimeExpectedSharedMemory)
			}

			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			timeout, exp, addr := int64(s[0]), uint32(s[1]), uintptr(s[2])
			base := uintptr(unsafe.Pointer(&mem.Buffer[0]))

			offset := uint32(addr - base)
			res := mem.Wait32(offset, exp, timeout, func(mem *wasm.MemoryInstance, offset uint32) uint32 {
				addr := unsafe.Add(unsafe.Pointer(&mem.Buffer[0]), offset)
				return atomic.LoadUint32((*uint32)(addr))
			})
			s[0] = res
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeMemoryWait64:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance
			if !mem.Shared {
				panic(wasmruntime.ErrRuntimeExpectedSharedMemory)
			}

			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			timeout, exp, addr := int64(s[0]), uint64(s[1]), uintptr(s[2])
			base := uintptr(unsafe.Pointer(&mem.Buffer[0]))

			offset := uint32(addr - base)
			res := mem.Wait64(offset, exp, timeout, func(mem *wasm.MemoryInstance, offset uint32) uint64 {
				addr := unsafe.Add(unsafe.Pointer(&mem.Buffer[0]), offset)
				return atomic.LoadUint64((*uint64)(addr))
			})
			s[0] = uint64(res)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeMemoryNotify:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance

			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			count, addr := uint32(s[0]), s[1]
			offset := uint32(uintptr(addr) - uintptr(unsafe.Pointer(&mem.Buffer[0])))
			res := mem.Notify(offset, count)
			s[0] = uint64(res)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeUnreachable:
			panic(wasmruntime.ErrRuntimeUnreachable)
		case wazevoapi.ExitCodeMemoryOutOfBounds:
			panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
		case wazevoapi.ExitCodeTableOutOfBounds:
			panic(wasmruntime.ErrRuntimeInvalidTableAccess)
		case wazevoapi.ExitCodeIndirectCallNullPointer:
			panic(wasmruntime.ErrRuntimeInvalidTableAccess)
		case wazevoapi.ExitCodeIndirectCallTypeMismatch:
			panic(wasmruntime.ErrRuntimeIndirectCallTypeMismatch)
		case wazevoapi.ExitCodeIntegerOverflow:
			panic(wasmruntime.ErrRuntimeIntegerOverflow)
		case wazevoapi.ExitCodeIntegerDivisionByZero:
			panic(wasmruntime.ErrRuntimeIntegerDivideByZero)
		case wazevoapi.ExitCodeInvalidConversionToInteger:
			panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
		case wazevoapi.ExitCodeUnalignedAtomic:
			panic(wasmruntime.ErrRuntimeUnalignedAtomic)
		case wazevoapi.ExitCodeThrowAlloc:
			// Allocate the Exception heap object sized exactly to the tag's
			// param count. Sets exceptionParamsPtr so compiled code can
			// store params, and returns the exnref via the stack slot.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			tagIndex := int(s[0])
			mod := c.callerModuleInstance()
			tag := mod.Tags[tagIndex]
			nParams := len(tag.Type.Params)
			exn := &wasm.Exception{Tag: tag, Params: make([]uint64, nParams)}
			c.pendingException = exn // GC root: keeps exn alive while compiled code writes params
			if nParams > 0 {
				c.execCtx.exceptionParamsPtr = uintptr(unsafe.Pointer(&exn.Params[0]))
			}
			// Return the exnref to compiled code via the stack slot.
			s[0] = uint64(uintptr(unsafe.Pointer(exn)))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeThrow:
			// Throw trampoline: (execCtx, exnref) → ().
			// Reads the exnref from the stack, searches for a matching handler.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			// Read the Exception pointer directly from the uint64 value to avoid
			// conversion from uintptr into unsafe.Pointer, which triggers checkptr.
			exn := *(**wasm.Exception)(unsafe.Pointer(&s[0]))
			if !c.doHandleException(exn) {
				panic(wasmruntime.ErrRuntimeUncaughtException)
			}
			if len(exn.Params) > 0 {
				c.execCtx.exceptionParamsPtr = uintptr(unsafe.Pointer(&exn.Params[0]))
			}
			c.execCtx.exceptionPtr = uintptr(unsafe.Pointer(exn))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeNullReference:
			panic(wasmruntime.ErrRuntimeNullReference)
		case wazevoapi.ExitCodeTryTableEnter:
			// Save current state as a try handler checkpoint using stack cloning
			// (same approach as experimental.Snapshot).
			// The encoded exit code (with tryTableID in upper bits) is on the
			// Go call stack as the second trampoline argument, not in execCtx.exitCode.
			tryTableEnterStack := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			catchClauseTableIdx := wazevoapi.TryTableIDFromExitCode(wazevoapi.ExitCode(tryTableEnterStack[0]))
			mod := c.callerModuleInstance()
			me := mod.Engine.(*moduleEngine)
			clauses := me.parent.catchClauseTable[catchClauseTableIdx]
			returnAddress := c.execCtx.goCallReturnAddress
			oldTop, oldSp := c.stackTop, uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
			newSP, newFP, newTop, newStack := c.cloneStack(uintptr(len(c.stack)) + 16)
			adjustClonedStack(oldSp, oldTop, newSP, newFP, newTop)
			c.tryHandlers = append(c.tryHandlers, tryHandler{
				sp:             newSP,
				fp:             newFP,
				top:            newTop,
				returnAddress:  returnAddress,
				savedRegisters: c.execCtx.savedRegisters,
				stack:          newStack,
				catchClauses:   clauses,
				moduleInstance: mod,
			})
			// Set clauseIdx = -1 (no exception) in execCtx for the compiled code
			// to read after the trampoline returns.
			c.execCtx.caughtExceptionClauseIdx = -1
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeTryTableLeave:
			// Pop the most recent try handler.
			if len(c.tryHandlers) > 0 {
				c.tryHandlers = c.tryHandlers[:len(c.tryHandlers)-1]
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallIndirectSubtypeCheck:
			// wasm-gc subtype-aware runtime check for call_indirect /
			// call_ref. The trampoline passes (actualTypeID, expectedTypeID)
			// on the goCallStack. We consult Store.IsSubtype and either
			// resume (subtype match) or panic with the type-mismatch trap.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			actualTypeID := wasm.FunctionTypeID(uint32(s[0]))
			expectedTypeID := wasm.FunctionTypeID(uint32(s[1]))
			if actualTypeID != expectedTypeID {
				mod := c.callerModuleInstance()
				if !mod.GetStore().IsSubtype(actualTypeID, expectedTypeID) {
					panic(wasmruntime.ErrRuntimeIndirectCallTypeMismatch)
				}
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeAllocateStruct:
			// wasm-gc struct.new / struct.new_default. Trampoline passes
			// (typeIdx, fieldCount). When fieldCount > 0, the operand
			// values for the fields are in gcScratchBuffer[0..fieldCount].
			// When fieldCount == 0, fields are default-initialised.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			typeIdx := wasm.Index(uint32(s[0]))
			fieldCount := int(uint32(s[1]))
			mod := c.callerModuleInstance()
			schema := &mod.Source.TypeSection[typeIdx]
			fields := make([]any, len(schema.Fields))
			if fieldCount == 0 {
				for i, f := range schema.Fields {
					fields[i] = wasm.DefaultFieldValue(f)
				}
			} else {
				if fieldCount > len(c.execCtx.gcScratchBuffer) {
					panic(fmt.Errorf("wasm-gc struct.new field count %d exceeds scratch buffer size %d", fieldCount, len(c.execCtx.gcScratchBuffer)))
				}
				if fieldCount != len(schema.Fields) {
					panic(fmt.Errorf("wasm-gc struct.new field count %d does not match type field count %d", fieldCount, len(schema.Fields)))
				}
				for i := 0; i < fieldCount; i++ {
					fields[i] = wasm.EncodeFieldValue(schema.Fields[i], c.execCtx.gcScratchBuffer[i])
				}
			}
			ws := wasm.NewWasmStructWith(mod.TypeIDs[typeIdx], fields)
			c.gcKeepAlive = append(c.gcKeepAlive, ws)
			s[0] = uint64(uintptr(unsafe.Pointer(ws)))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeAllocateArray:
			// wasm-gc array.new / array.new_default / array.new_fixed /
			// array.new_data / array.new_elem. Trampoline passes
			// (typeIdx, mode, arg1, arg2, arg3); semantics depend on mode.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			typeIdx := wasm.Index(uint32(s[0]))
			mode := uint32(s[1])
			arg1, arg2, arg3 := s[2], s[3], s[4]
			mod := c.callerModuleInstance()
			schema := &mod.Source.TypeSection[typeIdx]
			elemField := schema.ArrayField
			wa := allocateWasmArray(c, mod, schema, elemField, typeIdx, mode, arg1, arg2, arg3)
			c.gcKeepAlive = append(c.gcKeepAlive, wa)
			s[0] = uint64(uintptr(unsafe.Pointer(wa)))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeGCAccess:
			// wasm-gc unified heap-access dispatcher. See GCAccessMode
			// for the per-mode arg layout.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			mode := wazevoapi.GCAccessMode(uint32(s[0]))
			typeIdx := wasm.Index(uint32(s[1]))
			auxIdx := uint32(s[2])
			arg1, arg2, arg3, arg4, arg5 := s[3], s[4], s[5], s[6], s[7]
			mod := c.callerModuleInstance()
			result := performGCAccess(c, mod, mode, typeIdx, auxIdx, arg1, arg2, arg3, arg4, arg5)
			s[0] = result
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		default:
			panic("BUG")
		}
	}
}

// doHandleException tries to match the given exception against active try handlers.
// If a match is found, it restores the execution state to the handler's checkpoint
// (like snapshot.doRestore) and writes the matched clause index as the trampoline
// return value. Returns true if handled.
func (c *callEngine) doHandleException(exn *wasm.Exception) bool {
	// Search try handlers from innermost (last) to outermost (first).
	for i := len(c.tryHandlers) - 1; i >= 0; i-- {
		h := &c.tryHandlers[i]
		for clauseIdx, clause := range h.catchClauses {
			// Use the module that set up the handler (not the one that threw)
			// because clause.TagIndex is relative to that module's tag space.
			mod := h.moduleInstance
			matched := false
			switch clause.Kind {
			case wasm.CatchKindCatch, wasm.CatchKindCatchRef:
				matched = mod.Tags[clause.TagIndex] == exn.Tag
			case wasm.CatchKindCatchAll, wasm.CatchKindCatchAllRef:
				matched = true
			}
			if matched {
				// Pop all handlers at and above this one.
				c.tryHandlers = c.tryHandlers[:i]

				// Store the caught exception so handler code can read params.
				c.pendingException = exn

				// Restore the cloned stack (like snapshot.doRestore).
				spp := *(**uint64)(unsafe.Pointer(&h.sp))
				c.stack = h.stack
				c.stackTop = h.top
				ec := &c.execCtx
				ec.stackBottomPtr = &c.stack[0]
				ec.stackPointerBeforeGoCall = spp
				ec.framePointerBeforeGoCall = h.fp
				ec.goCallReturnAddress = h.returnAddress
				ec.savedRegisters = h.savedRegisters

				// Set the matched clause index in execCtx for compiled code to read.
				ec.caughtExceptionClauseIdx = int64(clauseIdx)
				return true
			}
		}
	}
	return false
}

func (c *callEngine) callerModuleInstance() *wasm.ModuleInstance {
	return moduleInstanceFromOpaquePtr(c.execCtx.callerModuleContextPtr)
}

// performGCAccess dispatches the wasm-gc heap-access ops. See
// wazevoapi.GCAccessMode for per-mode arg layout.
func performGCAccess(c *callEngine, mod *wasm.ModuleInstance,
	mode wazevoapi.GCAccessMode, typeIdx wasm.Index, auxIdx uint32,
	arg1, arg2, arg3, arg4, arg5 uint64,
) uint64 {
	switch mode {
	case wazevoapi.GCAccessStructGet:
		fieldIdx := int(auxIdx)
		signedness := wasm.FieldReadKind(arg1)
		refRaw := arg2
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		ws := wasmStructFromUintptr(refRaw)
		fieldSchema := mod.Source.TypeSection[typeIdx].Fields[fieldIdx]
		return wasm.DecodeFieldValueRead(fieldSchema, ws.Get(fieldIdx), signedness)
	case wazevoapi.GCAccessStructSet:
		fieldIdx := int(auxIdx)
		refRaw := arg2
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		ws := wasmStructFromUintptr(refRaw)
		fieldSchema := mod.Source.TypeSection[typeIdx].Fields[fieldIdx]
		if err := ws.Set(fieldIdx, wasm.EncodeFieldValue(fieldSchema, arg3)); err != nil {
			panic(err)
		}
		return 0
	case wazevoapi.GCAccessArrayGet:
		signedness := wasm.FieldReadKind(arg1)
		refRaw := arg2
		idx := uint32(arg3)
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		wa := wasmArrayFromUintptr(refRaw)
		if idx >= wa.Len() {
			panic(wasmruntime.ErrRuntimeOutOfBoundsArrayAccess)
		}
		schema := mod.Source.TypeSection[typeIdx].ArrayField
		return wasm.DecodeFieldValueRead(schema, wa.Get(idx), signedness)
	case wazevoapi.GCAccessArraySet:
		refRaw := arg2
		idx := uint32(arg3)
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		wa := wasmArrayFromUintptr(refRaw)
		if idx >= wa.Len() {
			panic(wasmruntime.ErrRuntimeOutOfBoundsArrayAccess)
		}
		schema := mod.Source.TypeSection[typeIdx].ArrayField
		if err := wa.Set(idx, wasm.EncodeFieldValue(schema, arg4)); err != nil {
			panic(err)
		}
		return 0
	case wazevoapi.GCAccessArrayLen:
		refRaw := arg2
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		return uint64(wasmArrayFromUintptr(refRaw).Len())
	case wazevoapi.GCAccessArrayFill:
		refRaw := arg2
		offset := uint32(arg3)
		valueRaw := arg4
		count := uint32(arg5)
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		wa := wasmArrayFromUintptr(refRaw)
		if uint64(offset)+uint64(count) > uint64(wa.Len()) {
			panic(wasmruntime.ErrRuntimeOutOfBoundsArrayAccess)
		}
		schema := mod.Source.TypeSection[typeIdx].ArrayField
		v := wasm.EncodeFieldValue(schema, valueRaw)
		for i := uint32(0); i < count; i++ {
			if err := wa.Set(offset+i, v); err != nil {
				panic(err)
			}
		}
		return 0
	case wazevoapi.GCAccessArrayCopy:
		dstRefRaw := arg1
		dstOff := uint32(arg2)
		srcRefRaw := arg3
		srcOff := uint32(arg4)
		count := uint32(arg5)
		if dstRefRaw == 0 || srcRefRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		dst := wasmArrayFromUintptr(dstRefRaw)
		src := wasmArrayFromUintptr(srcRefRaw)
		if uint64(dstOff)+uint64(count) > uint64(dst.Len()) {
			panic(wasmruntime.ErrRuntimeOutOfBoundsArrayAccess)
		}
		if uint64(srcOff)+uint64(count) > uint64(src.Len()) {
			panic(wasmruntime.ErrRuntimeOutOfBoundsArrayAccess)
		}
		// Overlap-safe iteration.
		if dst == src && dstOff > srcOff {
			for i := int64(count) - 1; i >= 0; i-- {
				if err := dst.Set(dstOff+uint32(i), src.Get(srcOff+uint32(i))); err != nil {
					panic(err)
				}
			}
		} else {
			for i := uint32(0); i < count; i++ {
				if err := dst.Set(dstOff+i, src.Get(srcOff+i)); err != nil {
					panic(err)
				}
			}
		}
		return 0
	case wazevoapi.GCAccessArrayInitData:
		dataIdx := auxIdx
		refRaw := arg2
		offset := uint32(arg3)
		srcOff := uint32(arg4)
		count := uint32(arg5)
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		wa := wasmArrayFromUintptr(refRaw)
		if uint64(offset)+uint64(count) > uint64(wa.Len()) {
			panic(wasmruntime.ErrRuntimeOutOfBoundsArrayAccess)
		}
		schema := mod.Source.TypeSection[typeIdx].ArrayField
		elemSize, ok := wasm.ArrayDataElemSize(schema)
		if !ok {
			panic(fmt.Errorf("array.init_data on unsupported element type"))
		}
		data := mod.DataInstances[dataIdx]
		if uint64(srcOff)+uint64(count)*uint64(elemSize) > uint64(len(data)) {
			panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
		}
		for i := uint32(0); i < count; i++ {
			off := srcOff + i*elemSize
			if err := wa.Set(offset+i, wasm.ReadDataElement(schema, data, off)); err != nil {
				panic(err)
			}
		}
		return 0
	case wazevoapi.GCAccessArrayInitElem:
		elemIdx := auxIdx
		refRaw := arg2
		offset := uint32(arg3)
		srcOff := uint32(arg4)
		count := uint32(arg5)
		if refRaw == 0 {
			panic(wasmruntime.ErrRuntimeNullReference)
		}
		wa := wasmArrayFromUintptr(refRaw)
		if uint64(offset)+uint64(count) > uint64(wa.Len()) {
			panic(wasmruntime.ErrRuntimeOutOfBoundsArrayAccess)
		}
		elem := mod.ElementInstances[elemIdx]
		if uint64(srcOff)+uint64(count) > uint64(len(elem)) {
			panic(wasmruntime.ErrRuntimeInvalidTableAccess)
		}
		for i := uint32(0); i < count; i++ {
			if err := wa.Set(offset+i, uintptr(elem[srcOff+i])); err != nil {
				panic(err)
			}
		}
		return 0
	}
	panic(fmt.Errorf("wasm-gc: unknown GCAccessMode %d", mode))
}

// wasmStructFromUintptr recovers a *wasm.WasmStruct from a uintptr
// using the safe double-pointer reinterpretation pattern (same as
// functionFromUintptr — avoids checkptr arithmetic violations).
func wasmStructFromUintptr(p uint64) *wasm.WasmStruct {
	var ptr uintptr = uintptr(p)
	return *(**wasm.WasmStruct)(unsafe.Pointer(&ptr))
}

// wasmArrayFromUintptr recovers a *wasm.WasmArray from a uintptr.
func wasmArrayFromUintptr(p uint64) *wasm.WasmArray {
	var ptr uintptr = uintptr(p)
	return *(**wasm.WasmArray)(unsafe.Pointer(&ptr))
}

// allocateWasmArray builds a *wasm.WasmArray according to the
// allocation mode and args passed by the ExitCodeAllocateArray
// trampoline. See the exit-code comment for mode semantics.
func allocateWasmArray(c *callEngine, mod *wasm.ModuleInstance, schema *wasm.FunctionType, elemField wasm.FieldType,
	typeIdx wasm.Index, mode uint32, arg1, arg2, arg3 uint64,
) *wasm.WasmArray {
	_ = schema // currently unused; kept for future expansion (e.g. validation hooks)
	switch mode {
	case 0: // array.new(init, length)
		initRaw := arg1
		length := uint32(arg2)
		init := wasm.EncodeFieldValue(elemField, initRaw)
		elems := make([]any, length)
		for i := uint32(0); i < length; i++ {
			elems[i] = init
		}
		return wasm.NewWasmArrayWith(mod.TypeIDs[typeIdx], elems)
	case 1: // array.new_default(length)
		length := uint32(arg1)
		def := wasm.DefaultFieldValue(elemField)
		elems := make([]any, length)
		for i := uint32(0); i < length; i++ {
			elems[i] = def
		}
		return wasm.NewWasmArrayWith(mod.TypeIDs[typeIdx], elems)
	case 2: // array.new_fixed(length) — values in gcScratchBuffer[0..length]
		length := uint32(arg1)
		if int(length) > len(c.execCtx.gcScratchBuffer) {
			panic(fmt.Errorf("wasm-gc array.new_fixed length %d exceeds scratch buffer size %d", length, len(c.execCtx.gcScratchBuffer)))
		}
		elems := make([]any, length)
		for i := uint32(0); i < length; i++ {
			elems[i] = wasm.EncodeFieldValue(elemField, c.execCtx.gcScratchBuffer[i])
		}
		return wasm.NewWasmArrayWith(mod.TypeIDs[typeIdx], elems)
	case 3: // array.new_data(dataIdx, srcOffset, length)
		dataIdx := uint32(arg1)
		srcOff := uint32(arg2)
		length := uint32(arg3)
		data := mod.DataInstances[dataIdx]
		elemSize, ok := wasm.ArrayDataElemSize(elemField)
		if !ok {
			panic(fmt.Errorf("array.new_data on unsupported element type"))
		}
		total := uint64(length) * uint64(elemSize)
		if uint64(srcOff)+total > uint64(len(data)) {
			panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
		}
		elems := make([]any, length)
		for i := uint32(0); i < length; i++ {
			off := srcOff + i*elemSize
			elems[i] = wasm.ReadDataElement(elemField, data, off)
		}
		return wasm.NewWasmArrayWith(mod.TypeIDs[typeIdx], elems)
	case 4: // array.new_elem(elemIdx, srcOffset, length)
		elemIdx := uint32(arg1)
		srcOff := uint32(arg2)
		length := uint32(arg3)
		elem := mod.ElementInstances[elemIdx]
		if uint64(srcOff)+uint64(length) > uint64(len(elem)) {
			panic(wasmruntime.ErrRuntimeInvalidTableAccess)
		}
		elems := make([]any, length)
		for i := uint32(0); i < length; i++ {
			elems[i] = uintptr(elem[srcOff+i])
		}
		return wasm.NewWasmArrayWith(mod.TypeIDs[typeIdx], elems)
	}
	panic(fmt.Errorf("wasm-gc: unknown array allocation mode %d", mode))
}

const callStackCeiling = uintptr(50000000) // in uint64 (8 bytes) == 400000000 bytes in total == 400mb.

func (c *callEngine) growStackWithGuarded() (newSP uintptr, newFP uintptr, err error) {
	if wazevoapi.StackGuardCheckEnabled {
		wazevoapi.CheckStackGuardPage(c.stack)
	}
	newSP, newFP, err = c.growStack()
	if err != nil {
		return
	}
	if wazevoapi.StackGuardCheckEnabled {
		c.execCtx.stackBottomPtr = &c.stack[wazevoapi.StackGuardCheckGuardPageSize]
	}
	return
}

// growStack grows the stack, and returns the new stack pointer.
func (c *callEngine) growStack() (newSP, newFP uintptr, err error) {
	currentLen := uintptr(len(c.stack))
	if callStackCeiling < currentLen {
		err = wasmruntime.ErrRuntimeStackOverflow
		return
	}

	newLen := 2*currentLen + c.execCtx.stackGrowRequiredSize + 16 // Stack might be aligned to 16 bytes, so add 16 bytes just in case.
	newSP, newFP, c.stackTop, c.stack = c.cloneStack(newLen)
	c.execCtx.stackBottomPtr = &c.stack[0]
	return
}

func (c *callEngine) cloneStack(l uintptr) (newSP, newFP, newTop uintptr, newStack []byte) {
	newStack = make([]byte, l)

	relSp := c.stackTop - uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
	relFp := c.stackTop - c.execCtx.framePointerBeforeGoCall

	// Copy the existing contents in the previous Go-allocated stack into the new one.
	var prevStackAligned, newStackAligned []byte
	{
		//nolint:staticcheck
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&prevStackAligned))
		sh.Data = c.stackTop - relSp
		sh.Len = int(relSp)
		sh.Cap = int(relSp)
	}
	newTop = alignedStackTop(newStack)
	{
		newSP = newTop - relSp
		newFP = newTop - relFp
		//nolint:staticcheck
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&newStackAligned))
		sh.Data = newSP
		sh.Len = int(relSp)
		sh.Cap = int(relSp)
	}
	copy(newStackAligned, prevStackAligned)
	return
}

func (c *callEngine) stackIterator(onHostCall bool) experimental.StackIterator {
	c.stackIteratorImpl.reset(c, onHostCall)
	return &c.stackIteratorImpl
}

// stackIterator implements experimental.StackIterator.
type stackIterator struct {
	retAddrs      []uintptr
	retAddrCursor int
	eng           *engine
	pc            uint64

	currentDef *wasm.FunctionDefinition
}

func (si *stackIterator) reset(c *callEngine, onHostCall bool) {
	if onHostCall {
		si.retAddrs = append(si.retAddrs[:0], uintptr(unsafe.Pointer(c.execCtx.goCallReturnAddress)))
	} else {
		si.retAddrs = si.retAddrs[:0]
	}
	si.retAddrs = unwindStack(uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall, c.stackTop, si.retAddrs)
	si.retAddrs = si.retAddrs[:len(si.retAddrs)-1] // the last return addr is the trampoline, so we skip it.
	si.retAddrCursor = 0
	si.eng = c.parent.parent.parent
}

// Next implements the same method as documented on experimental.StackIterator.
func (si *stackIterator) Next() bool {
	if si.retAddrCursor >= len(si.retAddrs) {
		return false
	}

	addr := si.retAddrs[si.retAddrCursor]
	cm := si.eng.compiledModuleOfAddr(addr)
	if cm != nil {
		index := cm.functionIndexOf(addr)
		def := cm.module.FunctionDefinition(cm.module.ImportFunctionCount + index)
		si.currentDef = def
		si.retAddrCursor++
		si.pc = uint64(addr)
		return true
	}
	return false
}

// ProgramCounter implements the same method as documented on experimental.StackIterator.
func (si *stackIterator) ProgramCounter() experimental.ProgramCounter {
	return experimental.ProgramCounter(si.pc)
}

// Function implements the same method as documented on experimental.StackIterator.
func (si *stackIterator) Function() experimental.InternalFunction {
	return si
}

// Definition implements the same method as documented on experimental.InternalFunction.
func (si *stackIterator) Definition() api.FunctionDefinition {
	return si.currentDef
}

// SourceOffsetForPC implements the same method as documented on experimental.InternalFunction.
func (si *stackIterator) SourceOffsetForPC(pc experimental.ProgramCounter) uint64 {
	upc := uintptr(pc)
	cm := si.eng.compiledModuleOfAddr(upc)
	return cm.getSourceOffset(upc)
}

// snapshot implements experimental.Snapshot
type snapshot struct {
	sp, fp, top    uintptr
	returnAddress  *byte
	stack          []byte
	savedRegisters [64][2]uint64
	ret            []uint64
	c              *callEngine
}

// Snapshot implements the same method as documented on experimental.Snapshotter.
func (c *callEngine) Snapshot() experimental.Snapshot {
	returnAddress := c.execCtx.goCallReturnAddress
	oldTop, oldSp := c.stackTop, uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
	newSP, newFP, newTop, newStack := c.cloneStack(uintptr(len(c.stack)) + 16)
	adjustClonedStack(oldSp, oldTop, newSP, newFP, newTop)
	return &snapshot{
		sp:             newSP,
		fp:             newFP,
		top:            newTop,
		savedRegisters: c.execCtx.savedRegisters,
		returnAddress:  returnAddress,
		stack:          newStack,
		c:              c,
	}
}

// Restore implements the same method as documented on experimental.Snapshot.
func (s *snapshot) Restore(ret []uint64) {
	s.ret = ret
	panic(s)
}

func (s *snapshot) doRestore() {
	spp := *(**uint64)(unsafe.Pointer(&s.sp))
	view := goCallStackView(spp)
	copy(view, s.ret)

	c := s.c
	c.stack = s.stack
	c.stackTop = s.top
	ec := &c.execCtx
	ec.stackBottomPtr = &c.stack[0]
	ec.stackPointerBeforeGoCall = spp
	ec.framePointerBeforeGoCall = s.fp
	ec.goCallReturnAddress = s.returnAddress
	ec.savedRegisters = s.savedRegisters
}

// Error implements the same method on error.
func (s *snapshot) Error() string {
	return "unhandled snapshot restore, this generally indicates restore was called from a different " +
		"exported function invocation than snapshot"
}

func snapshotRecoverFn(c *callEngine) {
	if r := recover(); r != nil {
		if s, ok := r.(*snapshot); ok && s.c == c {
			s.doRestore()
		} else {
			panic(r)
		}
	}
}

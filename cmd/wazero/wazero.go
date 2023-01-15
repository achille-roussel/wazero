package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	gojs "github.com/tetratelabs/wazero/imports/go"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/syscallfs"
	"github.com/tetratelabs/wazero/internal/version"
	"github.com/tetratelabs/wazero/sys"
)

func main() {
	doMain(os.Stdout, os.Stderr, os.Exit)
}

// doMain is separated out for the purpose of unit testing.
func doMain(stdOut io.Writer, stdErr logging.Writer, exit func(code int)) {
	flag.CommandLine.SetOutput(stdErr)

	var help bool
	flag.BoolVar(&help, "h", false, "print usage")

	flag.Parse()

	if help || flag.NArg() == 0 {
		printUsage(stdErr)
		exit(0)
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printUsage(stdErr)
		exit(1)
	}

	subCmd := flag.Arg(0)
	switch subCmd {
	case "compile":
		doCompile(flag.Args()[1:], stdErr, exit)
	case "run":
		doRun(flag.Args()[1:], stdOut, stdErr, exit)
	case "version":
		fmt.Fprintln(stdOut, version.GetWazeroVersion())
		exit(0)
	default:
		fmt.Fprintln(stdErr, "invalid command")
		printUsage(stdErr)
		exit(1)
	}
}

func doCompile(args []string, stdErr io.Writer, exit func(code int)) {
	flags := flag.NewFlagSet("compile", flag.ExitOnError)
	flags.SetOutput(stdErr)

	var help bool
	flags.BoolVar(&help, "h", false, "print usage")

	cacheDir := cacheDirFlag(flags)

	_ = flags.Parse(args)

	if help {
		printCompileUsage(stdErr, flags)
		exit(0)
	}

	if flags.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printCompileUsage(stdErr, flags)
		exit(1)
	}
	wasmPath := flags.Arg(0)

	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		fmt.Fprintf(stdErr, "error reading wasm binary: %v\n", err)
		exit(1)
	}

	c := wazero.NewRuntimeConfig()
	if cache := maybeUseCacheDir(cacheDir, stdErr, exit); cache != nil {
		c.WithCompilationCache(cache)
	}

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	if _, err = rt.CompileModule(ctx, wasm); err != nil {
		fmt.Fprintf(stdErr, "error compiling wasm binary: %v\n", err)
		exit(1)
	} else {
		exit(0)
	}
}

func doRun(args []string, stdOut io.Writer, stdErr logging.Writer, exit func(code int)) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	flags.SetOutput(stdErr)

	var help bool
	flags.BoolVar(&help, "h", false, "print usage")

	var envs sliceFlag
	flags.Var(&envs, "env", "key=value pair of environment variable to expose to the binary. "+
		"Can be specified multiple times.")

	var mounts sliceFlag
	flags.Var(&mounts, "mount",
		"filesystem path to expose to the binary in the form of <host path>[:<wasm path>][:ro]. "+
			"This may be specified multiple times. When <wasm path> is unset, the host path is used. "+
			"For read-only mounts, append the suffix ':ro'.")

	hostLogging := hostLoggingFlag(flags)

	cacheDir := cacheDirFlag(flags)

	_ = flags.Parse(args)

	if help {
		printRunUsage(stdErr, flags)
		exit(0)
	}

	if flags.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printRunUsage(stdErr, flags)
		exit(1)
	}
	wasmPath := flags.Arg(0)

	wasmArgs := flags.Args()[1:]
	if len(wasmArgs) > 1 {
		// Skip "--" if provided
		if wasmArgs[0] == "--" {
			wasmArgs = wasmArgs[1:]
		}
	}

	// Don't use map to preserve order
	var env []string
	for _, e := range envs {
		fields := strings.SplitN(e, "=", 2)
		if len(fields) != 2 {
			fmt.Fprintf(stdErr, "invalid environment variable: %s\n", e)
			exit(1)
		}
		env = append(env, fields[0], fields[1])
	}

	//rootFS := validateMounts(mounts, stdErr, exit)
	rootFS, err := mountRootFS(mounts)
	if err != nil {
		fmt.Fprintf(stdErr, "failed to open file system: %s\n", err)
		exit(1)
	}

	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		fmt.Fprintf(stdErr, "error reading wasm binary: %v\n", err)
		exit(1)
	}

	wasmExe := filepath.Base(wasmPath)

	ctx := maybeHostLogging(context.Background(), hostLogging, stdErr, exit)

	rtc := wazero.NewRuntimeConfig()
	if cache := maybeUseCacheDir(cacheDir, stdErr, exit); cache != nil {
		rtc.WithCompilationCache(cache)
	}

	rt := wazero.NewRuntimeWithConfig(ctx, rtc)
	defer rt.Close(ctx)

	// Because we are running a binary directly rather than embedding in an application,
	// we default to wiring up commonly used OS functionality.
	conf := wazero.NewModuleConfig().
		WithStdout(stdOut).
		WithStderr(stdErr).
		WithStdin(os.Stdin).
		WithRandSource(rand.Reader).
		WithSysNanosleep().
		WithSysNanotime().
		WithSysWalltime().
		WithArgs(append([]string{wasmExe}, wasmArgs...)...)
	for i := 0; i < len(env); i += 2 {
		conf = conf.WithEnv(env[i], env[i+1])
	}
	if rootFS != nil {
		conf = conf.WithFS(rootFS)
	}

	code, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		fmt.Fprintf(stdErr, "error compiling wasm binary: %v\n", err)
		exit(1)
	}

	needsWASI, needsGo := detectImports(code.ImportedFunctions())

	if needsWASI {
		wasi_snapshot_preview1.MustInstantiate(ctx, rt)
		_, err = rt.InstantiateModule(ctx, code, conf)
	} else if needsGo {
		gojs.MustInstantiate(ctx, rt)
		err = gojs.Run(ctx, rt, code, conf)
	}

	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok {
			exit(int(exitErr.ExitCode()))
		}
		fmt.Fprintf(stdErr, "error instantiating wasm binary: %v\n", err)
		exit(1)
	}

	// We're done, _start was called as part of instantiating the module.
	exit(0)
}

func mountRootFS(mounts sliceFlag) (experimentalsys.FS, error) {
	var mountPoints []experimentalsys.MountPoint
	var base = experimentalsys.ReadOnlyFS(experimentalsys.ErrFS(experimentalsys.ErrNotExist))

	for _, mount := range mounts {
		if len(mount) == 0 {
			return nil, fmt.Errorf("invalid mount: %q", mount)
		}

		readOnly := strings.HasSuffix(mount, ":ro")
		if readOnly {
			mount = mount[:len(mount)-3]
		}

		// TODO(anuraaga): Support wasm paths with colon in them.
		var host, guest string
		if clnIdx := strings.LastIndexByte(mount, ':'); clnIdx != -1 {
			host, guest = mount[:clnIdx], mount[clnIdx+1:]
			guest = experimentalsys.CleanPath(strings.TrimPrefix(path.Clean(guest), "/"))
		} else {
			host = mount
			guest = "."
		}

		// Provide a better experience if duplicates are found later.
		if abs, err := filepath.Abs(host); err != nil {
			return nil, fmt.Errorf("invalid mount: host path %q invalid: %v\n", host, err)
		} else {
			host = abs
		}

		fsys := experimentalsys.DirFS(host)
		if readOnly {
			fsys = experimentalsys.ReadOnlyFS(fsys)
		}
		if guest == "." {
			base = fsys
		} else {
			mountPoints = append(mountPoints, experimentalsys.MountPoint{
				Path: guest,
				Fsys: fsys,
			})
		}
	}

	return experimentalsys.RootFS(base, mountPoints...), nil
}

func validateMounts(mounts sliceFlag, stdErr logging.Writer, exit func(code int)) syscallfs.FS {
	fs := make([]syscallfs.FS, 0, len(mounts))
	for _, mount := range mounts {
		if len(mount) == 0 {
			fmt.Fprintln(stdErr, "invalid mount: empty string")
			exit(1)
		}

		readOnly := false
		if trimmed := strings.TrimSuffix(mount, ":ro"); trimmed != mount {
			mount = trimmed
			readOnly = true
		}

		// TODO(anuraaga): Support wasm paths with colon in them.
		var host, guest string
		if clnIdx := strings.LastIndexByte(mount, ':'); clnIdx != -1 {
			host, guest = mount[:clnIdx], mount[clnIdx+1:]
		} else {
			host = mount
			guest = host
		}

		// Provide a better experience if duplicates are found later.
		if guest == "" {
			guest = "/"
		}
		if abs, err := filepath.Abs(host); err != nil {
			fmt.Fprintf(stdErr, "invalid mount: host path %q invalid: %v\n", host, err)
			exit(1)
		} else {
			host = abs
		}

		next, err := syscallfs.NewDirFS(guest, host)
		if err != nil {
			fmt.Fprintf(stdErr, "invalid mount: host path %q invalid: %v\n", host, err)
			exit(1)
		} else {
			if readOnly {
				next = syscallfs.NewReadFS(next)
			}
			fs = append(fs, next)
		}
	}
	if fs, err := syscallfs.NewRootFS(fs...); err != nil {
		fmt.Fprintf(stdErr, "invalid mounts: %v\n", err)
		exit(1)
		return nil
	} else {
		return fs
	}
}

func detectImports(imports []api.FunctionDefinition) (needsWASI, needsGo bool) {
	for _, f := range imports {
		moduleName, _, _ := f.Import()
		switch moduleName {
		case wasi_snapshot_preview1.ModuleName:
			needsWASI = true
			return // can't be both WASI and go
		case "go":
			needsGo = true
			return // can't be both WASI and go
		}
	}
	return
}

func hostLoggingFlag(flags *flag.FlagSet) *string {
	return flags.String("hostlogging", "", "Scope of host functions to log to stderr. "+
		"Current values: filesystem")
}

func maybeHostLogging(ctx context.Context, hostLogging *string, stdErr logging.Writer, exit func(code int)) context.Context {
	h := *hostLogging
	switch h {
	case "":
	case "filesystem":
		ctx = context.WithValue(ctx, experimental.FunctionListenerFactoryKey{}, logging.NewFilesystemLoggingListenerFactory(stdErr))
	default:
		fmt.Fprintf(stdErr, "invalid hostLogging value: %v\n", h)
		exit(1)
	}
	return ctx
}

func cacheDirFlag(flags *flag.FlagSet) *string {
	return flags.String("cachedir", "", "Writeable directory for native code compiled from wasm. "+
		"Contents are re-used for the same version of wazero.")
}

func maybeUseCacheDir(cacheDir *string, stdErr io.Writer, exit func(code int)) (cache wazero.CompilationCache) {
	if dir := *cacheDir; dir != "" {
		var err error
		cache, err = wazero.NewCompilationCacheWithDir(dir)
		if err != nil {
			fmt.Fprintf(stdErr, "invalid cachedir: %v\n", err)
			exit(1)
		} else {
			return
		}
	}
	return
}

func printUsage(stdErr io.Writer) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero <command>")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Commands:")
	fmt.Fprintln(stdErr, "  compile\tPre-compiles a WebAssembly binary")
	fmt.Fprintln(stdErr, "  run\t\tRuns a WebAssembly binary")
	fmt.Fprintln(stdErr, "  version\tDisplays the version of wazero CLI")
}

func printCompileUsage(stdErr io.Writer, flags *flag.FlagSet) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero compile <options> <path to wasm file>")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Options:")
	flags.PrintDefaults()
}

func printRunUsage(stdErr io.Writer, flags *flag.FlagSet) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero run <options> <path to wasm file> [--] <wasm args>")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Options:")
	flags.PrintDefaults()
}

type sliceFlag []string

func (f *sliceFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *sliceFlag) Set(s string) error {
	*f = append(*f, s)
	return nil
}

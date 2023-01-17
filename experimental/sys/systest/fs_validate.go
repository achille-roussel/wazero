package systest

import "github.com/tetratelabs/wazero/experimental/sys"

// The following test suites contain tests verifying input validation for
// implementations of the sys.FS interface.

var testValidateOpenFile = fsTestSuite{
	{
		name: "opening an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("/", sys.O_RDONLY|sys.O_DIRECTORY, 0)
			return err
		},
	},
}

var testValidateOpen = fsTestSuite{
	{
		name: "opening an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.Open("/")
			return err
		},
	},
}

var testValidateAccess = fsTestSuite{
	{
		name: "accessing a file with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Access(fsys, "/", sys.O_RDONLY) },
	},
}

var testValidateMknod = fsTestSuite{
	{
		name: "creating a directory with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Mknod(fsys, "/", 0600, sys.Dev(0, 0)) },
	},
}

var testValidateMkdir = fsTestSuite{
	{
		name: "creating a node with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Mkdir(fsys, "/", 0755) },
	},
}

var testValidateRmdir = fsTestSuite{
	{
		name: "removing a directory with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Rmdir(fsys, "/") },
	},
}

var testValidateUnlink = fsTestSuite{
	{
		name: "unlinking a file with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Unlink(fsys, "/") },
	},
}

var testValidateLink = fsTestSuite{
	{
		name: "linking a file with an invalid source name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Link(fsys, "/", "new") },
	},
	{
		name: "linking a file with an invalid target name fails with ErrInvalid",
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error { return sys.Link(fsys, "old", "/") },
	},
}

var testValidateSymlink = fsTestSuite{
	{
		name: "creating a symbolic link with an invalid target name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Symlink(fsys, "old", "/") },
	},
}

var testValidateReadlink = fsTestSuite{
	{
		name: "reading a symbolic link with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Readlink(fsys, "/")
			return err
		},
	},
}

var testValidateRename = fsTestSuite{
	{
		name: "renaming a file with an invalid source name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Rename(fsys, "/", "new") },
	},
	{
		name: "renaming a file with an invalid target name fails with ErrInvalid",
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error { return sys.Rename(fsys, "old", "/") },
	},
}

var testValidateChmod = fsTestSuite{
	{
		name: "changing permissions of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Chmod(fsys, "/", 0644) },
	},
}

var testValidateChtimes = fsTestSuite{
	{
		name: "changing times of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Chtimes(fsys, "/", epoch, epoch) },
	},
}

var testValidateTruncate = fsTestSuite{
	{
		name: "truncating a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Truncate(fsys, "/", 0) },
	},
}

var testValidateStat = fsTestSuite{
	{
		name: "stat of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Stat(fsys, "/")
			return err
		},
	},
}

var testValidateLstat = fsTestSuite{
	{
		name: "lstat of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Lstat(fsys, "/")
			return err
		},
	},
}

package nix_http_cachefs

import (
	"io"
	"io/fs"
	"os"
	"syscall"

	"github.com/samber/lo"
)

type narchivedFile struct {
	handle fs.File
	name   string
}

func (f *narchivedFile) Close() error {
	return f.handle.Close()
}

func (f *narchivedFile) Read(p []byte) (n int, err error) {
	return f.handle.Read(p)
}

func (f *narchivedFile) ReadAt(p []byte, off int64) (n int, err error) {
	// the narchived files support this interface
	readerAt, ok := f.handle.(io.ReaderAt)
	if !ok {
		return 0, syscall.ENOTSUP
	}
	return readerAt.ReadAt(p, off)
}

func (f *narchivedFile) Seek(offset int64, whence int) (int64, error) {
	// the narchived files support this interface
	return f.handle.(io.Seeker).Seek(offset, whence)
}

func (f *narchivedFile) Write(p []byte) (n int, err error) {
	return 0, syscall.EPERM
}

func (f *narchivedFile) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, syscall.EPERM
}

func (f *narchivedFile) Name() string {
	return f.name
}

func (f *narchivedFile) Readdir(count int) ([]os.FileInfo, error) {
	readDirrer, ok := f.handle.(fs.ReadDirFile)
	if !ok {
		return nil, syscall.ENOTDIR
	}
	dentries, err := readDirrer.ReadDir(count)
	if err != nil {
		return nil, err
	}
	return lo.Map(dentries, func(dentry fs.DirEntry, _ int) os.FileInfo {
		finfo, _ := dentry.Info()
		return finfo
	}), nil
}

func (f *narchivedFile) Readdirnames(n int) ([]string, error) {
	readDirrer, ok := f.handle.(fs.ReadDirFile)
	if !ok {
		return nil, syscall.ENOTDIR
	}
	dentries, err := readDirrer.ReadDir(n)
	if err != nil {
		return nil, err
	}
	return lo.Map(dentries, func(dentry fs.DirEntry, _ int) string {
		return dentry.Name()
	}), nil
}

func (f *narchivedFile) Stat() (os.FileInfo, error) {
	return f.handle.Stat()
}

func (f *narchivedFile) Sync() error {
	return nil
}

func (f *narchivedFile) Truncate(size int64) error {
	return syscall.EPERM
}

func (f *narchivedFile) WriteString(s string) (ret int, err error) {
	return 0, syscall.EPERM
}

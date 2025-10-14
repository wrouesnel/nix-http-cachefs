package nix_http_cachefs

import "os"

type nixHttpCacheFile struct {
	fs *nixHttpCacheFs
}

func (f *nixHttpCacheFile) Close() error {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Read(p []byte) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) ReadAt(p []byte, off int64) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Seek(offset int64, whence int) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Write(p []byte) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) WriteAt(p []byte, off int64) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Name() string {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Readdir(count int) ([]os.FileInfo, error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Readdirnames(n int) ([]string, error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Stat() (os.FileInfo, error) {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Sync() error {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) Truncate(size int64) error {
	//TODO implement me
	panic("implement me")
}

func (f *nixHttpCacheFile) WriteString(s string) (ret int, err error) {
	//TODO implement me
	panic("implement me")
}

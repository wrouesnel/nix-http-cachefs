package nix_http_cachefs

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

// cachedFile implements a locally cached file which will be deleted on close.
// These are constructed by opening the file handle and immediately deleting it.
type cachedFile struct {
	cached *os.File
}

// NewCacheFile instantiates a cache file which will be pre-deleted after being opened,
// ensuring it is cleaned up after use (or on crash).
func NewCacheFile(name string) (*cachedFile, error) {
	tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-*", name))
	if err != nil {
		return nil, err
	}
	if err := os.Remove(tempFile.Name()); err != nil {
		return nil, errors.Join(errors.New("could not pre-delete cache file"), err)
	}

	cached := &cachedFile{cached: tempFile}
	runtime.AddCleanup(cached, func(obj *os.File) {
		obj.Close()
	}, tempFile)
	return cached, nil
}

func (c *cachedFile) Read(p []byte) (n int, err error) {
	return c.cached.Read(p)
}

func (c *cachedFile) Write(p []byte) (n int, err error) {
	return c.cached.Write(p)
}

func (c *cachedFile) Seek(offset int64, whence int) (int64, error) {
	return c.cached.Seek(offset, whence)
}

func (c *cachedFile) Close() error {
	return c.cached.Close()
}

func (c *cachedFile) WriteAt(p []byte, off int64) (n int, err error) {
	return c.cached.WriteAt(p, off)
}

func (c *cachedFile) ReadAt(p []byte, off int64) (n int, err error) {
	return c.cached.ReadAt(p, off)
}

package nix_http_cachefs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/mholt/archives"
	"github.com/spf13/afero"
	"github.com/wrouesnel/nix-sigman/pkg/nixtypes"
	"zombiezen.com/go/nix/nar"
)

type nixHttpCacheFs struct {
	cacheUrl *url.URL
	opts     *options
	storeDir string
	// TODO: cached tracks the number of references to an opened NAR file to avoid redownloading it
	// TODO: this would also be a good way to assign inode numbers to use with bazil fuse.
	// cached map[*cachedFile]atomic.Int64
}

// NewNixHttpCacheFs instantiates a new Nix HTTP Binary Cache filesystem using
// the given cache URL and netrc file for authentication (credentials can also
// be supplied in the URL). NixHttpCacheFs filesystems are read-only.
// TODO: actually they could be writeable with a little magic...
func NewNixHttpCacheFs(cacheUrl *url.URL, opt ...Opt) afero.Fs {
	opts := &options{}
	for _, o := range opt {
		o(opts)
	}

	if opts.client == nil {
		opts.client = http.DefaultClient
	}

	return &nixHttpCacheFs{
		cacheUrl: cacheUrl,
		opts:     opts,
	}
}

func (fs *nixHttpCacheFs) debugLog(msg string, values ...string) {
	if fs.opts.debugFn != nil {
		fs.opts.debugFn(strings.Join(append([]string{msg}, values...), " "))
	}
}

func (fs *nixHttpCacheFs) errorLog(msg string, e error) {
	if fs.opts.errorFn != nil {
		fs.opts.errorFn(fmt.Sprintf("%v: %v", msg, e.Error()))
	}
}

func (fs *nixHttpCacheFs) client() *http.Client {
	return fs.opts.client
}

// getStoreDir retreives the store directory from the cache if it is not already known.
func (fs *nixHttpCacheFs) getStoreDir() string {
	if fs.storeDir == "" {
		req, err := fs.newRequest(http.MethodGet, fs.cacheUrl.JoinPath("nix-cache-info").String(), nil)
		if err != nil {
			return fs.storeDir
		}
		resp, err := fs.client().Do(req)
		if err != nil {
			return fs.storeDir
		}
		// TODO: move into nix-sigman as a type
		defer resp.Body.Close()
		bio := bufio.NewScanner(resp.Body)
		for bio.Scan() {
			fieldName, fieldValue, found := strings.Cut(bio.Text(), ":")
			if !found {
				continue
			}
			if fieldName == "StoreDir" {
				fs.storeDir = strings.TrimSpace(fieldValue)
				break
			}
		}
	}
	return fs.storeDir
}

func (fs *nixHttpCacheFs) newRequest(method, uri string, body io.Reader) (*http.Request, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, uri, nil)
	if err != nil {
		return nil, err
	}

	if fs.opts.netrcFile != nil {
		m := fs.opts.netrcFile.Machine(parsed.Hostname())
		if m != nil {
			req.SetBasicAuth(m.Get("login"), m.Get("password"))
		}
	}
	return req, nil
}

// getNarInfo retrieves a narinfo file for the given store path.
func (fs *nixHttpCacheFs) getNarInfo(name string) (*nixtypes.NarInfo, error) {
	fs.debugLog("getNarInfo", name)
	withErr := func(e error) (*nixtypes.NarInfo, error) {
		fs.errorLog("getNarInfo", e)
		return nil, e
	}

	// Remove the store path from the name
	narPathWithoutPrefix, _ := strings.CutPrefix(name, fs.getStoreDir())
	// Remove any nested paths
	splitPath := strings.Split(narPathWithoutPrefix, string(os.PathSeparator))
	if len(splitPath) < 2 {
		return withErr(errors.New("name is not valid"))
	}

	// Cut the first part of the path to get what should be the narHash
	shortPath, _, _ := strings.Cut(splitPath[1], "-")

	ninfoUrl := fs.cacheUrl.JoinPath(fmt.Sprintf("%s.narinfo", shortPath)).String()
	fs.debugLog("HTTP Request", http.MethodGet, ninfoUrl)

	// request the narinfo from the disk
	req, err := fs.newRequest(http.MethodGet, ninfoUrl, nil)
	if err != nil {
		return withErr(err)
	}

	response, err := fs.client().Do(req)
	if err != nil {
		return withErr(err)
	}

	defer response.Body.Close()
	ninfoResponse, err := io.ReadAll(response.Body)
	if err != nil {
		return withErr(err)
	}

	ninfo := &nixtypes.NarInfo{}
	if err := ninfo.UnmarshalText(ninfoResponse); err != nil {
		return withErr(err)
	}

	return ninfo, nil
}

// getNar makes a nar stored on a binary cache available as a seekable binary file
func (fs *nixHttpCacheFs) getNar(ninfo *nixtypes.NarInfo) (*cachedFile, error) {
	fs.debugLog("getNar", ninfo.StorePath)

	withErr := func(e error) (*cachedFile, error) {
		fs.errorLog("getNarInfo", e)
		return nil, e
	}

	narUrl, err := url.Parse(ninfo.URL)
	if err != nil {
		return nil, err
	}
	resolvedUrl := fs.cacheUrl.ResolveReference(narUrl)
	fs.debugLog("HTTP Request", http.MethodGet, resolvedUrl.String())

	req, err := fs.newRequest(http.MethodGet, resolvedUrl.String(), nil)
	if err != nil {
		return withErr(err)
	}

	resp, err := fs.client().Do(req)
	if err != nil {
		return withErr(err)
	}

	defer resp.Body.Close()
	narReader := resp.Body

	// Ensure we decompress the nar into the cache file
	var compressor archives.Decompressor
	switch ninfo.Compression {
	case "xz":
		compressor = new(archives.Xz)
	case "bzip2":
		compressor = new(archives.Bz2)
	case "gzip":
		compressor = new(archives.Gz)
	case "zstd":
		compressor = new(archives.Zstd)
	case "", "none":
		compressor = nil
	}

	cacheFile, err := NewCacheFile(path.Base(ninfo.StorePath))
	if err != nil {
		return withErr(err)
	}

	if compressor != nil {
		narReader, err = compressor.OpenReader(narReader)
		if err != nil {
			return withErr(err)
		}
		defer narReader.Close()
	}

	// Copy the nar to the cache file
	if _, err := io.Copy(cacheFile, narReader); err != nil {
		return withErr(err)
	}

	// Reset the cache file to the start
	if _, err := cacheFile.Seek(0, io.SeekStart); err != nil {
		return withErr(err)
	}

	return cacheFile, nil
}

func (fs *nixHttpCacheFs) Create(name string) (afero.File, error) {
	return nil, syscall.EPERM
}

func (fs *nixHttpCacheFs) Mkdir(name string, perm os.FileMode) error {
	return syscall.EPERM
}

func (fs *nixHttpCacheFs) MkdirAll(path string, perm os.FileMode) error {
	return syscall.EPERM
}

func (fs *nixHttpCacheFs) Open(name string) (afero.File, error) {
	return fs.OpenFile(name, os.O_RDONLY, os.FileMode(777))
}

func (fs *nixHttpCacheFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	withErr := func(e error) (afero.File, error) {
		fs.errorLog("OpenFile", e)
		return nil, e
	}
	// Open the narInfo file
	ninfo, err := fs.getNarInfo(name)
	if err != nil {
		return withErr(err)
	}
	// Open the narchive
	narchive, err := fs.getNar(ninfo)
	if err != nil {
		return withErr(err)
	}

	// Get a listing
	listing, err := nar.List(narchive)
	if err != nil {
		return withErr(err)
	}

	// Resolve the name within the archive, if any.
	nameWithinArchive, _ := strings.CutPrefix(name, ninfo.StorePath)
	nameWithinArchive, _ = strings.CutPrefix(nameWithinArchive, "/")
	if nameWithinArchive == "" {
		nameWithinArchive = "." // this is a quirk of the filename handling
	}

	// The upstream library implements an FS for us...but returns a private
	// file type and an iofs public type, which means we don't have enough
	// methods to support afero like we'd like to.
	narfs, err := nar.NewFS(narchive, listing)
	if err != nil {
		return withErr(err)
	}
	fh, err := narfs.Open(nameWithinArchive)
	if err != nil {
		return nil, err
	}
	return &narchivedFile{handle: fh}, nil
}

func (fs *nixHttpCacheFs) Remove(name string) error {
	return syscall.EPERM
}

func (fs *nixHttpCacheFs) RemoveAll(path string) error {
	return syscall.EPERM
}

func (fs *nixHttpCacheFs) Rename(oldname, newname string) error {
	return syscall.EPERM
}

func (fs *nixHttpCacheFs) Stat(name string) (os.FileInfo, error) {
	// Stat is reasonably complicated to do because we have to unpack the actual
	// nar file to know what we're stating. So let Open handle it.
	fh, err := fs.Open(name)
	if err != nil {
		return nil, err
	}
	return fh.Stat()
}

func (fs *nixHttpCacheFs) Name() string {
	return "nix-http-cache-fs"
}

func (fs *nixHttpCacheFs) Chmod(name string, mode os.FileMode) error {
	return syscall.EPERM
}

func (fs *nixHttpCacheFs) Chown(name string, uid, gid int) error {
	return syscall.EPERM
}

func (fs *nixHttpCacheFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return syscall.EPERM
}

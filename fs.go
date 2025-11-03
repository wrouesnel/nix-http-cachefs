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
	"go.uber.org/multierr"
	"zombiezen.com/go/nix/nar"
)

type nixHttpCacheFs struct {
	cacheUrls []*url.URL
	opts      *options
	storeDir  string
	// TODO: cached tracks the number of references to an opened NAR file to avoid redownloading it
	// TODO: this would also be a good way to assign inode numbers to use with bazil fuse.
	// cached map[*cachedFile]atomic.Int64
}

// ninfoWithOrigin retains the originating cache of a ninfo file.
type ninfoWithOrigin struct {
	cacheUrl *url.URL
	ninfo    *nixtypes.NarInfo
}

// NewNixHttpCacheFs instantiates a new Nix HTTP Binary Cache filesystem using
// the given cache URL and netrc file for authentication (credentials can also
// be supplied in the URL). NixHttpCacheFs filesystems are read-only.
// TODO: actually they could be writeable with a little magic...
func NewNixHttpCacheFs(cacheUrls []*url.URL, opt ...Opt) (afero.Fs, error) {
	opts := &options{}
	for _, o := range opt {
		o(opts)
	}

	if opts.client == nil {
		opts.client = http.DefaultClient
	}

	if len(cacheUrls) == 0 {
		return nil, errors.New("must specify at least 1 cache URL")
	}

	for idx, cacheUrl := range cacheUrls {
		if cacheUrl == nil {
			return nil, fmt.Errorf("cache url at position %v is nil - this is invalid", idx)
		}
	}

	return &nixHttpCacheFs{
		cacheUrls: cacheUrls,
		opts:      opts,
	}, nil
}

func (fs *nixHttpCacheFs) debugLog(msg string, values ...string) {
	if fs.opts.debugFn != nil {
		fs.opts.debugFn(strings.Join(append([]string{msg}, values...), " "))
	}
}

func (fs *nixHttpCacheFs) errorLog(msg string, e error) {
	if fs.opts.errorFn != nil {
		if e != nil {
			fs.opts.errorFn(fmt.Sprintf("%v: %v", msg, e.Error()))
		} else {
			fs.opts.errorFn(fmt.Sprintf("%v", msg))
		}

	}
}

func (fs *nixHttpCacheFs) client() *http.Client {
	return fs.opts.client
}

// getStoreDir retrieves the store directory from the cache if it is not already known.
// This function will only use the *first* configured cacheUrl - it's a mistake to configure
// multiple conflicting ones.
func (fs *nixHttpCacheFs) getStoreDir() string {
	if fs.storeDir == "" {
		req, err := fs.newRequest(http.MethodGet, fs.cacheUrls[0].JoinPath("nix-cache-info").String(), nil)
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
func (fs *nixHttpCacheFs) getNarInfo(name string) (*ninfoWithOrigin, error) {
	fs.debugLog("getNarInfo", name)
	withErr := func(e error) (*ninfoWithOrigin, error) {
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

	// Try every configured cache before failing
	var result *ninfoWithOrigin
	var errs error
	for _, cacheUrl := range fs.cacheUrls {
		ninfoUrl := cacheUrl.JoinPath(fmt.Sprintf("%s.narinfo", shortPath)).String()
		fs.debugLog("HTTP Request", http.MethodGet, ninfoUrl)

		// request the narinfo from the disk
		req, err := fs.newRequest(http.MethodGet, ninfoUrl, nil)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}

		response, err := fs.client().Do(req)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}

		defer response.Body.Close()
		ninfoResponse, err := io.ReadAll(response.Body)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}

		ninfo := new(nixtypes.NarInfo)
		if err := ninfo.UnmarshalText(ninfoResponse); err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		result = &ninfoWithOrigin{
			cacheUrl: cacheUrl,
			ninfo:    ninfo,
		}
	}

	if result == nil {
		// If we failed then return the complete multi-err for all our attempts
		return withErr(multierr.Append(errs, errors.New("no cache URL succeeded")))
	}

	return result, nil
}

// getNar makes a nar stored on a binary cache available as a seekable binary file.
func (fs *nixHttpCacheFs) getNar(ninfo *ninfoWithOrigin) (*cachedFile, error) {
	fs.debugLog("getNar", ninfo.ninfo.StorePath, ninfo.cacheUrl.String())

	withErr := func(e error) (*cachedFile, error) {
		fs.errorLog("getNarInfo", e)
		return nil, e
	}

	narUrl, err := url.Parse(ninfo.ninfo.URL)
	if err != nil {
		return nil, err
	}

	var cacheFile *cachedFile
	var errs error
	for _, cacheUrl := range append([]*url.URL{ninfo.cacheUrl}, fs.cacheUrls...) {
		resolvedUrl := cacheUrl.ResolveReference(narUrl)
		fs.debugLog("HTTP Request", http.MethodGet, resolvedUrl.String())

		req, err := fs.newRequest(http.MethodGet, resolvedUrl.String(), nil)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}

		resp, err := fs.client().Do(req)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}

		defer resp.Body.Close()
		narReader := resp.Body

		// Ensure we decompress the nar into the cache file
		var compressor archives.Decompressor
		switch ninfo.ninfo.Compression {
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

		cacheFile, err = NewCacheFile(path.Base(ninfo.ninfo.StorePath))
		if err != nil {
			// If this fails its an entirely local error we won't recover from.
			return withErr(err)
		}

		if compressor != nil {
			narReader, err = compressor.OpenReader(narReader)
			if err != nil {
				errs = multierr.Append(errs, err)
				continue
			}
			defer narReader.Close()
		}

		// Copy the nar to the cache file
		if _, err := io.Copy(cacheFile, narReader); err != nil {
			// This can be a product of a failed cache server, so we can retry.
			errs = multierr.Append(errs, err)
			cacheFile = nil
			continue
		}

		// Reset the cache file to the start
		if _, err := cacheFile.Seek(0, io.SeekStart); err != nil {
			// If this fails its an entirely local error we won't recover from.
			return withErr(err)
		}
	}

	if cacheFile == nil {
		return withErr(multierr.Append(errs, errors.New("no cache URL succeeded")))
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
	nameWithinArchive, _ := strings.CutPrefix(name, ninfo.ninfo.StorePath)
	// Might be a drv file in which case the above removal doesn't actually fully remove it.
	//nameWithinArchive, _ = strings.CutPrefix(nameWithinArchive, ".drv")
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

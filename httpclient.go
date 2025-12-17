package nix_http_cachefs

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/chigopher/pathlib"
)

// CachingRoundTripper wraps an http RoundTripper with a simple file-based
// caching mechanism. It is not a generic HTTP cache - this is specialized
// for working with nix http binary caches.
type CachingRoundTripper struct {
	PersistentCache *pathlib.Path
	http.RoundTripper
}

func (c *CachingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	cacheSuffix := path.Clean(request.URL.Path)

	cachePath := c.PersistentCache.Join(cacheSuffix)

	// TODO: validate the cache
	if fh, err := cachePath.Open(); err == nil {
		// File exists. Return the cache.
		return &http.Response{
			Body: fh,
		}, nil
	}

	resp, err := c.RoundTripper.RoundTrip(request)
	if err != nil {
		return resp, err
	}

	if fh, err := cachePath.OpenFile(os.O_CREATE | os.O_WRONLY); err == nil {
		_, err := io.Copy(fh, resp.Body)
		fh.Close()
		resp.Body.Close()
		if err != nil {
			return nil, errors.Join(errors.New("cache storage error"), err)
		} else {
			// No error. Re-open file and return handle.
			if fh, err := cachePath.Open(); err == nil {
				// File exists. Return the cache.
				return &http.Response{
					Body: fh,
				}, nil
			} else {
				return nil, errors.Join(errors.New("cache access error after storage"), err)
			}
		}
	}

	// Had an error! Return the body as is...
	return resp, err
}

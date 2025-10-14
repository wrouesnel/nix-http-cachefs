package nix_http_cachefs

import (
	"net/url"
	"testing"

	"github.com/samber/lo"
	. "gopkg.in/check.v1"
	"zombiezen.com/go/nix/nar"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type FsSuite struct {
	fs *nixHttpCacheFs
}

var _ = Suite(&FsSuite{})

const wellknownPublicPath = "/nix/store/2kgif7n5hi16qhkrnjnv5swnq9aq3qhj-gcc-14-20241116-libgcc"
const wellknownDrvPath = "/nix/store/ci1f3qvj2i3bgr2wibfxl52cfw0wfks6-gcc-14-20241116.drv"

func (s *FsSuite) SetUpSuite(c *C) {
	cacheUrl := lo.Must(url.Parse("https://cache.nixos.org/"))
	s.fs = NewNixHttpCacheFs(cacheUrl, ErrorLogger(func(msg string) {
		c.Logf("error: %s", msg)
	}), DebugLogger(func(msg string) {
		c.Logf("debug: %s", msg)
	})).(*nixHttpCacheFs)
}

func (s *FsSuite) TestGetStoreDir(c *C) {
	c.Assert(s.fs.getStoreDir(), Equals, "/nix/store")
}

func (s *FsSuite) TestGetNarInfoAndNarFile(c *C) {
	ninfo := s.fs.getNarInfo(wellknownPublicPath)
	c.Assert(ninfo, Not(IsNil))

	narchive, err := s.fs.getNar(ninfo)
	c.Assert(err, IsNil)
	c.Assert(narchive, Not(IsNil))

	// Let's check the nar is actually usable since it should be a locally cached object on
	// disk now.
	listing, err := nar.List(narchive)
	c.Assert(err, IsNil)

	fs, err := nar.NewFS(narchive, listing)
	c.Assert(err, IsNil)

	dentries, err := fs.ReadDir(".")
	c.Assert(err, IsNil)
	for _, dentry := range dentries {
		info, err := dentry.Info()
		c.Assert(err, IsNil)
		c.Logf("%v %v %v", dentry.Type().String(), info.Size(), dentry.Name())
	}
}

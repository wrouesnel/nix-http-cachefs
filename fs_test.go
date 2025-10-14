package nix_http_cachefs

import (
	"encoding/json"
	"io"
	"net/url"
	"testing"

	"github.com/nix-community/go-nix/pkg/derivation"
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

// TestGetNarInfoAndNarFile test we're parsing nar's correctly
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

// TestGetNarInfoAndNarFile test we're parsing drv files correctly.
func (s *FsSuite) TestGetNarInfoAndNarDrvFile(c *C) {
	ninfo := s.fs.getNarInfo(wellknownDrvPath)
	c.Assert(ninfo, Not(IsNil))

	narchive, err := s.fs.getNar(ninfo)
	c.Assert(err, IsNil)
	c.Assert(narchive, Not(IsNil))

	// Let's check the nar is actually usable since it should be a locally cached object on
	// disk now.
	// Let's check the nar is actually usable since it should be a locally cached object on
	// disk now.
	listing, err := nar.List(narchive)
	c.Assert(err, IsNil)
	c.Assert(listing, Not(IsNil))
	// drv's are weird - you can't list them, they just exist at the root of the Nar file
	drvReader := io.NewSectionReader(narchive, listing.Root.Header.ContentOffset, listing.Root.Header.Size)
	c.Assert(err, IsNil)
	drv, err := derivation.ReadDerivation(drvReader)
	c.Assert(err, IsNil)
	drvJson, err := json.MarshalIndent(drv, "", "  ")
	c.Assert(err, IsNil)
	c.Logf("%s\n", string(drvJson))

}

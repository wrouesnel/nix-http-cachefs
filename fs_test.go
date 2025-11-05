package nix_http_cachefs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"path"
	"testing"

	"github.com/nix-community/go-nix/pkg/derivation"
	"github.com/samber/lo"
	"github.com/wrouesnel/nix-sigman/pkg/nixtypes"
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

var knownHashes = map[string]string{
	path.Join(wellknownPublicPath, "lib/libgcc_s.so.1"): "75cd8476ad40708ee23b40866dd6c56f43d11475c8e2e659cb5ce88e588e43c7",
	wellknownDrvPath: "6d2e0fccc627c62797f094eba9831e1d61918a08c2a2929f1c5502553f21c59f",
}

func (s *FsSuite) SetUpSuite(c *C) {
	cacheUrl := lo.Must(url.Parse("https://cache.nixos.org/"))
	fs, err := NewNixHttpCacheFs([]*url.URL{cacheUrl}, ErrorLogger(func(msg string) {
		c.Logf("error: %s", msg)
	}), DebugLogger(func(msg string) {
		c.Logf("debug: %s", msg)
	}))
	c.Assert(err, IsNil)
	s.fs = fs.(*nixHttpCacheFs)
}

func (s *FsSuite) TestGetStoreDir(c *C) {
	c.Assert(s.fs.getStoreDir(), Equals, "/nix/store")
}

// TestGetNarInfoAndNarFile test we're parsing nar's correctly.
func (s *FsSuite) TestGetNarInfoAndNarFile(c *C) {
	ninfo, err := s.fs.getNarInfo(wellknownPublicPath)
	c.Assert(err, IsNil)
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

	dentries, err := fs.ReadDir("lib")
	c.Assert(err, IsNil)
	for _, dentry := range dentries {
		info, err := dentry.Info()
		c.Assert(err, IsNil)
		c.Logf("%v %v %v", dentry.Type().String(), info.Size(), dentry.Name())
	}
}

// TestGetNarInfoAndNarFile test we're parsing drv files correctly.
func (s *FsSuite) TestGetNarInfoAndNarDrvFile(c *C) {
	ninfo, err := s.fs.getNarInfo(wellknownDrvPath)
	c.Assert(err, IsNil)
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

func (s *FsSuite) TestReadFile(c *C) {
	filename := path.Join(wellknownPublicPath, "lib/libgcc_s.so.1")
	f, err := s.fs.Open(filename)
	c.Assert(err, IsNil)

	// Hash the file and check it matches.
	hasher := sha256.New()
	_, err = io.Copy(hasher, f)
	c.Assert(err, IsNil)
	readHash := hex.EncodeToString(hasher.Sum(nil))
	c.Assert(readHash, Equals, knownHashes[filename])
}

func (s *FsSuite) TestReadDerivationFile(c *C) {
	f, err := s.fs.Open(wellknownDrvPath)
	c.Assert(err, IsNil)

	// Hash the file and check it matches.
	hasher := sha256.New()
	_, err = io.Copy(hasher, f)
	c.Assert(err, IsNil)
	readHash := hex.EncodeToString(hasher.Sum(nil))
	c.Assert(readHash, Equals, knownHashes[wellknownDrvPath])
}

func (s *FsSuite) TestReadDirDir(c *C) {
	filename := path.Join(wellknownPublicPath, "lib")
	f, err := s.fs.Open(filename)
	c.Assert(err, IsNil)

	dirNames, err := f.Readdirnames(1000)
	c.Assert(err, IsNil)
	for _, name := range dirNames {
		c.Log(name)
	}

	expectedNames := []string{"libgcc_s.so", "libgcc_s.so.1"}
	c.Assert(lo.ElementsMatch(dirNames, expectedNames), Equals, true, Commentf("%v != %v", dirNames, expectedNames))
}

func (s *FsSuite) TestReadDirFile(c *C) {
	filename := path.Join(wellknownPublicPath, "lib/libgcc_s.so.1")
	f, err := s.fs.Open(filename)
	c.Assert(err, IsNil)
	dirNames, err := f.Readdirnames(1000)
	c.Assert(err, Not(IsNil))
	c.Assert(dirNames, IsNil)
}

func (s *FsSuite) TestReadNarInfo(c *C) {
	filename := wellknownPublicPath + ".narinfo"
	f, err := s.fs.Open(filename)
	c.Assert(err, IsNil)
	// Decode the narinfo file
	ninfoBytes, err := io.ReadAll(f)
	c.Assert(err, IsNil)
	ninfo := new(nixtypes.NarInfo)
	err = ninfo.UnmarshalText(ninfoBytes)
	c.Assert(err, IsNil)
}

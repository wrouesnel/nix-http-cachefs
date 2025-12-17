package nix_http_cachefs

import (
	"net/http"

	"github.com/chigopher/pathlib"
	"github.com/jdxcode/netrc"
)

type options struct {
	netrcFile       *netrc.Netrc
	errorFn         func(msg string)
	debugFn         func(msg string)
	roundTripper    http.RoundTripper
	persistentCache *pathlib.Path
}

type Opt func(opt *options)

func ErrorLogger(fn func(msg string)) Opt {
	return func(opt *options) {
		opt.errorFn = fn
	}
}

func DebugLogger(fn func(msg string)) Opt {
	return func(opt *options) {
		opt.debugFn = fn
	}
}

func NetrcFile(netrcFile string) Opt {
	return func(opt *options) {
		// No error if parsing fails - but the filesystem won't work.
		parsed, _ := netrc.Parse(netrcFile)
		opt.netrcFile = parsed
	}
}

func Netrc(netrcContent string) Opt {
	return func(opt *options) {
		// No error if parsing fails - but the filesystem won't work.
		parsed, _ := netrc.ParseString(netrcContent)
		opt.netrcFile = parsed
	}
}

// RoundTripper allows replacing the HTTP transport
func RoundTripper(roundTripper http.RoundTripper) Opt {
	return func(opt *options) {
		opt.roundTripper = roundTripper
	}
}

// PersistentCache specifies a writeable location where files from cache servers
// will be persistently stored.
func PersistentCache(path *pathlib.Path) Opt {
	return func(opt *options) {
		opt.persistentCache = path
	}
}

package nix_http_cachefs

import (
	"github.com/jdxcode/netrc"
	"net/http"
)

type options struct {
	netrcFile *netrc.Netrc
	errorFn   func(msg string)
	debugFn   func(msg string)
	client    *http.Client
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

func Client(client *http.Client) Opt {
	return func(opt *options) {
		opt.client = client
	}
}

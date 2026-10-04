//go:build windows

package web

import (
	"embed"
	"path"
	"strings"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

//go:embed web
var assets embed.FS

const origin = "https://weej.localhost"

// Styles need 'unsafe-inline' nowhere here: every page's styling lives in
// app.css, loaded as a same-origin stylesheet, not an inline <style> block.
const cspHeader = "Content-Security-Policy: default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'\r\n"

func contentTypeFor(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// onWebResourceRequested serves everything under web/* for the fake
// https://weej.localhost origin; nothing else is ever requested since the page
// has no external URLs and only ever navigates within this origin.
func onWebResourceRequested(chromium *edge.Chromium) func(*edge.ICoreWebView2WebResourceRequest, *edge.ICoreWebView2WebResourceRequestedEventArgs) {
	return func(req *edge.ICoreWebView2WebResourceRequest, args *edge.ICoreWebView2WebResourceRequestedEventArgs) {
		uri, err := req.GetUri()
		if err != nil || !strings.HasPrefix(uri, origin+"/") {
			return
		}
		name := strings.TrimPrefix(uri, origin+"/")
		if i := strings.IndexByte(name, '?'); i >= 0 {
			name = name[:i]
		}

		data, err := assets.ReadFile("web/" + name)
		status, reason := 200, "OK"
		if err != nil {
			data, status, reason = []byte("not found"), 404, "Not Found"
		}

		headers := "Content-Type: " + contentTypeFor(name) + "\r\n" + cspHeader
		resp, err := chromium.Environment().CreateWebResourceResponse(data, status, reason, headers)
		if err != nil {
			return
		}
		defer resp.Release()
		_ = args.PutResponse(resp)
	}
}

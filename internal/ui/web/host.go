//go:build windows

// Package web hosts WeeJ's four WebView2 windows (Settings, Calibration, the
// first-run Language prompt, Update progress) and the plain HTML/CSS/JS pages
// under web/. It does not import internal/ui/winui, which owns the UI-thread
// message loop and tray; Open instead takes that loop's own invoke function, so
// Send/Close/Focus/SetTitle can be called from any goroutine the way winui.Loop's
// own methods are.
package web

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"

	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

// Available reports the installed WebView2 Evergreen runtime's version, mirroring
// webviewloader.GetAvailableCoreWebView2BrowserVersionString("").
func Available() (version string, ok bool) {
	v, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

type Options struct {
	Title     string
	Width     int // DIP
	Height    int // DIP
	MinHeight int // DIP

	NoClose bool // update window: no close box
	Modal   bool // language prompt: runs a nested modal loop until closed

	OnClose func()
	// Runs before a Modal window's loop starts, so its onMessage can reach the Window.
	OnOpen func(*Window)
}

type Window struct {
	hwnd      uintptr
	chromium  *edge.Chromium
	invoke    func(func())
	opts      Options
	onMessage func(msg []byte)

	scale          float64
	frameW, frameH int32
	work           rect32
	widthPx        int32
	minHeightPx    int32

	closed int32 // atomic bool; 0 = open
}

func dipToPx(dip int, scale float64) int32 {
	return int32(math.Round(float64(dip) * scale))
}

// Open creates and shows a WebView2 window for the named page ("settings",
// "calibration", "language" or "update") and must run on the UI thread, the same
// one invoke marshals onto. For a Modal window it does not return until that
// window closes.
func Open(invoke func(func()), page string, opts Options, onMessage func(msg []byte)) (*Window, error) {
	hwnd := createWindowHidden(opts.Title)
	if hwnd == 0 {
		return nil, fmt.Errorf("web: CreateWindowExW failed for %q", page)
	}

	w := &Window{hwnd: hwnd, invoke: invoke, opts: opts, onMessage: onMessage}
	registerLive(hwnd, w)

	w.scale = dpiForWindow(hwnd)
	outer := windowRect(hwnd)
	inner := clientRect(hwnd)
	w.frameW = (outer.Right - outer.Left) - (inner.Right - inner.Left)
	w.frameH = (outer.Bottom - outer.Top) - (inner.Bottom - inner.Top)
	w.work = workArea()
	w.widthPx = dipToPx(opts.Width, w.scale)
	w.minHeightPx = dipToPx(opts.MinHeight, w.scale)
	w.resizeTo(dipToPx(opts.Height, w.scale))

	setDarkTitleBar(hwnd, !sys.AppsLight())
	setMica(hwnd)
	if opts.NoClose {
		disableCloseBox(hwnd)
	}

	if err := w.embed(page); err != nil {
		destroyWindow(hwnd)
		return nil, err
	}

	if opts.OnOpen != nil {
		opts.OnOpen(w)
	}
	showWindow(hwnd, swShow)

	if opts.Modal {
		w.runModalLoop()
	}

	return w, nil
}

func (w *Window) embed(page string) error {
	chromium := edge.NewChromium()
	chromium.DataPath = filepath.Join(os.Getenv("LOCALAPPDATA"), "WeeJ", "WebView2")
	chromium.SetErrorCallback(func(err error) {
		log.Printf("web: webview2 error: %v", err)
	})
	chromium.MessageCallback = w.onWebMessage
	chromium.WebResourceRequestedCallback = onWebResourceRequested(chromium)
	chromium.AcceleratorKeyCallback = func(vk uint) bool { return false }

	// Embed pumps its own nested message loop until the environment and
	// controller are created, then returns; proven in the S1 spike.
	chromium.Embed(w.hwnd)
	w.chromium = chromium

	if settings, err := chromium.GetSettings(); err != nil {
		log.Printf("web: GetSettings: %v", err)
	} else {
		_ = settings.PutAreDevToolsEnabled(false)
		_ = settings.PutAreDefaultContextMenusEnabled(false)
		_ = settings.PutIsZoomControlEnabled(false)
		_ = settings.PutAreBrowserAcceleratorKeysEnabled(false)
		_ = settings.PutIsStatusBarEnabled(false)
	}

	chromium.SetBackgroundColour(0, 0, 0, 0)
	chromium.AddWebResourceRequestedFilter(origin+"/*", edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL)
	chromium.Navigate(origin + "/" + page + ".html")
	chromium.Resize()
	return nil
}

// onWebMessage runs on the UI thread (WebView2's own callback dispatch), so it
// must never block or open another window directly; an onMessage handler that
// needs to open one should hop back through invoke itself, same as any other
// goroutine would.
func (w *Window) onWebMessage(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
	var probe struct {
		Type  string  `json:"type"`
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal([]byte(message), &probe); err != nil {
		log.Printf("web: bad JSON from page: %v", err)
		return
	}
	if probe.Type == "height" {
		w.resizeTo(dipToPx(int(probe.Value), w.scale))
		return
	}
	if w.onMessage != nil {
		w.onMessage([]byte(message))
	}
}

// resizeTo sets the window's client height to heightPx, clamped to [minHeightPx,
// work area height], keeping the window centred in the primary monitor's work
// area. Width never changes after Open; only the page's reported height does.
func (w *Window) resizeTo(heightPx int32) {
	if heightPx < w.minHeightPx {
		heightPx = w.minHeightPx
	}
	if maxH := (w.work.Bottom - w.work.Top) - w.frameH; maxH > 0 && heightPx > maxH {
		heightPx = maxH
	}
	totalW := w.widthPx + w.frameW
	totalH := heightPx + w.frameH
	x := w.work.Left + ((w.work.Right-w.work.Left)-totalW)/2
	y := w.work.Top + ((w.work.Bottom-w.work.Top)-totalH)/2
	setWindowPos(w.hwnd, x, y, totalW, totalH, swpNoZorder|swpNoActivate)
	if w.chromium != nil {
		w.chromium.Resize()
	}
}

func (w *Window) runModalLoop() {
	for atomic.LoadInt32(&w.closed) == 0 {
		if !pumpOne() {
			// A WM_QUIT belongs to the whole application, not this nested loop;
			// repost it so the outer loop (winui.Loop.Run) still sees it.
			postQuitMessage()
			return
		}
	}
}

// handleDestroyed runs synchronously from wndProcDispatch on WM_DESTROY, already
// on the UI thread, so it calls OnClose directly rather than through invoke.
func (w *Window) handleDestroyed() {
	atomic.StoreInt32(&w.closed, 1)
	if w.opts.OnClose != nil {
		w.opts.OnClose()
	}
}

// Send marshals v and hands it to the page's weej.receive(); a no-op once the
// window has closed. Safe from any goroutine.
func (w *Window) Send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("web: Send: %v", err)
		return
	}
	w.invoke(func() {
		if atomic.LoadInt32(&w.closed) != 0 || w.chromium == nil {
			return
		}
		w.chromium.Eval("weej.receive(" + string(data) + ")")
	})
}

// Close destroys the window's HWND, which releases WebView2 (there is no
// exposed Close on the controller) and then runs OnClose. Safe from any
// goroutine.
func (w *Window) Close() {
	w.invoke(func() {
		if atomic.LoadInt32(&w.closed) != 0 {
			return
		}
		destroyWindow(w.hwnd)
	})
}

// Focus restores the window if minimized and brings it to the foreground. Safe
// from any goroutine.
func (w *Window) Focus() {
	w.invoke(func() {
		if atomic.LoadInt32(&w.closed) != 0 {
			return
		}
		showWindow(w.hwnd, swRestore)
		setForegroundWindow(w.hwnd)
	})
}

// SetTitle changes the native title bar text. Safe from any goroutine.
func (w *Window) SetTitle(s string) {
	w.invoke(func() {
		if atomic.LoadInt32(&w.closed) != 0 {
			return
		}
		setWindowText(w.hwnd, s)
	})
}

//go:build windows

package web

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"

	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

var (
	logMu sync.Mutex
	logFn = func(string) {}
)

// SetLogger routes this package's messages into the app log; the GUI build has no stderr.
func SetLogger(f func(string)) {
	logMu.Lock()
	logFn = f
	logMu.Unlock()
}

func logf(format string, args ...any) {
	logMu.Lock()
	f := logFn
	logMu.Unlock()
	f(fmt.Sprintf(format, args...))
}

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

	NoClose bool
	Modal   bool
	// Resizable lets the user size and maximize it; once dragged to a size, it stops following
	// its page's height.
	Resizable bool

	OnClose func()
	// Runs before a Modal window's loop starts, so its onMessage can reach the Window.
	OnOpen func(*Window)

	// Owner makes a popup instead: borderless and owned by Owner. It stays hidden until
	// ShowAt opens it beside a rect in Owner's page the way a submenu opens, and it hides again,
	// rather than closing, as soon as it loses focus, so the next ShowAt is instant.
	// MaxHeight caps it (DIP); a longer page scrolls.
	Owner     *Window
	MaxHeight int
	OnHide    func()
}

// Rect is a rectangle in a page's own CSS pixels, as getBoundingClientRect reports it.
type Rect struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
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
	userSized      bool
	dark           bool
	brush          uintptr
	// neededPx is the client height the page last asked for, clamped to the work area.
	neededPx int32
	// roomPx is the tallest the client area can be on the window's monitor, as last sent.
	roomPx int32

	// Popups only.
	popup        bool
	visible      bool
	anchor       rect32 // screen pixels
	maxHeightPx  int32
	lastHeightPx int32

	// UI thread only.
	ready   bool
	pending []string
	shown   bool

	closed int32
}

// Closed windows' Chromium objects hold the COM callbacks WebView2 may still call into,
// so they stay reachable for the life of the process instead of being collected.
var (
	retiredMu sync.Mutex
	retired   []*edge.Chromium
)

const showFallbackTimer = 1

// The smallest a Resizable window can be dragged to, in DIP.
const minResizeW, minResizeH = 720, 480

func dipToPx(dip int, scale float64) int32 {
	return int32(math.Round(float64(dip) * scale))
}

// Matches app.css --bg, so nothing flashes before the page paints.
func themeRGB(dark bool) (r, g, b uint8) {
	if dark {
		return 0x20, 0x20, 0x20
	}
	return 0xF3, 0xF3, 0xF3
}

// Open must run on the UI thread. The window stays hidden until the page reports its
// height (or a short fallback timer fires), so it appears at its final size with content.
// A Modal window's Open returns only once it has closed.
func Open(invoke func(func()), page string, opts Options, onMessage func(msg []byte)) (*Window, error) {
	popup := opts.Owner != nil
	var hwnd uintptr
	if popup {
		// Created over its owner, so it takes that monitor's DPI from the start.
		origin := clientOrigin(opts.Owner.hwnd)
		hwnd = createPopupHidden(opts.Owner.hwnd, origin.X, origin.Y)
	} else {
		hwnd = createWindowHidden(opts.Title, opts.Resizable)
	}
	if hwnd == 0 {
		return nil, fmt.Errorf("web: CreateWindowExW failed for %q", page)
	}

	w := &Window{hwnd: hwnd, invoke: invoke, opts: opts, onMessage: onMessage, popup: popup}
	registerLive(hwnd, w)

	w.scale = dpiForWindow(hwnd)
	outer := windowRect(hwnd)
	inner := clientRect(hwnd)
	w.frameW = (outer.Right - outer.Left) - (inner.Right - inner.Left)
	w.frameH = (outer.Bottom - outer.Top) - (inner.Bottom - inner.Top)
	if popup {
		w.work = monitorWorkArea(hwnd)
	} else {
		w.work = workArea()
	}
	w.widthPx = dipToPx(opts.Width, w.scale)
	if maxW := (w.work.Right - w.work.Left) - w.frameW; !popup && maxW > 0 && w.widthPx > maxW {
		w.widthPx = maxW
	}
	w.minHeightPx = dipToPx(opts.MinHeight, w.scale)
	w.maxHeightPx = dipToPx(opts.MaxHeight, w.scale)
	w.applyTheme(!sys.AppsLight())
	if popup {
		// Laid out at its final width while hidden; ShowAt places it.
		r := windowRect(hwnd)
		setWindowPos(hwnd, r.Left, r.Top, w.widthPx, dipToPx(opts.Height, w.scale), swpNoZorder|swpNoActivate)
	} else {
		w.resizeTo(dipToPx(opts.Height, w.scale), true)
	}

	if opts.NoClose {
		disableCloseBox(hwnd)
	}

	w.embed(page)

	if opts.OnOpen != nil {
		opts.OnOpen(w)
	}
	if !popup {
		setTimer(hwnd, showFallbackTimer, 1500)
	}

	if opts.Modal {
		w.runModalLoop()
	}

	return w, nil
}

func (w *Window) embed(page string) {
	chromium := edge.NewChromium()
	chromium.DataPath = filepath.Join(os.Getenv("LOCALAPPDATA"), "WeeJ", "WebView2")
	chromium.SetErrorCallback(func(err error) {
		logf("web: webview2 error: %v", err)
	})
	chromium.MessageCallback = w.onWebMessage
	chromium.WebResourceRequestedCallback = onWebResourceRequested(chromium)
	chromium.AcceleratorKeyCallback = func(vk uint) bool { return false }

	// Embed pumps its own nested message loop until the controller exists.
	chromium.Embed(w.hwnd)
	w.chromium = chromium

	devTools := os.Getenv("WEEJ_DEVTOOLS") == "1"
	if settings, err := chromium.GetSettings(); err != nil {
		logf("web: GetSettings: %v", err)
	} else {
		_ = settings.PutAreDevToolsEnabled(devTools)
		_ = settings.PutAreDefaultContextMenusEnabled(devTools)
		_ = settings.PutIsZoomControlEnabled(false)
		_ = settings.PutAreBrowserAcceleratorKeysEnabled(devTools)
		_ = settings.PutIsStatusBarEnabled(false)
	}

	w.setWebViewBackground()
	chromium.AddWebResourceRequestedFilter(origin+"/*", edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL)
	chromium.Navigate(origin + "/" + page + ".html")
	w.resizeWebView()
}

func (w *Window) isClosed() bool { return atomic.LoadInt32(&w.closed) != 0 }

// Goes through the controller directly: Chromium's own wrappers turn any error into os.Exit.
func (w *Window) setWebViewBackground() {
	if w.chromium == nil {
		return
	}
	c := w.chromium.GetController()
	if c == nil {
		return
	}
	c2 := c.GetICoreWebView2Controller2()
	if c2 == nil {
		return
	}
	r, g, b := themeRGB(w.dark)
	if err := c2.PutDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 255, R: r, G: g, B: b}); err != nil {
		logf("web: background: %v", err)
	}
}

func (w *Window) resizeWebView() {
	if w.chromium == nil || w.isClosed() {
		return
	}
	w.chromium.Resize()
}

func (w *Window) focusWebView() {
	if w.chromium == nil || w.isClosed() {
		return
	}
	if c := w.chromium.GetController(); c != nil {
		_ = c.MoveFocus(edge.COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
	}
}

func (w *Window) applyTheme(dark bool) {
	w.dark = dark
	setDarkTitleBar(w.hwnd, dark)
	r, g, b := themeRGB(dark)
	old := w.brush
	w.brush = createSolidBrush(r, g, b)
	if old != 0 {
		deleteObject(old)
	}
	w.setWebViewBackground()
	invalidate(w.hwnd)
}

// SetTheme recolors the title bar and the background behind the page. Safe from any goroutine.
func (w *Window) SetTheme(dark bool) {
	w.invoke(func() {
		if w.isClosed() {
			return
		}
		w.applyTheme(dark)
	})
}

func (w *Window) showOnce() {
	if w.shown || w.isClosed() {
		return
	}
	w.shown = true
	killTimer(w.hwnd, showFallbackTimer)
	showWindow(w.hwnd, swShow)
	// A WebView created inside a hidden window stays blank until told it is visible again.
	if w.chromium != nil {
		if c := w.chromium.GetController(); c != nil {
			_ = c.PutIsVisible(true)
		}
	}
	w.resizeWebView()
	setForegroundWindow(w.hwnd)
	w.focusWebView()
}

func (w *Window) onWebMessage(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
	if w.isClosed() {
		return
	}
	var probe struct {
		Type    string  `json:"type"`
		Value   float64 `json:"value"`
		Message string  `json:"message"`
		Source  string  `json:"source"`
		Line    int     `json:"line"`
	}
	if err := json.Unmarshal([]byte(message), &probe); err != nil {
		logf("web: bad JSON from page: %v", err)
		return
	}
	switch probe.Type {
	case "height":
		w.resizeTo(int32(math.Ceil(probe.Value)), false)
		if !w.popup {
			w.showOnce()
		}
		return
	case "pageError":
		logf("web: page error: %s (%s:%d)", probe.Message, probe.Source, probe.Line)
		return
	case "ready":
		w.ready = true
		for _, script := range w.pending {
			w.chromium.Eval(script)
		}
		w.pending = nil
	}
	if w.onMessage != nil {
		w.onMessage([]byte(message))
	}
}

// resizeTo sets the client height in physical pixels, clamped to the work area. Only the
// first call centres the window; later ones keep it where the user put it.
func (w *Window) resizeTo(heightPx int32, center bool) {
	if w.popup {
		w.lastHeightPx = heightPx
		if w.visible {
			w.placePopup(heightPx)
		}
		return
	}
	work := w.work
	if !center {
		work = monitorWorkArea(w.hwnd)
	}
	room := (work.Bottom - work.Top) - w.frameH
	// The page fits its longest lists into this, rather than scroll as a whole. The first call
	// comes before the page exists, so the first one the page asks for sends it.
	if !center && room > 0 && room != w.roomPx {
		w.roomPx = room
		w.Send(map[string]any{"type": "room", "value": room})
	}
	if heightPx < w.minHeightPx {
		heightPx = w.minHeightPx
	}
	if room > 0 && heightPx > room {
		heightPx = room
	}
	w.neededPx = heightPx
	// Maximized, it keeps its size and a longer page scrolls.
	if zoomed, _, _ := procIsZoomed.Call(w.hwnd); zoomed != 0 {
		w.resizeWebView()
		return
	}
	cur := windowRect(w.hwnd)
	totalW := w.widthPx + w.frameW
	if w.userSized {
		// Sized by hand, it keeps that size unless the page needs more height.
		if in := clientRect(w.hwnd); in.Bottom-in.Top >= heightPx {
			w.resizeWebView()
			return
		}
		totalW = cur.Right - cur.Left
	}
	totalH := heightPx + w.frameH
	if center {
		x := work.Left + ((work.Right-work.Left)-totalW)/2
		y := work.Top + ((work.Bottom-work.Top)-totalH)/2
		setWindowPos(w.hwnd, x, y, totalW, totalH, swpNoZorder|swpNoActivate)
	} else {
		y := cur.Top
		if y+totalH > work.Bottom {
			y = work.Bottom - totalH
		}
		if y < work.Top {
			y = work.Top
		}
		setWindowPos(w.hwnd, cur.Left, y, totalW, totalH, swpNoZorder|swpNoActivate)
	}
	w.resizeWebView()
}

// placePopup opens the popup beside its anchor, the way a submenu opens: to the right, or to
// the left when the right has no room, with its top level with the anchor's and moved up as
// far as the screen needs.
func (w *Window) placePopup(heightPx int32) {
	a, work := w.anchor, w.work
	if w.maxHeightPx > 0 && heightPx > w.maxHeightPx {
		heightPx = w.maxHeightPx
	}
	if maxH := work.Bottom - work.Top; heightPx > maxH {
		heightPx = maxH
	}
	gap := dipToPx(4, w.scale)

	x := a.Right + gap
	if x+w.widthPx > work.Right {
		x = a.Left - gap - w.widthPx
	}
	if x+w.widthPx > work.Right {
		x = work.Right - w.widthPx
	}
	if x < work.Left {
		x = work.Left
	}

	y := a.Top
	if y+heightPx > work.Bottom {
		y = work.Bottom - heightPx
	}
	if y < work.Top {
		y = work.Top
	}
	setWindowPos(w.hwnd, x, y, w.widthPx, heightPx, swpNoZorder|swpNoActivate)
	w.resizeWebView()
}

// ShowAt opens a popup beside anchor, a rect in its owner's page, heightPx tall (physical
// pixels, capped by MaxHeight and the screen), and gives it focus. Safe from any goroutine.
func (w *Window) ShowAt(anchor Rect, heightPx int32) {
	w.invoke(func() {
		if w.isClosed() || !w.popup {
			return
		}
		owner := w.opts.Owner
		w.anchor = owner.screenRect(anchor)
		w.work = monitorWorkArea(owner.hwnd)
		if heightPx > 0 {
			w.lastHeightPx = heightPx
		}
		w.placePopup(w.lastHeightPx)
		w.visible = true
		showWindow(w.hwnd, swShow)
		// A WebView created inside a hidden window stays blank until told it is visible.
		if w.chromium != nil {
			if c := w.chromium.GetController(); c != nil {
				_ = c.PutIsVisible(true)
			}
		}
		setForegroundWindow(w.hwnd)
		w.focusWebView()
	})
}

// Hide puts a popup away until the next ShowAt. Safe from any goroutine.
func (w *Window) Hide() {
	w.invoke(func() {
		if !w.isClosed() {
			w.hidePopup()
		}
	})
}

func (w *Window) hidePopup() {
	if !w.visible {
		return
	}
	w.visible = false
	showWindow(w.hwnd, swHide)
	if w.opts.OnHide != nil {
		w.opts.OnHide()
	}
}

// screenRect converts a rect in this window's page to screen pixels.
func (w *Window) screenRect(r Rect) rect32 {
	origin := clientOrigin(w.hwnd)
	px := func(v float64) int32 { return int32(math.Round(v * w.scale)) }
	return rect32{
		Left:   origin.X + px(r.Left),
		Top:    origin.Y + px(r.Top),
		Right:  origin.X + px(r.Right),
		Bottom: origin.Y + px(r.Bottom),
	}
}

func (w *Window) runModalLoop() {
	for !w.isClosed() {
		if !pumpOne() {
			// WM_QUIT belongs to the whole application: repost it for the outer loop.
			postQuitMessage()
			return
		}
	}
}

func (w *Window) handleDestroyed() {
	atomic.StoreInt32(&w.closed, 1)
	if w.chromium != nil {
		retiredMu.Lock()
		retired = append(retired, w.chromium)
		retiredMu.Unlock()
	}
	if w.brush != 0 {
		deleteObject(w.brush)
		w.brush = 0
	}
	if w.opts.OnClose != nil {
		w.opts.OnClose()
	}
}

// Send hands v to the page's weej.receive(). Until the page has said it is ready,
// weej.receive does not exist yet, so messages wait in order. Safe from any goroutine.
func (w *Window) Send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		logf("web: Send: %v", err)
		return
	}
	script := "weej.receive(" + string(data) + ")"
	w.invoke(func() {
		if w.isClosed() || w.chromium == nil {
			return
		}
		if !w.ready {
			w.pending = append(w.pending, script)
			return
		}
		w.chromium.Eval(script)
	})
}

func (w *Window) Close() {
	w.invoke(func() {
		if w.isClosed() {
			return
		}
		destroyWindow(w.hwnd)
	})
}

func (w *Window) Focus() {
	w.invoke(func() {
		if w.isClosed() {
			return
		}
		showWindow(w.hwnd, swRestore)
		setForegroundWindow(w.hwnd)
		w.focusWebView()
	})
}

func (w *Window) SetTitle(s string) {
	w.invoke(func() {
		if w.isClosed() {
			return
		}
		setWindowText(w.hwnd, s)
	})
}

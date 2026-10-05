//go:build windows

package app

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/audio"
	"github.com/zolferfigueiredo/weej/internal/platform/display"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
	"github.com/zolferfigueiredo/weej/internal/platform/via"
	"github.com/zolferfigueiredo/weej/internal/ui/web"
	"github.com/zolferfigueiredo/weej/internal/ui/winui"
	"github.com/zolferfigueiredo/weej/internal/updater"
)

type NightLight interface{ Set(s float64) }

type Zoom interface {
	Set(s float64)
	Off()
}

var comPortPattern = regexp.MustCompile(`(?i)^COM\d+$`)

type App struct {
	loop *winui.Loop

	self              string
	launchArgs        []string
	forcedPort        string
	updateSiteFlag    string
	testNotifications bool

	log func(string)

	startTime time.Time

	mu          sync.Mutex
	settings    core.Settings
	connected   bool
	busy        bool
	currentPort string
	calibrating bool

	engine *core.Engine
	audio  *audio.Audio
	ddc    *display.DDC
	via    *via.VIA

	nightlight NightLight
	zoom       Zoom

	reconnectCh  chan struct{}
	cancelSerial context.CancelFunc
	serialDone   chan struct{}

	tray *winui.Tray
	hud  *winui.HUD

	faceMu    sync.Mutex
	faceFont  *parsedFont
	faceCache map[int]font.Face

	appCacheMu sync.Mutex
	appName    map[string]string
	appPath    map[string]string
	appIcon    map[string]string

	settingsWin        *web.Window
	settingsPendingTab string

	jobMenuWin      *web.Window
	jobMenuShown    bool
	jobMenuKnob     int
	jobMenuAnchor   web.Rect
	jobMenuHiddenAt time.Time

	calibWin      *web.Window
	calibrator    *core.Calibrator
	calibOnlyNew  bool
	calibTickStop func()

	updateWin   *web.Window
	checking    bool
	installing  bool
	installStep updater.Step
	lastRelease updater.Release

	firstConnectDone bool
	lastTerminalLine time.Time
	shutdownOnce     sync.Once
}

func Main(args []string) int {
	self, err := os.Executable()
	if err != nil {
		self = "WeeJ.exe"
	}

	flags, waitPIDs, exitCode, handled := parseFlags(self, args)
	if handled {
		return exitCode
	}

	for _, pid := range waitPIDs {
		sys.WaitPID(pid, 30*time.Second)
	}
	if len(waitPIDs) > 0 {
		updater.Cleanup(self)
	}

	logPath := flags.logPath
	if logPath == "" {
		logPath = sys.LocalDir() + `\weej.log`
	} else {
		logPath = sys.ExpandEnv(logPath)
	}
	w, logErr := sys.OpenLog(logPath, 1<<20)
	lg := newLogger(w)
	defer lg.Close()
	if logErr != nil {
		lg.Log("Could not open the log file: " + logErr.Error())
	}

	release, ok := sys.Acquire()
	if !ok {
		sys.Notify("OpenSettings")
		if sys.StdoutIsTerminal() {
			fmt.Println("WeeJ: WeeJ is already running, so its Settings opened instead. Quit it first to run this copy.")
		}
		return 0
	}
	defer release()

	app := &App{
		self:              self,
		launchArgs:        args,
		forcedPort:        flags.forcedPort,
		updateSiteFlag:    flags.updateSite,
		testNotifications: flags.testNotifications,
		log:               lg.Log,
		startTime:         time.Now(),
		reconnectCh:       make(chan struct{}, 1),
		appName:           map[string]string{},
		appPath:           map[string]string{},
		appIcon:           map[string]string{},
	}

	return winui.Run(app.setup)
}

type parsedFlags struct {
	forcedPort        string
	logPath           string
	updateSite        string
	testNotifications bool
}

func parseFlags(self string, args []string) (flags parsedFlags, waitPIDs []uint32, exitCode int, handled bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case comPortPattern.MatchString(a):
			flags.forcedPort = strings.ToUpper(a)

		case a == "--quit":
			sys.Notify("Quit")
			sys.WaitForRelease(10 * time.Second)
			return flags, nil, 0, true

		case a == "--login":
			rest := args[i+1:]
			if len(rest) == 0 || (rest[0] != "on" && rest[0] != "off") {
				fmt.Fprintln(os.Stderr, "WeeJ: --login needs on or off")
				return flags, nil, 1, true
			}
			on := rest[0] == "on"
			if err := sys.SetLogin(on, self, rest[1:]); err != nil {
				fmt.Fprintln(os.Stderr, "WeeJ: "+err.Error())
				return flags, nil, 1, true
			}
			return flags, nil, 0, true

		case a == "--detach":
			if err := sys.Detach(self, args[i+1:]); err != nil {
				fmt.Fprintln(os.Stderr, "WeeJ: "+err.Error())
				return flags, nil, 1, true
			}
			return flags, nil, 0, true

		case a == "--keep-alive":
			code := sys.KeepAlive(self, args[i+1:], func(msg string) { fmt.Println("WeeJ: " + msg) })
			return flags, nil, code, true

		case a == "--log":
			if i+1 < len(args) {
				flags.logPath = args[i+1]
				i++
			}

		case a == "--test-notifications":
			flags.testNotifications = true

		case a == "--update-site":
			if i+1 < len(args) {
				flags.updateSite = args[i+1]
				i++
			}

		case a == "--wait-pid":
			if i+1 < len(args) {
				if pid, err := strconv.ParseUint(args[i+1], 10, 32); err == nil {
					waitPIDs = append(waitPIDs, uint32(pid))
				}
				i++
			}
		}
	}
	return flags, waitPIDs, 0, false
}

func (app *App) updateSite() string {
	if app.updateSiteFlag != "" {
		return app.updateSiteFlag
	}
	return updater.DefaultSite
}

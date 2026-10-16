//go:build windows

package app

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/zolferfigueiredo/weej/internal/core"
	"github.com/zolferfigueiredo/weej/internal/platform/sys"
)

func runningProcessPath(exe string) string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(snap) }()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return ""
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name == exe {
			if path := fullProcessImagePath(entry.ProcessID); path != "" {
				return path
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			return ""
		}
	}
}

func fullProcessImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// existingExe is the exe a registry value names, when it is there.
func existingExe(value string) string {
	path := core.TrimExePath(sys.ExpandEnv(value))
	if path == "" {
		return ""
	}
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return ""
	}
	return path
}

// registryViews is where a key under HKLM\Software can be: the 64-bit view and the 32-bit one
// (WOW6432Node), named outright so the 32-bit build reads both too.
var registryViews = []struct {
	root   registry.Key
	access uint32
}{
	{registry.LOCAL_MACHINE, registry.WOW64_64KEY},
	{registry.LOCAL_MACHINE, registry.WOW64_32KEY},
	{registry.CURRENT_USER, 0},
}

func appPathsRegistryPath(exe string) string {
	const sub = `Software\Microsoft\Windows\CurrentVersion\App Paths\`
	for _, view := range registryViews {
		k, err := registry.OpenKey(view.root, sub+exe, registry.QUERY_VALUE|view.access)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("")
		k.Close()
		if err != nil {
			continue
		}
		if path := existingExe(v); path != "" {
			return path
		}
	}
	return ""
}

// ioReparseTagAppExecLink marks a Store app's alias in %LOCALAPPDATA%\Microsoft\WindowsApps.
const ioReparseTagAppExecLink = 0x8000001B

// aliasTarget is the exe behind a Store app's alias of that name: the alias is an empty file with
// no icon, and the real exe sits in the app's own folder under WindowsApps.
func aliasTarget(exe string) string {
	alias, err := windows.UTF16PtrFromString(filepath.Join(sys.ExpandEnv("%LOCALAPPDATA%"), "Microsoft", "WindowsApps", exe))
	if err != nil {
		return ""
	}
	h, err := windows.CreateFile(alias, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()

	buf := make([]byte, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var n uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &buf[0], uint32(len(buf)), &n, nil); err != nil {
		return ""
	}
	// The tag, the data's length and a reserved word, then a version and three NUL-terminated
	// UTF-16 strings: the package, the app's id and the exe.
	if n < 12 || binary.LittleEndian.Uint32(buf) != ioReparseTagAppExecLink {
		return ""
	}
	words := make([]uint16, (n-12)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(buf[12+2*i:])
	}
	var fields []string
	for len(words) > 0 && len(fields) < 3 {
		end := slices.Index(words, 0)
		if end < 0 {
			end = len(words)
		}
		fields = append(fields, windows.UTF16ToString(words[:end]))
		words = words[min(end+1, len(words)):]
	}
	if len(fields) < 3 {
		return ""
	}
	target := existingExe(fields[2])
	// An alias can start a helper (Spotify's starts SpotifyMigrator.exe), so the app's exe of the
	// alias's own name beside it gives the better name and icon.
	if target != "" {
		if own := existingExe(filepath.Join(filepath.Dir(target), exe)); own != "" {
			return own
		}
	}
	return target
}

// installedExes maps exe names to where they are installed, from what the registry and Steam
// record, for apps that are not running and have no App Paths entry. It walks folders, so it runs
// away from the UI.
func installedExes() map[string]string {
	found := map[string]string{}
	add := func(path string) {
		exe := core.ExeName(path)
		if old, ok := found[exe]; !ok || pathDepth(path) < pathDepth(old) {
			found[exe] = path
		}
	}
	uninstallExes(add)
	steamExes(add)
	return found
}

func pathDepth(path string) int { return strings.Count(path, `\`) }

// uninstallExes adds the exe each installed program's uninstall entry shows as its icon, which
// most installers point at the program itself.
func uninstallExes(add func(path string)) {
	const sub = `Software\Microsoft\Windows\CurrentVersion\Uninstall`
	for _, view := range registryViews {
		k, err := registry.OpenKey(view.root, sub, registry.ENUMERATE_SUB_KEYS|view.access)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		for _, name := range names {
			entry, err := registry.OpenKey(k, name, registry.QUERY_VALUE|view.access)
			if err != nil {
				continue
			}
			icon, _, err := entry.GetStringValue("DisplayIcon")
			entry.Close()
			if err != nil || !strings.HasSuffix(strings.ToLower(core.TrimExePath(icon)), ".exe") {
				continue
			}
			if path := existingExe(icon); path != "" {
				add(path)
			}
		}
		k.Close()
	}
}

// steamExes adds the exes in Steam's own folder and in its games', since nothing else records
// where a game's exe is, often a few folders down (common\Counter-Strike Global Offensive\game\
// bin\win64\cs2.exe).
func steamExes(add func(path string)) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	steam, _, err := k.GetStringValue("SteamPath")
	k.Close()
	if err != nil || steam == "" {
		return
	}
	steam = filepath.Clean(steam)
	walkExes(steam, 3, "steamapps", add)

	libraries := []string{steam}
	if vdf, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
		libraries = append(libraries, core.SteamLibraries(string(vdf))...)
	}
	seen := map[string]bool{}
	for _, lib := range libraries {
		lib = filepath.Clean(lib)
		if seen[strings.ToLower(lib)] {
			continue
		}
		seen[strings.ToLower(lib)] = true
		walkExes(filepath.Join(lib, "steamapps", "common"), 4, "", add)
	}
}

// walkExes adds every exe under root, going at most levels folders down and skipping any folder
// named skip.
func walkExes(root string, levels int, skip string, add func(path string)) {
	base := pathDepth(root)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (pathDepth(path)-base > levels || strings.EqualFold(d.Name(), skip)) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".exe") {
			add(path)
		}
		return nil
	})
}

func fileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}

	for _, block := range translationBlocks(buf) {
		sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, block.lang, block.codePage)
		if s := verQueryString(buf, sub); s != "" {
			return s
		}
	}
	// A handful of common blocks third-party installers use even without an
	// advertised translation table.
	for _, sub := range []string{
		`\StringFileInfo\040904b0\FileDescription`,
		`\StringFileInfo\040904e4\FileDescription`,
		`\StringFileInfo\000004b0\FileDescription`,
	} {
		if s := verQueryString(buf, sub); s != "" {
			return s
		}
	}
	return ""
}

type translationBlock struct{ lang, codePage uint16 }

func translationBlocks(buf []byte) []translationBlock {
	var value unsafe.Pointer
	var length uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&value), &length); err != nil || length < 4 {
		return nil
	}
	n := int(length) / 4
	out := make([]translationBlock, 0, n)
	words := unsafe.Slice((*uint16)(value), n*2)
	for i := 0; i < n; i++ {
		out = append(out, translationBlock{lang: words[i*2], codePage: words[i*2+1]})
	}
	return out
}

func verQueryString(buf []byte, subBlock string) string {
	var value unsafe.Pointer
	var length uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), subBlock, unsafe.Pointer(&value), &length); err != nil || length == 0 {
		return ""
	}
	return windows.UTF16PtrToString((*uint16)(value))
}

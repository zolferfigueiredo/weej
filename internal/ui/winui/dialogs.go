//go:build windows

package winui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comctl32 = windows.NewLazySystemDLL("comctl32.dll")
	comdlg32 = windows.NewLazySystemDLL("comdlg32.dll")

	procTaskDialogIndirect = comctl32.NewProc("TaskDialogIndirect")
	procGetOpenFileNameW   = comdlg32.NewProc("GetOpenFileNameW")
)

const (
	tdfAllowDialogCancellation = 0x0008
	tdcbfOKButton              = 0x0001

	mbYesNo       = 0x00000004
	mbYesNoCancel = 0x00000003

	idYes = 6
	idNo  = 7

	ofnExplorer      = 0x00080000
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnHideReadOnly  = 0x00000004

	maxPathBuf = 32768
)

// taskDialogConfig mirrors TASKDIALOGCONFIG; the two HICON/PCWSTR unions are represented as a
// single uintptr field since this package never uses the icon-resource form.
type taskDialogConfig struct {
	size                    uint32
	hwndParent              windows.HWND
	hInstance               windows.Handle
	dwFlags                 int32
	dwCommonButtons         int32
	pszWindowTitle          *uint16
	mainIcon                uintptr
	pszMainInstruction      *uint16
	pszContent              *uint16
	cButtons                uint32
	pButtons                uintptr
	nDefaultButton          int32
	cRadioButtons           uint32
	pRadioButtons           uintptr
	nDefaultRadioButton     int32
	pszVerificationText     *uint16
	pszExpandedInformation  *uint16
	pszExpandedControlText  *uint16
	pszCollapsedControlText *uint16
	footerIcon              uintptr
	pszFooter               *uint16
	pfCallback              uintptr
	lpCallbackData          uintptr
	cxWidth                 uint32
}

// taskDialogButton mirrors TASKDIALOG_BUTTON.
type taskDialogButton struct {
	id   int32
	text *uint16
}

func (l *Loop) Alert(title, text string) {
	cfg := taskDialogConfig{
		dwFlags:         tdfAllowDialogCancellation,
		dwCommonButtons: tdcbfOKButton,
		pszWindowTitle:  mustUTF16PtrFromString(title),
		pszContent:      mustUTF16PtrFromString(text),
	}
	cfg.size = uint32(unsafe.Sizeof(cfg))
	cfg.hwndParent = l.hwnd

	var chosen int32
	if !taskDialogIndirect(&cfg, &chosen) {
		// Alert has only one button; the choice MessageBox returns is not needed.
		_, _ = windows.MessageBox(l.hwnd, mustUTF16PtrFromString(text), mustUTF16PtrFromString(title), mbOK)
	}
}

func (l *Loop) Ask(title, text string, buttons []string) int {
	if len(buttons) == 0 {
		return -1
	}
	btns := make([]taskDialogButton, len(buttons))
	for i, b := range buttons {
		btns[i] = taskDialogButton{id: int32(100 + i), text: mustUTF16PtrFromString(b)}
	}

	cfg := taskDialogConfig{
		dwFlags:        tdfAllowDialogCancellation,
		pszWindowTitle: mustUTF16PtrFromString(title),
		pszContent:     mustUTF16PtrFromString(text),
		cButtons:       uint32(len(btns)),
		pButtons:       uintptr(unsafe.Pointer(&btns[0])),
		nDefaultButton: btns[0].id,
	}
	cfg.size = uint32(unsafe.Sizeof(cfg))
	cfg.hwndParent = l.hwnd

	var chosen int32
	if !taskDialogIndirect(&cfg, &chosen) {
		return askFallback(l.hwnd, title, text, buttons)
	}
	for i, b := range btns {
		if b.id == chosen {
			return i
		}
	}
	return -1
}

func taskDialogIndirect(cfg *taskDialogConfig, chosenButton *int32) bool {
	hr, _, _ := procTaskDialogIndirect.Call(uintptr(unsafe.Pointer(cfg)), uintptr(unsafe.Pointer(chosenButton)), 0, 0)
	return hr == 0 // S_OK; a nonzero HRESULT means no comctl32 v6 activation context (e.g. go run without the manifest)
}

// askFallback approximates Ask's custom buttons with MessageBoxW, which only offers fixed
// combinations; TaskDialogIndirect is expected to succeed in the real, manifested build, so this
// path only matters for ad hoc go run testing without the embedded manifest.
func askFallback(hwnd windows.HWND, title, text string, buttons []string) int {
	flags := uint32(mbOK)
	switch len(buttons) {
	case 2:
		flags = mbYesNo
	case 3:
		flags = mbYesNoCancel
	}
	ret, _ := windows.MessageBox(hwnd, mustUTF16PtrFromString(text), mustUTF16PtrFromString(title), flags)
	switch ret {
	case idYes:
		return 0
	case idNo:
		if len(buttons) >= 2 {
			return 1
		}
	}
	return -1
}

// openFileNameW mirrors OPENFILENAMEW.
type openFileNameW struct {
	structSize    uint32
	hwndOwner     windows.HWND
	hInstance     windows.Handle
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	fnHook        uintptr
	templateName  *uint16
	reservedPtr   uintptr
	reservedDword uint32
	flagsEx       uint32
}

func (l *Loop) OpenFile(title string, filter [][2]string, initialDir string) (string, bool) {
	fileBuf := make([]uint16, maxPathBuf)

	ofn := openFileNameW{
		hwndOwner: l.hwnd,
		filter:    buildFilterString(filter),
		file:      &fileBuf[0],
		maxFile:   uint32(len(fileBuf)),
		title:     mustUTF16PtrFromString(title),
		flags:     ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly,
	}
	if initialDir != "" {
		ofn.initialDir = mustUTF16PtrFromString(initialDir)
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))

	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(fileBuf), true
}

// buildFilterString joins description/pattern pairs into the double-NUL-terminated blob
// GetOpenFileNameW expects. UTF16FromString already NUL-terminates each part, which is exactly
// the separator the filter format wants; one more 0 closes the whole list.
func buildFilterString(filter [][2]string) *uint16 {
	var buf []uint16
	for _, pair := range filter {
		for _, part := range pair {
			u, err := windows.UTF16FromString(part)
			if err != nil {
				u = []uint16{0}
			}
			buf = append(buf, u...)
		}
	}
	buf = append(buf, 0)
	return &buf[0]
}

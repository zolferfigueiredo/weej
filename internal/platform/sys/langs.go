//go:build windows

package sys

import "golang.org/x/sys/windows"

func PreferredUILanguages() []string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return langs
}

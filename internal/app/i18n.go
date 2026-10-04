//go:build windows

package app

import "github.com/zolferfigueiredo/weej/internal/lang"

func (app *App) currentLanguage() string {
	app.mu.Lock()
	code := app.settings.Language
	app.mu.Unlock()
	if !lang.Valid(code) {
		return "en"
	}
	return code
}

func (app *App) tr(key string) string { return lang.T(app.currentLanguage(), key, nil) }

func (app *App) trVars(key string, vars map[string]string) string {
	return lang.T(app.currentLanguage(), key, vars)
}

func v1(name, value string) map[string]string { return map[string]string{name: value} }

func (app *App) trFunc() func(string, map[string]string) string {
	code := app.currentLanguage()
	return func(key string, vars map[string]string) string { return lang.T(code, key, vars) }
}

func (app *App) appDisplayName(exe string) string {
	app.appCacheMu.Lock()
	defer app.appCacheMu.Unlock()
	if name, ok := app.appName[exe]; ok && name != "" {
		return name
	}
	return ""
}

func (app *App) ctrlLabelName() string {
	if app.currentLanguage() == "de" {
		return "Strg"
	}
	return "Ctrl"
}

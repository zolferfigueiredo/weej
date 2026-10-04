//go:build tools

// Keeps the planned dependencies pinned in go.mod until real code imports them; delete it then.
package main

import (
	_ "github.com/go-ole/go-ole"
	_ "github.com/moutend/go-wca/pkg/wca"
	_ "github.com/wailsapp/go-webview2/pkg/edge"
	_ "go.bug.st/serial"
	_ "go.bug.st/serial/enumerator"
	_ "go.yaml.in/yaml/v3"
	_ "golang.org/x/image/font/opentype"
	_ "golang.org/x/image/vector"
	_ "golang.org/x/sys/windows"
)

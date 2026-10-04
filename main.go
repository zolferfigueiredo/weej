//go:build windows

package main

import (
	"fmt"

	"github.com/zolferfigueiredo/weej/internal/core"
)

func main() {
	fmt.Println(core.AppName, core.AppVersion)
}

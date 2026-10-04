//go:build windows

package main

import (
	"os"

	"github.com/zolferfigueiredo/weej/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}

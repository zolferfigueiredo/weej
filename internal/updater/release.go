//go:build windows

package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zolferfigueiredo/weej/internal/core"
)

const DefaultSite = "https://github.com/zolferfigueiredo/weej/releases/latest/download/"

type Release struct {
	Version string
	SHA256  string // of this build's zip, see Arch
}

func Check(ctx context.Context, site string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, site+"latest.json", nil)
	if err != nil {
		return Release{}, fmt.Errorf("updater: build request: %w", err)
	}
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("updater: check %s: %w", req.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("updater: check %s: %s", req.URL, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Release{}, fmt.Errorf("updater: read feed: %w", err)
	}

	feed, err := core.ParseFeed(data)
	if err != nil {
		return Release{}, err
	}
	return Release{Version: feed.Version, SHA256: feed.SHA256[Arch()]}, nil
}

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

const (
	DefaultSite = "https://github.com/zolferfigueiredo/weej/releases/latest/download/"
	releasesAPI = "https://api.github.com/repos/zolferfigueiredo/weej/releases"
)

type Release struct {
	Version string
	SHA256  string // of this build's zip, see Arch
}

// Check reads the newest release's latest.json. A pre-release is never GitHub's latest, so a beta
// finds the newest release in the list of them, the next beta or the release after it.
func Check(ctx context.Context, site string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if site == DefaultSite && core.IsBeta(core.AppVersion) {
		list, err := fetch(ctx, releasesAPI)
		if err != nil {
			return Release{}, err
		}
		version, err := core.NewestRelease(list)
		if err != nil {
			return Release{}, err
		}
		site = "https://github.com/zolferfigueiredo/weej/releases/download/v" + version + "/"
	}

	data, err := fetch(ctx, site+"latest.json")
	if err != nil {
		return Release{}, err
	}
	feed, err := core.ParseFeed(data)
	if err != nil {
		return Release{}, err
	}
	return Release{Version: feed.Version, SHA256: feed.SHA256[Arch()]}, nil
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("updater: build request: %w", err)
	}
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updater: check %s: %w", req.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater: check %s: %s", req.URL, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("updater: read %s: %w", req.URL, err)
	}
	return data, nil
}

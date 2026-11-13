package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// A natural-order compare, like NSString's .numeric option: digit runs compare by value (so
// "10" beats "9"), everything else compares byte by byte.
func numericCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigitByte(ca) && isDigitByte(cb) {
			starti, startj := i, j
			for i < len(a) && isDigitByte(a[i]) {
				i++
			}
			for j < len(b) && isDigitByte(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[starti:i], "0")
			nb := strings.TrimLeft(b[startj:j], "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			continue
		}
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case i < len(a):
		return 1
	case j < len(b):
		return -1
	default:
		return 0
	}
}

// IsNewer compares versions where past a "-" is a beta of the version before it: 2.7.0-beta.1,
// then 2.7.0-beta.2, then 2.7.0.
func IsNewer(remote, local string) bool {
	r, rBeta, _ := strings.Cut(remote, "-")
	l, lBeta, _ := strings.Cut(local, "-")
	if c := numericCompare(r, l); c != 0 {
		return c > 0
	}
	switch {
	case rBeta == "" || lBeta == "":
		return rBeta == "" && lBeta != ""
	default:
		return numericCompare(rBeta, lBeta) > 0
	}
}

func IsBeta(version string) bool { return strings.Contains(version, "-") }

// NewestRelease is the newest version in GitHub's list of releases, betas among them, drafts not.
func NewestRelease(data []byte) (string, error) {
	var list []struct {
		Tag   string `json:"tag_name"`
		Draft bool   `json:"draft"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return "", fmt.Errorf("core: releases: %w", err)
	}
	newest := ""
	for _, r := range list {
		if v := ReleaseVersion(r.Tag); !r.Draft && v != "" && (newest == "" || IsNewer(v, newest)) {
			newest = v
		}
	}
	if newest == "" {
		return "", errors.New("core: releases: none published")
	}
	return newest, nil
}

func ReleaseVersion(tag string) string {
	if strings.HasPrefix(tag, "v") {
		return tag[1:]
	}
	return tag
}

func NextPatch(version string) string {
	parts := strings.Split(version, ".")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	parts[len(parts)-1] = strconv.Itoa(n + 1)
	return strings.Join(parts, ".")
}

// RestartArgs drops the --wait-pid of earlier restarts: that process is gone, and Windows may
// have given its id to another one the new copy would then wait on.
func RestartArgs(args []string, pid int) []string {
	out := make([]string, 0, len(args)+2)
	for i := 0; i < len(args); i++ {
		if args[i] == "--wait-pid" {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return append(out, "--wait-pid", strconv.Itoa(pid))
}

func IsDue(every int, last, now time.Time) bool {
	return every > 0 && now.Sub(last) >= time.Duration(every)*time.Second
}

type Feed struct {
	Version string
	SHA256  map[string]string
}

func ParseFeed(data []byte) (Feed, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Feed{}, fmt.Errorf("core: invalid feed: %w", err)
	}
	versionRaw, ok := raw["version"]
	if !ok {
		return Feed{}, errors.New("core: feed missing version")
	}
	var version string
	if err := json.Unmarshal(versionRaw, &version); err != nil || version == "" {
		return Feed{}, errors.New("core: feed has invalid version")
	}
	feed := Feed{Version: version}
	if sumsRaw, ok := raw["sha256"]; ok {
		var sums map[string]string
		if err := json.Unmarshal(sumsRaw, &sums); err == nil {
			feed.SHA256 = sums
		}
	}
	return feed, nil
}

func CheckSHA256(r io.Reader, hexSum string) error {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(sum, hexSum) {
		return fmt.Errorf("core: checksum mismatch: got %s want %s", sum, hexSum)
	}
	return nil
}

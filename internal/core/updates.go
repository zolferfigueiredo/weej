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

func IsNewer(remote, local string) bool { return numericCompare(remote, local) > 0 }

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

package core

import (
	"strings"
	"testing"
	"time"
)

func TestNextPatchVersion(t *testing.T) {
	cases := map[string]string{"1.7.0": "1.7.1", "0.9": "0.10", "?": "1"}
	for in, want := range cases {
		if got := NextPatch(in); got != want {
			t.Errorf("NextPatch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		remote, local string
		want          bool
	}{
		{"0.1.3", "0.1.2", true},
		{"0.1.10", "0.1.9", true}, // numeric, not alphabetical
		{"1.0.0", "0.9.9", true},
		{"0.1.2", "0.1.2", false},
		{"0.1.1", "0.1.2", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.remote, c.local); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.remote, c.local, got, c.want)
		}
	}
}

func TestUpdateCheckSchedule(t *testing.T) {
	now := time.Now()
	hour := time.Hour
	day := 24 * hour
	if !IsDue(int(day.Seconds()), time.Time{}, now) {
		t.Error("never checked before: should be due")
	}
	if IsDue(0, time.Time{}, now) {
		t.Error("every == 0 means never")
	}
	if IsDue(int(day.Seconds()), now.Add(-23*hour), now) {
		t.Error("23h < a day: should not be due yet")
	}
	if !IsDue(int(day.Seconds()), now.Add(-25*hour), now) {
		t.Error("25h > a day: should be due")
	}
	if IsDue(int(7*day.Seconds()), now.Add(-6*day), now) {
		t.Error("6 days < a week: should not be due yet")
	}
	if !IsDue(int(7*day.Seconds()), now.Add(-7*day), now) {
		t.Error("7 days >= a week: should be due")
	}
}

func TestReleaseTagsGiveVersions(t *testing.T) {
	if got := ReleaseVersion("v1.2.3"); got != "1.2.3" {
		t.Errorf("ReleaseVersion(v1.2.3) = %q, want 1.2.3", got)
	}
	if got := ReleaseVersion("1.2.3"); got != "1.2.3" {
		t.Errorf("ReleaseVersion(1.2.3) = %q, want 1.2.3", got)
	}
}

func TestParseFeed(t *testing.T) {
	feed, err := ParseFeed([]byte(`{"version":"1.2.3","sha256":{"WeeJ-1.2.3.exe":"abc"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if feed.Version != "1.2.3" || feed.SHA256["WeeJ-1.2.3.exe"] != "abc" {
		t.Errorf("feed = %+v", feed)
	}

	if _, err := ParseFeed([]byte(`{"sha256":{}}`)); err == nil {
		t.Error("want error: missing version")
	}
	if _, err := ParseFeed([]byte(`["not", "an", "object"]`)); err == nil {
		t.Error("want error: non-object feed")
	}
	if _, err := ParseFeed([]byte(`not json`)); err == nil {
		t.Error("want error: invalid JSON")
	}
}

func TestCheckSHA256(t *testing.T) {
	// sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	const sum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if err := CheckSHA256(strings.NewReader("hello"), sum); err != nil {
		t.Errorf("CheckSHA256() = %v, want nil", err)
	}
	if err := CheckSHA256(strings.NewReader("hello"), "0000"); err == nil {
		t.Error("want error: checksum mismatch")
	}
}

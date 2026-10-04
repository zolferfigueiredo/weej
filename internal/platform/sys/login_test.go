//go:build windows

package sys

import "testing"

func TestRunCommand(t *testing.T) {
	cases := []struct {
		name string
		exe  string
		args []string
		want string
	}{
		{
			name: "plain",
			exe:  `C:\Program Files\WeeJ\WeeJ.exe`,
			args: nil,
			want: `"C:\Program Files\WeeJ\WeeJ.exe"`,
		},
		{
			name: "args need no quoting",
			exe:  `C:\WeeJ\WeeJ.exe`,
			args: []string{"--tray"},
			want: `C:\WeeJ\WeeJ.exe --tray`,
		},
		{
			name: "arg with a space",
			exe:  `C:\WeeJ\WeeJ.exe`,
			args: []string{"--profile", "my profile"},
			want: `C:\WeeJ\WeeJ.exe --profile "my profile"`,
		},
		{
			name: "arg with an embedded quote",
			exe:  `C:\WeeJ\WeeJ.exe`,
			args: []string{`say "hi"`},
			want: `C:\WeeJ\WeeJ.exe "say \"hi\""`,
		},
		{
			name: "empty arg",
			exe:  `C:\WeeJ\WeeJ.exe`,
			args: []string{""},
			want: `C:\WeeJ\WeeJ.exe ""`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runCommand(c.exe, c.args); got != c.want {
				t.Errorf("runCommand(%q, %v) = %q, want %q", c.exe, c.args, got, c.want)
			}
		})
	}
}

func TestCommandExe(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"quoted with args", `"C:\Program Files\WeeJ\WeeJ.exe" --tray`, `C:\Program Files\WeeJ\WeeJ.exe`},
		{"bare no args", `C:\WeeJ\WeeJ.exe`, `C:\WeeJ\WeeJ.exe`},
		{"bare with args", `C:\WeeJ\WeeJ.exe --tray`, `C:\WeeJ\WeeJ.exe`},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := commandExe(c.line); got != c.want {
				t.Errorf("commandExe(%q) = %q, want %q", c.line, got, c.want)
			}
		})
	}
}

func TestCommandPointsTo(t *testing.T) {
	line := `"C:\Program Files\WeeJ\WeeJ.exe" --tray`
	if !commandPointsTo(line, `c:\program files\weej\weej.exe`) {
		t.Errorf("commandPointsTo(%q) = false, want true (case insensitive match)", line)
	}
	if commandPointsTo(line, `C:\Other\WeeJ.exe`) {
		t.Errorf("commandPointsTo(%q) matched an unrelated exe", line)
	}
}

func TestStartupApprovedDisabled(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"enabled 02", []byte{0x02, 0, 0, 0, 0, 0, 0, 0}, false},
		{"enabled 06", []byte{0x06, 0, 0, 0, 0, 0, 0, 0}, false},
		{"disabled 03", []byte{0x03, 0, 0, 0, 0, 0, 0, 0}, true},
		{"disabled 07", []byte{0x07, 0, 0, 0, 0, 0, 0, 0}, true},
		{"empty", []byte{}, false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := startupApprovedDisabled(c.data); got != c.want {
				t.Errorf("startupApprovedDisabled(%v) = %v, want %v", c.data, got, c.want)
			}
		})
	}
}

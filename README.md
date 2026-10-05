<p align="center">
  <img src="docs/icon.png" width="128" height="128" alt="WeeJ icon">
</p>

<h1 align="center">WeeJ</h1>

<h3 align="center">Real knobs for your PC.</h3>

<p align="center">
  The Windows client for <a href="https://github.com/omriharel/deej">deej</a>. Volume for Windows and your apps, brightness and more,<br>
  straight from the Arduino mixer on your desk.
</p>

<p align="center">
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.27-00ADD8" alt="Go 1.27"></a>
  <img src="https://img.shields.io/badge/Platform-Windows%2010%20%7C%2011-blue" alt="Windows 10 or 11">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow" alt="MIT license"></a>
  <a href="https://github.com/zolferfigueiredo/weej/actions/workflows/ci.yml"><img src="https://github.com/zolferfigueiredo/weej/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
</p>

<p align="center">
  <a href="https://github.com/zolferfigueiredo/weej/releases/latest"><b>Download for Windows</b></a>
  &nbsp;·&nbsp;
  <a href="https://github.com/zolferfigueiredo/theej">TheeJ, for the Mac</a>
</p>

<p align="center">
  <img src="docs/screenshot.png" width="462" alt="WeeJ Settings: five knobs, each with the jobs it does">
</p>

## Install

1. [Download `WeeJ-x64-setup.exe`](https://github.com/zolferfigueiredo/weej/releases/latest) and run it. It installs for your user only, so it needs no administrator rights, and it can launch WeeJ at login.
2. The installer isn't code-signed, so SmartScreen may say "Windows protected your PC". Click **More info**, then **Run anyway**.
3. Open WeeJ. It asks which language to use, starting from Windows' own. Its icon sits in the tray: if you don't see it, open **Show hidden icons** on the taskbar and drag it next to the clock.
4. Plug in your deej board. Calibrate opens on its own: move each knob from end to end, and Settings opens to choose what each one does.

Coming from deej? **Import from deej…** in Settings turns your `config.yaml` into a profile.

Or take the portable [`WeeJ-x64.zip`](https://github.com/zolferfigueiredo/weej/releases/latest) and unzip it anywhere.

You need:

- Windows 10 or 11, 64-bit
- Any deej board, over USB. Your Arduino sketch stays as it is.
- The Microsoft Edge WebView2 Runtime, for the Settings windows. Windows 11 has it, and on Windows 10 it comes with Edge. WeeJ tells you if it's missing.
- For external screens: monitors with DDC/CI turned on in their own menu

**Quit deej first.** Only one app can open the board's port at a time; while another one has it, WeeJ shows the port as in use by another app. **Quit Twinkle Tray, Monitorian or any similar app** before you give a knob a screen: two apps writing the same screen over DDC/CI fight over the value.

## Features

- **One knob, several jobs.** The volume of Windows, your mic, one app, the focused app or every other app, a screen's brightness or contrast, Night light, a keyboard backlight, screen zoom. Tick several, of any kind, and they all follow the knob.
- **An on-screen indicator.** A flyout in the style of Windows 11's own shows up on the display the knob controls, with the app's own icon for an app's volume. Switching profiles shows the new profile's name.
- **Finds your knobs by itself.** Calibrate opens when your board first connects, works out how many knobs you have and which input each is on, then sweeps a jumpy pot clean.
- **Profiles on a shortcut.** Switch every knob's jobs at once, from the tray or with a shortcut from any app.
- **No jumps.** A knob takes up a new job the next time you move it, so switching profiles never jumps the volume or a screen.
- **Real per-app volume.** Windows keeps a volume for every app, and WeeJ turns that same slider you see in the Volume mixer. Nothing is captured or delayed.
- **Follows your audio device.** Switch outputs or inputs, Bluetooth headphones included, and the knobs follow.
- **Any number of knobs.** As many as your sketch sends, up to 26. One switch inverts them all for pots wired the other way round.
- **Same firmware.** Speaks the deej serial protocol, unchanged, at 9600 baud or whatever your sketch uses.
- **Speaks 12 languages.** Deutsch, English, Español, Français, Italiano, Polski, Português, Русский, Українська, 中文, 日本語 and 한국어. **Language** in the menu and in App settings changes it at once, open windows included.
- **Lives in the tray.** A left click opens Settings and a right click opens the menu. It reconnects on its own, can launch at login, and installs updates in one click.

## What a knob can do

- **Master volume**: the default output device, through Windows Core Audio.
- **Microphone volume**: the default input device, the same level as in Sound settings.
- **System sounds**: Windows' own notification and alert sounds.
- **An app's volume**: an app picked under Apps, through its own Windows audio session, the slider the Volume mixer shows. Apps lists the apps that make sound: the ones playing right now, well-known players, browsers and call apps you have installed, open or not, and any app already on a knob. **Other…** at its end picks any program. An app that starts playing gets the knob's level within a second.
- **Focused app**: whichever app owns the window in front, as deej's `deej.current` does.
- **Other apps**: every app that has no knob of its own in the active profile, as deej's `deej.unmapped` does.
- **Built-in display brightness**: a laptop's panel, through WMI. Desktops have none.
- **Screen brightness** and **Screen contrast**: each external screen over DDC/CI. Screens count left to right by their position in Display settings.
- **Night light warmth** (Experimental): off at the bottom of the knob, then from least to most warm. It is Windows' own Night light, so a schedule still switches it on and off at its set times.
- **External keyboard backlight**: a QMK keyboard with VIA, such as a Keychron K8 Pro, on its USB cable (not Bluetooth). Nothing is saved to the keyboard, so unplugging it brings back its own level. The knob sets brightness only.
- **Screen zoom**: 1x at the bottom of the knob, up to 10x at the top, with the pointer kept in view as it moves. It's the Windows full-screen magnifier, driven directly. Quitting WeeJ zooms back out.

The volumes follow the knob as it turns. Everything else waits for the knob to settle, as described under On-screen feedback.

## How it works

Upstream deej already runs on Windows; WeeJ is a different take on the same idea. It's the Windows sibling of [TheeJ](https://github.com/zolferfigueiredo/theej): one Go program that speaks the same serial protocol and adds profiles, calibration, more jobs and Settings windows.

- It reads the deej serial protocol at 9600 baud by default (the Connection tab sets another speed), however many values the sketch sends. It finds the board among your USB serial ports by itself and reconnects when it's unplugged.
- The **volumes** go through Windows Core Audio: the endpoint volume for the output and input devices, and audio sessions for apps and system sounds.
- **External screens** go over DDC/CI through the Windows Monitor Configuration API, one write at a time. The **built-in display** goes through WMI.
- **Night light** is stored by Windows in an undocumented setting, so a future Windows update could break it. That's why it's marked Experimental.
- **Screen zoom** uses the Magnification API, and a **VIA keyboard's backlight** goes over USB HID.
- The tray icon, its menu and the indicator are native Win32. The Settings, Calibration, Language and Update windows are pages in WebView2, laid out like Windows 11's own Settings and following its light or dark theme and accent color.
- **Check for updates…** asks GitHub for the newest release and sends nothing about you. An update downloads that release's zip, checks it against the release's SHA-256, and installs only into the installed copy (`%LOCALAPPDATA%\Programs\WeeJ`).
- Only one copy runs at a time. Opening WeeJ again while it runs brings up Settings, which is the way back with the tray icon hidden.

<details>
<summary><b>The tray icon</b></summary>

The icon shows whether the board is connected. Settings offers three looks:

- **Mixer**, the default: five fader caps that trace a W. Disconnected, they drop to a flat line.
- **Dial**: a knob in a track, lit up to its pointer. Disconnected, the pointer drops to the minimum and the track dims.
- **App icon**: the app icon itself, the same whether connected or not.

Mixer and Dial follow a light or dark taskbar. All three are drawn in code in [internal/draw](internal/draw).

A left click, or Enter on the focused icon, opens Settings. A right click opens the menu: the profiles, with a check by the active one and each one's shortcut, then Settings, Calibrate and Language, then the current port with Reconnect under it, then Launch at login, About WeeJ, which opens the About tab of Settings, Check for updates… with Check automatically (daily, weekly by default, or never), and Quit WeeJ. Settings can leave the profiles out of the menu, or hide the icon altogether.

</details>

<details>
<summary><b>Settings and calibration</b></summary>

**Settings** has four tabs, laid out in groups as Windows Settings is: **General** for the profile and its knobs, **App settings** for the language, shortcuts, the tray icon and sensitivity, **Connection** for the board's port and speed, and **About**. General lists every knob by the letter on the box with a menu for what it does: any of the jobs under [What a knob can do](#what-a-knob-can-do), each with its icon. Clicking a job ticks it and clicking it again unticks it, so one knob can do several at once, of any kind: two screens' brightness, or an app's volume and a keyboard backlight. Clear, at the top of the menu, unticks them all. The + and - buttons beside Knobs add or remove the last knob. A knob that has not been calibrated yet shows "Needs calibration" in red under its name, and does nothing until it is. **Invert knobs** flips every knob's direction, and **Import from deej…** turns a deej `config.yaml` into a new profile (its `com_port` is left out on purpose, because WeeJ finds the board by itself and Windows can renumber ports).

Those jobs belong to a **profile**: a name, the jobs of every knob, and an optional keyboard shortcut. The menu at the top picks the profile you are editing, + adds one, and - removes the one shown. In App settings, under **Shortcuts**, **Next profile** and **Previous profile** step through them in order from any app, and every profile is listed below them with its own shortcut. To set one, click Record Shortcut and press it: it needs Ctrl or Alt with a key, Delete clears it and Escape cancels.

Under **Tray icon**, **Hide tray icon** removes the icon (open WeeJ again to get back to Settings), **Icon** picks Mixer, Dial or App icon, and **Profile list** puts the profiles in the menu. Under **Sensitivity**, **Speed** sets how long a knob has to be still before its change lands: Slow (0.3 seconds, recommended), Medium (0.22), Fast (0.18) or Super fast (0.15). The volumes are not affected; they always follow the knob.

**Connection** shows the port and whether the board is connected. **Port** is Automatic, which finds the board by itself, or a fixed COM port; **Baud rate** must match `Serial.begin()` in your sketch.

**About** shows the version, with Check for updates…, and links to the website, to zolfer.com and to TheeJ, WeeJ's sibling for the Mac.

Save applies at once, makes the profile shown the active one, and leaves the window open. A knob given a new job, by Save or by switching profiles, takes it over the next time you move it.

**Calibration** finds your knobs by itself. **Calibrate** in the menu runs it any time the board is connected, as does the Calibrate button under the knobs in Settings, and asks for knob A, then B, and so on. It also opens on its own when a knob needs it, such as the first time the board connects. For each knob it asks you to:

1. Move it from one end to the other, so WeeJ can tell which knob it is.
2. Turn it back and forth, from one end to the other, for 20 seconds.

Step 2 is the cure for jumpy knobs (below). Its timer only runs while the knob turns. Skip keeps a knob that's already set up, and Finish ends the run when WeeJ asks for a knob you don't have. Once every value the sketch sends has a knob, it says so. Every knob holds still for the whole run, Cancel leaves everything as it was, and Settings comes to the front at the end to choose what each knob does.

</details>

<details>
<summary><b>On-screen feedback</b></summary>

Turning a knob shows a flyout like the one Windows shows for its own volume keys, on the display that knob controls: a speaker, microphone, sun, half-filled circle, moon, keyboard or magnifying glass, or the app's own icon, with the level. It follows the light or dark theme. Switching profiles shows the profile's name the same way.

The indicator tracks the knob live. Everything but the volumes only changes once the knob has been still for a moment (the Speed setting), so a turn lands as one clean change when you let go instead of flickering a screen through every position on the way. The volumes follow the knob immediately.

</details>

<details>
<summary><b>Jumpy knobs</b></summary>

If a knob's level jumps around while you turn it, the pot's track is oxidised. The wiper loses contact for anywhere from 15ms to a few hundred ms and the Arduino reads a stray value, often near the ends of travel. It builds up on knobs that rarely move, which is why the volume knob stays clean.

Sweep the knob slowly from end to end a dozen or so times. **Calibrate** in the menu walks you through it, one knob at a time. A drop of potentiometer contact cleaner makes it last. Screens and lights are protected meanwhile: everything but the volumes only applies once the knob settles, so a stray reading shorter than the Speed setting's wait never reaches them. If a screen flashes on a faster Speed, go back to Slow.

</details>

## Build from source

Needs Windows 10 or 11 and Go 1.27 (`winget install GoLang.Go`).

```bash
run.bat
```

`run.bat` quits any running copy it built, builds with `build.bat` and runs the new build in the terminal, where it prints live knob values so you can see which physical knob is which input. Its first run also turns on the repository's git hook, which refuses commits made directly on `main`. `build.bat` runs the tests, draws the icons, embeds them and the version into the exe and produces `build\WeeJ.exe`.

```bash
run.bat COM6                  # force a specific port
go test ./...                 # run the tests
installer.bat                 # build dist\WeeJ-<version>-x64-setup.exe on this PC
```

`installer.bat` needs Inno Setup 6 (`winget install JRSoftware.InnoSetup`).

**Run at login.** The installed copy turns this on with **Launch at login** in the menu. For a build run from this folder, `install.bat` starts it at login and restarts it if it crashes, logging to `%TEMP%\weej.log`; `install.bat --uninstall` removes it. Quit from the menu really does quit.

**Release.** Raise `AppVersion` in [version.go](internal/core/version.go), merge to `main`, then run `release.bat` on `main`. It tags the commit `v<version>`, pushes the tag and prints the installer's download link, and the [release workflow](.github/workflows/release.yml) builds and publishes the installer, the zip, `latest.json` and checksums as a GitHub release. It runs on a Windows runner, so it also runs the tests of the Windows-only packages.

**CI** runs on Linux for every pull request: the tests, golangci-lint and a build for Windows. It also checks the git hook with shellcheck, keeps em and en dashes out of the translations, and fails a pull request that changes the app without raising `AppVersion`. A pull request that touches the release workflow or the installer also gets a build-only release run.

<details>
<summary><b>Tuning</b></summary>

What each knob does and which input it is on live in Settings, not in source. A fresh install has no knobs, and calibration finds them. Settings are stored as JSON in `%APPDATA%\WeeJ\settings.json`: delete it and restart WeeJ to start over. The log is `%LOCALAPPDATA%\WeeJ\weej.log`.

Constants, then rebuild:

- `Speed.Settle`, in [setup.go](internal/core/setup.go): `0.3`, `0.22`, `0.18` and `0.15` seconds, the waits behind the Speed setting. Every knob but the volumes applies only once it has been still this long. This is also what hides wiper contact bounce, where a moving pot briefly reports its neighbour's value for up to about 0.11s, so keep every one above that.
- `turnSeconds`, in [calibrator.go](internal/core/calibrator.go): `20` seconds of turning per knob.

Turning a brightness knob fully down sets the backlight to 0, and a contrast knob at 0 leaves a screen close to black. The knob is the way back.

</details>

## Disclaimer

Unofficial. An independent client for the [deej](https://github.com/omriharel/deej) serial protocol, not affiliated with deej or with Microsoft. Night light relies on an undocumented Windows setting that a future Windows update may change.

## License

[MIT](LICENSE)

---

<p align="center">
  If WeeJ is useful to you, please consider giving it a ⭐<br>
  It helps other deej builders find it. Thank you!
</p>

<p align="center">
  Made with ❤️ for the deej community by <a href="https://zolfer.com">zolfer.com</a>
</p>

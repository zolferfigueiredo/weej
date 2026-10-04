# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

<!-- The windows shell (internal/ui/web) is native Win32 + WebView2, but the surface this record
covers is the HTML/CSS/JS rendered inside it, so the web schema applies. -->

## Stack

static HTML/CSS/JS, no build step, no framework (fixed by the task brief: vanilla JS modules, served from a Go embed.FS, no external URLs)

## Users

[Inferred from the task brief and TheeJ (WeeJ's macOS predecessor), not confirmed by interview: no interactive user was available in this session.] A single person, the owner of the PC, who wired a DIY knob box (an Arduino-class board with physical potentiometers) to their Windows machine and uses WeeJ to map each knob to something it controls: app/system volume, monitor brightness or contrast, keyboard backlight, night light warmth, screen zoom. They open these windows rarely, in short sessions, mostly right after plugging the board in for the first time, after adding a knob, or when something needs re-pairing.

## Product Purpose

WeeJ is a Windows tray companion app (ported from the user's own macOS app, TheeJ) that reads serial input from a knob box and drives OS and app volume/display/keyboard controls from it. These four windows are its only GUI surface: Settings (profiles, knob-to-job mapping, shortcuts, language, tray and sensitivity options), Calibration (a short guided wizard that finds which physical knob is which), the first-run Language prompt, and the Update progress window. Success is a user who can set up or adjust their knob mapping without confusion, in a window that behaves like a native part of Windows 11.

## Positioning

[Inferred.] Unlike a general macro-key or stream-deck app, WeeJ is purpose-built around physical rotary knobs reporting a 0-1023 analog value per turn, with a calibration flow that teaches the app which wire is which knob and a job system that targets OS volume/display primitives directly (not emulated keystrokes).

## Operating Context

- Runs from the Windows system tray; these windows are opened from the tray menu or on first run, never the main interaction surface of the OS session.
- The knob box connects over a serial (COM) port; Settings' Calibrate flow and the standalone Calibration window both depend on that connection being live.
- Settings is opened far more often than the other three windows; Language and Update are seen rarely (first run, and whenever an update is offered).
- The app and its windows must look native on Windows 11 in both the light and dark OS themes, and follow the OS accent color, since that is what signals "this app belongs here" to the user.

## Capabilities and Constraints

- No build step; pages are plain HTML/CSS/JS (ES modules allowed), no external URLs, served over `https://weej.localhost` by the Go host via WebResourceRequested.
- Every visible string must come from `internal/lang/catalogs/<lang>.json` through a `t(key, vars)` helper; no hardcoded copy, no em or en dashes anywhere.
- The JS/Go message protocol, window sizing (DIP, height self-reported by the page and capped to the work area), and the four windows' init payloads live in `internal/ui/web` (bridge.js and the page scripts) and `internal/app`; this record does not repeat them.
- Only strings may be posted from JS to Go (`chrome.webview.postMessage(JSON.stringify(...))`); the host echoes every message back to the page, which the bridge must ignore.
- Fonts available: Segoe UI Variable where present (Windows 11), falling back to Segoe UI, then system sans; no web font loading (no external URLs allowed).
- Twelve languages are supported (`internal/lang.Languages`), several written right-to-left-adjacent scripts are not among them (all are LTR), but several use longer average string lengths (German, Russian, Ukrainian, Polish) and two run without word spaces in the calibration prose (zh, ja): layout must not assume English string lengths.

## Brand Commitments

- Name: WeeJ. Predecessor/sibling app: TheeJ (macOS), credited in the About tab ("Inspired by deej", deej being the open-source project TheeJ and WeeJ both descend from).
- Visual reference is explicitly pinned by the brief: the Windows 11 Settings app's own look (Fluent Design / WinUI 3 conventions), in both light and dark OS themes, using the OS accent color and Segoe UI Variable.

## Evidence on Hand

- `internal/lang/catalogs/en.json`: the full English copy catalog; every string these pages show must be a key from it.
- TheeJ's Settings.swift, CalibrationWindow.swift, LanguagePrompt.swift and Updates.swift (macOS, fetched read-only for behavior reference, not ported verbatim): establish the tab structure, controls, state machine and flows these Windows pages adapt.
- No screenshots, logos, or icon assets were supplied beyond what `internal/draw` can render server-side as data URIs (catalog job icons, app icon). No real app-store listing, pricing, or customer evidence exists or is implied; none is fabricated here.

## Product Principles

1. Feel native first. A Windows 11 user should never notice these are web pages: system fonts, system accent, system light/dark switching, native-feeling controls (toggles, segmented tabs, dropdowns), no web chrome.
2. Say only what the catalog says. Copy is a translated, tested asset owned by `internal/lang`; these pages arrange and emphasize it, never invent or rephrase it.
3. Small, rare, task-shaped windows. Nobody lives in these windows: get them to show the right state fast, keep controls dense but legible, and never block on decorative motion.
4. One state machine, driven from Go. Calibration and Update progress are fully server-pushed (`step`/`update` messages); the page is a thin renderer of whatever Go last sent, never a place that re-derives app state.

## Accessibility & Inclusion

[Inferred from the task brief's own requirement ("Keyboard accessible, with visible focus") and from Windows 11's own accessibility bar, not confirmed by interview.] Full keyboard operability (tab order, Enter/Escape where TheeJ's macOS originals used Return/Escape, visible focus rings matching Fluent's focus-rect style), 4.5:1 text contrast in both themes, and respect for the OS light/dark and accent settings rather than a fixed palette.

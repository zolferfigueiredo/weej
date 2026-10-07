---
name: WeeJ
description: A Windows 11 tray companion app for DIY knob boxes and MIDI mixers; its three WebView2 windows execute the Windows 11 Settings app's own Fluent language rather than inventing one.
colors:
  accent: "#0078d4"
  accent-text-light: "#ffffff"
  accent-text-dark: "#1a1a1a"
  bg-light: "#f3f3f3"
  bg-dark: "#202020"
  card-light: "#fbfbfb"
  card-dark: "#2c2c2c"
  control-light: "#ffffff"
  control-dark: "#2c2c2c"
  text-light: "#1a1a1a"
  text-dark: "#ffffff"
  text-secondary-light: "#5d5d5d"
  text-secondary-dark: "#c5c5c5"
  danger-light: "#c42b1c"
  danger-dark: "#ff99a4"
  warning-light: "#9d5d00"
  warning-dark: "#ffb956"
  success-light: "#0f7b0f"
  success-dark: "#6ccb5f"
  scrim: "rgba(0, 0, 0, 0.35)"
typography:
  label:
    fontFamily: "Segoe UI Variable Text, Segoe UI Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "10px"
    fontWeight: 600
    lineHeight: 1
    letterSpacing: "0.02em"
  title:
    fontFamily: "Segoe UI Variable Display, Segoe UI Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: 1.3
  subtitle:
    fontFamily: "Segoe UI Variable Text, Segoe UI Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 600
    lineHeight: 1.3
  body:
    fontFamily: "Segoe UI Variable Text, Segoe UI Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.4
  caption:
    fontFamily: "Segoe UI Variable Text, Segoe UI Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "12px"
    fontWeight: 400
    lineHeight: 1.4
rounded:
  xs: "3px"
  sm: "4px"
  md: "6px"
  lg: "8px"
  icon: "14px"
  full: "999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "20px"
  xl: "24px"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-text-light}"
    rounded: "{rounded.md}"
    padding: "7px 14px"
  button-secondary:
    backgroundColor: "{colors.control-light}"
    textColor: "{colors.text-light}"
    rounded: "{rounded.md}"
    padding: "7px 14px"
  card:
    backgroundColor: "{colors.card-light}"
    rounded: "{rounded.lg}"
---

# Design System: WeeJ

## Overview

**Creative North Star: "The Settings App You Already Trust"**

WeeJ's three windows (Settings, the first-run Language prompt, Update progress) are not a brand exercise. The brief pinned the direction before any design round could run: look and behave like the Windows 11 Settings app itself, in both OS themes, following the OS accent color. The system here is Fluent executed faithfully, not Fluent as inspiration. Nothing in these pages should let a Windows 11 user guess they are looking at HTML.

That constraint shapes everything. Density stays native-Settings: grouped cards of rows (SettingsCard), a left label plus a right-aligned control, hairline dividers between rows, generous but not loose vertical rhythm between groups. Color stays restrained to the bone: the accent appears only on the primary action, the active tab's underline, a focused toggle, and nothing else. Depth is almost entirely absent; a 1px border carries every card, and only what floats above the page (the job picker, the dialogs) earns a shadow.

The three windows share one visual system (one stylesheet, one component set) but differ in posture: Settings is a working document (tabs, dense rows, two persistent footer actions) with its few dialogs (a board's settings, Add board, calibration, confirm), while Language and Update are small, centered, single-purpose moments with no chrome beyond a title, body text and one or two buttons.

**Key Characteristics:**
- Segoe UI Variable throughout; no secondary/display typeface
- Restrained color: accent used only for selection, primary actions and active/on states
- Flat cards on hairline borders; the flyout is the only shadowed surface
- A single native-style focus rect (2px, offset 2px) on every interactive element
- Light and dark are driven by the OS theme message, not a toggle the page owns itself

## Colors

Restrained by the brief, not by default: a single accent carries every call to action, every selection state and nothing else.

### Primary
- **Windows Accent** (`#0078d4` fallback; the real value arrives live from `sys.Accent()` via the `theme` message): the Apply/Continue/Next/Finish primary buttons, the active tab's underline, a checked toggle or checkbox, the outline of the control picked on a drawn board, the calibration level bar, and the focus ring on interactive controls. Never used for body text or decoration.

### Neutral
- **Mica Ground** (`#f3f3f3` light / `#202020` dark): the window background, standing in for the native Mica backdrop the host applies to the HWND itself (see Elevation & Depth).
- **Card Surface** (`#fbfbfb` light / `#2c2c2c` dark): every SettingsCard, the job-picker flyout, and confirm dialogs.
- **Control Surface** (`#ffffff` light / `#2c2c2c` dark): text inputs, selects, and secondary buttons, one step brighter than the card they usually sit inside.
- **Primary Text** (`#1a1a1a` light / `#ffffff` dark) and **Secondary Text** (`#5d5d5d` light / `#c5c5c5` dark): row titles versus row descriptions/hints; both verified at or above 4.5:1 against their surface.
- **Danger** (`#c42b1c` light / `#ff99a4` dark): Remove board and the remove dialogs' destructive action, and a port another app holds.
- **Warning** (`#9d5d00` light / `#ffb956` dark): the calibration dialog's warning line (a control already found, a press on another button).
- **Success** (`#0f7b0f` light / `#6ccb5f` dark): a board's Connected status, the one place a state is shown in color as well as words.

### Named Rules
**The One Signal Rule.** The accent color appears on exactly the controls that are either the primary action or currently selected/on. It never fills a card, a background, or decorative chrome: Windows Settings uses it as a pointer, not a paint.

## Typography

**Display/Body Font:** Segoe UI Variable Text, falling back to Segoe UI Variable, then Segoe UI, then the platform sans. No second typeface anywhere in the system; Windows 11 Settings itself does not pair fonts, and neither does this.

**Character:** A single system sans at a tight, product-appropriate scale (1.0-1.43 ratio between steps), never a display face standing in for brand voice.

### Hierarchy
- **Title** (600, 20px, 1.3): the one page-level heading at the top of Settings, and the equivalent heading in the Language/Update windows.
- **Subtitle** (600, 14px, 1.3): group headers above a card ("Boards", "Profile", "Knobs"), the dialog title, and the calibration dialog's instruction.
- **Body** (400, 14px, 1.4): row titles, button labels, input text, dialog body copy.
- **Caption** (400, 12px, 1.4): row descriptions/hints, a board's status, the calibration counter.
- **Label** (600, 10px, 1, +0.02em, uppercase): the one place type drops below Caption, the "Experimental" badge next to the Night light job in the picker; reserved for that single chip role, not a general-purpose tiny-text size.

### Named Rules
**The No Second Voice Rule.** Weight and size carry hierarchy; there is no italic, no letter-spaced small-caps kicker, and no second font family anywhere in the system.

## Layout

Each window stacks its children top to bottom with a consistent gap: `.app` (Settings) or `.center-page` (the other two). Settings is the one window that can be resized and maximized. Until the user does either, the page reports its own rendered height to the host after every change and the host resizes the window's client area to match, capped to the work area; once sized by hand or maximized, it keeps its size and the page scrolls. Its one breakpoint, at 760px, folds the side-by-side layouts (General's two columns, a board's rows, List's three cards) into a single column.

Settings uses a persistent two-button footer (`position: sticky; bottom: 0`) so Close/Apply stay reachable regardless of tab content length. The two single-purpose windows center their content both axes, with a fixed 320px measure for body copy so translated strings with longer average length (German, Russian, Polish) still read comfortably.

## Elevation & Depth

Mostly flat, by the brief's own evidence (Windows 11 Settings is a Mica surface with hairline cards, not a shadow-heavy system). The window's native Mica backdrop is requested by the host at the HWND level (`DWMWA_SYSTEMBACKDROP_TYPE`); the page content itself renders as fully opaque cards in a solid approximation of that Mica tone, so the page looks correct standing alone (including in a plain browser preview) rather than depending on true backdrop blending.

### Shadow Vocabulary
- **Flyout** (`0 4px 16px rgba(0,0,0,.14), 0 0 2px rgba(0,0,0,.08)`): the only shadow in the system, reserved for the confirm dialogs, which visually float above the page rather than belong to it. The job picker floats too, but as a window of its own, so Windows draws its shadow.

### Named Rules
**The Flat-At-Rest Rule.** Every card, row, button and input is flat with a 1px border at rest. A shadow appears only on a surface that is actually layered above the page (a flyout, a dialog), never on a card that merely wants emphasis.

## Shapes

Two radii carry most of the system: **8px** for cards (`.card`, the flyout, dialogs) and **4-6px** for controls nested inside them (buttons, inputs, selects). Three smaller or larger tokens round it out: **3px** for the checkbox, slightly tighter than a button since it is a smaller target; **14px** for the 64px app icon shown in About and the Language/Update windows, matching the corner proportion of a Windows 11 app icon; and **999px (full)** for anything that is a true pill at rest, the toggle switch's track and the experimental badge. Borders are hairline (1px, 6-12% opacity depending on theme) and never colored; a border's only job is to separate a surface from the Mica ground behind it, matching Fluent's own restraint. One micro-exception: the active tab's 2px-thick underline bar rounds its own two end-pixels (`border-radius: 2px`), which is the bar's thickness, not a shape decision, so it is not promoted to a scale token.

## Components

### Buttons
- **Shape:** 6px radius, 1px border, 7px/14px padding.
- **Order:** Windows' commit order, right-aligned: the action first, then Cancel or Close, with Apply last of all. So Settings reads Close · Apply, the confirm dialogs read Remove · Cancel, and a board's settings put Remove board alone on the left.
- **Primary** (Apply, Continue, Next, Finish, Save): accent background, white text; disabled state drops to the neutral control-border color so it reads as inert rather than a dimmed accent.
- **Secondary** (Close, Cancel, Skip, Start again, Record Shortcut, Calibrate): control-surface background with a hairline border; hover shifts to the card-hover tint.
- **Icon** (+/- on profiles and controls, the gear, the arrows that move a drawn control): 30x30px square, same radius and border language as a secondary button.
- **Link** (Website, Made by, Inspired by): accent text, no border, underline on hover/focus only.

### Cards / Containers (SettingsCard)
- **Corner Style:** 8px.
- **Background:** Card Surface, 1px border.
- **Shadow Strategy:** none (see Elevation & Depth); the border alone separates it from the Mica ground.
- **Internal rows:** 12px/16px padding, 44px min height, a 1px divider between rows and none after the last.

### Inputs / Fields
- **Style:** Control Surface background, 1px border, 6px radius, 7px/10px padding.
- **Focus:** the system-wide 2px accent outline, offset 2px, replacing the border-only hover state.
- **Select:** a custom two-triangle chevron instead of the browser default, kept in the same neutral ink as secondary text.

### Toggle Switch
- A 40x20px pill, outlined in secondary text when off with a dot thumb; fills accent and slides the thumb to accent-text when on. 150ms ease on both the fill and the thumb's transform.

### Checkbox (job picker)
- 16x16px, 3px radius, same hairline border as inputs; checked state fills accent and draws a white checkmark via `clip-path`, scaled in over 100ms.

### Flyout (job picker)
- **Style:** Card Surface, a borderless popup window of its own (`jobs.html`) that Windows rounds to 8px and shadows like a menu.
- **Behavior:** it opens beside the knob's card the way a submenu does: to the right, or to the left when the screen has no room there, its top level with the knob's row and moved up as far as the screen needs. It is up to 490px tall and 280px wide, past the Settings window's own edge; a longer list scrolls. It is made once, hidden, when Settings loads, and only hides between opens, so it opens at once. Section headers (Volume, Brightness, ...) group its checklist exactly as Go's catalog orders them, with Clear first and Other... last. Every tick goes straight back to Settings; Escape, a click anywhere else, or a second click on the same knob closes it.

### Board row (General)
- A row in the Boards card: the on/off toggle at its left, the board's name, its status (Connected in Success, Disconnected or Off in secondary text) and an icon-button gear at its right that opens the board's settings.

### Drawn board (Boards tab, Draw)
- An SMC-Mixer is drawn as the device itself; a DIY or MIDI board as rows of knobs, faders and buttons in the places the user put them. A drawn knob turns with the level it sets, clockwise from 0% to 100%. The control picked is outlined in the accent, and one being moved or pressed takes the selection tint for a moment. An inspector card edits the control picked: its kind, finding its input, four arrows laid out as a keyboard's (up above left, down and right), Clear and remove.

### List (Boards tab, List)
- Three cards side by side, Knobs, Faders and Buttons, one row per control with its jobs or actions, ten rows at most before the card scrolls; a row takes the selection tint while its control moves or is pressed.

### Calibration dialog
- One control at a time: the step count, the instruction in subtitle type, a level bar in the accent that follows the control live, a sweep or press counter, and the warning line. Start again and Skip on the left, Cancel on the right, and Finish beside it once every control is found.

### Tabs (Settings' Pivot)
- Underlined style: unselected tabs sit in secondary text; the selected tab goes to primary text, 600 weight, with a 2px accent underline inset 4px from each edge. The Boards tab has a second row of tabs, one per board, and a segmented Draw | List switch.

### Dialog (confirm sheets, a board's settings, Add board, calibration)
- Centered over the Scrim color (`rgba(0, 0, 0, 0.35)`, the one color in the system not tied to light/dark since a dimming layer reads the same over either), Card Surface, the system's one shadow, title (subtitle type) + body (secondary text) + right-aligned Cancel/destructive-action button pair.

## Do's and Don'ts

### Do:
- **Do** source every visible string from `t(key, vars)` against the catalog Go sends; a hardcoded string anywhere in these pages is a defect, not a shortcut.
- **Do** keep the accent reserved for primary actions, selection and on-states (The One Signal Rule).
- **Do** give every interactive element the 2px accent focus rect; these windows are expected to be fully keyboard-operable.
- **Do** keep cards flat with a hairline border; reach for the flyout shadow only on a surface that truly floats above the page.

### Don't:
- **Don't** introduce a second typeface or a display face for any heading; Segoe UI Variable carries the whole hierarchy.
- **Don't** add a drop shadow to a card, row, or button; that visual weight is reserved for the flyout and dialogs.
- **Don't** hardcode a light or dark color; every token has both a `prefers-color-scheme` value and a `data-theme` override for the live `theme` message, and a new color needs both.
- **Don't** use em or en dashes in any string shown in these windows, per the project's house style.

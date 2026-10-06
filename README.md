# Zephyr — soft drifting signal

Wi-Fi list + connect, band switch, QR share with password, live up/down
speeds, Wi-Fi restart. Pink Cat Boo, zero idle RAM (exits on quit).

## Run
`zephyr` (Go binary; Waybar network click opens it floating).

## Build
`go build -o zephyr .` (Bubble Tea/Lipgloss, see `go.mod`),
or straight from the network with no clone:

```bash
go install github.com/sherqo/zephyr@latest
```

## Install (Arch Linux)

Runtime deps (the backend shells out to them):

```bash
sudo pacman -S --needed networkmanager curl iputils
```

A Nerd Font is recommended for the wifi/lock glyphs, but not required —
without one Zephyr falls back to plain ASCII marks automatically
(`ZEPHYR_ASCII=1` forces the fallback).

The backend ships inside the binary: on first run Zephyr extracts its
helpers to `~/.local/share/zephyr` (or `$XDG_DATA_HOME/zephyr`) and
re-syncs them whenever they change. No Makefile, no manual copying.

## Layout
- `zephyr.go` — the TUI.
- `bin/omarchy-*` — network backend vendored from Omarchy (see `ATTRIBUTION.md`).
- `waybar/` — module snippet + Hyprland float rule.

## Controls
`↑↓/jk` move · `enter` select · `f` forget saved network · `h/l` move
between band pills · `r` refresh · `q/esc` quit.
Type a password inline when joining a new network; any key leaves QR view.

## License
MIT — see `LICENSE`. Omarchy-derived files keep their original terms;
see `ATTRIBUTION.md` and `LICENSE.omarchy`.

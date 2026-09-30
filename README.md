# Zephyr — soft drifting signal

Wi-Fi list + connect, DNS provider switch, QR share, speed test, Wi-Fi restart.
Pink Cat Boo, CaskaydiaMono Nerd Font, zero idle RAM (exits on quit).

## Run
`zephyr` (Go binary; Waybar network click opens it floating).

## Build
`go build -o bin/zephyr zephyr.go` (stdlib only, no modules).

## Layout
- `zephyr.go` — the TUI (self-prepends its `bin/` to PATH for helpers).
- `bin/` — backend: all Omarchy network helpers (`omarchy-network-*`,
  `omarchy-dns`, `omarchy-restart-wifi`, `omarchy-disk-speedtest`,
  `omarchy-cmd-present`).
- `waybar/` — module snippet + Hyprland float rule.

## Controls
`↑↓/jk` move · `enter` connect/select · `r` refresh · `q/esc` quit.
Type a password inline when joining a new network; any key leaves QR view.

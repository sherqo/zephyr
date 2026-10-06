// Zephyr — soft drifting signal (Bubble Tea TUI).
// Faithful port of Omarchy's network panel: hero with QR/power actions,
// live throughput, WI-FI BAND pills, KNOWN/OTHER networks.
// Backend is Omarchy's network scripts, byte-identical (see bin/); wifi
// list/connect goes through nmcli exactly like the panel does.
// Keys: up/down or j/k move · enter select · f forget · r refresh · q quit.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Pink Cat Boo
var (
	cBg      = lipgloss.Color("#202330")
	cFg      = lipgloss.Color("#FFF0F5")
	cAccent  = lipgloss.Color("#FF4C7A")
	cMuted   = lipgloss.Color("#565970")
	cDim     = lipgloss.Color("#8A8DA3")
	cGreen   = lipgloss.Color("#3BC089")
	cBlue    = lipgloss.Color("#6767CE")
	cYellow  = lipgloss.Color("#FEC831")
	titleSt  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	nameSt   = lipgloss.NewStyle().Foreground(cFg)
	dimSt    = lipgloss.NewStyle().Foreground(cMuted)
	selSt    = lipgloss.NewStyle().Foreground(cFg).Background(lipgloss.Color("#3A3048")).Bold(true)
	headSt   = lipgloss.NewStyle().Foreground(cFg).Bold(true)
	boxSt    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cMuted).Padding(1, 2).Background(cBg)
	helpSt   = lipgloss.NewStyle().Foreground(cMuted)
	activeNm = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	pillOn   = lipgloss.NewStyle().Foreground(cBg).Background(cAccent).Bold(true).Padding(0, 1)
	pillOff  = lipgloss.NewStyle().Foreground(cDim).Padding(0, 1)
)

//go:embed bin/omarchy-*
var backendFS embed.FS

func backendRoot() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "zephyr")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "zephyr")
}

func ensureBackend() string {
	root := backendRoot()
	binDir := filepath.Join(root, "bin")
	_ = os.MkdirAll(binDir, 0o755)
	_ = fs.WalkDir(backendFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == "." {
			return nil
		}
		data, err := backendFS.ReadFile(path)
		if err != nil {
			return nil
		}
		dst := filepath.Join(root, path)
		if cur, err := os.ReadFile(dst); err == nil && string(cur) == string(data) {
			return nil
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, data, 0o755)
		return nil
	})
	return binDir
}

func hasNerdFont() bool {
	out, err := exec.Command("fc-list", ":", "family").Output()
	if err != nil {
		return true
	}
	return strings.Contains(strings.ToLower(string(out)), "nerd")
}

var useASCII = false

type Network struct {
	SSID     string
	Signal   int
	Security string
	Active   bool
	Known    bool
}

type Sel struct {
	Kind string // header-qr, header-power, band, dns, net
	Ref  string // band value / dns name / ssid
}

// Selectable order: header actions, band row, net rows.
type State struct {
	Type     string
	SSID     string
	Signal   int
	Freq     string
	Bitrate  string
	Down     float64
	Up       float64
	RouterMs string
	NetMs    string
	DNS      string
	Band     string // current live band label: 2.4/5/6/""
	Selected string // pinned band: auto/2.4/5/6
	Bands    []string
	Nets     []Network
	iface    string
	rx       float64
	tx       float64
	sampleT  float64
	QR       []string
	QROpen   bool
	Password string
	Prompt   string
	Input    string
	Message  string
	WifiOn   bool
}

func sh(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	return string(out)
}

func parseKV(raw string) map[string]string {
	next := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		idx := strings.Index(line, "\t")
		if idx == -1 {
			continue
		}
		next[line[:idx]] = strings.TrimSpace(line[idx+1:])
	}
	return next
}

func knownProfiles() map[string]bool {
	known := map[string]bool{}
	for _, line := range strings.Split(sh("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show"), "\n") {
		p := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(p) == 2 && strings.Contains(p[1], "wireless") {
			known[p[0]] = true
		}
	}
	return known
}

func sysfsBytes(iface, dir string) float64 {
	data, err := os.ReadFile("/sys/class/net/" + iface + "/statistics/" + dir + "_bytes")
	if err != nil {
		return -1
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return -1
	}
	return v
}

// snapshotFast refreshes live numbers without a wifi scan: plain status,
// sysfs byte counters, band script. Lists/pings ride from prev.
func snapshotFast(prev *State) State {
	st := *prev
	f := strings.Fields(sh("omarchy-network-status"))
	if len(f) >= 4 {
		st.Type, st.SSID = f[0], f[1]
		st.Signal, _ = strconv.Atoi(f[2])
		st.Freq = f[3]
	}
	iface := st.iface
	if iface == "" {
		for _, line := range strings.Split(sh("nmcli", "-t", "-f", "DEVICE,TYPE", "dev"), "\n") {
			p := strings.SplitN(strings.TrimSpace(line), ":", 2)
			if len(p) == 2 && p[1] == "wifi" {
				iface = p[0]
				break
			}
		}
		st.iface = iface
	}
	if iface != "" {
		rx, tx := sysfsBytes(iface, "rx"), sysfsBytes(iface, "tx")
		now := float64(time.Now().UnixNano()) / 1e9
		if rx >= 0 && tx >= 0 && st.sampleT > 0 {
			if dt := now - st.sampleT; dt > 0 {
				st.Down = max(0, (rx-st.rx)/dt)
				st.Up = max(0, (tx-st.tx)/dt)
			}
		}
		if rx >= 0 && tx >= 0 {
			st.rx, st.tx, st.sampleT = rx, tx, now
		}
	}
	band := parseKV(sh("omarchy-network-band"))
	if s := band["selected"]; s != "" {
		st.Selected = s
	}
	if b := band["band"]; b != "" {
		st.Band = b
	}
	return st
}

func snapshot(prev *State) State {
	var st State
	if prev != nil {
		st.QR, st.QROpen = prev.QR, prev.QROpen
		st.Prompt, st.Input = prev.Prompt, prev.Input
		st.Message = prev.Message
	}
	kv := parseKV(sh("omarchy-network-status", "--verbose"))
	st.Type = kv["type"]
	iface := kv["iface"]
	// details line (non-verbose contract): kind, ssid, signal, freq
	f := strings.Fields(sh("omarchy-network-status"))
	if len(f) >= 4 {
		st.Type, st.SSID = f[0], f[1]
		st.Signal, _ = strconv.Atoi(f[2])
		st.Freq = f[3]
	}
	st.Bitrate = kv["bitrate"]
	st.RouterMs = kv["router_ping_ms"]
	st.NetMs = kv["internet_ping_ms"]
	if prev != nil {
		st.Password = prev.Password
		st.QR, st.QROpen = prev.QR, prev.QROpen
	}
	band := parseKV(sh("omarchy-network-band"))
	st.Selected = band["selected"]
	if st.Selected == "" {
		st.Selected = "auto"
	}
	avail := strings.Fields(band["available"])
	st.Bands = []string{"auto"}
	st.Bands = append(st.Bands, avail...)
	st.Band = band["band"]
	// throughput from byte-counter deltas (ports throughputState)
	rx, _ := strconv.ParseFloat(kv["rx_bytes"], 64)
	tx, _ := strconv.ParseFloat(kv["tx_bytes"], 64)
	now := float64(time.Now().UnixNano()) / 1e9
	if prev != nil && prev.iface == iface && prev.sampleT > 0 && iface != "" {
		if dt := now - prev.sampleT; dt > 0 {
			st.Down = max(0, (rx-prev.rx)/dt)
			st.Up = max(0, (tx-prev.tx)/dt)
		}
	}
	st.rx, st.tx, st.sampleT, st.iface = rx, tx, now, iface

	known := knownProfiles()
	ifaceName := ""
	for _, line := range strings.Split(sh("nmcli", "-t", "-f", "DEVICE,TYPE", "dev"), "\n") {
		p := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(p) == 2 && p[1] == "wifi" {
			ifaceName = p[0]
			break
		}
	}
	args := []string{"-t", "-f", "SSID,SIGNAL,SECURITY,IN-USE", "dev", "wifi", "list"}
	if ifaceName != "" {
		args = append(args, "ifname", ifaceName)
	}
	type rawNet struct {
		ssid, sec string
		sig       int
		active    bool
	}
	seen := map[string]int{}
	for _, line := range strings.Split(sh(append([]string{"nmcli"}, args...)...), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := strings.Split(line, ":")
		if len(p) < 4 {
			continue
		}
		sec, inuse := p[len(p)-2], p[len(p)-1]
		sig, _ := strconv.Atoi(p[len(p)-3])
		ssid := strings.ReplaceAll(strings.Join(p[:len(p)-3], ":"), "\\:", ":")
		if ssid == "" {
			continue
		}
		if i, ok := seen[ssid]; ok {
			if sig > st.Nets[i].Signal {
				st.Nets[i].Signal = sig
				st.Nets[i].Security = sec
			}
			if inuse == "*" {
				st.Nets[i].Active = true
			}
			continue
		}
		seen[ssid] = len(st.Nets)
		st.Nets = append(st.Nets, Network{ssid, sig, sec, inuse == "*", known[ssid]})
		_ = rawNet{}
	}
	// sort: connected, known, signal (ports sortWifiRows)
	for i := 0; i < len(st.Nets); i++ {
		for j := i + 1; j < len(st.Nets); j++ {
			a, b := st.Nets[i], st.Nets[j]
			swap := false
			if a.Active != b.Active {
				swap = !a.Active
			} else if a.Known != b.Known {
				swap = !a.Known
			} else if b.Signal > a.Signal {
				swap = true
			}
			if swap {
				st.Nets[i], st.Nets[j] = st.Nets[j], st.Nets[i]
			}
		}
	}
	st.WifiOn = true
	if out := strings.TrimSpace(sh("nmcli", "radio", "wifi")); out == "disabled" {
		st.WifiOn = false
	}
	return st
}

func formatRate(bps float64) string {
	if bps < 0 {
		bps = 0
	}
	switch {
	case bps < 1024:
		return fmt.Sprintf("%d B/s", int(bps+0.5))
	case bps < 1024*1024:
		return fmt.Sprintf("%.1f KB/s", bps/1024)
	case bps < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB/s", bps/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB/s", bps/(1024*1024*1024))
	}
}

func formatMs(raw string) string {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 {
		return "--"
	}
	if v > 0 && v < 10 {
		return fmt.Sprintf("%.1f ms", v)
	}
	return fmt.Sprintf("%.0f ms", v)
}

func formatFreq(mhz string) string {
	v, err := strconv.ParseFloat(strings.TrimSpace(mhz), 64)
	if err != nil || v == 0 {
		return ""
	}
	switch {
	case v >= 2400 && v < 2500:
		return "2.4ghz"
	case v >= 4900 && v < 5925:
		return "5ghz"
	case v >= 5925 && v < 7125:
		return "6ghz"
	}
	ghz := v / 1000
	return fmt.Sprintf("%.1fghz", ghz)
}

func sigBar(s, w int) string {
	if s < 0 {
		s = 0
	}
	if s > 100 {
		s = 100
	}
	n := s * w / 100
	fg := cGreen
	if s < 35 {
		fg = cYellow
	}
	if s < 15 {
		fg = cAccent
	}
	f := lipgloss.NewStyle().Foreground(fg).Render(strings.Repeat("━", n))
	e := lipgloss.NewStyle().Foreground(cMuted).Render(strings.Repeat("━", w-n))
	return f + e
}

func shortName(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func wifiGlyph(active bool) string {
	g := ""
	if useASCII {
		g = "[~]"
	}
	if active {
		return lipgloss.NewStyle().Foreground(cAccent).Render(g)
	}
	return lipgloss.NewStyle().Foreground(cDim).Render(g)
}

func lockGlyph() string {
	if useASCII {
		return "#"
	}
	return ""
}

// layoutWidths derives box and name widths from the terminal width.
func layoutWidths(termW int) (boxW, nameW int) {
	boxW = termW - 2
	if termW <= 0 {
		boxW = 70
	}
	if boxW < 48 {
		boxW = 48
	}
	if boxW > 100 {
		boxW = 100
	}
	nameW = boxW - 2 - 4 - 28
	if nameW < 14 {
		nameW = 14
	}
	if nameW > 28 {
		nameW = 28
	}
	return boxW, nameW
}

type model struct {
	st         State
	cursor     int
	bandCursor int
	width      int
	height     int
	busy       bool
	flash      string
	scanTick   int
}

type refreshMsg State
type doneMsg string

func doSnapshot(prev *State, full bool) tea.Cmd {
	p := *prev
	return func() tea.Msg {
		if full {
			return refreshMsg(snapshot(&p))
		}
		return refreshMsg(snapshotFast(&p))
	}
}

type tickMsg struct{}

func tickRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Init() tea.Cmd {
	// paint instantly with empty state; first snapshot lands async
	return tea.Batch(tickRefresh(), doSnapshot(&State{}, true))
}

func selsOf(st *State) []Sel {
	var out []Sel
	out = append(out, Sel{"header-qr", ""}, Sel{"header-power", ""})
	out = append(out, Sel{"band", ""})
	for _, n := range st.Nets {
		out = append(out, Sel{"net", n.SSID})
	}
	return out
}

func clampCursor(m *model, n int) {
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
}

func availH(termH int) int {
	if termH <= 0 {
		return 1 << 30
	}
	h := termH - 6 - 2
	if h < 3 {
		h = 3
	}
	return h
}

func windowStart(total, cursorPos, maxH int) int {
	if total <= maxH {
		return 0
	}
	s := cursorPos - maxH/2
	if s < 0 {
		s = 0
	}
	if s > total-maxH {
		s = total - maxH
	}
	return s
}

func bandLabel(b string) string {
	if b == "auto" {
		return "Auto"
	}
	if b == "" {
		return ""
	}
	return b + "ghz"
}

var qrStore []string
var qrPassword string

func selectRow(m *model, s Sel) tea.Cmd {
	switch s.Kind {
	case "header-qr":
		if m.st.QROpen {
			m.st.QROpen = false
			m.st.QR = nil
			return nil
		}
		return func() tea.Msg {
			out := sh("omarchy-network-qr")
			var m_ []string
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				ok := true
				for _, ch := range line {
					if ch != '0' && ch != '1' {
						ok = false
						break
					}
				}
				if ok {
					m_ = append(m_, line)
				}
			}
			if len(m_) == 0 {
				return doneMsg("no QR available")
			}
			qrStore = m_
			ssid := strings.TrimSpace(sh("nmcli", "-t", "-f", "NAME,TYPE,DEVICE", "connection", "show", "--active"))
			pw := ""
			for _, line := range strings.Split(ssid, "\n") {
				p := strings.SplitN(strings.TrimSpace(line), ":", 3)
				if len(p) == 3 && strings.Contains(p[1], "wireless") {
					pw = strings.TrimSpace(sh("nmcli", "-s", "-g", "802-11-wireless-security.psk", "connection", "show", "id", p[0]))
					break
				}
			}
			qrPassword = pw
			return doneMsg("QRREADY")
		}
	case "header-power":
		m.busy = true
		if m.st.WifiOn {
			m.flash = "disabling…"
		} else {
			m.flash = "enabling…"
		}
		on := m.st.WifiOn
		return func() tea.Msg {
			if on {
				sh("nmcli", "radio", "wifi", "off")
				return doneMsg("Wi-Fi off")
			}
			sh("nmcli", "radio", "wifi", "on")
			return doneMsg("Wi-Fi on")
		}
	case "band":
		if len(m.st.Bands) == 0 {
			return nil
		}
		if m.bandCursor < 0 || m.bandCursor >= len(m.st.Bands) {
			m.bandCursor = 0
		}
		target := m.st.Bands[m.bandCursor]
		m.busy = true
		m.flash = "pinning band " + bandLabel(target) + "…"
		return func() tea.Msg {
			out := strings.TrimSpace(sh("omarchy-network-band", target))
			if out == "" {
				return doneMsg("band → " + bandLabel(target))
			}
			return doneMsg(shortName(out, 60))
		}
	case "net":
		ssid := s.Ref
		var active, known bool
		for _, n := range m.st.Nets {
			if n.SSID == ssid {
				active, known = n.Active, n.Known
			}
		}
		if active {
			m.flash = "already on " + ssid
			return nil
		}
		_ = known
		m.busy = true
		m.flash = "connecting to " + ssid + "…"
		return func() tea.Msg {
			out := sh("nmcli", "dev", "wifi", "connect", ssid)
			if strings.Contains(out, "successfully") {
				return doneMsg("connected to " + ssid)
			}
			return doneMsg("CONNECTFAIL:" + ssid + ":" + strings.TrimSpace(out))
		}
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.scanTick++
		full := m.scanTick%8 == 0
		if m.scanTick >= 10 {
			m.scanTick = 0
			go sh("nmcli", "dev", "wifi", "rescan")
		}
		prev := m.st
		return m, func() tea.Msg {
			if full {
				return refreshMsg(snapshot(&prev))
			}
			return refreshMsg(snapshotFast(&prev))
		}
	case refreshMsg:
		incoming := State(msg)
		incoming.QR, incoming.QROpen = m.st.QR, m.st.QROpen
		incoming.Password = m.st.Password
		incoming.Prompt, incoming.Input = m.st.Prompt, m.st.Input
		m.st = incoming
		if m.bandCursor >= len(m.st.Bands) {
			m.bandCursor = 0
		}
		clampCursor(&m, len(selsOf(&m.st)))
		return m, tickRefresh()
	case doneMsg:
		m.busy = false
		s := string(msg)
		switch {
		case s == "QRREADY":
			m.st.QR = qrStore
			m.st.Password = qrPassword
			m.st.QROpen = len(qrStore) > 0
			m.flash = ""
			if !m.st.QROpen {
				m.flash = "no QR available"
			}
		case strings.HasPrefix(s, "CONNECTFAIL:"):
			rest := strings.TrimPrefix(s, "CONNECTFAIL:")
			parts := strings.SplitN(rest, ":", 2)
			out := ""
			if len(parts) == 2 {
				out = parts[1]
			}
			if strings.Contains(out, "Secrets were required") || strings.Contains(out, "password") {
				m.st.Prompt = parts[0]
				m.st.Input = ""
				m.flash = ""
			} else {
				m.flash = "failed: " + shortName(strings.TrimSpace(out), 50)
			}
		default:
			m.flash = shortName(s, 60)
		}
		return m, func() tea.Msg { return refreshMsg(snapshot(&m.st)) }
	case tea.MouseMsg:
		ss := selsOf(&m.st)
		hit := func() int { return rowAtY(&m, ss, m.width, m.height, m.cursor, msg.Y) }
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if i := hit(); i >= 0 {
				m.cursor = i
			} else if m.cursor > 0 {
				m.cursor--
			}
		case tea.MouseButtonWheelDown:
			if i := hit(); i >= 0 {
				m.cursor = i
			} else if m.cursor < len(ss)-1 {
				m.cursor++
			}
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress {
				if i := hit(); i >= 0 {
					m.cursor = i
					if !m.busy {
						return m, selectRow(&m, ss[i])
					}
				}
			}
		case tea.MouseButtonRight:
			if msg.Action == tea.MouseActionPress {
				m.st.Prompt, m.st.Input, m.st.QROpen, m.st.QR = "", "", false, nil
			}
		}
		return m, nil
	case tea.KeyMsg:
		if m.st.QROpen {
			m.st.QROpen = false
			m.st.QR = nil
			return m, nil
		}
		if m.st.Prompt != "" {
			switch msg.String() {
			case "esc":
				m.st.Prompt, m.st.Input = "", ""
			case "backspace":
				if len(m.st.Input) > 0 {
					m.st.Input = m.st.Input[:len(m.st.Input)-1]
				}
			case "enter":
				pw, ssid := m.st.Input, m.st.Prompt
				m.st.Prompt, m.st.Input = "", ""
				if m.busy {
					return m, nil
				}
				m.busy = true
				m.flash = "connecting to " + ssid + "…"
				return m, func() tea.Msg {
					out := sh("nmcli", "dev", "wifi", "connect", ssid, "password", pw)
					if strings.Contains(out, "successfully") {
						return doneMsg("connected to " + ssid)
					}
					return doneMsg("CONNECTFAIL:" + ssid + ":" + strings.TrimSpace(out))
				}
			default:
				if s := msg.String(); len(s) == 1 {
					m.st.Input += s
				}
			}
			return m, nil
		}
		ss := selsOf(&m.st)
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.flash = ""
			return m, func() tea.Msg { return refreshMsg(snapshot(&m.st)) }
		case "f":
			if m.cursor < len(ss) {
				r := ss[m.cursor]
				if r.Kind == "net" {
					for _, n := range m.st.Nets {
						if n.SSID == r.Ref && n.Known && !n.Active {
							m.flash = "forgetting " + r.Ref + "…"
							ref := r.Ref
							return m, func() tea.Msg {
								sh("nmcli", "connection", "delete", "id", ref)
								return doneMsg("forgot " + ref)
							}
						}
					}
					m.flash = "only saved networks can be forgotten"
				}
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(ss)-1 {
				m.cursor++
			}
		case "left", "h":
			if m.cursor < len(ss) && ss[m.cursor].Kind == "band" && len(m.st.Bands) > 0 {
				m.bandCursor--
				if m.bandCursor < 0 {
					m.bandCursor = len(m.st.Bands) - 1
				}
			}
		case "right", "l":
			if m.cursor < len(ss) && ss[m.cursor].Kind == "band" && len(m.st.Bands) > 0 {
				m.bandCursor++
				if m.bandCursor >= len(m.st.Bands) {
					m.bandCursor = 0
				}
			}
		case "enter":
			if m.cursor < len(ss) && !m.busy {
				return m, selectRow(&m, ss[m.cursor])
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.st.Prompt != "" {
		var b strings.Builder
		b.WriteString(titleSt.Render(" Password ") + " " + helpSt.Render("enter join · esc cancel") + "\n\n")
		b.WriteString(nameSt.Render("  "+m.st.Prompt) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(cFg).Render("  > "+strings.Repeat("•", len([]rune(m.st.Input)))) + "_\n")
		return boxSt.Render(b.String())
	}
	boxW, nameW := layoutWidths(m.width)
	var b strings.Builder
	b.WriteString(titleSt.Render(" Zephyr ") + "\n")
	flash := " "
	if m.flash != "" {
		flash = "  " + lipgloss.NewStyle().Foreground(cBlue).Render(m.flash)
	} else if m.busy {
		flash = "  " + lipgloss.NewStyle().Foreground(cYellow).Render("working…")
	}
	b.WriteString(flash + "\n\n")
	stats := fmt.Sprintf("  ↓%s ↑%s", formatRate(m.st.Down), formatRate(m.st.Up))
	extra := ""
	if m.st.Bitrate != "" {
		extra += " · " + m.st.Bitrate
	}
	if m.st.RouterMs != "" {
		extra += " · " + formatMs(m.st.RouterMs)
	}
	if m.st.NetMs != "" {
		extra += " · " + formatMs(m.st.NetMs)
	}
	b.WriteString(dimSt.Render(stats+extra) + "\n")
	// ---- one flat list drives render AND mouse mapping ----
	ss := selsOf(&m.st)
	vl := buildView(&m, ss, nameW)
	cpos := 0
	for pos, v := range vl {
		if v.row == m.cursor {
			cpos = pos
			break
		}
	}
	maxH := availH(m.height)
	start := windowStart(len(vl), cpos, maxH)
	end := start + maxH
	if end > len(vl) {
		end = len(vl)
	}
	for _, v := range vl[start:end] {
		if v.row < 0 {
			b.WriteString(v.text + "\n")
			continue
		}
		if v.row == m.cursor {
			b.WriteString(selSt.Render("▸ "+v.text) + "\n")
		} else {
			b.WriteString(dimSt.Render("  ") + v.text + "\n")
		}
	}
	return boxSt.Width(boxW).Render(b.String())
}

func main() {
	binDir := ensureBackend()
	paths := []string{binDir}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Dir(exe))
	}
	os.Setenv("PATH", strings.Join(paths, ":")+":"+os.Getenv("PATH"))
	useASCII = !hasNerdFont() || os.Getenv("ZEPHYR_ASCII") == "1"
	if len(os.Args) > 1 && os.Args[1] == "--dump" {
		st := snapshot(nil)
		fmt.Printf("type=%s ssid=%s signal=%d freq=%s dns=%s nets=%d down=%.0f up=%.0f\n", st.Type, st.SSID, st.Signal, st.Freq, st.DNS, len(st.Nets), st.Down, st.Up)
		for _, n := range st.Nets {
			fmt.Printf("net\t%s\t%d\t%s\tactive=%v known=%v\n", n.SSID, n.Signal, n.Security, n.Active, n.Known)
		}
		return
	}
	prev := State{}
	m := model{st: prev, cursor: 2}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "zephyr:", err)
		os.Exit(1)
	}
}

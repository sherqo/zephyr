// Zephyr — soft drifting signal (Bubble Tea TUI).
// Wi-Fi list + connect, DNS switch, QR share, speedtest, restart.
// Backend is Omarchy's network scripts, byte-identical (see bin/).
// Keys: up/down or j/k move · enter select · r refresh · q/esc quit.
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
	activeNm = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	boxSt    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cMuted).Padding(1, 2).Background(cBg)
	helpSt   = lipgloss.NewStyle().Foreground(cMuted)
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
}

type Row struct {
	Kind   string // NET, ACTION
	Text   string
	SSID   string
	Action string // connect, dns, qr, speed, restart
}

type State struct {
	Type    string
	SSID    string
	Signal  int
	Freq    string
	DNS     string
	Speed   string
	Testing bool
	Nets    []Network
	QR      []string
	QRShow  bool
	Prompt  string
	Input   string
	Message string
}

func sh(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	return string(out)
}

func snapshot() State {
	var st State
	f := strings.Fields(sh("omarchy-network-status"))
	if len(f) >= 4 {
		st.Type, st.SSID = f[0], f[1]
		st.Signal, _ = strconv.Atoi(f[2])
		st.Freq = f[3]
	} else if len(f) > 0 {
		st.Type = f[0]
	}
	st.DNS = strings.TrimSpace(sh("omarchy-dns"))
	iface := ""
	for _, line := range strings.Split(sh("nmcli", "-t", "-f", "DEVICE,TYPE", "dev"), "\n") {
		p := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(p) == 2 && p[1] == "wifi" {
			iface = p[0]
			break
		}
	}
	args := []string{"-t", "-f", "SSID,SIGNAL,SECURITY,IN-USE", "dev", "wifi", "list"}
	if iface != "" {
		args = append(args, "ifname", iface)
	}
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
		dup := false
		for i, n := range st.Nets {
			if n.SSID == ssid {
				if sig > st.Nets[i].Signal {
					st.Nets[i].Signal = sig
					st.Nets[i].Security = sec
				}
				if inuse == "*" {
					st.Nets[i].Active = true
				}
				dup = true
				break
			}
		}
		if !dup {
			st.Nets = append(st.Nets, Network{ssid, sig, sec, inuse == "*"})
		}
	}
	return st
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
	nameW = boxW - 2 - 4 - 30
	if nameW < 14 {
		nameW = 14
	}
	if nameW > 28 {
		nameW = 28
	}
	return boxW, nameW
}

type model struct {
	st     State
	cursor int
	width  int
	height int
	busy   bool
	flash  string
}

type refreshMsg State
type doneMsg string
type speedMsg string

func doSnapshot() tea.Msg { return refreshMsg(snapshot()) }

func tickRefresh() tea.Cmd {
	return tea.Tick(8*time.Second, func(t time.Time) tea.Msg { return doSnapshot() })
}

func (m model) Init() tea.Cmd { return tickRefresh() }

func rowsOf(st *State) []Row {
	var rows []Row
	for _, n := range st.Nets {
		rows = append(rows, Row{"NET", n.SSID, n.SSID, "connect"})
	}
	rows = append(rows, Row{"ACTION", "DNS switch (now " + st.DNS + ")", "", "dns"})
	rows = append(rows, Row{"ACTION", "Share Wi-Fi via QR code", "", "qr"})
	speedText := "Speed test" + st.Speed
	if st.Testing {
		speedText = "Speed test (testing…)"
	}
	rows = append(rows, Row{"ACTION", speedText, "", "speed"})
	rows = append(rows, Row{"ACTION", "Restart Wi-Fi", "", "restart"})
	return rows
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

// contentLines mirrors View's windowed area: NET rows then 4 ACTION rows.
// Returns data-row indices; header lines are rendered inline as kinds change.
func contentLines(nets int) []int {
	var out []int
	for i := 0; i < nets; i++ {
		out = append(out, i)
	}
	base := nets
	for i := 0; i < 4; i++ {
		out = append(out, base+i)
	}
	return out
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

// rowAtY maps a terminal line to a rowsOf index. Header occupies lines
// 2..5 (title, hint, flash, blank), content starts at 6.
func rowAtY(rs []Row, nets, termH, cursor, y int) int {
	lines := contentLines(nets)
	maxH := availH(termH)
	start := windowStart(len(lines), cursor, maxH)
	pos := start + (y - 6)
	if pos < start || pos >= start+maxH || pos < 0 || pos >= len(lines) {
		return -1
	}
	return lines[pos]
}

var dnsOrder = []string{"DHCP", "Cloudflare", "Google"}
var qrStore []string

func selectRow(m *model, r Row) tea.Cmd {
	switch r.Action {
	case "connect":
		ssid := r.SSID
		m.busy = true
		m.flash = "connecting to " + ssid + "…"
		return func() tea.Msg {
			out := sh("nmcli", "dev", "wifi", "connect", ssid)
			if strings.Contains(out, "successfully") {
				return doneMsg("connected to " + ssid)
			}
			return doneMsg("CONNECTFAIL:" + ssid + ":" + strings.TrimSpace(out))
		}
	case "dns":
		next := dnsOrder[0]
		for i, d := range dnsOrder {
			if d == m.st.DNS && i+1 < len(dnsOrder) {
				next = dnsOrder[i+1]
			}
		}
		m.busy = true
		m.flash = "switching DNS to " + next + "…"
		return func() tea.Msg {
			return doneMsg(strings.TrimSpace(sh("omarchy-dns", next)))
		}
	case "qr":
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
			return doneMsg("QRREADY")
		}
	case "speed":
		m.busy = true
		m.st.Testing = true
		m.flash = ""
		return func() tea.Msg {
			out := sh("omarchy-network-speedtest", "down")
			return speedMsg(strings.TrimSpace(out))
		}
	case "restart":
		m.busy = true
		m.flash = "restarting Wi-Fi…"
		return func() tea.Msg {
			return doneMsg(strings.TrimSpace(sh("omarchy-restart-wifi")))
		}
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case refreshMsg:
		incoming := State(msg)
		incoming.QR, incoming.QRShow = m.st.QR, m.st.QRShow
		incoming.Prompt, incoming.Input = m.st.Prompt, m.st.Input
		incoming.Speed, incoming.Testing = m.st.Speed, m.st.Testing
		m.st = incoming
		clampCursor(&m, len(rowsOf(&m.st)))
		return m, tickRefresh()
	case doneMsg:
		m.busy = false
		s := string(msg)
		switch {
		case s == "QRREADY":
			m.st.QR = qrStore
			m.st.QRShow = len(qrStore) > 0
			if !m.st.QRShow {
				m.flash = "no QR available"
			} else {
				m.flash = ""
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
		return m, func() tea.Msg { return doSnapshot() }
	case speedMsg:
		m.busy = false
		m.st.Testing = false
		m.st.Speed = " — " + shortName(string(msg), 40)
		return m, nil
	case tea.MouseMsg:
		rs := rowsOf(&m.st)
		nets := len(m.st.Nets)
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if i := rowAtY(rs, nets, m.height, m.cursor, msg.Y); i >= 0 {
				m.cursor = i
			} else if m.cursor > 0 {
				m.cursor--
			}
		case tea.MouseButtonWheelDown:
			if i := rowAtY(rs, nets, m.height, m.cursor, msg.Y); i >= 0 {
				m.cursor = i
			} else if m.cursor < len(rs)-1 {
				m.cursor++
			}
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress {
				if i := rowAtY(rs, nets, m.height, m.cursor, msg.Y); i >= 0 {
					m.cursor = i
					if !m.busy {
						return m, selectRow(&m, rs[i])
					}
				}
			}
		case tea.MouseButtonRight:
			if msg.Action == tea.MouseActionPress {
				m.st.Prompt, m.st.Input, m.st.QRShow = "", "", false
			}
		}
		return m, nil
	case tea.KeyMsg:
		if m.st.QRShow {
			m.st.QRShow = false
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
					return doneMsg("failed: " + shortName(strings.TrimSpace(out), 50))
				}
			default:
				if s := msg.String(); len(s) == 1 {
					m.st.Input += s
				}
			}
			return m, nil
		}
		rs := rowsOf(&m.st)
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.flash = ""
			return m, func() tea.Msg { return doSnapshot() }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(rs)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(rs) && !m.busy {
				return m, selectRow(&m, rs[m.cursor])
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.st.QRShow && len(m.st.QR) > 0 {
		var b strings.Builder
		b.WriteString(titleSt.Render(" Wi-Fi QR ") + " " + helpSt.Render("any key back") + "\n\n")
		for _, line := range m.st.QR {
			var row strings.Builder
			for _, ch := range line {
				if ch == '1' {
					row.WriteString("██")
				} else {
					row.WriteString("  ")
				}
			}
			b.WriteString("  " + row.String() + "\n")
		}
		return boxSt.Render(b.String())
	}
	if m.st.Prompt != "" {
		var b strings.Builder
		b.WriteString(titleSt.Render(" Password ") + " " + helpSt.Render("enter join · esc cancel") + "\n\n")
		b.WriteString(nameSt.Render("  "+m.st.Prompt) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(cFg).Render("  > "+strings.Repeat("•", len([]rune(m.st.Input)))) + "_\n")
		return boxSt.Render(b.String())
	}
	boxW, nameW := layoutWidths(m.width)
	var b strings.Builder
	b.WriteString(titleSt.Render(" Zephyr ") + " " + helpSt.Render("enter select · r refresh · q quit") + "\n")
	b.WriteString(helpSt.Render("  click select · wheel move") + "\n")
	flash := " "
	if m.flash != "" {
		flash = "  " + lipgloss.NewStyle().Foreground(cBlue).Render(m.flash)
	} else if m.busy {
		flash = "  " + lipgloss.NewStyle().Foreground(cYellow).Render("working…")
	}
	b.WriteString(flash + "\n\n")
	sigTxt := fmt.Sprintf("%d%%", m.st.Signal)
	statusLine := fmt.Sprintf("%s %s  %s %s  %sMHz", wifiGlyph(m.st.SSID != ""), nameSt.Render(shortName(m.st.SSID, nameW)), sigBar(m.st.Signal, 8), sigTxt, m.st.Freq)
	if m.st.SSID == "" {
		statusLine = dimSt.Render("  disconnected")
	}
	b.WriteString(statusLine + "\n" + dimSt.Render("  DNS "+m.st.DNS) + "\n")
	rs := rowsOf(&m.st)
	lines := contentLines(len(m.st.Nets))
	maxH := availH(m.height)
	start := windowStart(len(lines), m.cursor, maxH)
	end := start + maxH
	if end > len(lines) {
		end = len(lines)
	}
	lastKind := ""
	for pos := start; pos < end; pos++ {
		li := lines[pos]
		r := rs[li]
		if r.Kind != lastKind {
			if r.Kind == "NET" && lastKind != "" {
				b.WriteString("\n")
			} else if r.Kind == "ACTION" && lastKind == "NET" {
				b.WriteString("\n")
			}
			lastKind = r.Kind
		}
		var line string
		switch r.Kind {
		case "NET":
			var n Network
			for _, nn := range m.st.Nets {
				if nn.SSID == r.SSID {
					n = nn
					break
				}
			}
			sec := ""
			if n.Security != "" && n.Security != "--" {
				sec = " " + lockGlyph()
			}
			nm := shortName(n.SSID, nameW)
			if n.Active {
				nm = activeNm.Render(nm)
			}
			line = fmt.Sprintf("%s %s  %s%s", wifiGlyph(n.Active), nm, sigBar(n.Signal, 8), sec)
		case "ACTION":
			line = lipgloss.NewStyle().Foreground(cAccent).Render(r.Text)
		}
		if li == m.cursor {
			b.WriteString(selSt.Render("▸ "+line) + "\n")
		} else {
			b.WriteString(dimSt.Render("  ") + line + "\n")
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
		st := snapshot()
		fmt.Printf("type=%s ssid=%s signal=%d freq=%s dns=%s nets=%d\n", st.Type, st.SSID, st.Signal, st.Freq, st.DNS, len(st.Nets))
		for _, n := range st.Nets {
			fmt.Printf("net\t%s\t%d\t%s\t%v\n", n.SSID, n.Signal, n.Security, n.Active)
		}
		return
	}
	m := model{st: snapshot(), cursor: 0}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "zephyr:", err)
		os.Exit(1)
	}
}

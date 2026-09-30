// Zephyr — soft drifting signal (Go TUI, stdlib only).
// Wifi list + connect, DNS switch, QR share, speedtest, wifi restart.
// Keys: up/down or j/k move · enter select · r refresh · q/esc quit.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Pink Cat Boo
const (
	cReset  = "\x1b[0m"
	cFg     = "\x1b[38;2;255;240;245m"
	cAccent = "\x1b[38;2;255;76;122m"
	cMuted  = "\x1b[38;2;86;89;112m"
	cDim    = "\x1b[38;2;120;124;150m"
	cGreen  = "\x1b[38;2;59;192;137m"
	cBold   = "\x1b[1m"
	altOn   = "\x1b[?1049h\x1b[H"
	altOff  = "\x1b[?1049l"
	hideCur = "\x1b[?25l"
	showCur = "\x1b[?25h"
	clear   = "\x1b[H\x1b[2J"
)

type Network struct {
	SSID     string
	Signal   int
	Security string
	Active   bool
}

type Row struct {
	Kind   string // INFO, NET, ACTION
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
	Nets    []Network
	QR      []string
	QRShow  bool
	Speed   string
	Testing bool
	Message string
	Prompt  string // inline input prompt, "" = off
	Input   string
}

func sh(args ...string) string {
	out, _ := exec.Command(args[0], args[1:]...).Output()
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
		p := strings.SplitN(line, ":", 2)
		if len(p) == 2 && p[1] == "wifi" {
			iface = p[0]
			break
		}
	}
	_ = iface
	for _, line := range strings.Split(sh("nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY,IN-USE", "dev", "wifi", "list", "ifname", iface), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := strings.Split(line, ":")
		if len(p) < 4 {
			continue
		}
		sec := p[len(p)-2]
		inuse := p[len(p)-1]
		sig, _ := strconv.Atoi(p[len(p)-3])
		ssid := strings.Join(p[:len(p)-3], ":")
		ssid = strings.ReplaceAll(ssid, "\\:", ":")
		if ssid == "" {
			continue
		}
		st.Nets = append(st.Nets, Network{ssid, sig, sec, inuse == "*"})
	}
	return st
}

func sigBar(s int) string {
	const w = 8
	if s < 0 {
		s = 0
	}
	if s > 100 {
		s = 100
	}
	n := s * w / 100
	return strings.Repeat("█", n) + strings.Repeat("░", w-n)
}

func shortName(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func rows(st *State) []Row {
	var rows []Row
	rows = append(rows, Row{"INFO", fmt.Sprintf("%s  %s  %s%%  %sMHz", st.Type, st.SSID, sigBar(st.Signal), st.Freq), "", ""})
	rows = append(rows, Row{"INFO", "DNS  " + st.DNS, "", ""})
	for _, n := range st.Nets {
		mark := "  "
		if n.Active {
			mark = "● "
		}
		sec := ""
		if n.Security != "" && n.Security != "--" {
			sec = " 🔒"
		}
		rows = append(rows, Row{"NET", mark + shortName(n.SSID, 28) + "  " + sigBar(n.Signal) + sec, n.SSID, "connect"})
	}
	rows = append(rows, Row{"ACTION", "DNS: switch provider (now " + st.DNS + ")", "", "dns"})
	rows = append(rows, Row{"ACTION", "Share Wi-Fi via QR code", "", "qr"})
	speedLabel := "Speed test" + st.Speed
	if st.Testing {
		speedLabel = "Speed test (testing…)"
	}
	rows = append(rows, Row{"ACTION", speedLabel, "", "speed"})
	rows = append(rows, Row{"ACTION", "Restart Wi-Fi", "", "restart"})
	return rows
}

func render(st *State, cur int) string {
	if st.QRShow {
		var b strings.Builder
		b.WriteString(clear)
		b.WriteString(cBold + cFg + "  Wi-Fi QR  " + cDim + "(any key back)" + cReset + "\n\n")
		for _, line := range st.QR {
			var row strings.Builder
			for _, ch := range line {
				if ch == '1' {
					row.WriteString("██")
				} else {
					row.WriteString("  ")
				}
			}
			b.WriteString("  " + cFg + row.String() + cReset + "\n")
		}
		return b.String()
	}
	if st.Prompt != "" {
		var b strings.Builder
		b.WriteString(clear)
		b.WriteString(cBold + cFg + "  " + st.Prompt + cReset + "\n\n")
		b.WriteString(cFg + "  > " + st.Input + cReset + "_\n")
		return b.String()
	}
	var b strings.Builder
	b.WriteString(clear)
	b.WriteString(cBold + cFg + "  Zephyr" + cReset + cDim + "  ·  enter select · r refresh · q quit" + cReset + "\n\n")
	for i, r := range rows(st) {
		prefix := "  "
		if i == cur {
			prefix = cAccent + "▸ " + cReset
		}
		if r.Kind == "INFO" {
			b.WriteString(cFg + "  " + r.Text + cReset + "\n")
			continue
		}
		if r.Kind == "ACTION" {
			b.WriteString(prefix + cAccent + r.Text + cReset + "\n")
			continue
		}
		b.WriteString(prefix + cFg + r.Text + cReset + "\n")
	}
	if st.Message != "" {
		b.WriteString("\n" + cDim + "  " + st.Message + cReset + "\n")
	}
	if st.Speed != "" && !st.Testing {
		_ = st
	}
	return b.String()
}

var tty *os.File

func rawOn() {
	tty, _ = os.OpenFile("/dev/tty", os.O_RDWR, 0)
	exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1", "-echo").Run()
	fmt.Print(altOn + hideCur)
}

func rawOff() {
	fmt.Print(showCur + altOff)
	exec.Command("stty", "-F", "/dev/tty", "sane").Run()
	if tty != nil {
		tty.Close()
	}
}

func readKey() string {
	buf := make([]byte, 8)
	n, _ := tty.Read(buf)
	if n == 0 {
		return ""
	}
	if buf[0] == 0x1b {
		if n == 1 {
			return "esc"
		}
		switch string(buf[1:n]) {
		case "[A":
			return "up"
		case "[B":
			return "down"
		}
		return "esc"
	}
	switch buf[0] {
	case 'q', 'Q':
		return "quit"
	case 'r', 'R':
		return "refresh"
	case '\r', '\n':
		return "enter"
	case 'j':
		return "down"
	case 'k':
		return "up"
	case 0x7f, 0x08:
		return "backspace"
	default:
		if buf[0] >= 32 && buf[0] < 127 {
			return "char:" + string(buf[0])
		}
	}
	return ""
}

var dnsOrder = []string{"DHCP", "Cloudflare", "Google"}

func main() {
	// Helpers live beside this binary; ensure they resolve anywhere.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		os.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	}
	if len(os.Args) > 1 && os.Args[1] == "--dump" {
		st := snapshot()
		fmt.Printf("type=%s ssid=%s signal=%d freq=%s dns=%s nets=%d\n", st.Type, st.SSID, st.Signal, st.Freq, st.DNS, len(st.Nets))
		for _, n := range st.Nets {
			fmt.Printf("net\t%s\t%d\t%s\t%v\n", n.SSID, n.Signal, n.Security, n.Active)
		}
		return
	}
	st := snapshot()
	cur := 2 // first network row (after 2 INFO rows)
	rawOn()
	defer rawOff()
	fmt.Print(render(&st, cur))

	speedch := make(chan string, 1)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	keych := make(chan string, 8)
	go func() {
		for {
			keych <- readKey()
		}
	}()

	refresh := func() {
		msg := st.Message
		qr := st.QR
		st = snapshot()
		st.Message = msg
		st.QR = qr
	}

	for {
		select {
		case k := <-keych:
			if st.QRShow {
				st.QRShow = false
				fmt.Print(render(&st, cur))
				continue
			}
			if st.Prompt != "" {
				switch {
				case k == "esc":
					st.Prompt = ""
					st.Input = ""
				case k == "backspace":
					if len(st.Input) > 0 {
						st.Input = st.Input[:len(st.Input)-1]
					}
				case k == "enter":
					pw := st.Input
					ssid := st.Prompt
					st.Prompt = ""
					st.Input = ""
					st.Message = "connecting to " + ssid + "…"
					fmt.Print(render(&st, cur))
					out := sh("nmcli", "dev", "wifi", "connect", ssid, "password", pw)
					if strings.Contains(out, "successfully") {
						st.Message = "connected to " + ssid
					} else {
						st.Message = "failed: " + shortName(strings.TrimSpace(out), 60)
					}
					refresh()
				case strings.HasPrefix(k, "char:"):
					st.Input += strings.TrimPrefix(k, "char:")
				}
				fmt.Print(render(&st, cur))
				continue
			}
			rs := rows(&st)
			switch k {
			case "quit", "esc":
				return
			case "refresh":
				st.Message = ""
				refresh()
			case "up":
				if cur > 0 {
					cur--
				}
			case "down":
				if cur < len(rs)-1 {
					cur++
				}
			case "enter":
				if cur < len(rs) {
					r := rs[cur]
					switch r.Action {
					case "connect":
						active := false
						for _, n := range st.Nets {
							if n.SSID == r.SSID && n.Active {
								active = true
							}
						}
						if active {
							st.Message = "already on " + r.SSID
						} else {
							known := sh("nmcli", "-t", "-f", "NAME", "connection", "show") != ""
							_ = known
							out := sh("nmcli", "dev", "wifi", "connect", r.SSID)
							if strings.Contains(out, "successfully") {
								st.Message = "connected to " + r.SSID
							} else if strings.Contains(out, "Secrets were required") || strings.Contains(out, "password") {
								st.Prompt = r.SSID
								st.Input = ""
							} else {
								st.Message = "failed: " + shortName(strings.TrimSpace(out), 60)
							}
						}
						refresh()
					case "dns":
						next := dnsOrder[0]
						for i, d := range dnsOrder {
							if d == st.DNS && i+1 < len(dnsOrder) {
								next = dnsOrder[i+1]
							}
						}
						st.Message = sh("omarchy-dns", next)
						refresh()
					case "qr":
						out := sh("omarchy-network-qr")
						var m []string
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
								m = append(m, line)
							}
						}
						if len(m) == 0 {
							st.Message = "no QR available"
						} else {
							st.QR = m
						}
					case "speed":
						if !st.Testing {
							st.Testing = true
							st.Speed = ""
							go func() {
								out := sh("omarchy-network-speedtest", "down")
								speedch <- strings.TrimSpace(out)
							}()
						}
					case "restart":
						st.Message = sh("omarchy-restart-wifi")
						refresh()
					}
				}
			}
			fmt.Print(render(&st, cur))
		case s := <-speedch:
			st.Testing = false
			st.Speed = " — " + shortName(s, 40)
			fmt.Print(render(&st, cur))
		case <-tick.C:
			msg := st.Message
			refresh()
			st.Message = msg
			rs := rows(&st)
			if cur >= len(rs) && len(rs) > 0 {
				cur = len(rs) - 1
			}
			fmt.Print(render(&st, cur))
		}
	}
}

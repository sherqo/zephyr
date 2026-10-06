package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// vline is one flat content line. text has no cursor marker; row is the
// selsOf index (-1 for filler). buildView is the SINGLE source of truth
// for both rendering and mouse mapping.
type vline struct {
	text string
	row  int
}

func buildView(m *model, ss []Sel, nameW int) []vline {
	var vl []vline
	row := func(li int, text string) { vl = append(vl, vline{text, li}) }
	head := func(title string) {
		vl = append(vl, vline{"", -1}, vline{headSt.Render(title), -1})
	}
	lastKnown := false
	startedNet := false
	for li, r := range ss {
		switch r.Kind {
		case "header-qr":
			lbl := "QR code"
			if m.st.QROpen {
				lbl = "hide QR code"
			}
			row(li, "[QR] "+lbl)
		case "header-power":
			row(li, "Wi-Fi: "+map[bool]string{true: "on", false: "off"}[m.st.WifiOn])
			if m.st.QROpen {
				for i := 0; i < len(m.st.QR); i += 2 {
					top := m.st.QR[i]
					bot := ""
					if i+1 < len(m.st.QR) {
						bot = m.st.QR[i+1]
					}
					var qr strings.Builder
					n := len(top)
					if len(bot) > n {
						n = len(bot)
					}
					for c := 0; c < n; c++ {
						t, b := byte('0'), byte('0')
						if c < len(top) {
							t = top[c]
						}
						if c < len(bot) {
							b = bot[c]
						}
						switch {
						case t == '1' && b == '1':
							qr.WriteString("█")
						case t == '1':
							qr.WriteString("▀")
						case b == '1':
							qr.WriteString("▄")
						default:
							qr.WriteString(" ")
						}
					}
					vl = append(vl, vline{"  " + qr.String(), -1})
				}
				if m.st.Password != "" {
					vl = append(vl, vline{dimSt.Render("  Password: ") + nameSt.Render(m.st.Password), -1})
				}
			}
		case "band":
			title := "WI-FI BAND"
			if m.st.Selected == "auto" && m.st.Band != "" {
				title = "WI-FI BAND: " + strings.ToUpper(m.st.Band) + "GHZ"
			}
			head(title)
			var pills []string
			for bi, b := range m.st.Bands {
				if bi == m.bandCursor {
					pills = append(pills, pillOn.Render(bandLabel(b)))
				} else {
					pills = append(pills, pillOff.Render(bandLabel(b)))
				}
			}
			row(li, "  "+strings.Join(pills, " "))
		case "net":
			var n Network
			for _, nn := range m.st.Nets {
				if nn.SSID == r.Ref {
					n = nn
					break
				}
			}
			if !startedNet {
				startedNet = true
				lastKnown = n.Known
				if n.Known {
					vl = append(vl, vline{"", -1}, vline{headSt.Render("KNOWN NETWORKS"), -1})
				} else {
					vl = append(vl, vline{"", -1}, vline{headSt.Render("OTHER NETWORKS"), -1})
				}
				colHead := "  " + cell("NETWORK", nameW+2) + "  SIGNAL"
				vl = append(vl, vline{lipgloss.NewStyle().Foreground(cYellow).Bold(true).Render(colHead), -1})
			} else if n.Known != lastKnown {
				lastKnown = n.Known
				if n.Known {
					head("KNOWN NETWORKS")
				} else {
					head("OTHER NETWORKS")
				}
			}
			sec := ""
			if n.Security != "" && n.Security != "--" {
				sec = " " + lockGlyph()
			}
			nm := cell(n.SSID, nameW)
			if n.Active {
				nm = activeNm.Render(nm)
			}
			row(li, fmt.Sprintf("%s %s  %s%s", wifiGlyph(n.Active), nm, sigBar(n.Signal, 8), sec))
		}
	}
	return vl
}

// cell returns s truncated and padded to exactly w terminal cells,
// so columns align regardless of wide/ambiguous runes in SSIDs.
func cell(s string, w int) string {
	s = runewidth.Truncate(s, w, "…")
	return runewidth.FillRight(s, w)
}
func rowAtY(m *model, ss []Sel, termW, termH, cursor, y int) int {
	_, nameW := layoutWidths(termW)
	vl := buildView(m, ss, nameW)
	maxH := availH(termH)
	cpos := 0
	for pos, v := range vl {
		if v.row == cursor {
			cpos = pos
			break
		}
	}
	start := windowStart(len(vl), cpos, maxH)
	pos := start + (y - 5)
	if pos < start || pos >= start+maxH || pos < 0 || pos >= len(vl) {
		return -1
	}
	return vl[pos].row
}

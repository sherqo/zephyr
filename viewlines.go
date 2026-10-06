package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
				for _, ql := range m.st.QR {
					var qr strings.Builder
					for _, ch := range ql {
						if ch == '1' {
							qr.WriteString("██")
						} else {
							qr.WriteString("  ")
						}
					}
					vl = append(vl, vline{"  " + qr.String(), -1})
				}
			}
		case "band":
			if li == 2 {
				title := "WI-FI BAND"
				if m.st.Selected == "auto" && m.st.Band != "" {
					title = "WI-FI BAND: " + strings.ToUpper(m.st.Band) + "GHZ"
				}
				head(title)
			}
			pill := pillOff.Render(bandLabel(r.Ref))
			if m.st.Selected == r.Ref {
				pill = pillOn.Render(bandLabel(r.Ref))
			}
			row(li, "  "+pill)
		case "dns":
			if li == 2+len(m.st.Bands) {
				head("DNS")
			}
			dot := ""
			if m.st.DNS == r.Ref {
				dot = lipgloss.NewStyle().Foreground(cAccent).Render(" ●")
			}
			row(li, "  "+r.Ref+dot)
		case "net":
			var n Network
			for _, nn := range m.st.Nets {
				if nn.SSID == r.Ref {
					n = nn
					break
				}
			}
			if !startedNet || n.Known != lastKnown {
				startedNet = true
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
			nm := shortName(n.SSID, nameW)
			if n.Active {
				nm = activeNm.Render(nm)
			}
			row(li, fmt.Sprintf("%s %s  %s%s", wifiGlyph(n.Active), nm, sigBar(n.Signal, 8), sec))
		}
	}
	return vl
}

// rowAtY maps a terminal line to a selsOf index through the same flat
// list View renders. Header block occupies lines 2..5, content from 6.
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
	pos := start + (y - 6)
	if pos < start || pos >= start+maxH || pos < 0 || pos >= len(vl) {
		return -1
	}
	return vl[pos].row
}

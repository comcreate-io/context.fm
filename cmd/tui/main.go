// Command tui is the setup + status surface for context.fm.
// Thin client over local worker state; the background loop never needs it.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/comcreate-io/context.fm/internal/config"
	workctx "github.com/comcreate-io/context.fm/internal/context"
	"github.com/comcreate-io/context.fm/internal/memory"
	"github.com/comcreate-io/context.fm/internal/queue"
	"github.com/comcreate-io/context.fm/internal/spotify"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Faint(true)
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

type screen int

const (
	screenStatus screen = iota
	screenSetup
)

type model struct {
	screen    screen
	status    string
	msg       string
	cfg       config.Config
	codeIn    textinput.Model
	awaiting  bool // awaiting auth code input
	verifier  string
	state     string
	devices   []spotify.Device
	selCursor int
	picking   bool // picking a device from the list
}

func main() {
	m := model{screen: screenStatus}
	m.codeIn = textinput.New()
	m.codeIn.Placeholder = "paste Spotify auth code"
	m.codeIn.CharLimit = 512
	if cfg, err := config.Load(); err == nil {
		m.cfg = cfg
	}
	m.refreshStatus()
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "contextfm-tui:", err)
		os.Exit(1)
	}
}

func (m *model) refreshStatus() {
	q, err := queue.ReadAll()
	if err != nil {
		m.status = "queue: " + err.Error()
		return
	}
	inputs := make([]workctx.Input, 0, len(q))
	sessions := map[string]bool{}
	now := time.Now().UTC()
	for _, e := range q {
		ts := e.Timestamp
		if ts.IsZero() {
			ts = now
		}
		inputs = append(inputs, workctx.Input{Text: e.Prompt, Timestamp: ts})
		sessions[e.SessionID] = true
	}
	snap := workctx.Classify(inputs)
	memCount := -1
	if store, err := memory.Open(); err == nil {
		if n, err := store.Count(); err == nil {
			memCount = n
		}
		_ = store.Close()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "queue depth %d · sessions %d\n", len(q), len(sessions))
	fmt.Fprintf(&b, "context %s · confidence %.2f · evidence %d\n", snap.Label, snap.Confidence, snap.EvidenceCount)
	if memCount >= 0 {
		fmt.Fprintf(&b, "learned records %d\n", memCount)
	} else {
		b.WriteString("learned records: no store yet\n")
	}
	gate := "closed"
	if spotify.GateEnabled() {
		gate = "OPEN (pilot)"
	}
	fmt.Fprintf(&b, "playback gate %s · setup enabled=%v device=%s\n",
		gate, m.cfg.Enabled, displayDevice(m.cfg.DeviceID))
	m.status = b.String()
}

func displayDevice(id string) string {
	if id == "" {
		return "(none)"
	}
	if len(id) > 12 {
		return id[:12] + "…"
	}
	return id
}

// --- tea.Model ---

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	if m.awaiting {
		var cmd tea.Cmd
		m.codeIn, cmd = m.codeIn.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.awaiting && msg.String() != "esc" && msg.String() != "enter" {
		var cmd tea.Cmd
		m.codeIn, cmd = m.codeIn.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "1", "2":
		if m.screen == screenStatus {
			m.screen = screenSetup
		} else {
			m.screen = screenStatus
			m.refreshStatus()
		}
		m.msg = ""
		return m, nil
	}
	if m.screen == screenStatus {
		return m.handleStatusKey(msg)
	}
	return m.handleSetupKey(msg)
}

func (m model) handleStatusKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "r":
		m.refreshStatus()
	case "e":
		m.cfg.Enabled = !m.cfg.Enabled
		if err := config.Save(m.cfg); err != nil {
			m.msg = errStyle.Render("save: " + err.Error())
		} else {
			m.msg = okStyle.Render(fmt.Sprintf("pilot enabled=%v", m.cfg.Enabled))
		}
		m.refreshStatus()
	case "x":
		if store, err := memory.Open(); err == nil {
			if err := store.Reset(); err != nil {
				m.msg = errStyle.Render("reset: " + err.Error())
			} else {
				m.msg = okStyle.Render("learned records cleared")
			}
			_ = store.Close()
		} else {
			m.msg = errStyle.Render("reset: " + err.Error())
		}
		m.refreshStatus()
	}
	return m, nil
}

func (m model) handleSetupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.awaiting {
			return m.exchangeCode()
		}
		if m.picking && len(m.devices) > 0 {
			return m.selectDevice()
		}
	case "esc":
		m.awaiting, m.picking = false, false
		m.msg = ""
		return m, nil
	case "o":
		return m.startAuth()
	case "d":
		return m.fetchDevices()
	case "up", "k":
		if m.picking && m.selCursor > 0 {
			m.selCursor--
		}
	case "down", "j":
		if m.picking && m.selCursor < len(m.devices)-1 {
			m.selCursor++
		}
	}
	return m, nil
}

func (m model) startAuth() (tea.Model, tea.Cmd) {
	cid := spotify.ClientID()
	if cid == "" {
		m.msg = errStyle.Render("set CONTEXTFM_SPOTIFY_CLIENT_ID first")
		return m, nil
	}
	v, err := spotify.NewVerifier()
	if err != nil {
		m.msg = errStyle.Render(err.Error())
		return m, nil
	}
	st, err := spotify.NewState()
	if err != nil {
		m.msg = errStyle.Render(err.Error())
		return m, nil
	}
	m.verifier, m.state = v, st
	m.awaiting = true
	m.codeIn.SetValue("")
	m.codeIn.Focus()
	m.msg = "open this URL, approve, then paste the code:\n" + spotify.AuthCodeURL(cid, st, spotify.Challenge(v))
	return m, nil
}

func (m model) exchangeCode() (tea.Model, tea.Cmd) {
	code := strings.TrimSpace(m.codeIn.Value())
	if code == "" {
		m.msg = errStyle.Render("empty code")
		return m, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tok, err := spotify.Exchange(ctx, "", spotify.ClientID(), code, m.verifier)
	if err != nil {
		m.msg = errStyle.Render("exchange: " + err.Error())
		return m, nil
	}
	if err := spotify.SaveToken(tok); err != nil {
		m.msg = errStyle.Render("save token: " + err.Error())
		return m, nil
	}
	m.awaiting = false
	m.msg = okStyle.Render("Spotify connected. Press d to pick a device.")
	return m, nil
}

func (m model) fetchDevices() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := spotify.EnsureValidToken(ctx, "", spotify.ClientID())
	if err != nil {
		m.msg = errStyle.Render("auth first (press o): " + err.Error())
		return m, nil
	}
	devs, err := client.Devices(ctx)
	if err != nil {
		m.msg = errStyle.Render("devices: " + err.Error())
		return m, nil
	}
	if len(devs) == 0 {
		m.msg = warnStyle.Render("no devices: open Spotify on one device first")
		return m, nil
	}
	m.devices, m.picking, m.selCursor = devs, true, 0
	m.msg = "choose a device, enter to save"
	return m, nil
}

func (m model) selectDevice() (tea.Model, tea.Cmd) {
	d := m.devices[m.selCursor]
	m.cfg.DeviceID = d.ID
	if err := config.Save(m.cfg); err != nil {
		m.msg = errStyle.Render("save: " + err.Error())
		return m, nil
	}
	m.picking = false
	m.msg = okStyle.Render("device saved: " + d.Name)
	m.refreshStatus()
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("context.fm") + dimStyle.Render(" · tab switches screens · q quits") + "\n\n")
	if m.screen == screenStatus {
		b.WriteString(titleStyle.Render("status") + "\n" + m.status + "\n")
		b.WriteString(dimStyle.Render("e enable/disable pilot · x reset learned records · r refresh") + "\n")
	} else {
		b.WriteString(titleStyle.Render("setup") + "\n")
		cid := spotify.ClientID()
		if cid == "" {
			b.WriteString(warnStyle.Render("CONTEXTFM_SPOTIFY_CLIENT_ID not set") + "\n")
		} else {
			b.WriteString(okStyle.Render("client id set") + "\n")
		}
		if _, err := spotify.LoadToken(); err == nil {
			b.WriteString(okStyle.Render("token stored") + "\n")
		} else {
			b.WriteString(dimStyle.Render("no token yet · press o to connect") + "\n")
		}
		b.WriteString(fmt.Sprintf("device %s · pilot enabled=%v\n", displayDevice(m.cfg.DeviceID), m.cfg.Enabled))
		if m.awaiting {
			b.WriteString("\n" + m.codeIn.View() + dimStyle.Render("  (enter exchanges · esc cancels)") + "\n")
		}
		if m.picking {
			b.WriteString("\ndevices:\n")
			for i, d := range m.devices {
				mark := "  "
				if i == m.selCursor {
					mark = "> "
				}
				active := ""
				if d.IsActive {
					active = " (active)"
				}
				b.WriteString(fmt.Sprintf("%s%s%s\n", mark, d.Name, active))
			}
		}
		b.WriteString(dimStyle.Render("\no connect Spotify · d pick device · enter confirm · esc cancel") + "\n")
	}
	if m.msg != "" {
		b.WriteString("\n" + m.msg + "\n")
	}
	return b.String()
}

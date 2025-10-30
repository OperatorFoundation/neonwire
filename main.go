package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	maxWidth  = 80
	maxHeight = 24
	port      = 9999
)

type msgPacket struct {
	Sender    string    `json:"sender"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type udpMsg struct {
	data []byte
	addr *net.UDPAddr
}

type model struct {
	viewport    viewport.Model
	textarea    textarea.Model
	messages    []string
	username    string
	remoteAddr  string
	conn        *net.UDPConn
	err         error
}

var (
	// Retro-futuristic color palette
	cyan        = lipgloss.Color("#00FFFF")
	magenta     = lipgloss.Color("#FF00FF")
	green       = lipgloss.Color("#00FF00")
	darkCyan    = lipgloss.Color("#008B8B")
	darkMagenta = lipgloss.Color("#8B008B")
	gray        = lipgloss.Color("#808080")
	white       = lipgloss.Color("#FFFFFF")

	// Style definitions
	titleStyle = lipgloss.NewStyle().
			Foreground(cyan).
			Background(darkCyan).
			Bold(true).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(magenta).
			Italic(true)

	timestampStyle = lipgloss.NewStyle().
			Foreground(gray)

	usernameStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	remoteUsernameStyle = lipgloss.NewStyle().
				Foreground(magenta).
				Bold(true)

	messageStyle = lipgloss.NewStyle().
			Foreground(white)

	borderStyle = lipgloss.NewStyle().
			Foreground(cyan).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyan)
)

func initialModel(username, remoteAddr string) model {
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.Focus()
	ta.Prompt = "│ "
	ta.CharLimit = 280
	ta.SetWidth(76)
	ta.SetHeight(3)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(76, 16)
	vp.SetContent("Connected. Waiting for messages...")

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", remoteAddr, port))
	if err != nil {
		return model{err: err}
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return model{err: err}
	}

	m := model{
		textarea:   ta,
		viewport:   vp,
		messages:   []string{},
		username:   username,
		remoteAddr: addr.String(),
		conn:       conn,
	}

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		listenForMessages(m.conn),
	)
}

func listenForMessages(conn *net.UDPConn) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 1024)
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return errMsg{err}
		}
		return udpMsg{data: buf[:n], addr: addr}
	}
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.textarea.Value())
			if text != "" {
				// Send message
				packet := msgPacket{
					Sender:    m.username,
					Text:      text,
					Timestamp: time.Now(),
				}
				data, _ := json.Marshal(packet)
				addr, _ := net.ResolveUDPAddr("udp", m.remoteAddr)
				m.conn.WriteToUDP(data, addr)

				// Add to local display
				ts := timestampStyle.Render(packet.Timestamp.Format("15:04:05"))
				user := usernameStyle.Render(m.username)
				msgText := messageStyle.Render(text)
				m.messages = append(m.messages, fmt.Sprintf("%s %s: %s", ts, user, msgText))
				m.viewport.SetContent(strings.Join(m.messages, "\n"))
				m.viewport.GotoBottom()
				m.textarea.Reset()
			}
		}

	case udpMsg:
		var packet msgPacket
		if err := json.Unmarshal(msg.data, &packet); err == nil {
			ts := timestampStyle.Render(packet.Timestamp.Format("15:04:05"))
			user := remoteUsernameStyle.Render(packet.Sender)
			msgText := messageStyle.Render(packet.Text)
			m.messages = append(m.messages, fmt.Sprintf("%s %s: %s", ts, user, msgText))
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.viewport.GotoBottom()
		}
		return m, listenForMessages(m.conn)

	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	// Title bar
	title := titleStyle.Render("═══ VT100 CHAT ═══")
	status := statusStyle.Render(fmt.Sprintf("Connected to: %s", m.remoteAddr))
	titleBar := lipgloss.JoinHorizontal(
		lipgloss.Left,
		title,
		" ",
		status,
	)

	// Message viewport with border
	viewportContent := borderStyle.Width(76).Height(16).Render(m.viewport.View())

	// Input area with border
	inputLabel := lipgloss.NewStyle().
		Foreground(cyan).
		Bold(true).
		Render("┌─ INPUT ─")
	
	inputArea := borderStyle.Width(76).Height(3).Render(m.textarea.View())

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(gray).
		Render("ESC/Ctrl+C: quit • ENTER: send")

	// Combine all elements
	ui := lipgloss.JoinVertical(
		lipgloss.Left,
		titleBar,
		"",
		viewportContent,
		"",
		inputLabel,
		inputArea,
		footer,
	)

	return ui
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: chat <username> <remote-address>")
		fmt.Println("Example: chat alice 100.64.0.2")
		os.Exit(1)
	}

	username := os.Args[1]
	remoteAddr := os.Args[2]

	p := tea.NewProgram(
		initialModel(username, remoteAddr),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

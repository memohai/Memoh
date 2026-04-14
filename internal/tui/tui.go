package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/session"
)

type focusArea int

const (
	focusBots focusArea = iota
	focusSessions
	focusChat
	focusInput
)

type TUIModel struct {
	client *Client
	state  State

	panelWidth int
	width      int
	height     int

	focus focusArea

	bots     []botSummary
	botList  list.Model
	sessions []session.Session
	sessList list.Model

	input    textinput.Model
	viewport viewport.Model

	status   string
	dbStatus string

	chatContent        string
	viewportContent    string
	streamPreview      string
	streamPreviewOrder []int
	streamPreviewItems map[int]conversation.UIMessage
	streamCh           <-chan ChatEvent
}

type botSummary struct {
	ID          string
	DisplayName string
	Status      string
}

type selectorItem struct {
	id    string
	title string
}

func (i selectorItem) FilterValue() string { return i.title + " " + i.id }
func (i selectorItem) Title() string       { return i.title }
func (selectorItem) Description() string   { return "" }

var (
	memohPrimary = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A78BFA"}
	mutedBorder  = lipgloss.AdaptiveColor{Light: "#D7D8E0", Dark: "#3A3442"}
	mutedTitle   = lipgloss.AdaptiveColor{Light: "#666978", Dark: "#9A97A3"}
)

type dbStatusMsg struct {
	value string
	err   error
}

type botsLoadedMsg struct {
	items []botSummary
	err   error
}

type sessionsLoadedMsg struct {
	items []session.Session
	err   error
}

type turnsLoadedMsg struct {
	content string
	err     error
}

type chatStartedMsg struct {
	sessionID string
	streamCh  <-chan ChatEvent
	err       error
}

type chatEventMsg struct {
	event ChatEvent
}

type chatDoneMsg struct{}

func NewTUIModel(state State) *TUIModel {
	input := textinput.New()
	input.Placeholder = "Type a message and press Enter"
	input.Focus()

	botList := newSelectorList()
	sessList := newSelectorList()

	return &TUIModel{
		client:             NewClient(state.ServerURL, state.Token),
		state:              state,
		focus:              focusBots,
		input:              input,
		viewport:           viewport.New(80, 20),
		status:             "Loading environment status...",
		dbStatus:           "checking",
		botList:            botList,
		sessList:           sessList,
		streamPreviewItems: map[int]conversation.UIMessage{},
	}
}

func (m *TUIModel) Init() tea.Cmd {
	return tea.Batch(
		loadDBStatusCmd(),
		loadBotsCmd(m.client),
	)
}

func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.panelWidth = max(30, msg.Width-6)
		listWidth := max(20, m.panelWidth-2)
		chatWidth := max(18, m.panelWidth-4)
		listHeight := max(4, min(8, max(4, msg.Height/5)))
		m.botList.SetSize(listWidth, listHeight)
		m.sessList.SetSize(listWidth, listHeight)
		m.viewport.Width = chatWidth
		m.viewport.Height = max(8, msg.Height-(listHeight*2)-14)
		m.input.Width = listWidth
		m.syncViewport(false)
		return m, nil

	case dbStatusMsg:
		if msg.err != nil {
			m.dbStatus = "unavailable"
			m.status = "DB status unavailable: " + msg.err.Error()
		} else {
			m.dbStatus = msg.value
		}
		return m, nil

	case botsLoadedMsg:
		if msg.err != nil {
			m.status = "Failed to load bots: " + msg.err.Error()
			return m, nil
		}
		m.bots = msg.items
		m.syncBotList()
		if len(m.bots) > 0 {
			m.status = "Use Tab to switch focus. Enter on bots/sessions. Enter in input sends."
			return m, loadSessionsCmd(m.client, m.currentBotID())
		}
		if strings.TrimSpace(m.state.Token) == "" {
			m.status = "Login first with `memoh login`, then reopen the TUI."
		} else {
			m.status = "No accessible bots found."
		}
		return m, nil

	case sessionsLoadedMsg:
		if msg.err != nil {
			m.status = "Failed to load sessions: " + msg.err.Error()
			return m, nil
		}
		m.sessions = msg.items
		m.syncSessionList()
		if current := m.currentSessionID(); current != "" {
			return m, loadTurnsCmd(m.client, m.currentBotID(), current)
		}
		m.chatContent = ""
		m.clearStreamPreview()
		m.viewport.SetContent("")
		return m, nil

	case turnsLoadedMsg:
		if msg.err != nil {
			m.status = "Failed to load messages: " + msg.err.Error()
			return m, nil
		}
		m.chatContent = msg.content
		m.clearStreamPreview()
		m.syncViewport(true)
		return m, nil

	case chatStartedMsg:
		if msg.err != nil {
			m.status = "Failed to start chat: " + msg.err.Error()
			return m, nil
		}
		m.streamCh = msg.streamCh
		m.clearStreamPreview()
		if msg.sessionID != "" && msg.sessionID != m.currentSessionID() {
			return m, tea.Batch(
				loadSessionsCmd(m.client, m.currentBotID()),
				waitForChatEventCmd(msg.streamCh),
			)
		}
		return m, waitForChatEventCmd(msg.streamCh)

	case chatEventMsg:
		switch msg.event.Type {
		case "start":
			m.status = "Streaming reply..."
		case "message":
			m.updateStreamPreview(msg.event.Data)
			m.syncViewport(true)
		case "error":
			m.status = "Chat error: " + msg.event.Message
		case "end":
			m.status = "Reply finished."
		}
		return m, waitForChatEventCmd(m.streamCh)

	case chatDoneMsg:
		m.streamCh = nil
		return m, loadTurnsCmd(m.client, m.currentBotID(), m.currentSessionID())

	case tea.KeyMsg:
		if m.focus == focusChat {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "tab":
				m.focus = nextFocus(m.focus)
				cmd := m.input.Focus()
				return m, cmd
			case "shift+tab":
				m.focus = prevFocus(m.focus)
				return m, nil
			case "esc", "q":
				m.focus = focusInput
				cmd := m.input.Focus()
				return m, cmd
			case "down", "j":
				m.viewport.ScrollDown(1)
				return m, nil
			case "up", "k":
				m.viewport.ScrollUp(1)
				return m, nil
			case "pgdown", "f":
				m.viewport.PageDown()
				return m, nil
			case "pgup", "b":
				m.viewport.PageUp()
				return m, nil
			case "ctrl+d":
				m.viewport.HalfPageDown()
				return m, nil
			case "ctrl+u":
				m.viewport.HalfPageUp()
				return m, nil
			case "home", "g":
				m.viewport.GotoTop()
				return m, nil
			case "end", "G":
				m.viewport.GotoBottom()
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		if m.focus == focusInput {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "tab":
				m.focus = nextFocus(m.focus)
				m.input.Blur()
				return m, nil
			case "shift+tab":
				m.focus = prevFocus(m.focus)
				m.input.Blur()
				return m, nil
			case "esc":
				m.focus = focusChat
				m.input.Blur()
				return m, nil
			case "enter":
				text := strings.TrimSpace(m.input.Value())
				if text == "" {
					return m, nil
				}
				if m.currentBotID() == "" {
					m.status = "Select a bot first."
					return m, nil
				}
				m.appendTranscript(renderTurnMarkdown(conversation.UITurn{
					Role: "user",
					Text: text,
				}))
				m.input.SetValue("")
				return m, startChatCmd(m.client, m.currentBotID(), m.currentSessionID(), text)
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.focus = nextFocus(m.focus)
			if m.focus == focusInput {
				cmd := m.input.Focus()
				return m, cmd
			}
			m.input.Blur()
			return m, nil
		case "shift+tab":
			m.focus = prevFocus(m.focus)
			if m.focus == focusInput {
				cmd := m.input.Focus()
				return m, cmd
			}
			m.input.Blur()
			return m, nil
		case "enter":
			switch m.focus {
			case focusBots:
				return m, loadSessionsCmd(m.client, m.currentBotID())
			case focusSessions:
				if current := m.currentSessionID(); current != "" {
					return m, loadTurnsCmd(m.client, m.currentBotID(), current)
				}
				return m, nil
			case focusChat:
				m.viewport.GotoBottom()
				return m, nil
			}
		case "up", "k":
			// handled by list models below when focused
		case "down", "j":
			// handled by list models below when focused
		}
	}

	switch m.focus {
	case focusBots:
		var cmd tea.Cmd
		m.botList, cmd = m.botList.Update(msg)
		return m, cmd
	case focusSessions:
		var cmd tea.Cmd
		m.sessList, cmd = m.sessList.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *TUIModel) View() string {
	header := lipgloss.NewStyle().Bold(true).Render("memoh terminal ui")
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(m.status)
	focusHint := "focus=bots"
	switch m.focus {
	case focusSessions:
		focusHint = "focus=sessions"
	case focusChat:
		focusHint = "focus=chat (j/k pgup/pgdn ctrl+u/d g/G esc)"
	case focusInput:
		focusHint = "focus=input"
	}
	envBlock := panel("Status", strings.Join([]string{
		header,
		"server: " + m.client.BaseURL,
		"db: " + emptyFallback(m.dbStatus, "checking"),
		focusHint,
		status,
	}, "\n"), false, m.panelWidth)

	return lipgloss.JoinVertical(lipgloss.Left,
		envBlock,
		panel("Bots", m.botList.View(), m.focus == focusBots, m.panelWidth),
		panel("Sessions", m.sessList.View(), m.focus == focusSessions, m.panelWidth),
		panel("Chat", m.renderChatViewport(), m.focus == focusChat, m.panelWidth),
		panel("Input", m.input.View(), m.focus == focusInput, m.panelWidth),
	)
}

func loadDBStatusCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := ProvideConfig()
		if err != nil {
			return dbStatusMsg{err: err}
		}
		status, err := dbpkg.ReadMigrationStatus(cfg.Postgres, MigrationsFS())
		if err != nil {
			return dbStatusMsg{err: err}
		}
		return dbStatusMsg{value: fmt.Sprintf("version=%d dirty=%t", status.Version, status.Dirty)}
	}
}

func loadBotsCmd(client *Client) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(client.Token) == "" {
			return botsLoadedMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		items, err := client.ListBots(ctx)
		if err != nil {
			return botsLoadedMsg{err: err}
		}
		result := make([]botSummary, 0, len(items))
		for _, item := range items {
			result = append(result, botSummary{
				ID:          item.ID,
				DisplayName: item.DisplayName,
				Status:      item.Status,
			})
		}
		return botsLoadedMsg{items: result}
	}
}

func loadSessionsCmd(client *Client, botID string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(botID) == "" {
			return sessionsLoadedMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		items, err := client.ListSessions(ctx, botID)
		return sessionsLoadedMsg{items: items, err: err}
	}
}

func loadTurnsCmd(client *Client, botID, sessionID string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(botID) == "" || strings.TrimSpace(sessionID) == "" {
			return turnsLoadedMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		items, err := client.ListMessages(ctx, botID, sessionID)
		if err != nil {
			return turnsLoadedMsg{err: err}
		}
		lines := make([]string, 0, len(items))
		for _, turn := range items {
			lines = append(lines, renderTurnMarkdown(turn))
		}
		return turnsLoadedMsg{content: strings.Join(lines, "\n\n")}
	}
}

func startChatCmd(client *Client, botID, sessionID, text string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		activeSessionID := strings.TrimSpace(sessionID)
		if activeSessionID == "" {
			sess, err := client.CreateSession(ctx, botID, text)
			if err != nil {
				return chatStartedMsg{err: err}
			}
			activeSessionID = sess.ID
		}

		streamCh := make(chan ChatEvent, 32)
		go func() {
			defer close(streamCh)
			err := client.StreamChat(context.Background(), ChatRequest{
				BotID:     botID,
				SessionID: activeSessionID,
				Text:      text,
			}, func(event ChatEvent) error {
				streamCh <- event
				return nil
			})
			if err != nil {
				streamCh <- ChatEvent{Type: "error", Message: err.Error()}
			}
		}()

		return chatStartedMsg{
			sessionID: activeSessionID,
			streamCh:  streamCh,
		}
	}
}

func waitForChatEventCmd(ch <-chan ChatEvent) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return chatDoneMsg{}
		}
		event, ok := <-ch
		if !ok {
			return chatDoneMsg{}
		}
		return chatEventMsg{event: event}
	}
}

func newSelectorList() list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(memohPrimary).
		BorderForeground(memohPrimary).
		Bold(true)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color("252"))
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.Foreground(mutedTitle)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)
	l.Styles.NoItems = lipgloss.NewStyle().Foreground(mutedTitle)
	return l
}

func (m *TUIModel) syncBotList() {
	selectedID := m.currentBotID()
	items := make([]list.Item, 0, len(m.bots))
	selectedIdx := 0
	for i, item := range m.bots {
		entry := selectorItem{
			id:    item.ID,
			title: item.DisplayName,
		}
		items = append(items, entry)
		if item.ID == selectedID {
			selectedIdx = i
		}
	}
	m.botList.SetItems(items)
	if len(items) > 0 {
		m.botList.Select(selectedIdx)
	}
}

func (m *TUIModel) syncSessionList() {
	selectedID := m.currentSessionID()
	items := make([]list.Item, 0, len(m.sessions))
	selectedIdx := 0
	for i, item := range m.sessions {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = item.ID
		}
		entry := selectorItem{
			id:    item.ID,
			title: title,
		}
		items = append(items, entry)
		if item.ID == selectedID {
			selectedIdx = i
		}
	}
	m.sessList.SetItems(items)
	if len(items) > 0 {
		m.sessList.Select(selectedIdx)
	}
}

func (m *TUIModel) currentBotID() string {
	item, ok := m.botList.SelectedItem().(selectorItem)
	if !ok {
		return ""
	}
	return item.id
}

func (m *TUIModel) currentSessionID() string {
	item, ok := m.sessList.SelectedItem().(selectorItem)
	if !ok {
		return ""
	}
	return item.id
}

func (m *TUIModel) appendTranscript(line string) {
	if strings.TrimSpace(m.chatContent) == "" {
		m.chatContent = line
	} else {
		m.chatContent += "\n\n---\n\n" + line
	}
	m.syncViewport(true)
}

func (m *TUIModel) updateStreamPreview(msg conversation.UIMessage) {
	if _, ok := m.streamPreviewItems[msg.ID]; !ok {
		m.streamPreviewOrder = append(m.streamPreviewOrder, msg.ID)
	}
	m.streamPreviewItems[msg.ID] = msg
	parts := make([]string, 0, len(m.streamPreviewOrder))
	for _, id := range m.streamPreviewOrder {
		item, ok := m.streamPreviewItems[id]
		if !ok {
			continue
		}
		rendered := strings.TrimSpace(renderStreamPreviewMessage(item))
		if rendered == "" {
			continue
		}
		parts = append(parts, rendered)
	}
	m.streamPreview = strings.Join(parts, "\n\n")
}

func (m *TUIModel) clearStreamPreview() {
	m.streamPreview = ""
	m.streamPreviewOrder = nil
	m.streamPreviewItems = map[int]conversation.UIMessage{}
}

func renderStreamPreviewMessage(msg conversation.UIMessage) string {
	switch msg.Type {
	case conversation.UIMessageText:
		return strings.TrimSpace(msg.Content)
	case conversation.UIMessageReasoning:
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return ""
		}
		return "[reasoning]\n" + content
	case conversation.UIMessageTool:
		state := "done"
		if msg.Running != nil && *msg.Running {
			state = "running"
		}
		return fmt.Sprintf("[tool:%s %s]", strings.TrimSpace(msg.Name), state)
	case conversation.UIMessageAttachments:
		return fmt.Sprintf("[attachments:%d]", len(msg.Attachments))
	default:
		return strings.TrimSpace(msg.Content)
	}
}

func renderTurnMarkdown(turn conversation.UITurn) string {
	header := "Assistant"
	if strings.EqualFold(turn.Role, "user") {
		header = "You"
	}
	if turn.SenderDisplayName != "" {
		header = turn.SenderDisplayName
	}
	body := strings.TrimSpace(turn.Text)
	if body == "" && len(turn.Messages) > 0 {
		parts := make([]string, 0, len(turn.Messages))
		for _, msg := range turn.Messages {
			parts = append(parts, RenderUIMessageMarkdown(msg))
		}
		body = strings.Join(parts, "\n")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "_No content_"
	}
	return fmt.Sprintf("## %s\n\n%s", header, body)
}

func RenderUIMessage(msg conversation.UIMessage) string {
	return renderMarkdownToANSI(RenderUIMessageMarkdown(msg), 0)
}

func RenderUIMessageMarkdown(msg conversation.UIMessage) string {
	switch msg.Type {
	case conversation.UIMessageText:
		return strings.TrimSpace(msg.Content)
	case conversation.UIMessageReasoning:
		return fmt.Sprintf("> Reasoning\n>\n> %s", strings.ReplaceAll(strings.TrimSpace(msg.Content), "\n", "\n> "))
	case conversation.UIMessageTool:
		state := "done"
		if msg.Running != nil && *msg.Running {
			state = "running"
		}
		return fmt.Sprintf("**Tool:** `%s` (%s)", strings.TrimSpace(msg.Name), state)
	case conversation.UIMessageAttachments:
		return fmt.Sprintf("**Attachments:** %d", len(msg.Attachments))
	default:
		return strings.TrimSpace(msg.Content)
	}
}

func (m *TUIModel) syncViewport(gotoBottom bool) {
	base := renderMarkdownToANSI(m.chatContent, m.viewport.Width)
	preview := strings.TrimSpace(m.streamPreview)
	content := ""
	switch {
	case base == "":
		content = preview
	case preview == "":
		content = base
	default:
		content = base + "\n\n" + preview
	}
	m.viewportContent = content
	m.viewport.SetContent(content)
	if gotoBottom {
		m.viewport.GotoBottom()
	}
}

func (m *TUIModel) renderChatViewport() string {
	view := m.viewport.View()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	height := max(1, m.viewport.Height)
	if len(lines) < height {
		padded := make([]string, height)
		copy(padded, lines)
		for i := len(lines); i < height; i++ {
			padded[i] = ""
		}
		lines = padded
	} else if len(lines) > height {
		lines = lines[:height]
	}

	totalLines := 0
	if strings.TrimSpace(m.viewportContent) != "" {
		totalLines = len(strings.Split(m.viewportContent, "\n"))
	}
	if totalLines <= height {
		return strings.Join(lines, "\n")
	}

	thumbHeight := max(1, int(math.Round(float64(height*height)/float64(totalLines))))
	maxOffset := max(1, totalLines-height)
	thumbTop := int(math.Round(float64(m.viewport.YOffset) / float64(maxOffset) * float64(height-thumbHeight)))
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop > height-thumbHeight {
		thumbTop = height - thumbHeight
	}

	railStyle := lipgloss.NewStyle().Foreground(mutedTitle)
	thumbStyle := lipgloss.NewStyle().Foreground(memohPrimary).Bold(true)
	withBar := make([]string, 0, len(lines))
	for i, line := range lines {
		bar := railStyle.Render("│")
		if i >= thumbTop && i < thumbTop+thumbHeight {
			bar = thumbStyle.Render("█")
		}
		withBar = append(withBar, line+" "+bar)
	}
	return strings.Join(withBar, "\n")
}

func renderMarkdownToANSI(markdown string, width int) string {
	if strings.TrimSpace(markdown) == "" {
		return ""
	}
	opts := []glamour.TermRendererOption{
		glamour.WithStandardStyle("dark"),
	}
	if width > 0 {
		opts = append(opts, glamour.WithWordWrap(width))
	}
	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return markdown
	}
	out, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return strings.TrimRight(out, "\n")
}

func nextFocus(current focusArea) focusArea {
	return (current + 1) % 4
}

func prevFocus(current focusArea) focusArea {
	if current == 0 {
		return 3
	}
	return current - 1
}

func panel(title, body string, focused bool, width int) string {
	borderColor := mutedBorder
	titleColor := mutedTitle
	if focused {
		borderColor = memohPrimary
		titleColor = memohPrimary
	}

	titleLine := lipgloss.NewStyle().
		Bold(focused).
		Foreground(titleColor).
		Render(title)

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width).
		Padding(0, 1)
	return style.Render(titleLine + "\n" + body)
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

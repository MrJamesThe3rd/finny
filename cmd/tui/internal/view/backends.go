package view

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/document"
)

type backendState int

const (
	backendStateList backendState = iota
	backendStateAdding
)

type BackendsModel struct {
	CommonModel
	docService *document.Service

	state    backendState
	backends []document.BackendConfig
	cursor   int
	form     *huh.Form
	status   string
	err      error

	// Form bindings
	formType    string
	formName    string
	formBaseURL string
	formToken   string
	formPath    string
}

func NewBackendsModel(baseCtx context.Context, docSvc *document.Service) BackendsModel {
	return BackendsModel{
		CommonModel: CommonModel{baseCtx: baseCtx},
		docService:  docSvc,
	}
}

func (m BackendsModel) Title() string { return "Manage Backends" }

func (m BackendsModel) ShortHelp() string {
	switch m.state {
	case backendStateAdding:
		return "Esc: cancel | Enter/Tab: navigate form"
	}

	return "Esc: back | a: add | d: delete | space: toggle enabled"
}

func (m BackendsModel) Init() tea.Cmd {
	return m.loadBackendsCmd()
}

func (m BackendsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadBackendsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.backends = msg.backends
		m.err = nil
		return m, nil

	case backendSaveMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.status = "Saved."
		}
		m.state = backendStateList
		m.form = nil
		return m, m.loadBackendsCmd()
	}

	switch m.state {
	case backendStateList:
		return m.updateList(msg)
	case backendStateAdding:
		return m.updateAdding(msg)
	}

	return m, nil
}

func (m BackendsModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "esc":
		return m, Back
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.backends)-1 {
			m.cursor++
		}
	case "a":
		return m.startAdding()
	case "d":
		return m, m.deleteBackendCmd()
	case " ":
		return m, m.toggleEnabledCmd()
	}

	return m, nil
}

func (m BackendsModel) startAdding() (tea.Model, tea.Cmd) {
	m.formType = "local"
	m.formName = ""
	m.formBaseURL = ""
	m.formToken = ""
	m.formPath = ""

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("type").
				Title("Backend Type").
				Options(
					huh.NewOption("Local filesystem", "local"),
					huh.NewOption("Paperless-ngx", "paperless"),
				).
				Value(&m.formType),

			huh.NewInput().
				Key("name").
				Title("Name").
				Placeholder("e.g. My Documents").
				Value(&m.formName).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().
				Key("base_url").
				Title("Paperless Base URL").
				Placeholder("https://paperless.example.com").
				Value(&m.formBaseURL),

			huh.NewInput().
				Key("token").
				Title("Paperless API Token").
				Value(&m.formToken),
		).WithHideFunc(func() bool { return m.formType != "paperless" }),
		huh.NewGroup(
			huh.NewInput().
				Key("path").
				Title("Base Path").
				Placeholder("/home/user/documents").
				Value(&m.formPath).
				Validate(func(s string) error {
					if m.formType == "local" && strings.TrimSpace(s) == "" {
						return fmt.Errorf("base path is required for local backend")
					}
					return nil
				}),
		).WithHideFunc(func() bool { return m.formType != "local" }),
	).WithWidth(55).WithShowHelp(false)

	m.state = backendStateAdding

	return m, m.form.Init()
}

func (m BackendsModel) updateAdding(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			m.state = backendStateList
			m.form = nil
			return m, nil
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State != huh.StateCompleted {
		return m, cmd
	}

	return m, m.createBackendCmd()
}

func (m BackendsModel) View() string {
	switch m.state {
	case backendStateAdding:
		if m.form != nil {
			return lipgloss.NewStyle().Padding(1).Render(m.form.View())
		}
	case backendStateList:
		return m.viewList()
	}

	return ""
}

func (m BackendsModel) viewList() string {
	if m.err != nil {
		return lipgloss.NewStyle().Padding(2).Render(fmt.Sprintf("Error: %v", m.err))
	}

	var sb strings.Builder

	if len(m.backends) == 0 {
		sb.WriteString("No backends configured.\n\nPress 'a' to add one.\n")
	} else {
		for i, b := range m.backends {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			check := "[ ]"
			if b.Enabled {
				check = "[✓]"
			}

			line := fmt.Sprintf("%s%s %s (%s)", cursor, check, b.Name, b.Type)
			if i == m.cursor {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(line)
			}

			sb.WriteString(line + "\n")
		}
	}

	if m.status != "" {
		sb.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(m.status))
	}

	return lipgloss.NewStyle().Padding(1).Render(sb.String())
}

// Messages

type loadBackendsMsg struct {
	backends []document.BackendConfig
	err      error
}

type backendSaveMsg struct {
	err error
}

func (m BackendsModel) loadBackendsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := DbCtx(m.baseCtx)
		defer cancel()

		backends, err := m.docService.ListBackends(ctx)
		return loadBackendsMsg{backends: backends, err: err}
	}
}

func (m BackendsModel) createBackendCmd() tea.Cmd {
	backendType := m.form.GetString("type")
	name := m.form.GetString("name")
	baseURL := m.form.GetString("base_url")
	token := m.form.GetString("token")
	path := m.form.GetString("path")
	docSvc := m.docService
	baseCtx := m.baseCtx

	return func() tea.Msg {
		ctx, cancel := DbCtx(baseCtx)
		defer cancel()

		var config json.RawMessage
		var err error

		switch backendType {
		case "paperless":
			config, err = json.Marshal(map[string]string{
				"base_url": baseURL,
				"token":    token,
			})
		case "local":
			config, err = json.Marshal(map[string]string{
				"base_path": path,
			})
		}

		if err != nil {
			return backendSaveMsg{err: fmt.Errorf("building config: %w", err)}
		}

		cfg := &document.BackendConfig{
			Type:    backendType,
			Name:    name,
			Config:  config,
			Enabled: true,
		}

		if err := docSvc.CreateBackend(ctx, cfg); err != nil {
			return backendSaveMsg{err: err}
		}

		return backendSaveMsg{}
	}
}

func (m BackendsModel) deleteBackendCmd() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.backends) {
		return nil
	}

	id := m.backends[m.cursor].ID
	docSvc := m.docService
	baseCtx := m.baseCtx

	return func() tea.Msg {
		ctx, cancel := DbCtx(baseCtx)
		defer cancel()

		if err := docSvc.DeleteBackend(ctx, id); err != nil {
			return backendSaveMsg{err: err}
		}

		return backendSaveMsg{}
	}
}

func (m BackendsModel) toggleEnabledCmd() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.backends) {
		return nil
	}

	b := m.backends[m.cursor]
	newEnabled := !b.Enabled
	docSvc := m.docService
	baseCtx := m.baseCtx

	return func() tea.Msg {
		ctx, cancel := DbCtx(baseCtx)
		defer cancel()

		if err := docSvc.UpdateBackend(ctx, b.ID, nil, nil, &newEnabled); err != nil {
			return backendSaveMsg{err: err}
		}

		return backendSaveMsg{}
	}
}

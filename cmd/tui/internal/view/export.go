package view

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/export"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type exportState int

const (
	exportStateTimeframe exportState = iota
	exportStatePath
	exportStateExporting
	exportStateResult
)

type ExportModel struct {
	CommonModel
	exportService *export.Service

	state           exportState
	err             error
	timeframePicker TimeframePicker

	startDate time.Time
	endDate   time.Time
	allTime   bool

	form    *huh.Form
	path    string
	spinner spinner.Model
	summary string
}

func NewExportModel(svc *export.Service) ExportModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return ExportModel{
		exportService:   svc,
		state:           exportStateTimeframe,
		timeframePicker: NewTimeframePicker(TimeframeThisMonth),
		path:            "./exports",
		spinner:         s,
	}
}

func (m ExportModel) Title() string { return "Export Transactions" }

func (m ExportModel) ShortHelp() string {
	switch m.state {
	case exportStateResult:
		return "Esc: back to menu"
	case exportStateExporting:
		return "Exporting..."
	}
	return "Esc: back | Enter: confirm"
}

func (m ExportModel) Init() tea.Cmd {
	return nil
}

func (m ExportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if tfMsg, ok := msg.(TimeframeSelectedMsg); ok {
		m.startDate = tfMsg.Start
		m.endDate = tfMsg.End
		m.allTime = tfMsg.All
		m.form = m.buildPathForm()
		m.state = exportStatePath
		return m, m.form.Init()
	}

	switch m.state {
	case exportStateTimeframe:
		return m.updateTimeframe(msg)
	case exportStatePath:
		return m.updatePath(msg)
	case exportStateExporting:
		return m.updateExporting(msg)
	case exportStateResult:
		return m.updateResult(msg)
	}

	return m, nil
}

func (m ExportModel) updateTimeframe(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc && m.timeframePicker.IsSelecting() {
			return m, Back
		}
	}

	var cmd tea.Cmd
	m.timeframePicker, cmd = m.timeframePicker.Update(msg)
	return m, cmd
}

func (m ExportModel) updatePath(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			m.state = exportStateTimeframe
			m.timeframePicker.Reset()
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

	m.state = exportStateExporting
	m.err = nil
	return m, tea.Batch(m.spinner.Tick, m.runExportCmd(m.startDate, m.endDate, m.path))
}

func (m ExportModel) updateExporting(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(exportResultMsg); ok {
		m.state = exportStateResult
		if result.err != nil {
			m.err = result.err
		}
		m.summary = result.body
		return m, nil
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m ExportModel) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			return m, Back
		}
	}
	return m, nil
}

func (m ExportModel) buildPathForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("path").
				Title("Output Path").
				Description("Directory will be created if it doesn't exist").
				Placeholder("./exports").
				Value(&m.path),
		),
	).WithWidth(50).WithShowHelp(false)
}

func (m ExportModel) View() string {
	switch m.state {
	case exportStateTimeframe:
		return lipgloss.NewStyle().Padding(1).Render(m.timeframePicker.View())

	case exportStatePath:
		return lipgloss.NewStyle().Padding(1).Render(m.form.View())

	case exportStateExporting:
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf("%s Exporting transactions and downloading invoices...", m.spinner.View()),
		)

	case exportStateResult:
		return m.viewResult()
	}

	return ""
}

func (m ExportModel) viewResult() string {
	if m.err != nil {
		return lipgloss.NewStyle().Padding(1).Render(
			lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v", m.err)),
		)
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("46")).
		Render("Export Complete!")

	return lipgloss.NewStyle().Padding(1).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			header,
			"",
			"Summary:",
			"",
			m.summary,
		),
	)
}

type exportResultMsg struct {
	body string
	err  error
}

const exportTimeout = 2 * time.Minute

func (m ExportModel) runExportCmd(start, end time.Time, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
		defer cancel()

		filter := transaction.ListFilter{}
		if !m.allTime {
			filter.StartDate = &start
			filter.EndDate = &end
		}

		items, err := m.exportService.Export(ctx, filter, path)
		if err != nil {
			return exportResultMsg{err: err}
		}

		body := m.exportService.GenerateSummary(items)
		return exportResultMsg{body: body}
	}
}

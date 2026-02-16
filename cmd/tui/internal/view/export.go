package view

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/export"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type exportState int

const (
	stateSelectTimeframe exportState = iota
	stateInputCustomDate
	stateInputPath
	stateExporting
	stateResult
)

type ExportModel struct {
	CommonModel
	exportService *export.Service

	state             exportState
	err               error
	selectedTimeframe Timeframe

	// Custom Date Inputs
	startDateInput textinput.Model
	endDateInput   textinput.Model
	dateFocusIndex int

	// Path Input
	pathInput textinput.Model

	spinner spinner.Model

	emailBody string
}

func NewExportModel(svc *export.Service) ExportModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	si := textinput.New()
	si.Placeholder = "YYYY-MM-DD"
	si.Width = 20
	si.Prompt = "Start Date: "

	ei := textinput.New()
	ei.Placeholder = "YYYY-MM-DD"
	ei.Width = 20
	ei.Prompt = "End Date:   "

	pi := textinput.New()
	pi.Placeholder = "./exports"
	pi.Width = 30
	pi.SetValue("./exports")
	pi.Prompt = "Output Path: "
	pi.Focus()

	return ExportModel{
		exportService:     svc,
		state:             stateSelectTimeframe,
		selectedTimeframe: TimeframeThisMonth,
		startDateInput:    si,
		endDateInput:      ei,
		pathInput:         pi,
		spinner:           s,
		dateFocusIndex:    0,
	}
}

func (m ExportModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ExportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Clear temporary errors on user interaction in input states
	if m.err != nil && m.state != stateExporting && m.state != stateResult {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.err = nil
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.state == stateResult {
				m.state = stateSelectTimeframe
				m.err = nil
				m.emailBody = ""

				return m, Back
			}

			if m.state == stateInputPath {
				if m.selectedTimeframe == TimeframeCustom {
					m.state = stateInputCustomDate
					return m, nil
				}

				m.state = stateSelectTimeframe

				return m, nil
			}

			if m.state == stateInputCustomDate {
				m.state = stateSelectTimeframe
				return m, nil
			}

			return m, Back
		}

		// Specific state handling
		switch m.state {
		case stateSelectTimeframe:
			switch msg.String() {
			case "up":
				if m.selectedTimeframe > 0 {
					m.selectedTimeframe--
				}
			case "down":
				if m.selectedTimeframe < TimeframeCustom {
					m.selectedTimeframe++
				}
			case "enter":
				if m.selectedTimeframe == TimeframeCustom {
					m.state = stateInputCustomDate
					m.startDateInput.Focus()
					m.dateFocusIndex = 0

					return m, textinput.Blink
				}
				// Go to path input
				m.state = stateInputPath
				m.pathInput.Focus()

				return m, textinput.Blink
			}

		case stateInputCustomDate:
			switch msg.String() {
			case "tab", "shift+tab", "up", "down":
				m.dateFocusIndex = (m.dateFocusIndex + 1) % 2
				if m.dateFocusIndex == 0 {
					m.startDateInput.Focus()
					m.endDateInput.Blur()
				} else {
					m.startDateInput.Blur()
					m.endDateInput.Focus()
				}

				return m, textinput.Blink
			case "enter":
				// Validate dates before moving on?
				// Simple validation check
				if _, err := time.Parse("2006-01-02", m.startDateInput.Value()); err != nil {
					m.err = fmt.Errorf("invalid start date: %v", err)
					return m, nil
				}

				if _, err := time.Parse("2006-01-02", m.endDateInput.Value()); err != nil {
					m.err = fmt.Errorf("invalid end date: %v", err)
					return m, nil
				}

				m.state = stateInputPath
				m.pathInput.Focus()

				return m, textinput.Blink
			}

		case stateInputPath:
			switch msg.String() {
			case "enter":
				return m.submit()
			}
		}

	case exportResultMsg:
		m.state = stateResult
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.emailBody = msg.body
		}

		return m, nil
	}

	var cmd tea.Cmd

	switch m.state {
	case stateInputCustomDate:
		var cmds []tea.Cmd

		var c tea.Cmd
		m.startDateInput, c = m.startDateInput.Update(msg)
		cmds = append(cmds, c)
		m.endDateInput, c = m.endDateInput.Update(msg)
		cmds = append(cmds, c)
		cmd = tea.Batch(cmds...)
	case stateInputPath:
		m.pathInput, cmd = m.pathInput.Update(msg)
	case stateExporting:
		m.spinner, cmd = m.spinner.Update(msg)
	}

	return m, cmd
}

func (m ExportModel) View() string {
	if m.state != stateSelectTimeframe && m.state != stateInputCustomDate && m.state != stateInputPath && m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress Esc to back.", m.err)
	}

	errStr := ""
	if m.err != nil {
		errStr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("\n\nError: %v", m.err))
	}

	switch m.state {
	case stateSelectTimeframe:
		s := "Export Transactions\nSelect Timeframe:\n\n"

		for i := TimeframeThisMonth; i <= TimeframeCustom; i++ {
			cursor := " "
			if m.selectedTimeframe == i {
				cursor = ">"
			}

			s += fmt.Sprintf("%s %s\n", cursor, i.String())
		}

		s += "\n(Enter to select, Esc to back)"

		return lipgloss.NewStyle().Padding(1).Render(s + errStr)

	case stateInputCustomDate:
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf(
				"Enter Custom Range:\n\n%s\n%s\n\n(Enter to continue, Esc to back)%s",
				m.startDateInput.View(),
				m.endDateInput.View(),
				errStr,
			),
		)

	case stateInputPath:
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf(
				"Enter Output Path:\n\n%s\n\n(Enter to Start Export, Esc to back)%s",
				m.pathInput.View(),
				errStr,
			),
		)

	case stateExporting:
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf("%s Exporting transactions and downloading invoices...", m.spinner.View()),
		)

	case stateResult:
		return lipgloss.NewStyle().Padding(1).Render(
			fmt.Sprintf(
				"Export Complete!\n\nEmail Body:\n\n%s\n\n(Press Esc to return to menu)",
				m.emailBody,
			),
		)
	}

	return ""
}

func (m *ExportModel) submit() (tea.Model, tea.Cmd) {
	var start, end time.Time

	switch m.selectedTimeframe {
	case TimeframeAll:
		break
	case TimeframeCustom:
		var err error

		start, err = time.Parse("2006-01-02", m.startDateInput.Value())
		if err != nil {
			m.err = fmt.Errorf("invalid start date: %v", err)
			return *m, nil
		}

		end, err = time.Parse("2006-01-02", m.endDateInput.Value())
		if err != nil {
			m.err = fmt.Errorf("invalid end date: %v", err)
			return *m, nil
		}
	default:
		start, end = TimeframeToDateRange(m.selectedTimeframe)
	}

	if m.selectedTimeframe != TimeframeAll {
		start, end = NormalizeDateRange(start, end)
	}

	pathStr := m.pathInput.Value()
	m.state = stateExporting

	return *m, tea.Batch(m.spinner.Tick, m.runExportCmd(start, end, pathStr))
}

type exportResultMsg struct {
	body string
	err  error
}

func (m ExportModel) runExportCmd(start, end time.Time, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		items, err := m.exportService.Export(ctx, transaction.ListFilter{
			StartDate: &start,
			EndDate:   &end,
		}, path)
		if err != nil {
			return exportResultMsg{err: err}
		}

		body := m.exportService.GenerateEmailBody(items)

		return exportResultMsg{body: body}
	}
}

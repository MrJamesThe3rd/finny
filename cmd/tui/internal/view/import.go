package view

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/importer"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type ImportModel struct {
	CommonModel
	txService     *transaction.Service
	importService *importer.Service

	pathInput textinput.Model
	status    string
	err       error
}

func NewImportModel(txSvc *transaction.Service, impSvc *importer.Service) ImportModel {
	ti := textinput.New()
	ti.Placeholder = "/path/to/bank_export.xlsx"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 50

	return ImportModel{
		txService:     txSvc,
		importService: impSvc,
		pathInput:     ti,
		status:        "Enter file path to import:",
	}
}

func (m ImportModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ImportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return m, Back
		case tea.KeyEnter:
			path := m.pathInput.Value()
			if path == "" {
				m.status = "Please enter a valid path."
				return m, nil
			}
			// Trigger import
			return m, m.importCmd(path)
		}
	case importResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %v", msg.err)
			m.err = msg.err
		} else {
			m.status = fmt.Sprintf("Success! Imported %d transactions.", msg.count)
			m.pathInput.SetValue("")
		}

		return m, nil
	}

	m.pathInput, cmd = m.pathInput.Update(msg)

	return m, cmd
}

func (m ImportModel) View() string {
	return lipgloss.NewStyle().Padding(2).Render(
		fmt.Sprintf("Import Transactions (CGD)\n\n%s\n\n%s\n\n(Esc to back, Enter to import)",
			m.status,
			m.pathInput.View(),
		),
	)
}

type importResultMsg struct {
	count int
	err   error
}

func (m ImportModel) importCmd(path string) tea.Cmd {
	return func() tea.Msg {
		f, err := os.Open(path)
		if err != nil {
			return importResultMsg{err: err}
		}

		defer func() {
			_ = f.Close()
		}()

		// Hardcoded to CGD for now
		params, err := m.importService.Import(importer.BankCGD, f)
		if err != nil {
			return importResultMsg{err: err}
		}

		ctx := context.Background()
		count := 0

		for _, param := range params {
			_, err := m.txService.Create(ctx, param)
			if err != nil {
				// Continue on error? or stop?
				// For now stop
				return importResultMsg{err: err}
			}

			count++
		}

		return importResultMsg{count: count}
	}
}

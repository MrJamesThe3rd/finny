package view

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/importer"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

const importTimeout = 2 * time.Minute

type importState int

const (
	importStateBankSelect importState = iota
	importStateFilePick
	importStateImporting
	importStateResult
)

type ImportModel struct {
	CommonModel
	txService     *transaction.Service
	importService *importer.Service

	state        importState
	filePicker   filepicker.Model
	selectedBank importer.Bank
	bankOptions  []importer.Bank
	bankCursor   int

	status string
	err    error
}

func NewImportModel(txSvc *transaction.Service, impSvc *importer.Service) ImportModel {
	fp := filepicker.New()
	fp.CurrentDirectory, _ = os.Getwd()
	fp.ShowHidden = false
	fp.DirAllowed = false
	fp.FileAllowed = true
	fp.SetHeight(15)

	return ImportModel{
		txService:     txSvc,
		importService: impSvc,
		filePicker:    fp,
		bankOptions:   []importer.Bank{importer.BankCGD},
	}
}

func (m ImportModel) Title() string     { return "Import Transactions" }
func (m ImportModel) ShortHelp() string { return "Esc: back | Enter: select" }

func (m ImportModel) Init() tea.Cmd {
	return m.filePicker.Init()
}

func (m ImportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			return m.handleEsc()
		}

		if m.state == importStateBankSelect {
			return m.updateBankSelect(msg)
		}

	case importResultMsg:
		m.state = importStateResult
		if msg.err != nil {
			m.err = msg.err
			m.status = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.status = fmt.Sprintf("Imported %d transactions.", msg.count)
		return m, nil
	}

	if m.state != importStateFilePick {
		return m, nil
	}

	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)

	if didSelect, path := m.filePicker.DidSelectFile(msg); didSelect {
		m.state = importStateImporting
		m.status = fmt.Sprintf("Importing from %s...", path)
		return m, m.importCmd(path)
	}

	return m, cmd
}

func (m ImportModel) handleEsc() (tea.Model, tea.Cmd) {
	switch m.state {
	case importStateFilePick:
		m.state = importStateBankSelect
		return m, nil
	case importStateResult:
		m.state = importStateBankSelect
		m.err = nil
		m.status = ""
		return m, nil
	}
	return m, Back
}

func (m ImportModel) updateBankSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.bankCursor > 0 {
			m.bankCursor--
		}
	case tea.KeyDown:
		if m.bankCursor < len(m.bankOptions)-1 {
			m.bankCursor++
		}
	case tea.KeyEnter:
		m.selectedBank = m.bankOptions[m.bankCursor]
		m.state = importStateFilePick
		return m, m.filePicker.Init()
	}
	return m, nil
}

func (m ImportModel) View() string {
	switch m.state {
	case importStateBankSelect:
		return m.viewBankSelect()
	case importStateFilePick:
		return m.viewFilePick()
	case importStateImporting:
		return lipgloss.NewStyle().Padding(2).Render(m.status)
	case importStateResult:
		return m.viewResult()
	}
	return ""
}

func (m ImportModel) viewBankSelect() string {
	s := "Select Bank:\n\n"
	for i, bank := range m.bankOptions {
		cursor := " "
		if i == m.bankCursor {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %s\n", cursor, string(bank))
	}
	return lipgloss.NewStyle().Padding(2).Render(s)
}

func (m ImportModel) viewFilePick() string {
	return lipgloss.NewStyle().Padding(1).Render(
		fmt.Sprintf("Select file to import (%s):\n\n%s", m.selectedBank, m.filePicker.View()),
	)
}

func (m ImportModel) viewResult() string {
	style := lipgloss.NewStyle().Padding(2)
	if m.err != nil {
		return style.Render(
			lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.status) +
				"\n\n(Esc to go back)",
		)
	}
	return style.Render(
		lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(m.status) +
			"\n\n(Esc to go back)",
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
		defer f.Close()

		params, err := m.importService.Import(m.selectedBank, f)
		if err != nil {
			return importResultMsg{err: err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), importTimeout)
		defer cancel()

		count := 0
		for _, param := range params {
			_, err := m.txService.Create(ctx, param)
			if err != nil {
				return importResultMsg{count: count, err: fmt.Errorf("imported %d/%d: %w", count, len(params), err)}
			}
			count++
		}

		return importResultMsg{count: count}
	}
}

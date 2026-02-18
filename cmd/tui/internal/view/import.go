package view

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
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
	importStateConflicts
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

	newParams    []transaction.CreateParams
	conflicts    []transaction.Conflict
	conflictList list.Model
	selected     map[int]bool

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
		selected:      make(map[int]bool),
	}
}

func (m ImportModel) Title() string { return "Import Transactions" }

func (m ImportModel) ShortHelp() string {
	switch m.state {
	case importStateConflicts:
		return "Space: toggle | a: all | n: none | Enter: confirm | Esc: cancel"
	}

	return "Esc: back | Enter: select"
}

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

		if m.state == importStateConflicts {
			return m.updateConflicts(msg)
		}

	case importResultMsg:
		if msg.err != nil {
			m.state = importStateResult
			m.err = msg.err
			m.status = fmt.Sprintf("Error: %v", msg.err)

			return m, nil
		}

		if len(msg.result.Conflicts) == 0 {
			m.state = importStateResult
			m.status = fmt.Sprintf("Imported %d transactions.", len(msg.result.Imported))

			return m, nil
		}

		m.newParams = msg.result.New
		m.conflicts = msg.result.Conflicts
		m.selected = make(map[int]bool)
		m.state = importStateConflicts

		items := make([]list.Item, len(m.conflicts))
		for i, c := range m.conflicts {
			items[i] = conflictItem{conflict: c, index: i}
		}

		delegate := conflictDelegate{selected: &m.selected}
		m.conflictList = list.New(items, delegate, 80, 20)
		m.conflictList.Title = "Duplicate Conflicts"
		m.conflictList.SetShowStatusBar(false)
		m.conflictList.SetFilteringEnabled(false)
		m.conflictList.SetShowHelp(false)

		return m, nil

	case confirmResultMsg:
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
	case importStateConflicts:
		m.state = importStateBankSelect
		m.conflicts = nil
		m.newParams = nil
		m.selected = make(map[int]bool)

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

func (m ImportModel) updateConflicts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case " ":
		idx := m.conflictList.Index()
		m.selected[idx] = !m.selected[idx]

		return m, nil
	case "a":
		for i := range m.conflicts {
			m.selected[i] = true
		}

		return m, nil
	case "n":
		for i := range m.conflicts {
			m.selected[i] = false
		}

		return m, nil
	case "enter":
		return m, m.confirmCmd()
	}

	var cmd tea.Cmd
	m.conflictList, cmd = m.conflictList.Update(msg)

	return m, cmd
}

func (m ImportModel) View() string {
	switch m.state {
	case importStateBankSelect:
		return m.viewBankSelect()
	case importStateFilePick:
		return m.viewFilePick()
	case importStateImporting:
		return lipgloss.NewStyle().Padding(2).Render(m.status)
	case importStateConflicts:
		return lipgloss.NewStyle().Padding(1).Render(m.conflictList.View())
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

// Messages

type importResultMsg struct {
	result *transaction.ImportResult
	err    error
}

type confirmResultMsg struct {
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

		result, err := m.txService.ImportBatch(ctx, params)
		if err != nil {
			return importResultMsg{err: err}
		}

		return importResultMsg{result: result}
	}
}

func (m ImportModel) confirmCmd() tea.Cmd {
	newParams := m.newParams
	conflicts := m.conflicts
	selected := m.selected

	return func() tea.Msg {
		var allParams []transaction.CreateParams
		allParams = append(allParams, newParams...)

		for i, c := range conflicts {
			if !selected[i] {
				continue
			}

			allParams = append(allParams, c.Incoming)
		}

		ctx, cancel := context.WithTimeout(context.Background(), importTimeout)
		defer cancel()

		txs, err := m.txService.CreateBatch(ctx, allParams)
		if err != nil {
			return confirmResultMsg{err: err}
		}

		return confirmResultMsg{count: len(txs)}
	}
}

// Conflict list item

type conflictItem struct {
	conflict transaction.Conflict
	index    int
}

func (i conflictItem) Title() string       { return "" }
func (i conflictItem) Description() string { return "" }
func (i conflictItem) FilterValue() string { return "" }

// Conflict list delegate

type conflictDelegate struct {
	selected *map[int]bool
}

func (d conflictDelegate) Height() int                             { return 3 }
func (d conflictDelegate) Spacing() int                            { return 0 }
func (d conflictDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d conflictDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(conflictItem)
	if !ok {
		return
	}

	checkbox := "[ ]"
	if (*d.selected)[item.index] {
		checkbox = "[x]"
	}

	cursor := "  "
	if index == m.Index() {
		cursor = "> "
	}

	incoming := item.conflict.Incoming
	existing := item.conflict.Existing

	line1 := fmt.Sprintf("%s%s %s  %s  %s",
		cursor, checkbox,
		FormatDate(incoming.Date),
		FormatAmount(incoming.Amount),
		incoming.Description,
	)

	line2 := fmt.Sprintf("      Existing: %s  %s  %s [%s]",
		FormatDate(existing.Date),
		FormatAmount(existing.Amount),
		existing.Description,
		existing.Status,
	)

	fmt.Fprintf(w, "%s\n%s\n", line1, line2)
}

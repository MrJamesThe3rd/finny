package view

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/document"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type listState int

const (
	listStateBrowse listState = iota
	listStateEdit
	listStateFilePick
)

type ListModel struct {
	CommonModel
	txService  *transaction.Service
	docService *document.Service

	state      listState
	table      table.Model
	filePicker filepicker.Model
	txs        []*transaction.Transaction
	form       *huh.Form

	// Filter cycling
	statusFilterIdx int
	dateFilterIdx   int

	filter  transaction.ListFilter
	loading bool
	err     error
	status  string

	// Form bindings
	formDesc      string
	formDocAction string
	editIdx       int // index of the transaction being edited
}

func NewListModel(baseCtx context.Context, txSvc *transaction.Service, docSvc *document.Service) ListModel {
	columns := []table.Column{
		{Title: "Date", Width: 12},
		{Title: "Status", Width: 15},
		{Title: "Amount", Width: 10},
		{Title: "Description", Width: 40},
		{Title: "Document", Width: 40},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	fp := filepicker.New()
	fp.CurrentDirectory, _ = os.Getwd()
	fp.ShowHidden = false
	fp.DirAllowed = false
	fp.FileAllowed = true
	fp.SetHeight(15)

	return ListModel{
		CommonModel: CommonModel{baseCtx: baseCtx},
		txService:   txSvc,
		docService:  docSvc,
		table:       t,
		filePicker:  fp,
		filter:      transaction.ListFilter{},
		editIdx:     -1,
	}
}

func (m ListModel) Title() string { return "Transactions List" }
func (m ListModel) ShortHelp() string {
	switch m.state {
	case listStateEdit:
		return "Navigate form | Esc: cancel"
	case listStateFilePick:
		return "Esc: cancel | Enter: select file"
	}

	return "Esc: back | e: edit | s: status filter | d: date filter | r: refresh"
}

func (m ListModel) Init() tea.Cmd {
	return m.loadTxsCmd()
}

func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadListMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		m.txs = msg.txs
		m.status = ""
		m.refreshTable()

		return m, nil

	case listSaveMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error saving: %v", msg.err)
		}

		m.state = listStateBrowse
		m.form = nil
		m.editIdx = -1
		m.table.Focus()

		return m, m.loadTxsCmd()

	case tea.WindowSizeMsg:
		m.table.SetHeight(msg.Height - 10)
		return m, nil
	}

	switch m.state {
	case listStateBrowse:
		return m.updateBrowse(msg)
	case listStateEdit:
		return m.updateEdit(msg)
	case listStateFilePick:
		return m.updateFilePick(msg)
	}

	return m, nil
}

func (m ListModel) updateBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if ok {
		switch keyMsg.String() {
		case "esc":
			return m, Back
		case "r":
			m.loading = true
			return m, m.loadTxsCmd()
		case "e":
			return m.enterEditMode()
		case "s":
			m.statusFilterIdx = (m.statusFilterIdx + 1) % 5
			m.applyFilter()

			return m, m.loadTxsCmd()
		case "d":
			m.dateFilterIdx = (m.dateFilterIdx + 1) % 3
			m.applyFilter()

			return m, m.loadTxsCmd()
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)

	return m, cmd
}

func (m ListModel) enterEditMode() (tea.Model, tea.Cmd) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.txs) {
		return m, nil
	}

	tx := m.txs[idx]
	m.formDesc = tx.Description
	m.formDocAction = "skip"
	m.editIdx = idx

	var docOptions []huh.Option[string]
	if tx.DocumentID != nil {
		docOptions = []huh.Option[string]{
			huh.NewOption("Keep current document", "skip"),
			huh.NewOption("Replace with new file", "upload"),
			huh.NewOption("Remove document", "remove"),
		}
	} else {
		docOptions = []huh.Option[string]{
			huh.NewOption("Upload file", "upload"),
			huh.NewOption("No invoice needed", "no_invoice"),
			huh.NewOption("Skip (decide later)", "skip"),
		}
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("description").
				Title("Description").
				Value(&m.formDesc).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("description cannot be empty")
					}
					return nil
				}),

			huh.NewSelect[string]().
				Key("document_action").
				Title("Document").
				Options(docOptions...).
				Value(&m.formDocAction),
		),
	).WithWidth(45).WithShowHelp(false)

	m.state = listStateEdit
	m.table.Blur()

	return m, m.form.Init()
}

func (m ListModel) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			m.state = listStateBrowse
			m.form = nil
			m.editIdx = -1
			m.table.Focus()

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

	action := m.form.GetString("document_action")
	desc := m.form.GetString("description")
	m.formDesc = desc
	m.formDocAction = action

	if action == "upload" {
		m.state = listStateFilePick
		return m, m.filePicker.Init()
	}

	return m, m.saveCmd()
}

func (m ListModel) updateFilePick(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			m.state = listStateEdit
			return m, m.form.Init()
		}
	}

	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)

	if didSelect, path := m.filePicker.DidSelectFile(msg); didSelect {
		return m, m.saveWithFileCmd(path)
	}

	return m, cmd
}

func (m ListModel) View() string {
	if m.state == listStateFilePick {
		return lipgloss.NewStyle().Padding(1).Render(
			"Select a file to upload:\n\n" + m.filePicker.View(),
		)
	}

	if m.loading {
		return lipgloss.NewStyle().Padding(2).Render("Loading transactions...")
	}

	if m.err != nil {
		return lipgloss.NewStyle().Padding(2).Render(fmt.Sprintf("Error: %v", m.err))
	}

	statusLabels := []string{"All", "Draft", "Pending", "Complete", "No Invoice"}
	dateLabels := []string{"All Time", "This Month", "Last Month"}

	header := fmt.Sprintf(
		"Filter: [s] Status: %s | [d] Date: %s",
		activeStyle(statusLabels[m.statusFilterIdx]),
		activeStyle(dateLabels[m.dateFilterIdx]),
	)

	tableView := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Render(m.table.View())

	content := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().PaddingBottom(1).Render(header),
		tableView,
	)

	if m.state == listStateEdit && m.form != nil {
		rawDesc := ""

		if m.editIdx >= 0 && m.editIdx < len(m.txs) {
			rawDesc = m.txs[m.editIdx].RawDescription
		}

		panel := lipgloss.NewStyle().
			Padding(1, 2).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Width(48).
			Render(
				fmt.Sprintf("Edit Transaction\n\nOriginal: %s\n\n%s", rawDesc, m.form.View()),
			)

		content = lipgloss.JoinHorizontal(lipgloss.Top, content, panel)
	}

	if m.status != "" {
		content = lipgloss.NewStyle().Faint(true).Render(m.status) + "\n" + content
	}

	return lipgloss.NewStyle().Padding(1).Render(content)
}

func activeStyle(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(s)
}

func (m *ListModel) applyFilter() {
	switch m.statusFilterIdx {
	case 1:
		v := transaction.StatusDraft
		m.filter.Status = &v
	case 2:
		v := transaction.StatusPendingInvoice
		m.filter.Status = &v
	case 3:
		v := transaction.StatusComplete
		m.filter.Status = &v
	case 4:
		v := transaction.StatusNoInvoice
		m.filter.Status = &v
	default:
		m.filter.Status = nil
	}

	now := time.Now()

	switch m.dateFilterIdx {
	case 1:
		s := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		e := s.AddDate(0, 1, 0).Add(-time.Nanosecond)
		m.filter.StartDate = &s
		m.filter.EndDate = &e
	case 2:
		s := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		e := s.AddDate(0, 1, 0).Add(-time.Nanosecond)
		m.filter.StartDate = &s
		m.filter.EndDate = &e
	default:
		m.filter.StartDate = nil
		m.filter.EndDate = nil
	}
}

func (m *ListModel) refreshTable() {
	rows := make([]table.Row, 0, len(m.txs))

	for _, tx := range m.txs {
		docFilename := ""
		if tx.Document != nil {
			docFilename = tx.Document.Filename
		}

		rows = append(rows, table.Row{
			FormatDate(tx.Date),
			string(tx.Status),
			FormatAmountSigned(tx.Amount, tx.Type),
			tx.Description,
			docFilename,
		})
	}

	m.table.SetRows(rows)
}

// Messages

type loadListMsg struct {
	txs []*transaction.Transaction
	err error
}

func (m ListModel) loadTxsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := DbCtx(m.baseCtx)
		defer cancel()

		txs, err := m.txService.List(ctx, m.filter)

		return loadListMsg{txs: txs, err: err}
	}
}

type listSaveMsg struct {
	err error
}

func (m ListModel) saveCmd() tea.Cmd {
	if m.editIdx < 0 || m.editIdx >= len(m.txs) {
		return nil
	}

	txCopy := *m.txs[m.editIdx]
	desc := m.formDesc
	action := m.formDocAction
	txSvc := m.txService
	docSvc := m.docService
	baseCtx := m.baseCtx

	return func() tea.Msg {
		ctx, cancel := DbCtx(baseCtx)
		defer cancel()

		txCopy.Description = desc

		switch action {
		case "no_invoice":
			txCopy.Status = transaction.StatusNoInvoice
		case "remove":
			if txCopy.DocumentID != nil {
				if err := docSvc.Delete(ctx, *txCopy.DocumentID); err != nil {
					return listSaveMsg{err: err}
				}
				if err := txSvc.DetachDocument(ctx, txCopy.ID); err != nil {
					return listSaveMsg{err: err}
				}
				txCopy.Status = transaction.StatusPendingInvoice
			}
		}

		if err := txSvc.Update(ctx, &txCopy); err != nil {
			return listSaveMsg{err: err}
		}

		return listSaveMsg{}
	}
}

func (m ListModel) saveWithFileCmd(filePath string) tea.Cmd {
	if m.editIdx < 0 || m.editIdx >= len(m.txs) {
		return nil
	}

	txCopy := *m.txs[m.editIdx]
	desc := m.formDesc
	txSvc := m.txService
	docSvc := m.docService
	baseCtx := m.baseCtx

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(baseCtx, importTimeout)
		defer cancel()

		txCopy.Description = desc

		f, err := os.Open(filePath)
		if err != nil {
			return listSaveMsg{err: fmt.Errorf("opening file: %w", err)}
		}
		defer f.Close()

		mimeType := detectMIMEFromFile(f, filePath)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return listSaveMsg{err: fmt.Errorf("seeking file: %w", err)}
		}

		if txCopy.DocumentID != nil {
			if err := docSvc.Delete(ctx, *txCopy.DocumentID); err != nil {
				return listSaveMsg{err: err}
			}
			if err := txSvc.DetachDocument(ctx, txCopy.ID); err != nil {
				return listSaveMsg{err: err}
			}
		}

		doc, err := docSvc.Upload(ctx, filepath.Base(filePath), mimeType, f)
		if err != nil {
			return listSaveMsg{err: fmt.Errorf("uploading document: %w", err)}
		}

		if err := txSvc.AttachDocument(ctx, txCopy.ID, doc.ID); err != nil {
			return listSaveMsg{err: fmt.Errorf("attaching document: %w", err)}
		}

		txCopy.Status = transaction.StatusComplete
		if err := txSvc.Update(ctx, &txCopy); err != nil {
			return listSaveMsg{err: err}
		}

		return listSaveMsg{}
	}
}

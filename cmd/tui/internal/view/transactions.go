package view

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/document"
	"github.com/MrJamesThe3rd/finny/internal/matching"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type txState int

const (
	txStateTimeframe txState = iota
	txStateList
	txStateEditing
	txStateFilePick
)

// txItem wraps a transaction to implement list.Item.
type txItem struct {
	tx *transaction.Transaction
}

func (i txItem) Title() string {
	status := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("[%s]", i.tx.Status))

	desc := i.tx.Description
	if desc == "" {
		desc = i.tx.RawDescription
	}

	return fmt.Sprintf("%s  %s  %s  %s", FormatDate(i.tx.Date), FormatAmountSigned(i.tx.Amount, i.tx.Type), status, desc)
}

func (i txItem) Description() string {
	if i.tx.Document != nil && i.tx.Document.Filename != "" {
		return fmt.Sprintf("Document: %s", i.tx.Document.Filename)
	}

	return ""
}

func (i txItem) FilterValue() string {
	desc := i.tx.Description
	if desc == "" {
		desc = i.tx.RawDescription
	}

	return desc
}

type TransactionsModel struct {
	CommonModel
	txService       *transaction.Service
	matchingService *matching.Service
	docService      *document.Service

	state           txState
	timeframePicker TimeframePicker
	list            list.Model
	form            *huh.Form
	filePicker      filepicker.Model
	txs             []*transaction.Transaction
	selectedTx      *transaction.Transaction

	startDate time.Time
	endDate   time.Time
	allTime   bool
	loading   bool
	status    string

	// Form field bindings
	formDesc      string
	formDocAction string
}

func NewTransactionsModel(baseCtx context.Context, txSvc *transaction.Service, matchSvc *matching.Service, docSvc *document.Service) TransactionsModel {
	l := list.New([]list.Item{}, txItemDelegate{}, 0, 0)
	l.Title = "Transactions"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

	fp := filepicker.New()
	fp.CurrentDirectory, _ = os.Getwd()
	fp.ShowHidden = false
	fp.DirAllowed = false
	fp.FileAllowed = true
	fp.SetHeight(15)

	return TransactionsModel{
		CommonModel:     CommonModel{baseCtx: baseCtx},
		txService:       txSvc,
		matchingService: matchSvc,
		docService:      docSvc,
		timeframePicker: NewTimeframePicker(TimeframeThisWeek),
		list:            l,
		filePicker:      fp,
	}
}

func (m TransactionsModel) Title() string { return "Manage Transactions" }

func (m TransactionsModel) ShortHelp() string {
	switch m.state {
	case txStateTimeframe:
		return "Esc: back | Enter: select"
	case txStateList:
		return "Esc: back | Enter: edit | /: filter"
	case txStateEditing:
		return "Esc: cancel | Enter/Tab: navigate form"
	case txStateFilePick:
		return "Esc: cancel | Enter: select file"
	}

	return ""
}

func (m TransactionsModel) Init() tea.Cmd {
	return nil
}

func (m TransactionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TimeframeSelectedMsg:
		m.startDate = msg.Start
		m.endDate = msg.End
		m.allTime = msg.All
		m.loading = true
		m.state = txStateList

		return m, m.loadTxsCmd()

	case loadTxsMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}

		m.txs = msg.txs
		m.refreshListItems()

		if len(msg.txs) == 0 {
			m.status = "No transactions found."
		}

		return m, nil

	case saveTxResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error saving: %v", msg.err)
			m.state = txStateList

			return m, nil
		}

		m.status = "Saved."
		m.state = txStateList

		return m, m.loadTxsCmd()

	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width-4, msg.Height-8)
		return m, nil
	}

	switch m.state {
	case txStateTimeframe:
		return m.updateTimeframe(msg)
	case txStateList:
		return m.updateList(msg)
	case txStateEditing:
		return m.updateEditing(msg)
	case txStateFilePick:
		return m.updateFilePick(msg)
	}

	return m, nil
}

func (m TransactionsModel) updateTimeframe(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc && m.timeframePicker.IsSelecting() {
			return m, Back
		}
	}

	var cmd tea.Cmd
	m.timeframePicker, cmd = m.timeframePicker.Update(msg)

	return m, cmd
}

func (m TransactionsModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEsc:
			if m.list.FilterState() == list.Filtering {
				break // let the list handle it (close filter)
			}

			return m, Back
		case tea.KeyEnter:
			if m.list.FilterState() == list.Filtering {
				break // let the list handle it (confirm filter)
			}

			return m.startEditing()
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m TransactionsModel) startEditing() (tea.Model, tea.Cmd) {
	selected, ok := m.list.SelectedItem().(txItem)
	if !ok {
		return m, nil
	}

	m.selectedTx = selected.tx
	m.formDesc = selected.tx.Description
	m.formDocAction = "skip"

	// Pre-populate description with suggestion if empty
	if m.formDesc == "" && selected.tx.RawDescription != "" {
		ctx, cancel := DbCtx(m.baseCtx)
		defer cancel()

		suggestion, _ := m.matchingService.Suggest(ctx, selected.tx.RawDescription)
		if suggestion != "" {
			m.formDesc = suggestion
		}

		if m.formDesc == "" {
			m.formDesc = selected.tx.RawDescription
		}
	}

	var docOptions []huh.Option[string]
	if selected.tx.DocumentID != nil {
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
	).WithWidth(50).WithShowHelp(false)

	m.state = txStateEditing

	return m, m.form.Init()
}

func (m TransactionsModel) updateEditing(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			m.state = txStateList
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

	action := m.form.GetString("document_action")
	desc := m.form.GetString("description")
	m.formDesc = desc
	m.formDocAction = action

	if action == "upload" {
		m.state = txStateFilePick
		return m, m.filePicker.Init()
	}

	return m, m.saveTxCmd()
}

func (m TransactionsModel) updateFilePick(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			m.state = txStateEditing
			return m, m.form.Init()
		}
	}

	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)

	if didSelect, path := m.filePicker.DidSelectFile(msg); didSelect {
		return m, m.saveTxWithFileCmd(path)
	}

	return m, cmd
}

func (m TransactionsModel) View() string {
	switch m.state {
	case txStateTimeframe:
		return lipgloss.NewStyle().Padding(1).Render(m.timeframePicker.View())

	case txStateList:
		if m.loading {
			return lipgloss.NewStyle().Padding(2).Render("Loading transactions...")
		}

		statusLine := ""
		if m.status != "" {
			statusLine = lipgloss.NewStyle().Faint(true).Render(m.status) + "\n"
		}

		return lipgloss.NewStyle().Padding(1).Render(statusLine + m.list.View())

	case txStateEditing:
		if m.form == nil {
			return ""
		}

		info := m.txInfoView()

		return lipgloss.NewStyle().Padding(1).Render(
			info + "\n" + m.form.View(),
		)

	case txStateFilePick:
		return lipgloss.NewStyle().Padding(1).Render(
			"Select a file to upload:\n\n" + m.filePicker.View(),
		)
	}

	return ""
}

func (m TransactionsModel) txInfoView() string {
	if m.selectedTx == nil {
		return ""
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(fmt.Sprintf(
			"Date: %s  |  Type: %s  |  Amount: %s\nRaw: %s",
			FormatDate(m.selectedTx.Date),
			m.selectedTx.Type,
			FormatAmountSigned(m.selectedTx.Amount, m.selectedTx.Type),
			m.selectedTx.RawDescription,
		))
}

func (m *TransactionsModel) refreshListItems() {
	items := make([]list.Item, len(m.txs))
	for i, tx := range m.txs {
		items[i] = txItem{tx: tx}
	}

	m.list.SetItems(items)
}

// Messages

type loadTxsMsg struct {
	txs []*transaction.Transaction
	err error
}

func (m TransactionsModel) loadTxsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := DbCtx(m.baseCtx)
		defer cancel()

		filter := transaction.ListFilter{}

		if !m.allTime {
			start, end := m.startDate, m.endDate
			filter.StartDate = &start
			filter.EndDate = &end
		}

		txs, err := m.txService.List(ctx, filter)

		return loadTxsMsg{txs: txs, err: err}
	}
}

type saveTxResultMsg struct {
	err error
}

// saveTxCmd handles all non-upload actions (skip, no_invoice, remove).
func (m TransactionsModel) saveTxCmd() tea.Cmd {
	txCopy := *m.selectedTx
	desc := m.formDesc
	action := m.formDocAction
	rawDesc := txCopy.RawDescription
	matchSvc := m.matchingService
	txSvc := m.txService
	docSvc := m.docService
	baseCtx := m.baseCtx

	return func() tea.Msg {
		ctx, cancel := DbCtx(baseCtx)
		defer cancel()

		if rawDesc != "" && desc != "" {
			_ = matchSvc.Learn(ctx, rawDesc, desc)
		}

		txCopy.Description = desc

		switch action {
		case "no_invoice":
			txCopy.Status = transaction.StatusNoInvoice
		case "remove":
			if txCopy.DocumentID != nil {
				if err := docSvc.Delete(ctx, *txCopy.DocumentID); err != nil {
					return saveTxResultMsg{err: err}
				}
				if err := txSvc.DetachDocument(ctx, txCopy.ID); err != nil {
					return saveTxResultMsg{err: err}
				}
				txCopy.Status = transaction.StatusPendingInvoice
			}
		case "skip":
			// Keep current status
		}

		if err := txSvc.Update(ctx, &txCopy); err != nil {
			return saveTxResultMsg{err: err}
		}

		return saveTxResultMsg{}
	}
}

// saveTxWithFileCmd uploads the selected file and attaches it to the transaction.
func (m TransactionsModel) saveTxWithFileCmd(filePath string) tea.Cmd {
	txCopy := *m.selectedTx
	desc := m.formDesc
	rawDesc := txCopy.RawDescription
	matchSvc := m.matchingService
	txSvc := m.txService
	docSvc := m.docService
	baseCtx := m.baseCtx

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(baseCtx, importTimeout)
		defer cancel()

		if rawDesc != "" && desc != "" {
			_ = matchSvc.Learn(ctx, rawDesc, desc)
		}

		txCopy.Description = desc

		f, err := os.Open(filePath)
		if err != nil {
			return saveTxResultMsg{err: fmt.Errorf("opening file: %w", err)}
		}
		defer f.Close()

		mimeType := detectMIMEFromFile(f, filePath)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return saveTxResultMsg{err: fmt.Errorf("seeking file: %w", err)}
		}

		// If replacing, delete the existing document first.
		if txCopy.DocumentID != nil {
			if err := docSvc.Delete(ctx, *txCopy.DocumentID); err != nil {
				return saveTxResultMsg{err: err}
			}
			if err := txSvc.DetachDocument(ctx, txCopy.ID); err != nil {
				return saveTxResultMsg{err: err}
			}
		}

		doc, err := docSvc.Upload(ctx, filepath.Base(filePath), mimeType, f)
		if err != nil {
			return saveTxResultMsg{err: fmt.Errorf("uploading document: %w", err)}
		}

		if err := txSvc.AttachDocument(ctx, txCopy.ID, doc.ID); err != nil {
			return saveTxResultMsg{err: fmt.Errorf("attaching document: %w", err)}
		}

		// AttachDocument already set status=complete; Update only touches description/status.
		txCopy.Status = transaction.StatusComplete
		if err := txSvc.Update(ctx, &txCopy); err != nil {
			return saveTxResultMsg{err: err}
		}

		return saveTxResultMsg{}
	}
}

// detectMIMEFromFile sniffs MIME type from the first 512 bytes, falling back to
// the file extension.
func detectMIMEFromFile(f *os.File, filePath string) string {
	buf := make([]byte, 512)
	n, _ := f.Read(buf)

	detected := http.DetectContentType(buf[:n])
	if detected != "application/octet-stream" {
		return detected
	}

	if ext := filepath.Ext(filePath); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}

	return detected
}

// txItemDelegate renders items in the list.
type txItemDelegate struct{}

func (d txItemDelegate) Height() int                             { return 2 }
func (d txItemDelegate) Spacing() int                            { return 0 }
func (d txItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d txItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(txItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	title := i.Title()
	desc := i.Description()

	if isSelected {
		title = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render("> " + title)
	}

	fmt.Fprintf(w, "  %s\n", title)

	if desc == "" {
		fmt.Fprintln(w)
		return
	}

	fmt.Fprintf(w, "    %s\n", lipgloss.NewStyle().Faint(true).Render(desc))
}

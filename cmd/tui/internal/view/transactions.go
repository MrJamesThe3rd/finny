package view

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/matching"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type txState int

const (
	txStateTimeframe txState = iota
	txStateList
	txStateEditing
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

	return fmt.Sprintf("%s  %s  %s  %s", FormatDate(i.tx.Date), FormatAmount(i.tx.Amount), status, desc)
}

func (i txItem) Description() string {
	if i.tx.Invoice != nil && i.tx.Invoice.URL != "" {
		return fmt.Sprintf("Invoice: %s", i.tx.Invoice.URL)
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

	state           txState
	timeframePicker TimeframePicker
	list            list.Model
	form            *huh.Form
	txs             []*transaction.Transaction
	selectedTx      *transaction.Transaction

	startDate time.Time
	endDate   time.Time
	allTime   bool
	loading   bool
	status    string

	// Form field bindings
	formDesc  string
	formURL   string
	formNoInv bool
}

func NewTransactionsModel(txSvc *transaction.Service, matchSvc *matching.Service) TransactionsModel {
	l := list.New([]list.Item{}, txItemDelegate{}, 0, 0)
	l.Title = "Transactions"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

	return TransactionsModel{
		txService:       txSvc,
		matchingService: matchSvc,
		timeframePicker: NewTimeframePicker(TimeframeThisWeek),
		list:            l,
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
	m.formURL = ""
	m.formNoInv = false

	if selected.tx.Invoice != nil {
		m.formURL = selected.tx.Invoice.URL
	}

	// Pre-populate description with suggestion if empty
	if m.formDesc == "" && selected.tx.RawDescription != "" {
		ctx, cancel := DbCtx()
		defer cancel()

		suggestion, _ := m.matchingService.Suggest(ctx, selected.tx.RawDescription)
		if suggestion != "" {
			m.formDesc = suggestion
		}

		if m.formDesc == "" {
			m.formDesc = selected.tx.RawDescription
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

			huh.NewInput().
				Key("invoice_url").
				Title("Invoice URL (optional)").
				Placeholder("https://...").
				Value(&m.formURL),

			huh.NewConfirm().
				Key("no_invoice").
				Title("Mark as no invoice needed?").
				Affirmative("Yes").
				Negative("No").
				Value(&m.formNoInv),
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

	// Form completed - determine status and save
	return m, m.saveTxCmd()
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
			FormatAmount(m.selectedTx.Amount),
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
		ctx, cancel := DbCtx()
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

func (m TransactionsModel) saveTxCmd() tea.Cmd {
	tx := m.selectedTx
	desc := m.formDesc
	url := m.formURL
	noInvoice := m.formNoInv
	rawDesc := tx.RawDescription
	matchSvc := m.matchingService
	txSvc := m.txService

	return func() tea.Msg {
		ctx, cancel := DbCtx()
		defer cancel()

		// Learn description mapping
		if rawDesc != "" && desc != "" {
			_ = matchSvc.Learn(ctx, rawDesc, desc)
		}

		tx.Description = desc

		// Determine new status
		switch {
		case noInvoice:
			tx.Status = transaction.StatusNoInvoice
		case url != "":
			tx.Status = transaction.StatusComplete
		default:
			tx.Status = transaction.StatusPendingInvoice
		}

		if err := txSvc.Update(ctx, tx); err != nil {
			return saveTxResultMsg{err: err}
		}

		if url != "" {
			if err := txSvc.AttachInvoice(ctx, tx.ID, url); err != nil {
				return saveTxResultMsg{err: err}
			}
		}

		return saveTxResultMsg{}
	}
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

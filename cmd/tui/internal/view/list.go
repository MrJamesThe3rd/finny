package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type listState int

const (
	listStateBrowse listState = iota
	listStateEdit
)

type ListModel struct {
	CommonModel
	txService *transaction.Service

	state listState
	table table.Model
	txs   []*transaction.Transaction
	form  *huh.Form

	// Filter cycling
	statusFilterIdx int
	dateFilterIdx   int

	filter  transaction.ListFilter
	loading bool
	err     error
	status  string

	// Form bindings
	formDesc string
	formURL  string
}

func NewListModel(txSvc *transaction.Service) ListModel {
	columns := []table.Column{
		{Title: "Date", Width: 12},
		{Title: "Status", Width: 15},
		{Title: "Amount", Width: 10},
		{Title: "Description", Width: 40},
		{Title: "InvoiceURL", Width: 40},
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

	return ListModel{
		txService: txSvc,
		table:     t,
		filter:    transaction.ListFilter{},
	}
}

func (m ListModel) Title() string { return "Transactions List" }
func (m ListModel) ShortHelp() string {
	if m.state == listStateEdit {
		return "Navigate form | Esc: cancel"
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
	m.formURL = ""
	if tx.Invoice != nil {
		m.formURL = tx.Invoice.URL
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
				Title("Invoice URL").
				Placeholder("https://...").
				Value(&m.formURL),
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

	return m, m.saveCmd()
}

func (m ListModel) View() string {
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
		idx := m.table.Cursor()
		rawDesc := ""
		if idx >= 0 && idx < len(m.txs) {
			rawDesc = m.txs[idx].RawDescription
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
		m.filter.Status = new(transaction.StatusDraft)
	case 2:
		m.filter.Status = new(transaction.StatusPendingInvoice)
	case 3:
		m.filter.Status = new(transaction.StatusComplete)
	case 4:
		m.filter.Status = new(transaction.StatusNoInvoice)
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
		invoiceURL := ""
		if tx.Invoice != nil {
			invoiceURL = tx.Invoice.URL
		}
		rows = append(rows, table.Row{
			FormatDate(tx.Date),
			string(tx.Status),
			FormatAmount(tx.Amount),
			tx.Description,
			invoiceURL,
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
		ctx, cancel := DbCtx()
		defer cancel()

		txs, err := m.txService.List(ctx, m.filter)
		return loadListMsg{txs: txs, err: err}
	}
}

type listSaveMsg struct {
	err error
}

func (m ListModel) saveCmd() tea.Cmd {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.txs) {
		return nil
	}

	tx := m.txs[idx]
	desc := m.formDesc
	url := m.formURL

	return func() tea.Msg {
		ctx, cancel := DbCtx()
		defer cancel()

		tx.Description = desc
		if err := m.txService.Update(ctx, tx); err != nil {
			return listSaveMsg{err: err}
		}

		if url != "" {
			if err := m.txService.AttachInvoice(ctx, tx.ID, url); err != nil {
				return listSaveMsg{err: err}
			}
		}

		return listSaveMsg{}
	}
}

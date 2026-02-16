package view

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type ListModel struct {
	CommonModel
	txService *transaction.Service

	table table.Model
	txs   []*transaction.Transaction

	// Edit Mode State
	editMode     bool
	descInput    textinput.Model
	invoiceInput textinput.Model
	focusIndex   int // 0: Desc, 1: Invoice

	// Filtering State
	statusFilterIdx int // 0: All, 1: Draft, 2: Pending, 3: Complete, 4: NoInvoice
	dateFilterIdx   int // 0: All Time, 1: This Month, 2: Last Month

	filter  transaction.ListFilter
	loading bool
	err     error
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

	di := textinput.New()
	di.Placeholder = "Description"
	di.Width = 30

	ii := textinput.New()
	ii.Placeholder = "Invoice URL"
	ii.Width = 30

	return ListModel{
		txService:    txSvc,
		table:        t,
		descInput:    di,
		invoiceInput: ii,
		filter:       transaction.ListFilter{},
	}
}

func (m ListModel) Init() tea.Cmd {
	return m.loadTxsCmd()
}

func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Handle Edit Mode
	if m.editMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.editMode = false
				m.table.Focus()

				return m, nil
			case "tab", "shift+tab":
				m.focusIndex = (m.focusIndex + 1) % 2
				if m.focusIndex == 0 {
					m.descInput.Focus()
					m.invoiceInput.Blur()
				} else {
					m.descInput.Blur()
					m.invoiceInput.Focus()
				}

				return m, nil
			case "enter":
				// Save
				return m, m.saveChangesCmd()
			}
		case saveTxMsg:
			if msg.err != nil {
				m.err = msg.err
			} else {
				m.editMode = false
				m.table.Focus()
				// Refresh list to show updates
				return m, m.loadTxsCmd()
			}
		}

		// Update inputs
		if m.focusIndex == 0 {
			m.descInput, cmd = m.descInput.Update(msg)
		} else {
			m.invoiceInput, cmd = m.invoiceInput.Update(msg)
		}

		return m, cmd
	}

	// Normal List Mode
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, Back
		case "r":
			m.loading = true
			return m, m.loadTxsCmd()
		case "e":
			// Enter Edit Mode
			sel := m.table.SelectedRow()
			if len(sel) > 0 && len(m.txs) > 0 {
				idx := m.table.Cursor()
				if idx >= 0 && idx < len(m.txs) {
					tx := m.txs[idx]
					m.editMode = true
					m.table.Blur()

					m.descInput.SetValue(tx.Description)
					m.descInput.Focus()
					m.invoiceInput.Blur()
					m.focusIndex = 0

					if tx.Invoice != nil {
						m.invoiceInput.SetValue(tx.Invoice.URL)
					} else {
						m.invoiceInput.SetValue("")
					}

					return m, nil
				}
			}
		case "s": // Cycle Status
			m.statusFilterIdx = (m.statusFilterIdx + 1) % 5
			m.updateFilter()

			return m, m.loadTxsCmd()
		case "d": // Cycle Date
			m.dateFilterIdx = (m.dateFilterIdx + 1) % 3
			m.updateFilter()

			return m, m.loadTxsCmd()
		}

	case loadListMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		m.txs = msg.txs
		m.updateTable()
	}

	m.table, cmd = m.table.Update(msg)

	return m, cmd
}

func (m *ListModel) updateFilter() {
	// Status
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

	// Date
	now := time.Now()

	var start, end *time.Time

	switch m.dateFilterIdx {
	case 1: // This Month
		s := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		e := s.AddDate(0, 1, 0).Add(-time.Nanosecond)
		start, end = &s, &e
	case 2: // Last Month
		s := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		e := s.AddDate(0, 1, 0).Add(-time.Nanosecond)
		start, end = &s, &e
	default:
		start, end = nil, nil // All Time
	}

	m.filter.StartDate = start
	m.filter.EndDate = end
}

func (m ListModel) View() string {
	if m.loading {
		return "Loading transactions..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}

	tableView := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Render(m.table.View())

	// Build Header with Filters
	statusLabels := []string{"All", "Draft", "Pending", "Complete", "No Invoice"}
	dateLabels := []string{"All Time", "This Month", "Last Month"}

	header := fmt.Sprintf(
		"Filter: [s] Status: %s | [d] Date: %s",
		activeStyle(statusLabels[m.statusFilterIdx]),
		activeStyle(dateLabels[m.dateFilterIdx]),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().PaddingBottom(1).Render(header),
		tableView,
	)

	if m.editMode {
		// Side Panel
		idx := m.table.Cursor()

		var rawDesc string

		if idx >= 0 && idx < len(m.txs) {
			rawDesc = m.txs[idx].RawDescription
		}

		form := fmt.Sprintf(
			"Edit Transaction\n\nOriginal: %s\n\nDescription:\n%s\n\nInvoice URL:\n%s\n\n(Enter to Save, Esc to Cancel)",
			rawDesc,
			m.descInput.View(),
			m.invoiceInput.View(),
		)

		panel := lipgloss.NewStyle().
			Padding(1, 2).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("63")).
			Width(40).
			Height(15).
			Render(form)

		content = lipgloss.JoinHorizontal(lipgloss.Top, content, panel)
	}

	return lipgloss.NewStyle().Padding(1).Render(
		"Transactions List (ESC to back, r to refresh, e to edit)\n" +
			content,
	)
}

func activeStyle(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(s)
}

func (m *ListModel) updateTable() {
	rows := []table.Row{}

	for _, tx := range m.txs {
		dateStr := tx.Date.Format("2006-01-02")
		amountStr := fmt.Sprintf("%.2f", float64(tx.Amount)/100.0)

		var invoiceURL string
		if tx.Invoice != nil {
			invoiceURL = tx.Invoice.URL
		}

		rows = append(rows, table.Row{
			dateStr,
			string(tx.Status),
			amountStr,
			tx.Description,
			invoiceURL,
		})
	}

	m.table.SetRows(rows)
}

type loadListMsg struct {
	txs []*transaction.Transaction
	err error
}

func (m ListModel) loadTxsCmd() tea.Cmd {
	return func() tea.Msg {
		txs, err := m.txService.List(context.Background(), m.filter)
		return loadListMsg{txs: txs, err: err}
	}
}

type saveTxMsg struct {
	err error
}

func (m ListModel) saveChangesCmd() tea.Cmd {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.txs) {
		return nil
	}

	tx := m.txs[idx]

	newDesc := m.descInput.Value()
	newInvoiceURL := m.invoiceInput.Value()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 1. Update Description
		tx.Description = newDesc
		if err := m.txService.Update(ctx, tx); err != nil {
			return saveTxMsg{err: err}
		}

		// 2. Attach Invoice if provided
		if newInvoiceURL != "" {
			if err := m.txService.AttachInvoice(ctx, tx.ID, newInvoiceURL); err != nil {
				return saveTxMsg{err: err}
			}
		}

		return saveTxMsg{err: nil}
	}
}

package view

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type InvoiceModel struct {
	CommonModel
	txService *transaction.Service

	queue     []*transaction.Transaction
	currentTx *transaction.Transaction

	urlInput textinput.Model

	loading    bool
	status     string
	totalCount int
}

func NewInvoiceModel(txSvc *transaction.Service) InvoiceModel {
	ti := textinput.New()
	ti.Placeholder = "https://example.com/invoice.pdf"
	ti.Width = 60

	return InvoiceModel{
		txService: txSvc,
		urlInput:  ti,
		status:    "Load pending invoices...",
	}
}

func (m InvoiceModel) Init() tea.Cmd {
	return m.loadPendingCmd()
}

func (m InvoiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, Back
		case "enter":
			if m.currentTx != nil {
				return m, m.saveInvoiceCmd(m.urlInput.Value())
			}
		case "i": // Ignore (No Invoice)
			if m.currentTx != nil {
				return m, m.markNoInvoiceCmd()
			}
		}

	case loadPendingMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}

		m.queue = msg.txs
		m.totalCount = len(m.queue)
		m.nextTx()

	case invoiceActionMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error saving: %v", msg.err)
			break
		}
		// Success
		m.nextTx()
	}

	m.urlInput, cmd = m.urlInput.Update(msg)

	return m, cmd
}

func (m InvoiceModel) View() string {
	if m.loading {
		return "Loading pending invoices..."
	}

	if m.currentTx == nil {
		if m.totalCount == 0 {
			return lipgloss.NewStyle().Padding(2).Render("No pending invoices found.\n\n(Esc to back)")
		}

		return lipgloss.NewStyle().Padding(2).Render(m.status + "\n\n(Esc to back)")
	}

	amount := float64(m.currentTx.Amount) / 100.0
	info := fmt.Sprintf(
		"Date: %s\nDesc: %s\nAmount: %.2f\n",
		m.currentTx.Date.Format("2006-01-02"),
		m.currentTx.Description,
		amount,
	)

	return lipgloss.NewStyle().Padding(2).Render(
		fmt.Sprintf("Pending Invoice (%d remaining)\n\n%s\nAdd Invoice URL:\n%s\n\n(Enter to save, 'i' to ignore/no-invoice, Esc to back)",
			len(m.queue)+1, info, m.urlInput.View()),
	)
}

func (m *InvoiceModel) nextTx() {
	if len(m.queue) == 0 {
		m.currentTx = nil
		m.status = "All done!"
		m.urlInput.SetValue("")

		return
	}

	m.currentTx = m.queue[0]
	m.queue = m.queue[1:]
	m.urlInput.SetValue("")
	m.urlInput.Focus()
}

type loadPendingMsg struct {
	txs []*transaction.Transaction
	err error
}

func (m InvoiceModel) loadPendingCmd() tea.Cmd {
	return func() tea.Msg {
		filter := transaction.ListFilter{Status: new(transaction.StatusPendingInvoice)}
		// We want oldest first, which is default sort now.
		txs, err := m.txService.List(context.Background(), filter)

		return loadPendingMsg{txs: txs, err: err}
	}
}

type invoiceActionMsg struct {
	err error
}

func (m InvoiceModel) saveInvoiceCmd(url string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := m.txService.AttachInvoice(ctx, m.currentTx.ID, url); err != nil {
			return invoiceActionMsg{err: err}
		}

		err := m.txService.UpdateStatus(ctx, m.currentTx.ID, transaction.StatusComplete)

		return invoiceActionMsg{err: err}
	}
}

func (m InvoiceModel) markNoInvoiceCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := m.txService.UpdateStatus(ctx, m.currentTx.ID, transaction.StatusNoInvoice)

		return invoiceActionMsg{err: err}
	}
}

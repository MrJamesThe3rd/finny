package view

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MrJamesThe3rd/finny/internal/matching"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type ReviewModel struct {
	CommonModel
	txService       *transaction.Service
	matchingService *matching.Service

	state ReviewState

	queue     []*transaction.Transaction
	currentTx *transaction.Transaction

	// Inputs
	descInput textinput.Model

	// Date Selection
	startDateInput textinput.Model
	endDateInput   textinput.Model
	focusIndex     int // 0: Start, 1: End

	status     string
	loading    bool
	totalCount int

	selectedTimeframe Timeframe
	customStartDate   time.Time
	customEndDate     time.Time
}

type ReviewState int

const (
	StateSelectTimeframe ReviewState = iota
	StateInputCustomDate
	StateReviewing
)

func NewReviewModel(txSvc *transaction.Service, matchSvc *matching.Service) ReviewModel {
	ti := textinput.New()
	ti.Placeholder = "Description"
	ti.Width = 50

	startIn := textinput.New()
	startIn.Placeholder = "YYYY-MM-DD"
	startIn.CharLimit = 10
	startIn.Width = 12
	startIn.Prompt = "Start Date: "

	endIn := textinput.New()
	endIn.Placeholder = "YYYY-MM-DD"
	endIn.CharLimit = 10
	endIn.Width = 12
	endIn.Prompt = "End Date:   "

	return ReviewModel{
		txService:       txSvc,
		matchingService: matchSvc,
		descInput:       ti,
		startDateInput:  startIn,
		endDateInput:    endIn,
		state:           StateSelectTimeframe,
		status:          "Select timeframe to review",
		loading:         false, // Not loading initially
	}
}

func (m ReviewModel) Init() tea.Cmd {
	// Reset state when entering view
	return nil
}

func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch msg.Type {
		case tea.KeyEsc:
			// If in sub-states, go back to selection
			if m.state == StateInputCustomDate {
				m.state = StateSelectTimeframe
				return m, nil
			}

			return m, Back

		case tea.KeyEnter:
			if m.state == StateSelectTimeframe {
				// Handle Selection
				if m.selectedTimeframe == TimeframeCustom {
					m.state = StateInputCustomDate
					m.startDateInput.Focus()
					m.focusIndex = 0

					return m, textinput.Blink
				}
				// Start Review
				m.loading = true
				m.state = StateReviewing

				return m, m.loadDraftsCmd()
			} else if m.state == StateInputCustomDate {
				if m.focusIndex == 1 {
					// Validate and Submit
					start, err1 := time.Parse("2006-01-02", m.startDateInput.Value())
					end, err2 := time.Parse("2006-01-02", m.endDateInput.Value())

					if err1 != nil || err2 != nil {
						m.status = "Invalid date format (YYYY-MM-DD)"
						return m, nil
					}

					m.customStartDate = start
					m.customEndDate = end
					m.state = StateReviewing
					m.loading = true

					return m, m.loadDraftsCmd()
				}
				// Move to next input
				m.focusIndex++
				m.startDateInput.Blur()
				m.endDateInput.Focus()

				return m, textinput.Blink
			} else if m.state == StateReviewing && m.currentTx != nil {
				return m, m.saveAndNextCmd(m.descInput.Value())
			}

		case tea.KeyUp, tea.KeyDown:
			if m.state == StateSelectTimeframe {
				// Cycle timeframes
				if msg.Type == tea.KeyUp {
					if m.selectedTimeframe > 0 {
						m.selectedTimeframe--
					}
				} else {
					if m.selectedTimeframe < TimeframeCustom {
						m.selectedTimeframe++
					}
				}
			}

		case tea.KeyTab:
			if m.state == StateInputCustomDate {
				m.focusIndex = (m.focusIndex + 1) % 2
				if m.focusIndex == 0 {
					m.startDateInput.Focus()
					m.endDateInput.Blur()
				} else {
					m.startDateInput.Blur()
					m.endDateInput.Focus()
				}

				return m, textinput.Blink
			}
		}

	case loadDraftsMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Error loading details: %v", msg.err)
			break
		}

		m.queue = msg.txs
		m.totalCount = len(m.queue)

		if len(m.queue) > 0 {
			m.nextTx()
			return m, textinput.Blink
		}

		m.status = "No draft transactions found."

	case saveResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error saving: %v", msg.err)
			break
		}

		// Saved successfully
		if len(m.queue) > 0 {
			m.nextTx()
			return m, textinput.Blink
		}

		m.currentTx = nil
		m.status = "All done!"
		m.descInput.SetValue("")
	}

	// Handle Inputs based on state
	if m.state == StateInputCustomDate {
		var cmd1, cmd2 tea.Cmd
		m.startDateInput, cmd1 = m.startDateInput.Update(msg)
		m.endDateInput, cmd2 = m.endDateInput.Update(msg)

		return m, tea.Batch(cmd, cmd1, cmd2)
	}

	if m.state == StateReviewing {
		m.descInput, cmd = m.descInput.Update(msg)
	}

	return m, cmd
}

func (m ReviewModel) View() string {
	if m.state == StateSelectTimeframe {
		s := "Select Timeframe:\n\n"

		for i := TimeframeThisWeek; i <= TimeframeCustom; i++ {
			cursor := " "
			if m.selectedTimeframe == i {
				cursor = ">"
			}

			s += fmt.Sprintf("%s %s\n", cursor, i.String())
		}

		return lipgloss.NewStyle().Padding(2).Render(s)
	}

	if m.state == StateInputCustomDate {
		return lipgloss.NewStyle().Padding(2).Render(
			fmt.Sprintf("Enter Custom Range:\n\n%s\n%s\n\n(Enter to confirm, Esc to back)",
				m.startDateInput.View(),
				m.endDateInput.View(),
			),
		)
	}

	// StateReviewing
	content := ""
	if m.loading {
		content = "Loading drafts..."
	} else if m.currentTx != nil {
		amount := float64(m.currentTx.Amount) / 100.0
		info := fmt.Sprintf(
			"Date: %s\nType: %s\nAmount: %.2f\nRaw:  %s\n",
			m.currentTx.Date.Format("2006-01-02"),
			m.currentTx.Type,
			amount,
			m.currentTx.RawDescription,
		)
		content = fmt.Sprintf("%s\n%s\n\nRename Description:\n%s\n\n(Enter to save & next, Esc to quit)", m.status, info, m.descInput.View())
	} else {
		content = m.status + "\n\n(Esc to back)"
	}

	return lipgloss.NewStyle().Padding(2).Render(content)
}

type loadDraftsMsg struct {
	txs []*transaction.Transaction
	err error
}

func (m ReviewModel) loadDraftsCmd() tea.Cmd {
	return func() tea.Msg {
		filter := transaction.ListFilter{Status: new(transaction.StatusDraft)}

		var start, end time.Time

		switch m.selectedTimeframe {
		case TimeframeAll:
			break
		case TimeframeCustom:
			start = m.customStartDate
			end = m.customEndDate
		default:
			start, end = TimeframeToDateRange(m.selectedTimeframe)
		}

		if m.selectedTimeframe != TimeframeAll {
			start, end = NormalizeDateRange(start, end)
			filter.StartDate = &start
			filter.EndDate = &end
		}

		txs, err := m.txService.List(context.Background(), filter)

		return loadDraftsMsg{txs: txs, err: err}
	}
}

func (m *ReviewModel) nextTx() {
	if len(m.queue) == 0 {
		m.currentTx = nil
		m.status = "All done! No more drafts."
		m.descInput.Blur()

		return
	}

	tx := m.queue[0]
	m.queue = m.queue[1:]
	m.currentTx = tx

	// Progress
	// queue size is now N-1.
	// Reviewed = Total - len(queue)
	// Example: Total 10. Pop 1. queue 9. Reviewed = 1. "Reviewing 1/10"
	currentIdx := m.totalCount - len(m.queue)
	m.status = fmt.Sprintf("Reviewing %d/%d", currentIdx, m.totalCount)
	m.descInput.Focus()

	// Suggestion logic
	suggestion := ""

	if m.currentTx.RawDescription != "" {
		s, _ := m.matchingService.Suggest(context.Background(), m.currentTx.RawDescription)
		suggestion = s
	}

	suggestionDesc := m.currentTx.RawDescription // Default to raw
	if suggestion != "" {
		suggestionDesc = suggestion
	}

	m.descInput.SetValue(suggestionDesc)
}

type saveResultMsg struct {
	err error
}

func (m ReviewModel) saveAndNextCmd(newDesc string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 1. Learn mapping
		if m.currentTx.RawDescription != "" && newDesc != "" {
			_ = m.matchingService.Learn(ctx, m.currentTx.RawDescription, newDesc)
		}

		// 2. Update Transaction
		m.currentTx.Description = newDesc
		m.currentTx.Status = transaction.StatusPendingInvoice

		err := m.txService.Update(ctx, m.currentTx)

		return saveResultMsg{err: err}
	}
}

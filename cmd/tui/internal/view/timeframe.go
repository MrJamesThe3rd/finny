package view

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Timeframe represents a predefined or custom date range selection.
type Timeframe int

const (
	TimeframeThisWeek  Timeframe = 0
	TimeframeLastWeek  Timeframe = 1
	TimeframeThisMonth Timeframe = 2
	TimeframeLastMonth Timeframe = 3
	TimeframeAll       Timeframe = 4
	TimeframeCustom    Timeframe = 5
)

func (t Timeframe) String() string {
	switch t {
	case TimeframeThisWeek:
		return "This Week"
	case TimeframeLastWeek:
		return "Last Week"
	case TimeframeThisMonth:
		return "This Month"
	case TimeframeLastMonth:
		return "Last Month"
	case TimeframeAll:
		return "All Time"
	case TimeframeCustom:
		return "Custom Range"
	}

	return "Unknown"
}

func timeframeToDateRange(tf Timeframe) (time.Time, time.Time) {
	now := time.Now()

	var start, end time.Time

	switch tf {
	case TimeframeThisWeek:
		offset := int(now.Weekday())
		if offset == 0 {
			offset = 7
		}

		start = now.AddDate(0, 0, -offset+1)
		end = now
	case TimeframeLastWeek:
		offset := int(now.Weekday())
		if offset == 0 {
			offset = 7
		}

		end = now.AddDate(0, 0, -offset)
		start = end.AddDate(0, 0, -6)
	case TimeframeThisMonth:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = now
	case TimeframeLastMonth:
		lastMonth := now.AddDate(0, -1, 0)
		start = time.Date(lastMonth.Year(), lastMonth.Month(), 1, 0, 0, 0, 0, lastMonth.Location())
		end = start.AddDate(0, 1, -1)
	}

	return start, end
}

func normalizeDateRange(start, end time.Time) (time.Time, time.Time) {
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC),
		time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, time.UTC)
}

// TimeframeSelectedMsg is emitted when the user has selected a valid date range.
// Start and End are zero values when All is true.
type TimeframeSelectedMsg struct {
	Start time.Time
	End   time.Time
	All   bool
}

type timeframeState int

const (
	timeframeStateSelect timeframeState = iota
	timeframeStateCustom
)

// TimeframePicker is a reusable component for selecting a date range.
type TimeframePicker struct {
	state    timeframeState
	selected Timeframe
	minFrame Timeframe

	startInput textinput.Model
	endInput   textinput.Model
	focusIndex int

	err error
}

// NewTimeframePicker creates a picker starting from the given minimum timeframe.
func NewTimeframePicker(minFrame Timeframe) TimeframePicker {
	si := textinput.New()
	si.Placeholder = "YYYY-MM-DD"
	si.CharLimit = 10
	si.Width = 12
	si.Prompt = "Start Date: "

	ei := textinput.New()
	ei.Placeholder = "YYYY-MM-DD"
	ei.CharLimit = 10
	ei.Width = 12
	ei.Prompt = "End Date:   "

	return TimeframePicker{
		state:      timeframeStateSelect,
		selected:   minFrame,
		minFrame:   minFrame,
		startInput: si,
		endInput:   ei,
	}
}

// Init returns the initial command for the picker.
func (m TimeframePicker) Init() tea.Cmd {
	return nil
}

// Update handles messages for the timeframe picker.
func (m TimeframePicker) Update(msg tea.Msg) (TimeframePicker, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.state {
		case timeframeStateSelect:
			return m.updateSelect(msg)
		case timeframeStateCustom:
			return m.updateCustom(msg)
		}
	}

	if m.state == timeframeStateCustom {
		return m.updateInputs(msg)
	}

	return m, nil
}

func (m TimeframePicker) updateSelect(msg tea.KeyMsg) (TimeframePicker, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.selected > m.minFrame {
			m.selected--
		}
	case tea.KeyDown:
		if m.selected < TimeframeCustom {
			m.selected++
		}
	case tea.KeyEnter:
		if m.selected == TimeframeCustom {
			m.state = timeframeStateCustom
			m.startInput.Focus()
			m.focusIndex = 0
			return m, textinput.Blink
		}

		if m.selected == TimeframeAll {
			return m, func() tea.Msg {
				return TimeframeSelectedMsg{All: true}
			}
		}

		start, end := timeframeToDateRange(m.selected)
		start, end = normalizeDateRange(start, end)
		return m, func() tea.Msg {
			return TimeframeSelectedMsg{Start: start, End: end}
		}
	}

	return m, nil
}

func (m TimeframePicker) updateCustom(msg tea.KeyMsg) (TimeframePicker, tea.Cmd) {
	switch msg.String() {
	case "tab", "shift+tab":
		m.focusIndex = (m.focusIndex + 1) % 2
		m.startInput.Blur()
		m.endInput.Blur()
		if m.focusIndex == 0 {
			m.startInput.Focus()
			return m, textinput.Blink
		}
		m.endInput.Focus()
		return m, textinput.Blink

	case "enter":
		start, err := time.Parse("2006-01-02", m.startInput.Value())
		if err != nil {
			m.err = fmt.Errorf("invalid start date (YYYY-MM-DD)")
			return m, nil
		}

		end, err := time.Parse("2006-01-02", m.endInput.Value())
		if err != nil {
			m.err = fmt.Errorf("invalid end date (YYYY-MM-DD)")
			return m, nil
		}

		m.err = nil
		start, end = normalizeDateRange(start, end)
		return m, func() tea.Msg {
			return TimeframeSelectedMsg{Start: start, End: end}
		}

	case "esc":
		m.state = timeframeStateSelect
		m.err = nil
		return m, nil
	}

	return m, nil
}

func (m TimeframePicker) updateInputs(msg tea.Msg) (TimeframePicker, tea.Cmd) {
	var cmds []tea.Cmd
	var c tea.Cmd

	m.startInput, c = m.startInput.Update(msg)
	cmds = append(cmds, c)
	m.endInput, c = m.endInput.Update(msg)
	cmds = append(cmds, c)

	return m, tea.Batch(cmds...)
}

// View renders the timeframe picker.
func (m TimeframePicker) View() string {
	errStr := ""
	if m.err != nil {
		errStr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("\n\nError: %v", m.err))
	}

	if m.state == timeframeStateCustom {
		return fmt.Sprintf(
			"Enter Custom Range:\n\n%s\n%s\n\n(Enter to confirm, Tab to switch, Esc to back)%s",
			m.startInput.View(),
			m.endInput.View(),
			errStr,
		)
	}

	s := "Select Timeframe:\n\n"
	for i := m.minFrame; i <= TimeframeCustom; i++ {
		cursor := " "
		if m.selected == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %s\n", cursor, i.String())
	}
	s += "\n(Enter to select, Esc to back)"

	return s + errStr
}

// IsSelecting returns true if the picker is in the selection state (not custom input).
func (m TimeframePicker) IsSelecting() bool {
	return m.state == timeframeStateSelect
}

// Reset returns the picker to its initial selection state.
func (m *TimeframePicker) Reset() {
	m.state = timeframeStateSelect
	m.selected = m.minFrame
	m.err = nil
	m.startInput.SetValue("")
	m.endInput.SetValue("")
}

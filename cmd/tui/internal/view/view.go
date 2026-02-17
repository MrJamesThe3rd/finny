package view

import (
	tea "github.com/charmbracelet/bubbletea"
)

// View is the interface that all TUI screens implement.
type View interface {
	tea.Model
	Title() string
	ShortHelp() string
}

// CommonModel is embedded by all views.
type CommonModel struct{}

type BackMsg struct{}

func Back() tea.Msg {
	return BackMsg{}
}

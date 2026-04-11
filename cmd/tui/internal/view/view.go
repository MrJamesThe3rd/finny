package view

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// View is the interface that all TUI screens implement.
type View interface {
	tea.Model
	Title() string
	ShortHelp() string
}

// CommonModel is embedded by all views. It carries the base context (with user
// identity) that command functions use as the parent for their DB timeouts.
type CommonModel struct {
	baseCtx context.Context
}

type BackMsg struct{}

func Back() tea.Msg {
	return BackMsg{}
}

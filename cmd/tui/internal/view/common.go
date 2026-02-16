package view

import (
	tea "github.com/charmbracelet/bubbletea"
)

type CommonModel struct {
	Width  int
	Height int
}

type BackMsg struct{}

func Back() tea.Msg {
	return BackMsg{}
}

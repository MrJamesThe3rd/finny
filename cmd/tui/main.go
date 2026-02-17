package main

import (
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"

	"github.com/MrJamesThe3rd/finny/cmd/tui/internal/view"
	"github.com/MrJamesThe3rd/finny/internal/config"
	"github.com/MrJamesThe3rd/finny/internal/database"
	"github.com/MrJamesThe3rd/finny/internal/export"
	"github.com/MrJamesThe3rd/finny/internal/importer"
	"github.com/MrJamesThe3rd/finny/internal/matching"
	matchingStore "github.com/MrJamesThe3rd/finny/internal/matching/store"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
	txStore "github.com/MrJamesThe3rd/finny/internal/transaction/store"
)

type model struct {
	txService       *transaction.Service
	matchingService *matching.Service
	importService   *importer.Service
	exportService   *export.Service

	activeView view.View // nil when showing menu
	width      int
	height     int
}

func initialModel() model {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := database.New(cfg.ConnectionString())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	txSvc := transaction.NewService(txStore.New(db))
	matchSvc := matching.NewService(matchingStore.New(db))
	impSvc := importer.NewService()
	expSvc := export.NewService(txSvc, cfg.Paperless.Token)

	return model{
		txService:       txSvc,
		matchingService: matchSvc,
		importService:   impSvc,
		exportService:   expSvc,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) navigate(v view.View) (model, tea.Cmd) {
	m.activeView = v
	return m, v.Init()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case view.BackMsg:
		m.activeView = nil
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.activeView == nil {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "1":
				return m.navigate(view.NewImportModel(m.txService, m.importService))
			case "2":
				return m.navigate(view.NewTransactionsModel(m.txService, m.matchingService))
			case "3":
				return m.navigate(view.NewListModel(m.txService))
			case "4":
				return m.navigate(view.NewExportModel(m.exportService))
			}
			return m, nil
		}
	}

	if m.activeView != nil {
		updated, cmd := m.activeView.Update(msg)
		m.activeView = updated.(view.View)
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	if m.activeView == nil {
		return lipgloss.NewStyle().Padding(2).Render(
			"Finny TUI\n\n" +
				"1. Import Transactions\n" +
				"2. Manage Transactions\n" +
				"3. List All Transactions\n" +
				"4. Export Transactions\n\n" +
				"q. Quit",
		)
	}

	title := lipgloss.NewStyle().Bold(true).Padding(1, 2).Render(m.activeView.Title())
	content := m.activeView.View()
	help := lipgloss.NewStyle().Faint(true).Padding(0, 2).Render(m.activeView.ShortHelp())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		slog.Error("failed to run TUI", "error", err)
		os.Exit(1)
	}
}

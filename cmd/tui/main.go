package main

import (
	"context"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"

	"github.com/MrJamesThe3rd/finny/cmd/tui/internal/view"
	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/config"
	"github.com/MrJamesThe3rd/finny/internal/database"
	"github.com/MrJamesThe3rd/finny/internal/document"
	"github.com/MrJamesThe3rd/finny/internal/document/local"
	"github.com/MrJamesThe3rd/finny/internal/document/paperless"
	docStore "github.com/MrJamesThe3rd/finny/internal/document/store"
	"github.com/MrJamesThe3rd/finny/internal/export"
	"github.com/MrJamesThe3rd/finny/internal/importer"
	"github.com/MrJamesThe3rd/finny/internal/matching"
	matchingStore "github.com/MrJamesThe3rd/finny/internal/matching/store"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
	txStore "github.com/MrJamesThe3rd/finny/internal/transaction/store"
)

type model struct {
	baseCtx         context.Context
	txService       *transaction.Service
	matchingService *matching.Service
	importService   *importer.Service
	documentService *document.Service
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

	registry := document.NewRegistry()
	registry.Register("paperless", paperless.NewFromConfig)
	registry.Register("local", local.NewFromConfig)

	txSvc := transaction.NewService(txStore.New(db))
	matchSvc := matching.NewService(matchingStore.New(db))
	impSvc := importer.NewService()
	docSvc := document.NewService(docStore.New(db), registry)
	expSvc := export.NewService(txSvc, docSvc)

	baseCtx := auth.WithUserID(context.Background(), auth.DefaultUserID)

	if cfg.Paperless.BaseURL != "" {
		if err := docSvc.SeedLegacyBackend(baseCtx, cfg.Paperless.BaseURL, cfg.Paperless.Token); err != nil {
			slog.Warn("failed to seed legacy paperless backend", "error", err)
		}
	}

	return model{
		baseCtx:         baseCtx,
		txService:       txSvc,
		matchingService: matchSvc,
		importService:   impSvc,
		documentService: docSvc,
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
				return m.navigate(view.NewImportModel(m.baseCtx, m.txService, m.importService))
			case "2":
				return m.navigate(view.NewTransactionsModel(m.baseCtx, m.txService, m.matchingService, m.documentService))
			case "3":
				return m.navigate(view.NewListModel(m.baseCtx, m.txService, m.documentService))
			case "4":
				return m.navigate(view.NewExportModel(m.baseCtx, m.exportService))
			case "5":
				return m.navigate(view.NewBackendsModel(m.baseCtx, m.documentService))
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
				"4. Export Transactions\n" +
				"5. Manage Backends\n\n" +
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

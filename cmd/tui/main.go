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

	currentView View

	importView  view.ImportModel
	reviewView  view.ReviewModel
	listView    view.ListModel
	invoiceView view.InvoiceModel
	exportView  view.ExportModel
}

type View int

const (
	ViewMenu    View = 0
	ViewImport  View = 1
	ViewReview  View = 2
	ViewList    View = 3
	ViewInvoice View = 4
	ViewExport  View = 5
)

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
		currentView:     ViewMenu,
		importView:      view.NewImportModel(txSvc, impSvc),
		reviewView:      view.NewReviewModel(txSvc, matchSvc),
		listView:        view.NewListModel(txSvc),
		invoiceView:     view.NewInvoiceModel(txSvc),
		exportView:      view.NewExportModel(expSvc),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.currentView == ViewMenu {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "1":
				m.currentView = ViewImport
				return m, m.importView.Init()
			case "2":
				m.currentView = ViewReview
				m.reviewView = view.NewReviewModel(m.txService, m.matchingService)

				return m, m.reviewView.Init()
			case "3":
				m.currentView = ViewList
				m.listView = view.NewListModel(m.txService)

				return m, m.listView.Init()
			case "4":
				m.currentView = ViewInvoice
				m.invoiceView = view.NewInvoiceModel(m.txService)

				return m, m.invoiceView.Init()
			case "5":
				m.currentView = ViewExport
				m.exportView = view.NewExportModel(m.exportService)

				return m, m.exportView.Init()
			}
		}
	case view.BackMsg:
		m.currentView = ViewMenu
		return m, nil
	}

	switch m.currentView {
	case ViewImport:
		var newModel tea.Model
		newModel, cmd = m.importView.Update(msg)
		m.importView = newModel.(view.ImportModel)
	case ViewReview:
		var newModel tea.Model
		newModel, cmd = m.reviewView.Update(msg)
		m.reviewView = newModel.(view.ReviewModel)
	case ViewList:
		var newModel tea.Model
		newModel, cmd = m.listView.Update(msg)
		m.listView = newModel.(view.ListModel)
	case ViewInvoice:
		var newModel tea.Model
		newModel, cmd = m.invoiceView.Update(msg)
		m.invoiceView = newModel.(view.InvoiceModel)
	case ViewExport:
		var newModel tea.Model
		newModel, cmd = m.exportView.Update(msg)
		m.exportView = newModel.(view.ExportModel)
	}

	return m, cmd
}

func (m model) View() string {
	switch m.currentView {
	case ViewMenu:
		return lipgloss.NewStyle().Padding(2).Render(
			"Finny TUI\n\n" +
				"1. Import Transactions\n" +
				"2. Review Transactions\n" +
				"3. List All Transactions\n" +
				"4. Manage Invoices\n" +
				"5. Export Transactions\n\n" +
				"q. Quit",
		)
	case ViewImport:
		return m.importView.View()
	case ViewReview:
		return m.reviewView.View()
	case ViewList:
		return m.listView.View()
	case ViewInvoice:
		return m.invoiceView.View()
	case ViewExport:
		return m.exportView.View()
	}

	return "Unknown View"
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		slog.Error("failed to run TUI", "error", err)
		os.Exit(1)
	}
}

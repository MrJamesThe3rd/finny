// Package storetest is the tenant-isolation suite. It runs against a real
// Postgres because that is the only place the WHERE org_id predicates exist:
// a mock repository would assert the boundary it is standing in for.
//
// No build tag and no TEST_DATABASE_URL — the suite provisions its own
// database and runs under plain `make test`, skipping loudly only if Docker
// is unavailable.
package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, as cmd/api uses
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/org"
	orgstore "github.com/MrJamesThe3rd/finny/internal/org/store"
)

var (
	testDB *sql.DB

	// dockerErr is set when no Docker daemon is reachable. Every test skips
	// with it rather than failing, but a container that fails to start for any
	// other reason is a real failure.
	dockerErr error
)

func TestMain(m *testing.M) {
	code, err := runSuite(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "storetest:", err)
		os.Exit(1)
	}

	os.Exit(code)
}

func runSuite(m *testing.M) (int, error) {
	ctx := context.Background()

	if err := checkDocker(ctx); err != nil {
		dockerErr = err
		return m.Run(), nil
	}

	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("finny"),
		postgres.WithUsername("finny"),
		postgres.WithPassword("secret"),
		// Occurrence 2: Postgres starts, runs its init scripts, then restarts,
		// so the first "ready" line is not the one to connect on.
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		return 0, fmt.Errorf("starting postgres: %w", err)
	}

	defer func() {
		if err := container.Terminate(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "storetest: terminating container:", err)
		}
	}()

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, fmt.Errorf("connection string: %w", err)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return 0, fmt.Errorf("opening database: %w", err)
	}

	defer db.Close() //nolint:errcheck

	if err := migrate(db); err != nil {
		return 0, err
	}

	testDB = db

	return m.Run(), nil
}

func checkDocker(ctx context.Context) error {
	provider, err := testcontainers.ProviderDocker.GetProvider()
	if err != nil {
		return err
	}

	return provider.Health(ctx)
}

func migrate(db *sql.DB) error {
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	if err := goose.Up(db, "../../migrations"); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	return nil
}

// requireDB skips loudly when there is no database to test against.
func requireDB(t *testing.T) {
	t.Helper()

	if dockerErr != nil {
		t.Skipf("skipping tenant-isolation suite: no Docker daemon (%v)", dockerErr)
	}

	require.NotNil(t, testDB, "test database not initialised")
}

// tenant is one organization plus the user who owns it, with a context shaped
// exactly as RequireOrg would shape it.
type tenant struct {
	UserID uuid.UUID
	OrgID  uuid.UUID
	Ctx    context.Context
}

// newTenant creates a user, an organization and the owner membership, using
// the real store so the fixture exercises the same code paths as production.
func newTenant(t *testing.T, name string) tenant {
	t.Helper()

	userID := insertUser(t)
	ctx := auth.WithUserID(context.Background(), userID)

	o := &org.Organization{Name: name}
	require.NoError(t, orgstore.New(testDB).CreateWithOwner(ctx, o, userID))

	return tenant{
		UserID: userID,
		OrgID:  o.ID,
		Ctx:    orgCtx(ctx, o.ID, userID, org.RoleOwner),
	}
}

func orgCtx(ctx context.Context, orgID, userID uuid.UUID, role org.Role) context.Context {
	return org.WithMembership(ctx, &org.Membership{
		ID:     uuid.New(),
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
	})
}

func insertUser(t *testing.T) uuid.UUID {
	t.Helper()

	id := uuid.New()
	suffix := id.String()[:8]

	_, err := testDB.ExecContext(context.Background(), `
		INSERT INTO users (id, email, username, name, password_hash, is_admin, created_at, updated_at)
		VALUES ($1, $2, $3, $4, '', FALSE, NOW(), NOW())
	`, id, "user-"+suffix+"@finny.test", "user-"+suffix, "Test User "+suffix)
	require.NoError(t, err)

	return id
}

// auditCount counts audit rows for one subject.
func auditCount(t *testing.T, orgID uuid.UUID, subjectType string, subjectID uuid.UUID) int {
	t.Helper()

	var n int

	require.NoError(t, testDB.QueryRowContext(context.Background(), `
		SELECT count(*) FROM audit_log
		WHERE org_id = $1 AND subject_type = $2 AND subject_id = $3
	`, orgID, subjectType, subjectID).Scan(&n))

	return n
}

// lastAudit returns the newest audit row for one subject.
func lastAudit(t *testing.T, orgID uuid.UUID, subjectType string, subjectID uuid.UUID) (action string, actor uuid.UUID, oldValue, newValue []byte) {
	t.Helper()

	require.NoError(t, testDB.QueryRowContext(context.Background(), `
		SELECT action, actor_user_id, old_value, new_value FROM audit_log
		WHERE org_id = $1 AND subject_type = $2 AND subject_id = $3
		ORDER BY at DESC
		LIMIT 1
	`, orgID, subjectType, subjectID).Scan(&action, &actor, &oldValue, &newValue))

	return action, actor, oldValue, newValue
}

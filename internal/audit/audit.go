// Package audit records who changed what, in the same transaction as the
// change itself. Append-only: nothing here updates or deletes a row.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrNoActor is returned when an entry carries no organization or no actor.
// Writing such a row is worse than failing: it looks like attribution.
var ErrNoActor = errors.New("audit entry needs an org and an actor")

// Action is what happened to the subject.
type Action string

const (
	ActionCreate       Action = "create"
	ActionUpdate       Action = "update"
	ActionDelete       Action = "delete"
	ActionStatusChange Action = "status_change"
	ActionAttach       Action = "attach"
	ActionDetach       Action = "detach"
	ActionGrant        Action = "grant"
	ActionRevoke       Action = "revoke"
)

// Subject types. Constants rather than literals so a typo cannot silently
// split one subject's history across two spellings.
const (
	SubjectTransaction  = "transaction"
	SubjectImportBatch  = "import_batch"
	SubjectDocument     = "document"
	SubjectBackend      = "backend"
	SubjectMembership   = "membership"
	SubjectOrganization = "organization"
)

// Entry is one audit_log row. OldValue and NewValue are opaque JSONB of the
// changed fields; audit asserts nothing about accounting meaning.
type Entry struct {
	OrgID       uuid.UUID
	ActorUserID uuid.UUID
	SubjectType string
	SubjectID   uuid.UUID
	Action      Action
	OldValue    any
	NewValue    any
}

// Record writes e inside tx. Taking *sql.Tx rather than an interface is the
// point: an audit row that can commit separately from the change it describes
// is not an audit row.
func Record(ctx context.Context, tx *sql.Tx, e Entry) error {
	if e.OrgID == uuid.Nil || e.ActorUserID == uuid.Nil {
		return ErrNoActor
	}

	oldValue, err := marshal(e.OldValue)
	if err != nil {
		return fmt.Errorf("marshalling old value: %w", err)
	}

	newValue, err := marshal(e.NewValue)
	if err != nil {
		return fmt.Errorf("marshalling new value: %w", err)
	}

	const query = `
		INSERT INTO audit_log (org_id, actor_user_id, subject_type, subject_id, action, old_value, new_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	if _, err := tx.ExecContext(ctx, query,
		e.OrgID, e.ActorUserID, e.SubjectType, e.SubjectID, string(e.Action), oldValue, newValue,
	); err != nil {
		return fmt.Errorf("recording audit entry: %w", err)
	}

	return nil
}

// marshal snapshots v as JSON now, so an entry never retains a caller's map.
func marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}

	return json.Marshal(v)
}

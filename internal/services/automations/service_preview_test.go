package automations

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/autogrr/rui/internal/dbinterface"
	"github.com/autogrr/rui/internal/models"
)

type testDBQuerier struct {
	db *sql.DB
}

func (q *testDBQuerier) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return q.db.QueryRowContext(ctx, query, args...)
}

func (q *testDBQuerier) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return q.db.ExecContext(ctx, query, args...)
}

func (q *testDBQuerier) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return q.db.QueryContext(ctx, query, args...)
}

func (q *testDBQuerier) BeginTx(ctx context.Context, opts *sql.TxOptions) (dbinterface.TxQuerier, error) {
	tx, err := q.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func TestSetupPreviewTrackerDisplayNames_LoadsWhenTrackerFieldUsed(t *testing.T) {
	ctx := context.Background()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	q := &testDBQuerier{db: sqlDB}
	_, err = q.ExecContext(ctx, `
		CREATE TABLE string_pool (
			id    INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL UNIQUE
		);
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username_id INTEGER REFERENCES string_pool(id)
		);
		INSERT INTO users (id) VALUES (1);
		CREATE TABLE tracker_customizations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id) ON DELETE CASCADE,
			display_name_id INTEGER NOT NULL REFERENCES string_pool(id),
			domains_id INTEGER NOT NULL REFERENCES string_pool(id),
			included_in_stats_id INTEGER REFERENCES string_pool(id),
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE VIEW tracker_customizations_view AS
		SELECT tc.id, tc.owner_id, sp_dn.value AS display_name, sp_d.value AS domains,
		       sp_is.value AS included_in_stats, tc.created_at, tc.updated_at
		FROM tracker_customizations tc
		JOIN string_pool sp_dn ON tc.display_name_id = sp_dn.id
		JOIN string_pool sp_d  ON tc.domains_id = sp_d.id
		LEFT JOIN string_pool sp_is ON tc.included_in_stats_id = sp_is.id
	`)
	require.NoError(t, err)

	_, err = q.ExecContext(ctx, `INSERT OR IGNORE INTO string_pool (value) VALUES ('BHD'), ('bhd.example'), ('')`)
	require.NoError(t, err)

	_, err = q.ExecContext(ctx, `
		INSERT INTO tracker_customizations (display_name_id, domains_id, included_in_stats_id)
		VALUES (
			(SELECT id FROM string_pool WHERE value = 'BHD'),
			(SELECT id FROM string_pool WHERE value = 'bhd.example'),
			NULL)
	`)
	require.NoError(t, err)

	store := models.NewTrackerCustomizationStore(q)
	s := &Service{
		trackerCustomizationStore: store,
	}

	evalCtx := &EvalContext{}
	cond := &RuleCondition{
		Field:    FieldTracker,
		Operator: OperatorNotEqual,
		Value:    "BHD",
	}

	s.setupPreviewTrackerDisplayNames(ctx, 1, cond, evalCtx)

	require.NotNil(t, evalCtx.TrackerDisplayNameByDomain)
	assert.Equal(t, "BHD", evalCtx.TrackerDisplayNameByDomain["bhd.example"])
}

func TestSetupPreviewTrackerDisplayNames_SkipsWhenTrackerFieldNotUsed(t *testing.T) {
	ctx := context.Background()

	// Use a dummy real DB querier — no DB calls are expected when tracker field is not used.
	dummyDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = dummyDB.Close() })

	s := &Service{
		trackerCustomizationStore: models.NewTrackerCustomizationStore(&testDBQuerier{db: dummyDB}),
	}

	evalCtx := &EvalContext{}
	cond := &RuleCondition{
		Field:    FieldTags,
		Operator: OperatorEqual,
		Value:    "tier1",
	}

	s.setupPreviewTrackerDisplayNames(ctx, 1, cond, evalCtx)

	assert.Nil(t, evalCtx.TrackerDisplayNameByDomain)
}

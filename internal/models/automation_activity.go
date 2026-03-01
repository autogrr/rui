// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/autogrr/rui/internal/dbinterface"
)

// Activity action types
const (
	ActivityActionDeletedRatio        = "deleted_ratio"
	ActivityActionDeletedSeeding      = "deleted_seeding"
	ActivityActionDeletedUnregistered = "deleted_unregistered"
	ActivityActionDeletedCondition    = "deleted_condition" // Expression-based deletion
	ActivityActionDeleteFailed        = "delete_failed"
	ActivityActionLimitFailed         = "limit_failed"
	ActivityActionTagsChanged         = "tags_changed"         // Batch tag operation
	ActivityActionCategoryChanged     = "category_changed"     // Batch category operation
	ActivityActionSpeedLimitsChanged  = "speed_limits_changed" // Batch speed limit operation
	ActivityActionShareLimitsChanged  = "share_limits_changed" // Batch share limit operation
	ActivityActionPaused              = "paused"               // Batch pause operation
	ActivityActionResumed             = "resumed"              // Batch resume operation
	ActivityActionRechecked           = "rechecked"            // Batch force recheck operation
	ActivityActionReannounced         = "reannounced"          // Batch force reannounce operation
	ActivityActionMoved               = "moved"                // Batch move operation
	ActivityActionDryRunNoMatch       = "dry_run_no_match"     // Manual dry-run completed with no matching actions
)

// Activity outcome types
const (
	ActivityOutcomeSuccess = "success"
	ActivityOutcomeFailed  = "failed"
	ActivityOutcomeDryRun  = "dry-run"
)

type AutomationActivity struct {
	ID            int             `json:"id"`
	OwnerID       int             `json:"ownerId"`
	InstanceID    int             `json:"instanceId"`
	Hash          string          `json:"hash"`
	TorrentName   string          `json:"torrentName,omitempty"`
	TrackerDomain string          `json:"trackerDomain,omitempty"`
	Action        string          `json:"action"`
	RuleID        *int            `json:"ruleId,omitempty"`
	RuleName      string          `json:"ruleName,omitempty"`
	Outcome       string          `json:"outcome"`
	Reason        string          `json:"reason,omitempty"`
	Details       json.RawMessage `json:"details,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type AutomationActivityStore struct {
	db dbinterface.Querier
}

func NewAutomationActivityStore(db dbinterface.Querier) *AutomationActivityStore {
	return &AutomationActivityStore{db: db}
}

func (s *AutomationActivityStore) Create(ctx context.Context, activity *AutomationActivity) error {
	if activity == nil {
		return nil
	}

	_, err := s.insert(ctx, activity)
	return err
}

func (s *AutomationActivityStore) CreateWithID(ctx context.Context, activity *AutomationActivity) (int, error) {
	if activity == nil {
		return 0, nil
	}

	res, err := s.insert(ctx, activity)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

func (s *AutomationActivityStore) insert(ctx context.Context, activity *AutomationActivity) (sql.Result, error) {
	if activity == nil {
		return nil, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Look up owner_id from the instance
	var ownerID int
	if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM instances WHERE id = ?`, activity.InstanceID).Scan(&ownerID); err != nil {
		return nil, fmt.Errorf("failed to get instance owner: %w", err)
	}

	// Intern required strings: hash, action, outcome
	ids, err := dbinterface.InternStrings(ctx, tx, activity.Hash, activity.Action, activity.Outcome)
	if err != nil {
		return nil, fmt.Errorf("failed to intern strings: %w", err)
	}
	hashID, actionID, outcomeID := ids[0], ids[1], ids[2]

	// Intern nullable strings: torrent_name, tracker_domain, rule_name, reason, details
	var detailsPtr *string
	if len(activity.Details) > 0 {
		d := string(activity.Details)
		detailsPtr = &d
	}
	nullIDs, err := dbinterface.InternStringNullable(ctx, tx,
		&activity.TorrentName,
		&activity.TrackerDomain,
		&activity.RuleName,
		&activity.Reason,
		detailsPtr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to intern nullable strings: %w", err)
	}

	var ruleID sql.NullInt64
	if activity.RuleID != nil {
		ruleID = sql.NullInt64{Int64: int64(*activity.RuleID), Valid: true}
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO automation_activity
			(owner_id, instance_id, hash_id, torrent_name_id, tracker_domain_id, action_id, rule_id, rule_name_id, outcome_id, reason_id, details_id)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ownerID, activity.InstanceID, hashID, nullIDs[0], nullIDs[1], actionID, ruleID, nullIDs[2], outcomeID, nullIDs[3], nullIDs[4])
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return res, nil
}

func (s *AutomationActivityStore) ListByInstance(ctx context.Context, instanceID int, limit int) ([]*AutomationActivity, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_id, instance_id, hash, torrent_name, tracker_domain, action, rule_id, rule_name, outcome, reason, details, created_at
		FROM automation_activity_view
		WHERE instance_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []*AutomationActivity
	for rows.Next() {
		var a AutomationActivity
		var torrentName, trackerDomain, ruleName, reason, details sql.NullString
		var ruleID sql.NullInt64

		if err := rows.Scan(
			&a.ID,
			&a.OwnerID,
			&a.InstanceID,
			&a.Hash,
			&torrentName,
			&trackerDomain,
			&a.Action,
			&ruleID,
			&ruleName,
			&a.Outcome,
			&reason,
			&details,
			&a.CreatedAt,
		); err != nil {
			return nil, err
		}

		if torrentName.Valid {
			a.TorrentName = torrentName.String
		}
		if trackerDomain.Valid {
			a.TrackerDomain = trackerDomain.String
		}
		if ruleID.Valid {
			id := int(ruleID.Int64)
			a.RuleID = &id
		}
		if ruleName.Valid {
			a.RuleName = ruleName.String
		}
		if reason.Valid {
			a.Reason = reason.String
		}
		if details.Valid && details.String != "" {
			a.Details = json.RawMessage(details.String)
		}

		activities = append(activities, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return activities, nil
}

func (s *AutomationActivityStore) DeleteOlderThan(ctx context.Context, instanceID int, days int) (int64, error) {
	// days == 0 means delete ALL activity for this instance
	if days == 0 {
		res, err := s.db.ExecContext(ctx, `DELETE FROM automation_activity WHERE instance_id = ?`, instanceID)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}

	if days < 0 {
		days = 7
	}

	res, err := s.db.ExecContext(ctx, `
		DELETE FROM automation_activity
		WHERE instance_id = ? AND created_at < datetime('now', '-' || ? || ' days')
	`, instanceID, days)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func (s *AutomationActivityStore) Prune(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 7
	}

	res, err := s.db.ExecContext(ctx, `
		DELETE FROM automation_activity
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`, retentionDays)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

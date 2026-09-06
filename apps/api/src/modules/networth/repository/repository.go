package repository

import (
	"context"
	"errors"
	"fmt"

	"ledgermeadow/src/modules/networth/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrResponseBound = errors.New("net worth response exceeds bound")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context, userID shared.UserID) (models.NetWorth, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return models.NetWorth{}, fmt.Errorf("begin net worth read: %w", err)
	}
	defer tx.Rollback(ctx)
	result := models.NetWorth{
		Accounts: make([]models.Account, 0), Assets: make([]models.ManualItem, 0),
		Liabilities: make([]models.ManualItem, 0), Snapshots: make([]models.Snapshot, 0),
	}
	rows, err := tx.Query(ctx, `
			SELECT a.id::text, a.name, a.account_type, a.balance_minor, a.currency
			FROM accounts a
			JOIN bank_connections bc ON bc.id = a.bank_connection_id
			WHERE a.user_id = $1 AND bc.status <> 'DISCONNECTED'
			ORDER BY a.name, a.id LIMIT 101
	`, string(userID))
	if err != nil {
		return models.NetWorth{}, fmt.Errorf("load net worth accounts: %w", err)
	}
	for rows.Next() {
		var account models.Account
		if err := rows.Scan(&account.ID, &account.Name, &account.Type, &account.BalanceMinor, &account.Currency); err != nil {
			rows.Close()
			return models.NetWorth{}, fmt.Errorf("scan net worth account: %w", err)
		}
		result.Accounts = append(result.Accounts, account)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return models.NetWorth{}, fmt.Errorf("iterate net worth accounts: %w", err)
	}
	rows.Close()
	if len(result.Accounts) > 100 {
		return models.NetWorth{}, ErrResponseBound
	}
	rows, err = tx.Query(ctx, `
		SELECT id::text, name, asset_type, value_minor, currency
		FROM manual_assets
		WHERE user_id = $1 AND include_in_net_worth = true
		ORDER BY created_at, id LIMIT 101
	`, string(userID))
	if err != nil {
		return models.NetWorth{}, fmt.Errorf("load net worth assets: %w", err)
	}
	for rows.Next() {
		var item models.ManualItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.AmountMinor, &item.Currency); err != nil {
			rows.Close()
			return models.NetWorth{}, fmt.Errorf("scan net worth asset: %w", err)
		}
		result.Assets = append(result.Assets, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return models.NetWorth{}, fmt.Errorf("iterate net worth assets: %w", err)
	}
	rows.Close()
	if len(result.Assets) > 100 {
		return models.NetWorth{}, ErrResponseBound
	}
	rows, err = tx.Query(ctx, `
		SELECT id::text, name, liability_type, balance_minor, currency
		FROM manual_liabilities
		WHERE user_id = $1 AND include_in_net_worth = true
		ORDER BY created_at, id LIMIT 101
	`, string(userID))
	if err != nil {
		return models.NetWorth{}, fmt.Errorf("load net worth liabilities: %w", err)
	}
	for rows.Next() {
		var item models.ManualItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.AmountMinor, &item.Currency); err != nil {
			rows.Close()
			return models.NetWorth{}, fmt.Errorf("scan net worth liability: %w", err)
		}
		result.Liabilities = append(result.Liabilities, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return models.NetWorth{}, fmt.Errorf("iterate net worth liabilities: %w", err)
	}
	rows.Close()
	if len(result.Liabilities) > 100 {
		return models.NetWorth{}, ErrResponseBound
	}
	rows, err = tx.Query(ctx, `
		SELECT snapshot_date::text, value_minor, currency, total_assets_minor, total_liabilities_minor
		FROM net_worth_snapshots
		WHERE user_id = $1 ORDER BY snapshot_date DESC, currency LIMIT 367
	`, string(userID))
	if err != nil {
		return models.NetWorth{}, fmt.Errorf("load net worth snapshots: %w", err)
	}
	for rows.Next() {
		var snapshot models.Snapshot
		if err := rows.Scan(
			&snapshot.Date, &snapshot.ValueMinor, &snapshot.Currency,
			&snapshot.TotalAssetsMinor, &snapshot.TotalLiabilitiesMinor,
		); err != nil {
			rows.Close()
			return models.NetWorth{}, fmt.Errorf("scan net worth snapshot: %w", err)
		}
		result.Snapshots = append(result.Snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return models.NetWorth{}, fmt.Errorf("iterate net worth snapshots: %w", err)
	}
	rows.Close()
	if len(result.Snapshots) > 366 {
		return models.NetWorth{}, ErrResponseBound
	}
	if err := tx.Commit(ctx); err != nil {
		return models.NetWorth{}, fmt.Errorf("commit net worth read: %w", err)
	}
	return result, nil
}

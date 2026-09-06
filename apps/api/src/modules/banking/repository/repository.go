package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ledgermeadow/src/entities"
	banking "ledgermeadow/src/modules/banking/types"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConnectionOwnershipConflict = errors.New("bank connection belongs to another user")
var ErrResponseBound = errors.New("banking response exceeds the configured bound")
var ErrNotFound = errors.New("banking resource not found")

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) StoreConnection(
	ctx context.Context,
	userID shared.UserID,
	providerItemID string,
	institutionID string,
	institutionName string,
	encryptedToken []byte,
	dedupeKey string,
) (shared.BankConnectionID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin bank connection transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var connectionID string
	err = tx.QueryRow(ctx, `
		INSERT INTO bank_connections (
			user_id, provider, provider_item_id, institution_id, institution_name,
			access_token_ciphertext, status, last_sync_error_code
		)
		VALUES ($1, 'PLAID', $2, NULLIF($3, ''), NULLIF($4, ''), $5, 'SYNC_PENDING', NULL)
		ON CONFLICT (provider, provider_item_id) DO UPDATE SET
			access_token_ciphertext = EXCLUDED.access_token_ciphertext,
			institution_id = COALESCE(EXCLUDED.institution_id, bank_connections.institution_id),
			institution_name = COALESCE(EXCLUDED.institution_name, bank_connections.institution_name),
			status = 'SYNC_PENDING', last_sync_error_code = NULL, disconnected_at = NULL, updated_at = now()
		WHERE bank_connections.user_id = EXCLUDED.user_id
		RETURNING id::text
	`, string(userID), providerItemID, institutionID, institutionName, encryptedToken).Scan(&connectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrConnectionOwnershipConflict
	}
	if err != nil {
		return "", fmt.Errorf("store bank connection: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sync_cursors (bank_connection_id) VALUES ($1)
		ON CONFLICT (bank_connection_id) DO NOTHING
	`, connectionID); err != nil {
		return "", fmt.Errorf("initialize sync cursor: %w", err)
	}
	payload, err := json.Marshal(map[string]string{"connection_id": connectionID})
	if err != nil {
		return "", fmt.Errorf("encode sync event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (
			user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload
		) VALUES ($1, 'BANK_CONNECTION', $2, 'BANK_CONNECTION_SYNC_REQUESTED', $3, $4)
		ON CONFLICT (dedupe_key) DO NOTHING
	`, string(userID), connectionID, dedupeKey, payload); err != nil {
		return "", fmt.Errorf("enqueue initial bank sync: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit bank connection: %w", err)
	}
	return shared.BankConnectionID(connectionID), nil
}

func (r *Repository) ListConnectionStatus(ctx context.Context, userID shared.UserID) ([]banking.ConnectionStatus, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, provider, COALESCE(institution_name, 'Connected institution'), status,
		       last_sync_at, last_sync_error_code
		FROM bank_connections
			WHERE user_id = $1 AND status <> 'DISCONNECTED'
		ORDER BY created_at, id
		LIMIT 101
	`, string(userID))
	if err != nil {
		return nil, fmt.Errorf("list bank connections: %w", err)
	}
	defer rows.Close()

	var statuses []banking.ConnectionStatus
	for rows.Next() {
		var status banking.ConnectionStatus
		var id string
		if err := rows.Scan(&id, &status.Provider, &status.InstitutionName, &status.Status, &status.LastSyncAt, &status.LastErrorCode); err != nil {
			return nil, fmt.Errorf("scan bank connection: %w", err)
		}
		status.ID = shared.BankConnectionID(id)
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bank connections: %w", err)
	}
	if len(statuses) > 100 {
		return nil, ErrResponseBound
	}
	return statuses, nil
}

func (r *Repository) ListAccounts(ctx context.Context, userID shared.UserID) ([]banking.Account, error) {
	rows, err := r.db.Query(ctx, `
			SELECT a.id::text, a.provider, a.name, a.official_name, a.mask, a.account_type, a.account_subtype,
			       a.balance_minor, a.available_balance_minor, a.currency
			FROM accounts a
			JOIN bank_connections bc ON bc.id = a.bank_connection_id
			WHERE a.user_id = $1 AND bc.status <> 'DISCONNECTED'
			ORDER BY a.name, a.id
		LIMIT 101
	`, string(userID))
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []banking.Account
	for rows.Next() {
		var account banking.Account
		var id string
		var currency string
		if err := rows.Scan(
			&id, &account.Provider, &account.Name, &account.OfficialName, &account.Mask, &account.Type,
			&account.Subtype, &account.BalanceMinor, &account.AvailableBalanceMinor, &currency,
		); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		account.ID = shared.AccountID(id)
		account.Currency = shared.Currency(currency)
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	if len(accounts) > 100 {
		return nil, ErrResponseBound
	}
	return accounts, nil
}

func (r *Repository) AccountDetail(
	ctx context.Context,
	userID shared.UserID,
	accountID shared.AccountID,
) (banking.AccountDetail, error) {
	var detail banking.AccountDetail
	var id, connectionID, currency string
	err := r.db.QueryRow(ctx, `
			SELECT a.id::text, a.bank_connection_id::text, a.provider, a.name, a.official_name, a.mask,
			       a.account_type, a.account_subtype, a.balance_minor, a.available_balance_minor, a.currency
			FROM accounts a
			JOIN bank_connections bc ON bc.id = a.bank_connection_id
			WHERE a.id = $1 AND a.user_id = $2 AND bc.status <> 'DISCONNECTED'
	`, string(accountID), string(userID)).Scan(
		&id, &connectionID, &detail.Provider, &detail.Name, &detail.OfficialName, &detail.Mask, &detail.Type,
		&detail.Subtype, &detail.BalanceMinor, &detail.AvailableBalanceMinor, &currency,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return banking.AccountDetail{}, ErrNotFound
	}
	if err != nil {
		return banking.AccountDetail{}, fmt.Errorf("load account detail: %w", err)
	}
	detail.ID = shared.AccountID(id)
	detail.ConnectionID = shared.BankConnectionID(connectionID)
	detail.Currency = shared.Currency(currency)
	detail.Transactions = make([]banking.AccountTransaction, 0, 50)
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, merchant_name, amount_minor, currency,
		       transaction_date, is_pending
		FROM transactions
		WHERE user_id = $1 AND account_id = $2 AND removed_at IS NULL
		ORDER BY transaction_date DESC, id DESC LIMIT 51
	`, string(userID), string(accountID))
	if err != nil {
		return banking.AccountDetail{}, fmt.Errorf("load account transactions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item banking.AccountTransaction
		var transactionID, transactionCurrency string
		if err := rows.Scan(&transactionID, &item.Name, &item.MerchantName, &item.AmountMinor,
			&transactionCurrency, &item.Date, &item.IsPending); err != nil {
			return banking.AccountDetail{}, fmt.Errorf("scan account transaction: %w", err)
		}
		item.ID = shared.TransactionID(transactionID)
		item.Currency = shared.Currency(transactionCurrency)
		detail.Transactions = append(detail.Transactions, item)
	}
	if err := rows.Err(); err != nil {
		return banking.AccountDetail{}, fmt.Errorf("iterate account transactions: %w", err)
	}
	if len(detail.Transactions) > 50 {
		return banking.AccountDetail{}, ErrResponseBound
	}
	return detail, nil
}

func (r *Repository) EnqueueManualSync(
	ctx context.Context,
	userID shared.UserID,
	connectionID shared.BankConnectionID,
	dedupeKey string,
) error {
	payload, err := json.Marshal(map[string]string{"connection_id": string(connectionID)})
	if err != nil {
		return fmt.Errorf("encode manual sync event: %w", err)
	}
	result, err := r.db.Exec(ctx, `
		INSERT INTO outbox_events (
			user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload
		)
		SELECT $1, 'BANK_CONNECTION', id, 'BANK_CONNECTION_SYNC_REQUESTED', $3, $4
			FROM bank_connections WHERE id = $2 AND user_id = $1 AND status <> 'DISCONNECTED'
		ON CONFLICT (dedupe_key) DO NOTHING
	`, string(userID), string(connectionID), dedupeKey, payload)
	if err != nil {
		return fmt.Errorf("enqueue manual bank sync: %w", err)
	}
	if result.RowsAffected() == 0 {
		var exists bool
		if err := r.db.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM bank_connections
					WHERE id = $1 AND user_id = $2 AND status <> 'DISCONNECTED'
				)
		`, string(connectionID), string(userID)).Scan(&exists); err != nil {
			return fmt.Errorf("verify manual bank sync ownership: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *Repository) LoadOwnedConnection(ctx context.Context, userID shared.UserID, connectionID shared.BankConnectionID) (banking.OwnedConnection, error) {
	var connection banking.OwnedConnection
	err := r.db.QueryRow(ctx, `
		SELECT provider, access_token_ciphertext FROM bank_connections
		WHERE id=$1 AND user_id=$2 AND status <> 'DISCONNECTED'
	`, string(connectionID), string(userID)).Scan(&connection.Provider, &connection.EncryptedToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return banking.OwnedConnection{}, ErrNotFound
	}
	if err != nil {
		return banking.OwnedConnection{}, fmt.Errorf("load owned bank connection: %w", err)
	}
	return connection, nil
}

func (r *Repository) LoadForSync(ctx context.Context, connectionID shared.BankConnectionID) (banking.SyncConnection, error) {
	var connection banking.SyncConnection
	var id, userID string
	err := r.db.QueryRow(ctx, `
		SELECT bc.id::text, bc.user_id::text, bc.access_token_ciphertext, sc.cursor
		FROM bank_connections bc
		JOIN sync_cursors sc ON sc.bank_connection_id = bc.id
		WHERE bc.id = $1 AND bc.status <> 'DISCONNECTED'
	`, string(connectionID)).Scan(&id, &userID, &connection.EncryptedToken, &connection.Cursor)
	if err != nil {
		return banking.SyncConnection{}, fmt.Errorf("load bank connection for sync: %w", err)
	}
	connection.ID = shared.BankConnectionID(id)
	connection.UserID = shared.UserID(userID)
	return connection, nil
}

func (r *Repository) MarkSyncing(ctx context.Context, connectionID shared.BankConnectionID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE bank_connections
		SET status = 'SYNCING', last_sync_error_code = NULL, updated_at = now()
		WHERE id = $1 AND status <> 'DISCONNECTED'
	`, string(connectionID))
	return err
}

func (r *Repository) MarkReady(ctx context.Context, connectionID shared.BankConnectionID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE bank_connections
		SET status = 'READY', last_sync_at = now(), last_sync_error_code = NULL, updated_at = now()
		WHERE id = $1 AND status <> 'DISCONNECTED'
	`, string(connectionID))
	return err
}

func (r *Repository) MarkError(ctx context.Context, connectionID shared.BankConnectionID, errorCode string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE bank_connections
		SET status = 'ERROR', last_sync_error_code = $2, updated_at = now()
		WHERE id = $1 AND status <> 'DISCONNECTED'
	`, string(connectionID), errorCode)
	return err
}

func (r *Repository) UpsertAccounts(
	ctx context.Context,
	connection banking.SyncConnection,
	accounts []banking.NormalizedAccount,
) (map[string]shared.AccountID, error) {
	if len(accounts) > 100 {
		return nil, errors.New("provider returned too many accounts")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin account sync: %w", err)
	}
	defer tx.Rollback(ctx)
	var activeConnectionID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM bank_connections
		WHERE id = $1 AND user_id = $2 AND status <> 'DISCONNECTED'
		FOR UPDATE
	`, string(connection.ID), string(connection.UserID)).Scan(&activeConnectionID); err != nil {
		return nil, fmt.Errorf("lock active bank connection: %w", err)
	}

	ids := make(map[string]shared.AccountID, len(accounts))
	for _, account := range accounts {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO accounts (
				user_id, bank_connection_id, provider, provider_account_id, name,
				official_name, mask, account_type, account_subtype, balance_minor,
				available_balance_minor, currency
			) VALUES ($1, $2, 'PLAID', $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (bank_connection_id, provider_account_id) DO UPDATE SET
				name = EXCLUDED.name, official_name = EXCLUDED.official_name,
				mask = EXCLUDED.mask, account_type = EXCLUDED.account_type,
				account_subtype = EXCLUDED.account_subtype,
				balance_minor = EXCLUDED.balance_minor,
				available_balance_minor = EXCLUDED.available_balance_minor,
				currency = EXCLUDED.currency, updated_at = now()
			WHERE accounts.user_id = EXCLUDED.user_id
			RETURNING id::text
		`, string(connection.UserID), string(connection.ID), account.ProviderAccountID, account.Name,
			account.OfficialName, account.Mask, account.Type, account.Subtype, account.BalanceMinor,
			account.AvailableBalanceMinor, string(account.Currency)).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("upsert account: %w", err)
		}
		ids[account.ProviderAccountID] = shared.AccountID(id)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit account sync: %w", err)
	}
	return ids, nil
}

func (r *Repository) ConnectionByProviderItem(ctx context.Context, providerItemID string) (shared.BankConnectionID, shared.UserID, error) {
	var connectionID, userID string
	err := r.db.QueryRow(ctx, `
		SELECT id::text, user_id::text FROM bank_connections
		WHERE provider = 'PLAID' AND provider_item_id = $1 AND status <> 'DISCONNECTED'
	`, providerItemID).Scan(&connectionID, &userID)
	if err != nil {
		return "", "", fmt.Errorf("find provider item: %w", err)
	}
	return shared.BankConnectionID(connectionID), shared.UserID(userID), nil
}

func (r *Repository) EnqueueWebhookSync(
	ctx context.Context,
	connectionID shared.BankConnectionID,
	userID shared.UserID,
	dedupeKey string,
) error {
	payload, _ := json.Marshal(map[string]string{"connection_id": string(connectionID)})
	_, err := r.db.Exec(ctx, `
		INSERT INTO outbox_events (
			user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload
		)
		SELECT $1, 'BANK_CONNECTION', id, 'BANK_CONNECTION_SYNC_REQUESTED', $3, $4
		FROM bank_connections
		WHERE id = $2 AND user_id = $1 AND status <> 'DISCONNECTED'
		ON CONFLICT (dedupe_key) DO NOTHING
	`, string(userID), string(connectionID), dedupeKey, payload)
	if err != nil {
		return fmt.Errorf("enqueue webhook sync: %w", err)
	}
	return nil
}

func (r *Repository) Disconnect(
	ctx context.Context,
	userID shared.UserID,
	connectionID shared.BankConnectionID,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bank disconnect: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownedConnectionID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM bank_connections
		WHERE id = $1 AND user_id = $2 AND status <> 'DISCONNECTED'
		FOR UPDATE
	`, string(connectionID), string(userID)).Scan(&ownedConnectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock owned bank connection: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE subscriptions
		SET payment_account_id = NULL, updated_at = now()
		WHERE user_id = $1
		  AND payment_account_id IN (
			SELECT id FROM accounts WHERE bank_connection_id = $2 AND user_id = $1
		  )
	`, string(userID), string(connectionID)); err != nil {
		return fmt.Errorf("detach disconnected subscription accounts: %w", err)
	}

	var resolvedInboxCount int
	if err := tx.QueryRow(ctx, `
		WITH resolved AS (
			UPDATE inbox_items
			SET status = 'RESOLVED', resolved_at = now()
			WHERE user_id = $1 AND item_type = 'BANK_CONNECTION_ERROR'
			  AND entity_type = $2 AND entity_id = $3 AND status = 'OPEN'
			RETURNING 1
		)
		SELECT count(*) FROM resolved
	`, string(userID), string(entities.BankConnection), string(connectionID)).Scan(&resolvedInboxCount); err != nil {
		return fmt.Errorf("resolve disconnected bank inbox items: %w", err)
	}
	if resolvedInboxCount > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE users
			SET open_inbox_count = GREATEST(open_inbox_count - $2, 0), updated_at = now()
			WHERE id = $1
		`, string(userID), resolvedInboxCount); err != nil {
			return fmt.Errorf("update disconnected bank inbox count: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM outbox_events
		WHERE user_id = $1 AND aggregate_type = $2 AND aggregate_id = $3
	`, string(userID), string(entities.BankConnection), string(connectionID)); err != nil {
		return fmt.Errorf("remove disconnected bank sync events: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sync_cursors WHERE bank_connection_id = $1`, string(connectionID)); err != nil {
		return fmt.Errorf("remove disconnected bank cursor: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM financial_projections WHERE user_id = $1`, string(userID)); err != nil {
		return fmt.Errorf("invalidate disconnected bank projections: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bank_connections
		SET access_token_ciphertext = NULL, status = 'DISCONNECTED', disconnected_at = now(),
		    last_sync_error_code = NULL, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND status <> 'DISCONNECTED'
	`, string(connectionID), string(userID)); err != nil {
		return fmt.Errorf("mark bank connection disconnected: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (
			user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload
		) VALUES ($1, $2, $1, 'FINANCIAL_MATERIALIZATION_REQUESTED', $3, '{}'::jsonb)
		ON CONFLICT (dedupe_key) DO NOTHING
	`, string(userID), string(entities.User), "financial-materialization:disconnect:"+string(connectionID)); err != nil {
		return fmt.Errorf("enqueue post-disconnect materialization: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'BANK_DISCONNECTED', $2, $3)
	`, string(userID), string(entities.BankConnection), string(connectionID)); err != nil {
		return fmt.Errorf("audit bank disconnect: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bank disconnect: %w", err)
	}
	return nil
}

func NewDedupeKey(prefix string, value string, now time.Time) string {
	return fmt.Sprintf("%s:%s:%d", prefix, value, now.UTC().UnixNano())
}

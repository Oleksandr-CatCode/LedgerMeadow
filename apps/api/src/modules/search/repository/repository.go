package repository

import (
	"context"
	"fmt"
	"strings"

	"ledgermeadow/src/modules/search/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Prefix(ctx context.Context, userID shared.UserID, query string) ([]models.Result, error) {
	prefix := escapeLikePrefix(strings.ToLower(strings.TrimSpace(query))) + "%"
	rows, err := r.db.Query(ctx, `
		SELECT id::text, kind, label, subtitle FROM (
			(SELECT id, 'TRANSACTION'::text kind, name label, transaction_date::text subtitle
			 FROM transactions WHERE user_id=$1 AND removed_at IS NULL AND lower(name) LIKE $2 ESCAPE '\'
			 ORDER BY lower(name),id LIMIT 5)
			UNION ALL
			(SELECT a.id,'ACCOUNT',a.name,a.account_type FROM accounts a
			 JOIN bank_connections bc ON bc.id=a.bank_connection_id
			 WHERE a.user_id=$1 AND bc.status<>'DISCONNECTED' AND lower(a.name) LIKE $2 ESCAPE '\'
			 ORDER BY lower(a.name),a.id LIMIT 5)
			UNION ALL
			(SELECT id,'SPACE',name,space_type FROM spaces
			 WHERE user_id=$1 AND lower(name) LIKE $2 ESCAPE '\' ORDER BY lower(name),id LIMIT 5)
			UNION ALL
			(SELECT id,'BILL',name,frequency FROM bills
			 WHERE user_id=$1 AND lower(name) LIKE $2 ESCAPE '\' ORDER BY lower(name),id LIMIT 5)
			UNION ALL
			(SELECT id,'SUBSCRIPTION',merchant_name,frequency FROM subscriptions
			 WHERE user_id=$1 AND lower(merchant_name) LIKE $2 ESCAPE '\' ORDER BY lower(merchant_name),id LIMIT 5)
			UNION ALL
			(SELECT id,'GOAL',name,status FROM goals
			 WHERE user_id=$1 AND lower(name) LIKE $2 ESCAPE '\' ORDER BY lower(name),id LIMIT 5)
			UNION ALL
			(SELECT id,'RULE',name,CASE WHEN enabled THEN 'ACTIVE' ELSE 'PAUSED' END FROM rules
			 WHERE user_id=$1 AND lower(name) LIKE $2 ESCAPE '\' ORDER BY lower(name),id LIMIT 5)
		) results ORDER BY label,kind,id LIMIT 30
	`, string(userID), prefix)
	if err != nil {
		return nil, fmt.Errorf("search financial workspace: %w", err)
	}
	defer rows.Close()
	items := make([]models.Result, 0, 30)
	for rows.Next() {
		var item models.Result
		if err := rows.Scan(&item.ID, &item.Kind, &item.Label, &item.Subtitle); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return items, nil
}

func escapeLikePrefix(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

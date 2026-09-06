package repository

import (
	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/loans/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

var ErrNotFound = errors.New("loan not found")
var ErrResponseBound = errors.New("loan response exceeds bound")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(c context.Context, u shared.UserID) ([]models.Loan, error) {
	rows, e := r.db.Query(c, `SELECT id::text,name,principal_remaining_minor,currency,interest_rate_basis_points,monthly_payment_minor,next_payment_at::text FROM loans WHERE user_id=$1 ORDER BY next_payment_at NULLS LAST,id LIMIT 101`, string(u))
	if e != nil {
		return nil, fmt.Errorf("list loans: %w", e)
	}
	defer rows.Close()
	v := make([]models.Loan, 0, 100)
	for rows.Next() {
		var i models.Loan
		if e := rows.Scan(&i.ID, &i.Name, &i.PrincipalRemainingMinor, &i.Currency, &i.InterestRateBasisPoints, &i.MonthlyPaymentMinor, &i.NextPaymentAt); e != nil {
			return nil, fmt.Errorf("scan loan: %w", e)
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate loans: %w", e)
	}
	if len(v) > 100 {
		return nil, ErrResponseBound
	}
	return v, nil
}

func (r *Repository) Create(c context.Context, u shared.UserID, command models.Create) (string, error) {
	tx, err := r.db.Begin(c)
	if err != nil {
		return "", fmt.Errorf("begin loan create: %w", err)
	}
	defer tx.Rollback(c)
	var id string
	err = tx.QueryRow(c, `INSERT INTO loans(user_id,name,principal_remaining_minor,currency,interest_rate_basis_points,monthly_payment_minor,next_payment_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, string(u), strings.TrimSpace(command.Name), command.PrincipalRemainingMinor, command.Currency, command.InterestRateBasisPoints, command.MonthlyPaymentMinor, command.NextPaymentAt).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create loan: %w", err)
	}
	if _, err = tx.Exec(c, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id) VALUES($1,'LOAN_CREATED',$2,$3)`, string(u), string(entities.Loan), id); err != nil {
		return "", fmt.Errorf("audit loan create: %w", err)
	}
	if _, err = tx.Exec(c, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(u), string(entities.User)); err != nil {
		return "", fmt.Errorf("enqueue loan materialization: %w", err)
	}
	if err = tx.Commit(c); err != nil {
		return "", fmt.Errorf("commit loan create: %w", err)
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, userID shared.UserID, id string, command models.Update) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin loan update: %w", err)
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE loans SET name=$3,principal_remaining_minor=$4,currency=$5,interest_rate_basis_points=$6,monthly_payment_minor=$7,next_payment_at=$8,updated_at=now() WHERE user_id=$1 AND id=$2`, string(userID), id, strings.TrimSpace(command.Name), command.PrincipalRemainingMinor, command.Currency, command.InterestRateBasisPoints, command.MonthlyPaymentMinor, command.NextPaymentAt)
	if err != nil {
		return fmt.Errorf("update loan: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id) VALUES($1,'LOAN_UPDATED',$2,$3)`, string(userID), string(entities.Loan), id); err != nil {
		return fmt.Errorf("audit loan update: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(userID), string(entities.User)); err != nil {
		return fmt.Errorf("enqueue loan materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit loan update: %w", err)
	}
	return nil
}
func (r *Repository) Detail(c context.Context, u shared.UserID, id string) (models.Detail, error) {
	var d models.Detail
	e := r.db.QueryRow(c, `SELECT id::text,name,principal_remaining_minor,currency,interest_rate_basis_points,monthly_payment_minor,next_payment_at::text FROM loans WHERE user_id=$1 AND id=$2`, string(u), id).Scan(&d.ID, &d.Name, &d.PrincipalRemainingMinor, &d.Currency, &d.InterestRateBasisPoints, &d.MonthlyPaymentMinor, &d.NextPaymentAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return models.Detail{}, ErrNotFound
	}
	if e != nil {
		return models.Detail{}, fmt.Errorf("load loan: %w", e)
	}
	rows, e := r.db.Query(c, `SELECT id::text,paid_at::text,amount_minor,principal_minor,interest_minor FROM loan_payments WHERE loan_id=$1 ORDER BY paid_at DESC,id DESC LIMIT 601`, id)
	if e != nil {
		return models.Detail{}, fmt.Errorf("load loan payments: %w", e)
	}
	defer rows.Close()
	d.Payments = make([]models.Payment, 0, 600)
	for rows.Next() {
		var p models.Payment
		if e := rows.Scan(&p.ID, &p.PaidAt, &p.AmountMinor, &p.PrincipalMinor, &p.InterestMinor); e != nil {
			return models.Detail{}, fmt.Errorf("scan loan payment: %w", e)
		}
		d.Payments = append(d.Payments, p)
	}
	if e := rows.Err(); e != nil {
		return models.Detail{}, fmt.Errorf("iterate loan payments: %w", e)
	}
	if len(d.Payments) > 600 {
		return models.Detail{}, ErrResponseBound
	}
	return d, nil
}

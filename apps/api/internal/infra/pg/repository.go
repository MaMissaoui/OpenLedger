// Package pg provides PostgreSQL-backed implementations of the app repository
// ports, using pgx/v5.
package pg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// Repository implements app repository ports over a pgx connection pool.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wraps a pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// InsertTransaction writes the transaction, its splits, and an audit_log row in
// a single DB transaction. The caller (PostingService) is responsible for
// having validated the balance invariant before this is reached.
func (r *Repository) InsertTransaction(ctx context.Context, t domain.Transaction, actor app.AuditActor) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO transactions (guid, currency_guid, num, post_date, enter_date, description)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			t.GUID, t.CurrencyGUID, t.Num, t.PostDate, t.EnterDate, t.Description,
		); err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}

		for _, s := range t.Splits {
			valueNum, valueDenom, err := s.Value.NumDenom()
			if err != nil {
				return fmt.Errorf("split %s value: %w", s.GUID, err)
			}
			qtyNum, qtyDenom, err := s.Quantity.NumDenom()
			if err != nil {
				return fmt.Errorf("split %s quantity: %w", s.GUID, err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO splits
				   (guid, tx_guid, account_guid, memo, action, reconcile_state,
				    value_num, value_denom, quantity_num, quantity_denom, lot_guid)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
				s.GUID, t.GUID, s.AccountGUID, s.Memo, s.Action, string(s.Reconcile),
				valueNum, valueDenom, qtyNum, qtyDenom, nullable(s.LotGUID),
			); err != nil {
				return fmt.Errorf("insert split %s: %w", s.GUID, err)
			}
		}

		detail, err := json.Marshal(map[string]any{
			"currency": t.CurrencyGUID,
			"splits":   len(t.Splits),
		})
		if err != nil {
			return fmt.Errorf("marshal audit detail: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO audit_log (actor_user_id, book_guid, action, entity_type, entity_guid, detail)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			nullable(actor.UserID), nullable(actor.BookGUID), "post", "transaction", t.GUID, detail,
		); err != nil {
			return fmt.Errorf("insert audit log: %w", err)
		}
		return nil
	})
}

// nullable maps an empty string to SQL NULL, so optional CHAR/UUID columns are
// stored as NULL rather than an empty/invalid value.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

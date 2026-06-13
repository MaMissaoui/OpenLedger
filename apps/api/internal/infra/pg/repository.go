// Package pg provides PostgreSQL-backed implementations of the app repository
// ports, using pgx/v5.
package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// Repository implements the app repository ports over a pgx connection pool.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wraps a pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// InsertTransaction writes the transaction, its splits, and an audit_log row in
// a single DB transaction. The caller (PostingService) is responsible for
// having validated the balance invariant before this is reached.
//
// To stay faithful to GnuCash, each split's value is stored at the transaction
// currency's fraction and its quantity at the account commodity's fraction
// (e.g. denominator 100 for a currency with cents), rather than as a reduced
// rational. AtDenom errors if an amount is not exact in that smallest unit,
// which is the correct rejection for, say, $50.001.
func (r *Repository) InsertTransaction(ctx context.Context, t domain.Transaction, actor app.AuditActor) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var currencyFraction int64
		if err := tx.QueryRow(ctx,
			`SELECT fraction FROM commodities WHERE guid = $1`, t.CurrencyGUID,
		).Scan(&currencyFraction); err != nil {
			return fmt.Errorf("lookup currency fraction for %s: %w", t.CurrencyGUID, err)
		}

		accountFractions, err := r.accountFractions(ctx, tx, t.Splits)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO transactions (guid, currency_guid, num, post_date, enter_date, description)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			t.GUID, t.CurrencyGUID, t.Num, t.PostDate, t.EnterDate, t.Description,
		); err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}

		for _, s := range t.Splits {
			valueNum, err := s.Value.AtDenom(currencyFraction)
			if err != nil {
				return fmt.Errorf("split %s value: %w", s.GUID, err)
			}
			accFraction, ok := accountFractions[s.AccountGUID]
			if !ok {
				return fmt.Errorf("split %s: unknown account %s", s.GUID, s.AccountGUID)
			}
			qtyNum, err := s.Quantity.AtDenom(accFraction)
			if err != nil {
				return fmt.Errorf("split %s quantity: %w", s.GUID, err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO splits
				   (guid, tx_guid, account_guid, memo, action, reconcile_state,
				    value_num, value_denom, quantity_num, quantity_denom, lot_guid)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
				s.GUID, t.GUID, s.AccountGUID, s.Memo, s.Action, string(s.Reconcile),
				valueNum, currencyFraction, qtyNum, accFraction, nullable(s.LotGUID),
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

// accountFractions returns the commodity fraction for each split's account.
func (r *Repository) accountFractions(ctx context.Context, tx pgx.Tx, splits []domain.Split) (map[string]int64, error) {
	guids := make([]string, 0, len(splits))
	for _, s := range splits {
		guids = append(guids, s.AccountGUID)
	}
	rows, err := tx.Query(ctx,
		`SELECT a.guid, c.fraction
		   FROM accounts a
		   JOIN commodities c ON c.guid = a.commodity_guid
		  WHERE a.guid = ANY($1)`, guids)
	if err != nil {
		return nil, fmt.Errorf("lookup account fractions: %w", err)
	}
	defer rows.Close()

	fractions := make(map[string]int64, len(splits))
	for rows.Next() {
		var guid string
		var fraction int64
		if err := rows.Scan(&guid, &fraction); err != nil {
			return nil, fmt.Errorf("scan account fraction: %w", err)
		}
		fractions[guid] = fraction
	}
	return fractions, rows.Err()
}

// AccountExists reports whether an account with the given GUID exists.
func (r *Repository) AccountExists(ctx context.Context, guid string) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx, `SELECT 1 FROM accounts WHERE guid = $1`, guid).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

const registerSQL = `
SELECT
    s.guid, s.tx_guid, s.memo, s.reconcile_state,
    s.value_num, s.value_denom, s.quantity_num, s.quantity_denom,
    t.post_date, t.description,
    SUM(s.quantity_num) OVER (
        ORDER BY t.post_date, t.enter_date, s.guid
        ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
    ) AS running_num,
    COUNT(*) OVER () AS total
FROM splits s
JOIN transactions t ON t.guid = s.tx_guid
WHERE s.account_guid = $1
ORDER BY t.post_date, t.enter_date, s.guid
LIMIT $2 OFFSET $3`

// ListAccountRegister returns one page of an account's splits ordered by date,
// each with the running balance (in the account's commodity) from the start of
// the ledger. The running balance is a window sum of quantity_num, which is
// exact because every split for an account shares the account commodity's
// fraction as its denominator. total is the full (unpaginated) row count.
func (r *Repository) ListAccountRegister(ctx context.Context, guid string, limit, offset int) ([]app.RegisterEntry, int64, error) {
	rows, err := r.pool.Query(ctx, registerSQL, guid, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query register: %w", err)
	}
	defer rows.Close()

	var (
		entries []app.RegisterEntry
		total   int64
	)
	for rows.Next() {
		var (
			e                                                          app.RegisterEntry
			reconcile                                                  string
			valueNum, valueDenom, qtyNum, qtyDenom, runningNum, rowTot int64
		)
		if err := rows.Scan(
			&e.SplitGUID, &e.TxGUID, &e.Memo, &reconcile,
			&valueNum, &valueDenom, &qtyNum, &qtyDenom,
			&e.PostDate, &e.Description, &runningNum, &rowTot,
		); err != nil {
			return nil, 0, fmt.Errorf("scan register row: %w", err)
		}
		total = rowTot
		e.ValueScale = valueDenom
		e.QuantityScale = qtyDenom
		if reconcile != "" {
			e.Reconcile = domain.ReconcileState([]rune(reconcile)[0])
		}
		if e.Value, err = domain.FromNumDenom(valueNum, valueDenom); err != nil {
			return nil, 0, err
		}
		if e.Quantity, err = domain.FromNumDenom(qtyNum, qtyDenom); err != nil {
			return nil, 0, err
		}
		if e.Balance, err = domain.FromNumDenom(runningNum, qtyDenom); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// nullable maps an empty string to SQL NULL, so optional CHAR/UUID columns are
// stored as NULL rather than an empty/invalid value.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

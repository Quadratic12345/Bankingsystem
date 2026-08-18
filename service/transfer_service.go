package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bankingsystem/db"
	"bankingsystem/models"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrSameAccount       = errors.New("cannot transfer to the same account")
	ErrInvalidAmount     = errors.New("amount must be positive")
)

type TransferService struct {
	pool         *pgxpool.Pool
	maxRetries   int
	retryBaseDur time.Duration
}

func NewTransferService(pool *pgxpool.Pool, maxRetries int, retryBaseDur time.Duration) *TransferService {
	return &TransferService{pool: pool, maxRetries: maxRetries, retryBaseDur: retryBaseDur}
}
func (s *TransferService) Transfer(ctx context.Context, req models.TransferRequest) (*models.Transaction, error) {
	if req.FromAccountID == req.ToAccountID {
		return nil, ErrSameAccount
	}
	if req.AmountCents <= 0 {
		return nil, ErrInvalidAmount
	}

	if existing, err := s.findByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	var result *models.Transaction

	err := db.WithSerializableTx(ctx, s.pool, s.maxRetries, s.retryBaseDur, func(tx pgx.Tx) error {
		txn, txErr := s.runTransferOnce(ctx, tx, req)
		if txErr != nil {
			return txErr
		}
		result = txn
		return nil
	})

	if err != nil {
		s.recordFailedAttempt(ctx, req, err)
		return nil, err
	}

	return result, nil
}

func (s *TransferService) runTransferOnce(ctx context.Context, tx pgx.Tx, req models.TransferRequest) (*models.Transaction, error) {

	firstID, secondID := req.FromAccountID, req.ToAccountID
	if secondID < firstID {
		firstID, secondID = secondID, firstID
	}

	type lockedAccount struct {
		id      string
		balance int64
	}
	locked := make(map[string]lockedAccount)

	for _, id := range []string{firstID, secondID} {
		var bal int64
		err := tx.QueryRow(ctx,
			`SELECT balance_cents FROM accounts WHERE id = $1 FOR UPDATE`, id,
		).Scan(&bal)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		locked[id] = lockedAccount{id: id, balance: bal}
	}

	fromBal := locked[req.FromAccountID].balance
	if fromBal < req.AmountCents {
		return nil, ErrInsufficientFunds
	}

	if _, err := tx.Exec(ctx,
		`UPDATE accounts SET balance_cents = balance_cents - $1, version = version + 1 WHERE id = $2`,
		req.AmountCents, req.FromAccountID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE accounts SET balance_cents = balance_cents + $1, version = version + 1 WHERE id = $2`,
		req.AmountCents, req.ToAccountID); err != nil {
		return nil, err
	}

	var txn models.Transaction
	err := tx.QueryRow(ctx, `
		INSERT INTO transactions
			(from_account_id, to_account_id, amount_cents, status, idempotency_key, completed_at)
		VALUES ($1, $2, $3, 'completed', $4, now())
		RETURNING id, from_account_id, to_account_id, amount_cents, status, idempotency_key, created_at, completed_at`,
		req.FromAccountID, req.ToAccountID, req.AmountCents, req.IdempotencyKey,
	).Scan(&txn.ID, &txn.FromAccountID, &txn.ToAccountID, &txn.AmountCents,
		&txn.Status, &txn.IdempotencyKey, &txn.CreatedAt, &txn.CompletedAt)
	if err != nil {
		return nil, err
	}

	return &txn, nil
}

func (s *TransferService) findByIdempotencyKey(ctx context.Context, key string) (*models.Transaction, error) {
	if key == "" {
		return nil, nil
	}
	var txn models.Transaction
	err := s.pool.QueryRow(ctx, `
		SELECT id, from_account_id, to_account_id, amount_cents, status, idempotency_key, created_at, completed_at
		FROM transactions WHERE idempotency_key = $1`, key,
	).Scan(&txn.ID, &txn.FromAccountID, &txn.ToAccountID, &txn.AmountCents,
		&txn.Status, &txn.IdempotencyKey, &txn.CreatedAt, &txn.CompletedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &txn, nil
}

func (s *TransferService) recordFailedAttempt(ctx context.Context, req models.TransferRequest, cause error) {
	if req.IdempotencyKey == "" {
		return
	}
	reason := cause.Error()
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO transactions (from_account_id, to_account_id, amount_cents, status, idempotency_key, failure_reason)
		VALUES ($1, $2, $3, 'failed', $4, $5)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		req.FromAccountID, req.ToAccountID, req.AmountCents, req.IdempotencyKey, reason)
}
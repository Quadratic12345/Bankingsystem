package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bankingsystem/models"
)

var ErrNotFound = errors.New("not found")
var ErrForbidden = errors.New("forbidden")

type AccountService struct {
	pool *pgxpool.Pool
}

func NewAccountService(pool *pgxpool.Pool) *AccountService {
	return &AccountService{pool: pool}
}

func (s *AccountService) Create(ctx context.Context, userID, currency string) (*models.Account, error) {
	if currency == "" {
		currency = "USD"
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO accounts (user_id, currency, balance_cents, version)
		VALUES ($1, $2, 0, 0)
		RETURNING id, user_id, currency, balance_cents, version, created_at`,
		userID, currency)

	var a models.Account
	if err := row.Scan(&a.ID, &a.UserID, &a.Currency, &a.BalanceCents, &a.Version, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *AccountService) ListForUser(ctx context.Context, userID string) ([]models.Account, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, currency, balance_cents, version, created_at
		FROM accounts WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.UserID, &a.Currency, &a.BalanceCents, &a.Version, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}
func (s *AccountService) GetOwned(ctx context.Context, accountID, userID string) (*models.Account, error) {
	var a models.Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, currency, balance_cents, version, created_at
		FROM accounts WHERE id = $1`, accountID,
	).Scan(&a.ID, &a.UserID, &a.Currency, &a.BalanceCents, &a.Version, &a.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if a.UserID != userID {
		return nil, ErrForbidden
	}
	return &a, nil
}
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"bankingsystem/auth"
	"bankingsystem/models"
)

type AuthHandler struct {
	pool *pgxpool.Pool
	tm   *auth.TokenManager
}

func NewAuthHandler(pool *pgxpool.Pool, tm *auth.TokenManager) *AuthHandler {
	return &AuthHandler{pool: pool, tm: tm}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email required and password must be at least 8 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	var userID string
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		req.Email, hash,
	).Scan(&userID)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { 
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": userID, "email": req.Email})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	var userID, hash string
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, password_hash FROM users WHERE email = $1`, req.Email,
	).Scan(&userID, &hash)

	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !auth.CheckPassword(hash, req.Password)) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	token, expiresAt, err := h.tm.Generate(userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, models.LoginResponse{
		Token:     token,
		ExpiresIn: int64(time.Until(expiresAt).Seconds()),
	})
}
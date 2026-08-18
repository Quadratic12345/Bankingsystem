package routing

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"bankingsystem/auth"
	"bankingsystem/handlers"
	appmw "bankingsystem/middleware"
	"bankingsystem/service"
)
func New(pool *pgxpool.Pool, tm *auth.TokenManager) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(appmw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(10 * time.Second))

	accountSvc := service.NewAccountService(pool)
	transferSvc := service.NewTransferService(pool, 5, 20*time.Millisecond)

	authHandler := handlers.NewAuthHandler(pool, tm)
	accountHandler := handlers.NewAccountHandler(accountSvc)
	transferHandler := handlers.NewTransferHandler(transferSvc, accountSvc)

	r.Get("/health", handlers.Health(pool))

	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/register", authHandler.Register)
		api.Post("/auth/login", authHandler.Login)

		api.Group(func(protected chi.Router) {
			protected.Use(appmw.Auth(tm))

			protected.Post("/accounts", accountHandler.Create)
			protected.Get("/accounts", accountHandler.List)
			protected.Get("/accounts/{id}", accountHandler.Get)

			protected.Post("/transfers", transferHandler.Transfer)
		})
	})

	return r
}
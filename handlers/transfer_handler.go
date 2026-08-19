package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	appmw "bankingsystem/middleware"
	"bankingsystem/models"
	"bankingsystem/service"
)

type TransferHandler struct {
	transferSvc *service.TransferService
	accountSvc  *service.AccountService
}

func NewTransferHandler(transferSvc *service.TransferService, accountSvc *service.AccountService) *TransferHandler {
	return &TransferHandler{transferSvc: transferSvc, accountSvc: accountSvc}
}

func (h *TransferHandler) Transfer(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmw.UserIDFromContext(r.Context())

	var req models.TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key is required")
		return
	}
	if _, err := h.accountSvc.GetOwned(r.Context(), req.FromAccountID, userID); err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrForbidden):
			writeError(w, http.StatusForbidden, "source account does not belong to you")
		default:
			writeError(w, http.StatusInternalServerError, "failed to verify account ownership")
		}
		return
	}

	txn, err := h.transferSvc.Transfer(r.Context(), req)
	switch {
	case errors.Is(err, service.ErrInsufficientFunds):
		writeError(w, http.StatusUnprocessableEntity, "insufficient funds")
	case errors.Is(err, service.ErrSameAccount):
		writeError(w, http.StatusBadRequest, "cannot transfer to the same account")
	case errors.Is(err, service.ErrInvalidAmount):
		writeError(w, http.StatusBadRequest, "amount must be positive")
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "one of the accounts does not exist")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "transfer failed, please retry with the same idempotency key")
	default:
		writeJSON(w, http.StatusOK, txn)
	}
}
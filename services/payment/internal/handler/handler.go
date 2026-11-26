package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"logistics/pkg/httpx"
	"logistics/services/payment/internal/domain"
	"logistics/services/payment/internal/service"
)

type Handler struct{ svc service.PaymentService }

func New(svc service.PaymentService) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(r *gin.Engine) {
	r.GET("/healthz", httpx.Health)
	r.POST("/v1/payments/intents", h.createIntent)
	r.GET("/v1/payments/:id", h.get)
	r.POST("/v1/webhooks/stripe", h.stripeWebhook)
}

type createIntentReq struct {
	TrackingNumber string `json:"tracking_number" binding:"required"`
	CustomerID     string `json:"customer_id" binding:"required"`
	AmountMinor    int64  `json:"amount_minor" binding:"required,gt=0"`
	Currency       string `json:"currency" binding:"required,len=3"`
}

func (h *Handler) createIntent(c *gin.Context) {
	var req createIntentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	res, err := h.svc.CreateIntent(c.Request.Context(), service.CreateIntentInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"payment": res.Payment, "client_secret": res.ClientSecret})
}

func (h *Handler) get(c *gin.Context) {
	p, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// stripeWebhook needs the RAW body: the signature covers the exact bytes, so
// it must not be parsed/re-encoded before verification.
func (h *Handler) stripeWebhook(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}
	err = h.svc.HandleWebhook(c.Request.Context(), body, c.GetHeader("Stripe-Signature"))
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"received": true})
	case errors.Is(err, domain.ErrInvalidSignature):
		httpx.Fail(c, http.StatusBadRequest, "invalid_signature", "invalid signature")
	default:
		// Non-2xx makes Stripe retry with backoff.
		httpx.Fail(c, http.StatusInternalServerError, "webhook_failed", "temporary failure")
	}
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Fail(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, domain.ErrValidation):
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_failed", err.Error())
	default:
		httpx.Fail(c, http.StatusBadGateway, "payment_failed", "could not create payment")
	}
}

package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"logistics/pkg/httpx"
	"logistics/services/shipment/internal/domain"
	"logistics/services/shipment/internal/service"
)

type Handler struct{ svc service.ShipmentService }

func New(svc service.ShipmentService) *Handler { return &Handler{svc: svc} }

// Register mounts the routes. The gateway forwards /api/v1/shipments/* here.
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/healthz", httpx.Health)
	v1 := r.Group("/v1/shipments")
	v1.POST("", h.create)
	v1.GET("/:tracking", h.get)
	v1.GET("/:tracking/events", h.events)
	v1.PATCH("/:tracking/status", h.updateStatus)
}

type createReq struct {
	CustomerID     string         `json:"customer_id" binding:"required"`
	RecipientName  string         `json:"recipient_name" binding:"required"`
	RecipientPhone string         `json:"recipient_phone" binding:"required,e164"`
	Origin         domain.Address `json:"origin" binding:"required"`
	Destination    domain.Address `json:"destination" binding:"required"`
	WeightKg       float64        `json:"weight_kg" binding:"required,gt=0"`
}

func (h *Handler) create(c *gin.Context) {
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	sh, err := h.svc.Create(c.Request.Context(), service.CreateInput(req))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, sh)
}

func (h *Handler) get(c *gin.Context) {
	sh, err := h.svc.Get(c.Request.Context(), c.Param("tracking"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sh)
}

func (h *Handler) events(c *gin.Context) {
	ev, err := h.svc.Events(c.Request.Context(), c.Param("tracking"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": ev})
}

type statusReq struct {
	Status   domain.Status `json:"status" binding:"required"`
	Location string        `json:"location" binding:"required"`
	Note     string        `json:"note"`
}

func (h *Handler) updateStatus(c *gin.Context) {
	var req statusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	sh, err := h.svc.UpdateStatus(c.Request.Context(), c.Param("tracking"), req.Status, req.Location, req.Note)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, sh)
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Fail(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		httpx.Fail(c, http.StatusConflict, "invalid_transition", err.Error())
	case errors.Is(err, domain.ErrValidation):
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_failed", err.Error())
	default:
		httpx.Fail(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

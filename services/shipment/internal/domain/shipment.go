package domain

import (
	"errors"
	"time"
)

type Status string

const (
	StatusCreated        Status = "CREATED"
	StatusPickedUp       Status = "PICKED_UP"
	StatusInTransit      Status = "IN_TRANSIT"
	StatusOutForDelivery Status = "OUT_FOR_DELIVERY"
	StatusDelivered      Status = "DELIVERED"
	StatusFailed         Status = "DELIVERY_FAILED"
	StatusCancelled      Status = "CANCELLED"
)

var (
	ErrNotFound          = errors.New("shipment not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrValidation        = errors.New("validation failed")
)

var transitions = map[Status][]Status{
	StatusCreated:        {StatusPickedUp, StatusCancelled},
	StatusPickedUp:       {StatusInTransit, StatusCancelled},
	StatusInTransit:      {StatusOutForDelivery, StatusFailed},
	StatusOutForDelivery: {StatusDelivered, StatusFailed},
	StatusFailed:         {StatusInTransit, StatusCancelled},
}

func (s Status) CanTransitionTo(next Status) bool {
	for _, t := range transitions[s] {
		if t == next {
			return true
		}
	}
	return false
}

func (s Status) Valid() bool {
	if s == StatusDelivered || s == StatusCancelled || s == StatusCreated {
		return true
	}
	_, ok := transitions[s]
	return ok
}

type Address struct {
	Line1      string `json:"line1" binding:"required"`
	City       string `json:"city" binding:"required"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code" binding:"required"`
	Country    string `json:"country" binding:"required,len=2"`
}

type Shipment struct {
	ID             string    `json:"id"`
	TrackingNumber string    `json:"tracking_number"`
	CustomerID     string    `json:"customer_id"`
	RecipientName  string    `json:"recipient_name"`
	RecipientPhone string    `json:"recipient_phone"` // E.164, used for SMS
	Origin         Address   `json:"origin"`
	Destination    Address   `json:"destination"`
	WeightKg       float64   `json:"weight_kg"`
	Status         Status    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TrackingEvent struct {
	ID         int64     `json:"id"`
	ShipmentID string    `json:"shipment_id"`
	Status     Status    `json:"status"`
	Location   string    `json:"location"`
	Note       string    `json:"note,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

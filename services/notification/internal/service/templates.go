package service

import (
	"fmt"
	"strings"

	"logistics/pkg/events"
)

// shipmentMessage returns false for statuses not worth an SMS (e.g.
// IN_TRANSIT would spam the recipient at every hop).
func shipmentMessage(eventType string, p shipmentPayload) (string, bool) {
	name := p.RecipientName
	if name == "" {
		name = "there"
	}
	if eventType == events.ShipmentCreated {
		return fmt.Sprintf("Hi %s, your shipment %s has been created.", name, p.TrackingNumber), true
	}
	switch p.Status {
	case "PICKED_UP":
		return fmt.Sprintf("Shipment %s has been picked up.", p.TrackingNumber), true
	case "OUT_FOR_DELIVERY":
		return fmt.Sprintf("Shipment %s is out for delivery today.", p.TrackingNumber), true
	case "DELIVERED":
		return fmt.Sprintf("Shipment %s was delivered. Thank you!", p.TrackingNumber), true
	case "DELIVERY_FAILED":
		return fmt.Sprintf("We couldn't deliver shipment %s. We'll try again soon.", p.TrackingNumber), true
	case "CANCELLED":
		return fmt.Sprintf("Shipment %s has been cancelled.", p.TrackingNumber), true
	}
	return "", false
}

func paymentMessage(eventType string, p paymentPayload) (string, bool) {
	amount := fmt.Sprintf("%.2f %s", float64(p.AmountMinor)/100, strings.ToUpper(p.Currency))
	if eventType == events.PaymentSucceeded {
		return fmt.Sprintf("Payment of %s received for shipment %s.", amount, p.TrackingNumber), true
	}
	return fmt.Sprintf("Payment of %s for shipment %s failed. Please try again.", amount, p.TrackingNumber), true
}

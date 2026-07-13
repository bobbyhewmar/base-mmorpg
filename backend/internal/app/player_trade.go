package app

import "time"

const (
	playerTradeInteractionRange = 4.5
	tradeNoticeKind             = "trade_notice"
	tradeNoticeStatusPending    = "pending"
	tradeNoticeStatusAccepted   = "accepted"
	tradeNoticeStatusDeclined   = "declined"
	tradeNoticeStatusCancelled  = "cancelled"
	tradeDirectionIncoming      = "incoming"
	tradeDirectionOutgoing      = "outgoing"
)

type playerTradeOffer struct {
	ID                string
	SourceSessionID   string
	SourceCharacterID string
	SourceName        string
	TargetSessionID   string
	TargetCharacterID string
	TargetName        string
	ItemInstanceID    string
	TemplateID        string
	Quantity          int
	RegionID          string
	CreatedAt         time.Time
}

func tradeNoticeMessage(
	status string,
	direction string,
	offerID string,
	counterpartyCharacterID string,
	counterpartyName string,
	itemTemplateID string,
	quantity int,
	message string,
) map[string]any {
	return map[string]any{
		"kind":                      tradeNoticeKind,
		"emitted_at_ms":             time.Now().UnixMilli(),
		"status":                    status,
		"direction":                 direction,
		"offer_id":                  offerID,
		"counterparty_character_id": counterpartyCharacterID,
		"counterparty_name":         counterpartyName,
		"item_template_id":          itemTemplateID,
		"quantity":                  quantity,
		"message":                   message,
	}
}

func incomingTradeNotice(offer *playerTradeOffer, status string, message string) map[string]any {
	return tradeNoticeMessage(
		status,
		tradeDirectionIncoming,
		offer.ID,
		offer.SourceCharacterID,
		offer.SourceName,
		offer.TemplateID,
		offer.Quantity,
		message,
	)
}

func outgoingTradeNotice(offer *playerTradeOffer, status string, message string) map[string]any {
	return tradeNoticeMessage(
		status,
		tradeDirectionOutgoing,
		offer.ID,
		offer.TargetCharacterID,
		offer.TargetName,
		offer.TemplateID,
		offer.Quantity,
		message,
	)
}

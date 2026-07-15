package app

type ExchangeOffer struct {
	ID             string
	NPCEntityID    string
	TemplateID     string
	Quantity       int
	CostTemplateID string
	CostAmount     int
}

var exchangeOffers = map[string]ExchangeOffer{
	"merchant_mantle_exchange": {
		ID:             "merchant_mantle_exchange",
		NPCEntityID:    "npc_merchant",
		TemplateID:     "wardkeeper_mantle",
		Quantity:       1,
		CostTemplateID: "duskgold",
		CostAmount:     10,
	},
	"merchant_ruinbound_greaves_exchange": {
		ID:             "merchant_ruinbound_greaves_exchange",
		NPCEntityID:    "npc_merchant",
		TemplateID:     "ruinbound_greaves",
		Quantity:       1,
		CostTemplateID: "ruin_shard",
		CostAmount:     6,
	},
	"merchant_whisperstep_boots_exchange": {
		ID:             "merchant_whisperstep_boots_exchange",
		NPCEntityID:    "npc_merchant",
		TemplateID:     "whisperstep_boots",
		Quantity:       1,
		CostTemplateID: "ruin_shard",
		CostAmount:     6,
	},
}

func exchangeOfferByID(offerID string) (ExchangeOffer, bool) {
	offer, exists := exchangeOffers[offerID]
	return offer, exists
}

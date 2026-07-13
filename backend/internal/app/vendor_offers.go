package app

type VendorOffer struct {
	ID                      string
	NPCEntityID             string
	TemplateID              string
	Quantity                int
	PriceCurrencyTemplateID string
	PriceAmount             int
}

const vendorInteractionRange = 4.5

var vendorOffers = map[string]VendorOffer{
	"merchant_spear_offer": {
		ID:                      "merchant_spear_offer",
		NPCEntityID:             "npc_merchant",
		TemplateID:              "ironwood_spear",
		Quantity:                1,
		PriceCurrencyTemplateID: "duskgold",
		PriceAmount:             8,
	},
	"merchant_ruin_shard_bundle": {
		ID:                      "merchant_ruin_shard_bundle",
		NPCEntityID:             "npc_merchant",
		TemplateID:              "ruin_shard",
		Quantity:                4,
		PriceCurrencyTemplateID: "duskgold",
		PriceAmount:             4,
	},
}

func vendorOfferByID(offerID string) (VendorOffer, bool) {
	offer, exists := vendorOffers[offerID]
	return offer, exists
}

package app

type VendorSellValue struct {
	CurrencyTemplateID string
	Amount             int
}

func vendorSellValue(templateID string) (VendorSellValue, bool) {
	switch templateID {
	case "ironwood_spear":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 4}, true
	case "novice_oak_staff":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 4}, true
	case "wardkeeper_mantle":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 3}, true
	case "moonthread_robe":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 3}, true
	case "watcher_gloves":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 2}, true
	case "runesewn_gloves":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 2}, true
	case "pathrunner_boots":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 2}, true
	case "whisperstep_boots":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 2}, true
	case "ruinbound_greaves":
		return VendorSellValue{CurrencyTemplateID: "duskgold", Amount: 4}, true
	default:
		return VendorSellValue{}, false
	}
}

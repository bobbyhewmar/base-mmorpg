package app

import "sort"

func initialCharacterItemSeed(character *Character) []CharacterItem {
	items := []CharacterItem{
		{
			ID:            randomID("item"),
			CharacterID:   character.ID,
			TemplateID:    "duskgold",
			Quantity:      12,
			ContainerKind: itemContainerInventory,
		},
		{
			ID:            randomID("item"),
			CharacterID:   character.ID,
			TemplateID:    "healing_potion",
			Quantity:      3,
			ContainerKind: itemContainerInventory,
		},
	}

	if character.BaseClass == "Mage" || character.BaseClass == "Reaver" {
		items = append(items,
			CharacterItem{
				ID:            randomID("item"),
				CharacterID:   character.ID,
				TemplateID:    "novice_oak_staff",
				Quantity:      1,
				ContainerKind: itemContainerEquipment,
				EquipSlot:     equipSlotWeapon,
			},
			CharacterItem{
				ID:            randomID("item"),
				CharacterID:   character.ID,
				TemplateID:    "moonthread_robe",
				Quantity:      1,
				ContainerKind: itemContainerEquipment,
				EquipSlot:     equipSlotChest,
			},
			CharacterItem{
				ID:            randomID("item"),
				CharacterID:   character.ID,
				TemplateID:    "runesewn_gloves",
				Quantity:      1,
				ContainerKind: itemContainerInventory,
				InstanceAttributes: &ItemInstanceAttributes{
					Attack: 1,
					MaxMP:  3,
				},
			},
			CharacterItem{
				ID:            randomID("item"),
				CharacterID:   character.ID,
				TemplateID:    "whisperstep_boots",
				Quantity:      1,
				ContainerKind: itemContainerInventory,
			},
		)
		return items
	}

	items = append(items,
		CharacterItem{
			ID:            randomID("item"),
			CharacterID:   character.ID,
			TemplateID:    "wardkeeper_mantle",
			Quantity:      1,
			ContainerKind: itemContainerEquipment,
			EquipSlot:     equipSlotChest,
		},
		CharacterItem{
			ID:            randomID("item"),
			CharacterID:   character.ID,
			TemplateID:    "watcher_gloves",
			Quantity:      1,
			ContainerKind: itemContainerInventory,
			InstanceAttributes: &ItemInstanceAttributes{
				Attack:  1,
				Defense: 1,
			},
		},
		CharacterItem{
			ID:            randomID("item"),
			CharacterID:   character.ID,
			TemplateID:    "pathrunner_boots",
			Quantity:      1,
			ContainerKind: itemContainerInventory,
		},
		CharacterItem{
			ID:            randomID("item"),
			CharacterID:   character.ID,
			TemplateID:    "ironwood_spear",
			Quantity:      1,
			ContainerKind: itemContainerEquipment,
			EquipSlot:     equipSlotWeapon,
		},
	)
	return items
}

func itemTemplateIsStackable(templateID string) bool {
	switch templateID {
	case "duskgold", "ruin_shard", "healing_potion":
		return true
	default:
		return false
	}
}

func itemTemplateIsConsumable(templateID string) bool {
	switch templateID {
	case "healing_potion":
		return true
	default:
		return false
	}
}

func itemTemplateConsumableHeal(templateID string) int {
	switch templateID {
	case "healing_potion":
		return 45
	default:
		return 0
	}
}

func itemTemplateEquipSlot(templateID string) (EquipSlot, bool) {
	switch templateID {
	case "ironwood_spear":
		return equipSlotWeapon, true
	case "novice_oak_staff":
		return equipSlotWeapon, true
	case "wardkeeper_mantle":
		return equipSlotChest, true
	case "moonthread_robe":
		return equipSlotChest, true
	case "watcher_gloves":
		return equipSlotGloves, true
	case "runesewn_gloves":
		return equipSlotGloves, true
	case "pathrunner_boots":
		return equipSlotBoots, true
	case "whisperstep_boots":
		return equipSlotBoots, true
	case "ruinbound_greaves":
		return equipSlotBoots, true
	default:
		return "", false
	}
}

func sortCharacterItems(items []CharacterItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ContainerKind == items[j].ContainerKind {
			if items[i].EquipSlot == items[j].EquipSlot {
				if items[i].TemplateID == items[j].TemplateID {
					return items[i].ID < items[j].ID
				}
				return items[i].TemplateID < items[j].TemplateID
			}
			return items[i].EquipSlot < items[j].EquipSlot
		}
		return items[i].ContainerKind < items[j].ContainerKind
	})
}

func snapshotCharacterItems(items []CharacterItem) CharacterItemSnapshot {
	snapshot := CharacterItemSnapshot{
		Inventory: make([]CharacterItem, 0),
		Equipment: make([]CharacterItem, 0),
		Warehouse: make([]CharacterItem, 0),
	}
	for _, item := range items {
		switch item.ContainerKind {
		case itemContainerEquipment:
			snapshot.Equipment = append(snapshot.Equipment, item)
		case itemContainerWarehouse:
			snapshot.Warehouse = append(snapshot.Warehouse, item)
		default:
			snapshot.Inventory = append(snapshot.Inventory, item)
		}
	}

	sortCharacterItems(snapshot.Inventory)
	sortCharacterItems(snapshot.Equipment)
	sortCharacterItems(snapshot.Warehouse)
	return snapshot
}

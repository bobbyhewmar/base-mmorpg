package app

import (
	"context"
	"time"
)

func (runtime *attachedRuntime) processHotbarCommand(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if parsed.commandType != "set_hotbar_state" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}

	normalized, reasonCode, message := runtime.validateHotbarStateLocked(ctx, store, parsed.hotbarState)
	if reasonCode != "" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, reasonCode, message))
	}
	if err := store.CharacterHotbars.ReplaceByCharacterID(ctx, runtime.characterID, normalized); err != nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist hotbar layout."))
	}

	runtime.hotbarState = normalized
	runtime.revision++
	outbound = append(outbound, deltaMessage(
		runtime.revision,
		command.CommandID,
		command.CommandSeq,
		runtime.selfDelta(time.Now(), nil),
		nil,
		nil,
	))
	return outbound
}

func (runtime *attachedRuntime) validateHotbarStateLocked(ctx context.Context, store *Store, hotbar CharacterHotbarState) (CharacterHotbarState, string, string) {
	if store == nil || store.CharacterHotbars == nil || store.Items == nil {
		return CharacterHotbarState{}, "system.persistence_failed", "Hotbar persistence is unavailable."
	}
	character := &Character{
		ID:        runtime.characterID,
		BaseClass: runtime.characterBaseClass,
		Level:     runtime.characterLevel,
	}
	normalized := normalizeCharacterHotbarState(hotbar, character)

	var itemIDs map[string]struct{}
	for _, slot := range normalized.Slots {
		switch slot.EntryType {
		case "":
			continue
		case "skill":
			category, known := knownSkillCategory(runtime.characterBaseClass, runtime.characterLevel, slot.SkillID)
			if !known || category != skillCategoryActive {
				return CharacterHotbarState{}, "hotbar.skill_not_available", "Hotbar skill is not an active learned skill."
			}
		case "item":
			if itemIDs == nil {
				items, err := store.Items.ListByCharacterID(ctx, runtime.characterID)
				if err != nil {
					return CharacterHotbarState{}, "system.persistence_failed", "Unable to validate hotbar item ownership."
				}
				itemIDs = make(map[string]struct{}, len(items))
				for _, item := range items {
					itemIDs[item.ID] = struct{}{}
				}
			}
			if _, exists := itemIDs[slot.ItemInstanceID]; !exists {
				return CharacterHotbarState{}, "hotbar.item_not_found", "Hotbar item is not owned by this character."
			}
		case "action":
			if !isSupportedHotbarActionID(slot.ActionID) {
				return CharacterHotbarState{}, "hotbar.action_not_supported", "Hotbar action is not supported."
			}
		default:
			return CharacterHotbarState{}, "hotbar.invalid_slot", "Hotbar slot binding is invalid."
		}
	}
	return normalized, "", ""
}

func isSupportedHotbarActionID(actionID string) bool {
	switch actionID {
	case "basic_attack", "pick_up_nearby", "party_invite", "party_leave", "tame_target", "summon_pet", "dismiss_pet", "mount_pet", "dismount_pet", "toggle_walk_run":
		return true
	default:
		return false
	}
}

func (runtime *attachedRuntime) processItemCommand(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if runtime.isPlayerDead() {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}

	var (
		items        []CharacterItem
		consumedItem CharacterItem
		err          error
	)

	switch parsed.commandType {
	case "equip_item":
		items, err = store.Items.EquipItem(ctx, runtime.characterID, parsed.itemID)
	case "unequip_item":
		items, err = store.Items.UnequipItem(ctx, runtime.characterID, parsed.equipSlot)
	case "split_item_stack":
		items, err = store.Items.SplitStack(ctx, runtime.characterID, parsed.itemID, parsed.quantity)
	case "merge_item_stacks":
		items, err = store.Items.MergeStacks(ctx, runtime.characterID, parsed.itemID, parsed.mergeItemID)
	case "use_item":
		if runtime.currentHP >= runtime.derivedStats.MaxHP {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.item_not_usable", "Consumable has no effect right now."))
		}
		items, consumedItem, err = store.Items.UseConsumable(ctx, runtime.characterID, parsed.itemID)
	default:
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}
	if err != nil {
		switch err {
		case errItemNotFound:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.item_not_found", "Referenced item is not in the character inventory."))
		case errItemNotEquippable:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.item_not_equippable", "Referenced item cannot be equipped."))
		case errItemNotEquipped:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.slot_empty", "Referenced equipment slot is empty."))
		case errItemSlotMismatch:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.slot_mismatch", "Referenced item does not match the expected equipment slot."))
		case errItemNotStackable:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.item_not_stackable", "Referenced item cannot be split or merged as a stack."))
		case errInvalidSplitQuantity:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.split_invalid_quantity", "Split quantity is invalid for the referenced stack."))
		case errItemMergeInvalid:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.merge_invalid", "Stack merge is not valid for the referenced items."))
		case errItemNotConsumable:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.item_not_usable", "Referenced item cannot be used as a consumable."))
		default:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist item placement."))
		}
	}

	if parsed.commandType == "use_item" {
		recoveredHP := itemTemplateConsumableHeal(consumedItem.TemplateID)
		if recoveredHP <= 0 {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "inventory.item_not_usable", "Referenced item cannot be used as a consumable."))
		}
		runtime.currentHP = min(runtime.derivedStats.MaxHP, runtime.currentHP+recoveredHP)
	}

	runtime.revision++
	runtime.recalculateDerivedStatsLocked(items)
	itemSnapshot := snapshotCharacterItems(items)
	outbound = append(outbound, deltaMessage(
		runtime.revision,
		command.CommandID,
		command.CommandSeq,
		runtime.selfDelta(time.Now(), nil),
		nil,
		&itemSnapshot,
	))
	return outbound
}

func (runtime *attachedRuntime) processVendorCommand(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if runtime.isPlayerDead() {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}
	if parsed.commandType != "buy_item" && parsed.commandType != "sell_item" && parsed.commandType != "exchange_item" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}

	vendor, exists := runtime.knownEntities["npc_merchant"]
	if !exists || vendor.EntityType != "npc" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced vendor is not in the current known-set."))
	}
	if distance(runtime.position, vendor.Position) > vendorInteractionRange {
		if parsed.commandType == "exchange_item" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.exchange_out_of_range", "Referenced vendor is not in exchange range."))
		}
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.vendor_out_of_range", "Referenced vendor is not in trading range."))
	}

	var (
		items []CharacterItem
		err   error
	)
	if parsed.commandType == "buy_item" {
		offer, exists := vendorOfferByID(parsed.vendorOfferID)
		if !exists {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.offer_not_found", "Referenced vendor offer is not available."))
		}
		if offer.NPCEntityID != vendor.EntityID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.offer_not_found", "Referenced vendor offer is not available."))
		}
		items, err = store.Items.BuyVendorOffer(ctx, runtime.characterID, offer, parsed.quantity)
	} else if parsed.commandType == "exchange_item" {
		offer, exists := exchangeOfferByID(parsed.exchangeOfferID)
		if !exists {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.exchange_offer_not_found", "Referenced exchange offer is not available."))
		}
		if offer.NPCEntityID != vendor.EntityID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.exchange_offer_not_found", "Referenced exchange offer is not available."))
		}
		items, err = store.Items.ExchangeOffer(ctx, runtime.characterID, offer, parsed.quantity)
	} else {
		items, err = store.Items.SellVendorItem(ctx, runtime.characterID, parsed.itemID, parsed.quantity)
	}
	if err != nil {
		switch err {
		case errVendorOfferNotFound:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.offer_not_found", "Referenced vendor offer is not available."))
		case errExchangeOfferNotFound:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.exchange_offer_not_found", "Referenced exchange offer is not available."))
		case errInsufficientFunds:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.insufficient_funds", "Actor lacks currency for this vendor purchase."))
		case errInsufficientMaterials:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.exchange_insufficient_materials", "Actor lacks the required materials for this exchange."))
		case errItemNotFound:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.sell_item_not_found", "Referenced item is not in the character inventory."))
		case errInvalidSplitQuantity:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.sell_invalid_quantity", "Sell quantity is invalid for the referenced item."))
		case errItemNotSellable:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.sell_not_allowed", "Referenced item cannot be sold to this vendor."))
		default:
			if parsed.commandType == "exchange_item" {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist exchange."))
			}
			if parsed.commandType == "sell_item" {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist vendor sale."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist vendor purchase."))
		}
	}

	runtime.revision++
	runtime.characterItems = cloneCharacterItems(items)
	itemSnapshot := snapshotCharacterItems(items)
	outbound = append(outbound, deltaMessage(
		runtime.revision,
		command.CommandID,
		command.CommandSeq,
		runtime.selfDelta(time.Now(), nil),
		nil,
		&itemSnapshot,
	))
	return outbound
}

func (runtime *attachedRuntime) processWarehouseCommand(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if runtime.isPlayerDead() {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}
	if parsed.commandType != "deposit_item" && parsed.commandType != "withdraw_item" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}

	warehouseKeeper, exists := runtime.knownEntities[warehouseNPCEntityID]
	if !exists || warehouseKeeper.EntityType != "npc" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced warehouse keeper is not in the current known-set."))
	}
	if distance(runtime.position, warehouseKeeper.Position) > warehouseInteractionRange {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.storage_out_of_range", "Referenced warehouse keeper is not in storage range."))
	}

	var (
		items []CharacterItem
		err   error
	)
	switch parsed.commandType {
	case "deposit_item":
		items, err = store.Items.DepositWarehouseItem(ctx, runtime.characterID, parsed.itemID, parsed.quantity)
	case "withdraw_item":
		items, err = store.Items.WithdrawWarehouseItem(ctx, runtime.characterID, parsed.itemID, parsed.quantity)
	}
	if err != nil {
		switch err {
		case errItemNotFound, errWarehouseItemNotFound:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.storage_item_not_found", "Referenced item is not available in the requested storage container."))
		case errInvalidSplitQuantity:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.storage_invalid_quantity", "Storage quantity is invalid for the referenced item."))
		case errItemNotStackable:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "economy.storage_invalid_quantity", "Storage quantity is invalid for the referenced item."))
		default:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist warehouse transfer."))
		}
	}

	runtime.revision++
	runtime.characterItems = cloneCharacterItems(items)
	itemSnapshot := snapshotCharacterItems(items)
	outbound = append(outbound, deltaMessage(
		runtime.revision,
		command.CommandID,
		command.CommandSeq,
		runtime.selfDelta(time.Now(), nil),
		nil,
		&itemSnapshot,
	))
	return outbound
}

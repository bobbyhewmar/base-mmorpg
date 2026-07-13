package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func (runtime *attachedRuntime) preValidate(command commandEnvelope) (*parsedCommand, map[string]any) {
	if command.ProtocolVersion != 1 {
		return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.unsupported_version", "Unsupported protocol version.")
	}
	if command.CommandSeq != runtime.expectedCommandSeq {
		return nil, rejectMessage(command.CommandID, command.CommandSeq, "sequence.out_of_order", sequenceMessage(runtime.expectedCommandSeq))
	}
	if command.CommandID == "" {
		return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Command ID is required.")
	}

	switch command.Type {
	case "move_intent":
		var payload struct {
			Point runtimePoint `json:"point"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Move payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			movePoint:   payload.Point,
		}, nil
	case "select_target":
		var payload struct {
			TargetID string `json:"target_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Target payload is invalid.")
		}
		if payload.TargetID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Target payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			targetID:    payload.TargetID,
		}, nil
	case "use_skill":
		var payload struct {
			SkillID  string `json:"skill_id"`
			TargetID string `json:"target_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Skill payload is invalid.")
		}
		if payload.SkillID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Skill payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			skillID:     payload.SkillID,
			targetID:    payload.TargetID,
		}, nil
	case "basic_attack":
		var payload struct {
			TargetID string `json:"target_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Basic attack payload is invalid.")
		}
		if payload.TargetID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "combat.target_required", "A valid target is required for a basic attack.")
		}
		return &parsedCommand{
			commandType: command.Type,
			targetID:    payload.TargetID,
		}, nil
	case "pick_up_loot":
		var payload struct {
			LootID string `json:"loot_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Loot pickup payload is invalid.")
		}
		if payload.LootID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Loot pickup payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			lootID:      payload.LootID,
		}, nil
	case "tame_mob":
		var payload struct {
			TargetID string `json:"target_id"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Tame payload is invalid.")
		}
		if payload.TargetID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Tame payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			targetID:    payload.TargetID,
		}, nil
	case "summon_pet", "dismiss_pet", "mount_pet", "dismount_pet":
		var payload struct{}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Pet command payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
		}, nil
	case "interact_npc":
		var payload struct {
			NPCID    string `json:"npc_id"`
			ActionID string `json:"action_id"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "NPC interaction payload is invalid.")
		}
		if payload.NPCID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "NPC interaction payload is invalid.")
		}
		if payload.ActionID != "" && payload.ActionID != "accept_task" && payload.ActionID != "turn_in_task" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "NPC interaction payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			npcID:       payload.NPCID,
			npcActionID: payload.ActionID,
		}, nil
	case "set_hotbar_state":
		var payload struct {
			OpenBarCount int `json:"open_bar_count"`
			Slots        []struct {
				SlotIndex      int    `json:"slot_index"`
				EntryType      string `json:"entry_type"`
				SkillID        string `json:"skill_id"`
				ItemInstanceID string `json:"item_instance_id"`
				ActionID       string `json:"action_id"`
			} `json:"slots"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
		}
		if payload.OpenBarCount < 1 || payload.OpenBarCount > 3 || len(payload.Slots) != 36 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
		}
		seenSlots := map[int]struct{}{}
		hotbar := CharacterHotbarState{
			OpenBarCount: payload.OpenBarCount,
			Slots:        make([]CharacterHotbarSlot, 0, len(payload.Slots)),
		}
		for _, slot := range payload.Slots {
			if slot.SlotIndex < 0 || slot.SlotIndex >= 36 {
				return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
			}
			if _, exists := seenSlots[slot.SlotIndex]; exists {
				return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
			}
			seenSlots[slot.SlotIndex] = struct{}{}
			switch slot.EntryType {
			case "":
				if slot.SkillID != "" || slot.ItemInstanceID != "" || slot.ActionID != "" {
					return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
				}
			case "skill":
				if slot.SkillID == "" || slot.ItemInstanceID != "" || slot.ActionID != "" {
					return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
				}
			case "item":
				if slot.ItemInstanceID == "" || slot.SkillID != "" || slot.ActionID != "" {
					return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
				}
			case "action":
				if slot.ActionID == "" || slot.SkillID != "" || slot.ItemInstanceID != "" {
					return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
				}
			default:
				return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Hotbar payload is invalid.")
			}
			hotbar.Slots = append(hotbar.Slots, CharacterHotbarSlot{
				SlotIndex:      slot.SlotIndex,
				EntryType:      slot.EntryType,
				SkillID:        slot.SkillID,
				ItemInstanceID: slot.ItemInstanceID,
				ActionID:       slot.ActionID,
			})
		}
		return &parsedCommand{
			commandType: command.Type,
			hotbarState: hotbar,
		}, nil
	case "equip_item":
		var payload struct {
			ItemID string `json:"item_instance_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Equip payload is invalid.")
		}
		if payload.ItemID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Equip payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			itemID:      payload.ItemID,
		}, nil
	case "unequip_item":
		var payload struct {
			EquipSlot EquipSlot `json:"equip_slot"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unequip payload is invalid.")
		}
		if payload.EquipSlot == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unequip payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			equipSlot:   payload.EquipSlot,
		}, nil
	case "split_item_stack":
		var payload struct {
			ItemID   string `json:"item_instance_id"`
			Quantity int    `json:"quantity"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Split payload is invalid.")
		}
		if payload.ItemID == "" || payload.Quantity <= 0 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Split payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			itemID:      payload.ItemID,
			quantity:    payload.Quantity,
		}, nil
	case "merge_item_stacks":
		var payload struct {
			SourceItemID string `json:"source_item_instance_id"`
			TargetItemID string `json:"target_item_instance_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Merge payload is invalid.")
		}
		if payload.SourceItemID == "" || payload.TargetItemID == "" || payload.SourceItemID == payload.TargetItemID {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Merge payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			itemID:      payload.SourceItemID,
			mergeItemID: payload.TargetItemID,
		}, nil
	case "use_item":
		var payload struct {
			ItemID string `json:"item_instance_id"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Use item payload is invalid.")
		}
		if payload.ItemID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Use item payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			itemID:      payload.ItemID,
		}, nil
	case "buy_item":
		var payload struct {
			VendorOfferID string `json:"vendor_offer_id"`
			Quantity      int    `json:"quantity"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Buy payload is invalid.")
		}
		if payload.VendorOfferID == "" || payload.Quantity <= 0 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Buy payload is invalid.")
		}
		return &parsedCommand{
			commandType:   command.Type,
			vendorOfferID: payload.VendorOfferID,
			quantity:      payload.Quantity,
		}, nil
	case "exchange_item":
		var payload struct {
			ExchangeOfferID string `json:"exchange_offer_id"`
			Quantity        int    `json:"quantity"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Exchange payload is invalid.")
		}
		if payload.ExchangeOfferID == "" || payload.Quantity <= 0 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Exchange payload is invalid.")
		}
		return &parsedCommand{
			commandType:     command.Type,
			exchangeOfferID: payload.ExchangeOfferID,
			quantity:        payload.Quantity,
		}, nil
	case "deposit_item":
		var payload struct {
			ItemID   string `json:"item_instance_id"`
			Quantity int    `json:"quantity"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Warehouse deposit payload is invalid.")
		}
		if payload.ItemID == "" || payload.Quantity <= 0 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Warehouse deposit payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			itemID:      payload.ItemID,
			quantity:    payload.Quantity,
		}, nil
	case "withdraw_item":
		var payload struct {
			ItemID   string `json:"item_instance_id"`
			Quantity int    `json:"quantity"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Warehouse withdraw payload is invalid.")
		}
		if payload.ItemID == "" || payload.Quantity <= 0 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Warehouse withdraw payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			itemID:      payload.ItemID,
			quantity:    payload.Quantity,
		}, nil
	case "sell_item":
		var payload struct {
			ItemID   string `json:"item_instance_id"`
			Quantity int    `json:"quantity"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Sell payload is invalid.")
		}
		if payload.ItemID == "" || payload.Quantity <= 0 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Sell payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			itemID:      payload.ItemID,
			quantity:    payload.Quantity,
		}, nil
	case "offer_trade_item":
		var payload struct {
			TargetCharacterID string `json:"target_character_id"`
			ItemID            string `json:"item_instance_id"`
			Quantity          int    `json:"quantity"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Trade offer payload is invalid.")
		}
		if payload.TargetCharacterID == "" || payload.ItemID == "" || payload.Quantity <= 0 {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Trade offer payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			targetID:    payload.TargetCharacterID,
			itemID:      payload.ItemID,
			quantity:    payload.Quantity,
		}, nil
	case "accept_trade_offer":
		var payload struct {
			TradeOfferID string `json:"trade_offer_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Trade accept payload is invalid.")
		}
		if payload.TradeOfferID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Trade accept payload is invalid.")
		}
		return &parsedCommand{
			commandType:  command.Type,
			tradeOfferID: payload.TradeOfferID,
		}, nil
	case "decline_trade_offer":
		var payload struct {
			TradeOfferID string `json:"trade_offer_id"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Trade decline payload is invalid.")
		}
		if payload.TradeOfferID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Trade decline payload is invalid.")
		}
		return &parsedCommand{
			commandType:  command.Type,
			tradeOfferID: payload.TradeOfferID,
		}, nil
	case "invite_party_member":
		var payload struct {
			TargetCharacterID string `json:"target_character_id"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party invite payload is invalid.")
		}
		if payload.TargetCharacterID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party invite payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			targetID:    payload.TargetCharacterID,
		}, nil
	case "accept_party_invite":
		var payload struct {
			InviteID string `json:"invite_id"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party accept payload is invalid.")
		}
		if payload.InviteID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party accept payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			inviteID:    payload.InviteID,
		}, nil
	case "decline_party_invite":
		var payload struct {
			InviteID string `json:"invite_id"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party decline payload is invalid.")
		}
		if payload.InviteID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party decline payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			inviteID:    payload.InviteID,
		}, nil
	case "leave_party":
		var payload struct{}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party leave payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
		}, nil
	case "kick_party_member":
		var payload struct {
			TargetCharacterID string `json:"target_character_id"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party kick payload is invalid.")
		}
		if payload.TargetCharacterID == "" {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Party kick payload is invalid.")
		}
		return &parsedCommand{
			commandType: command.Type,
			targetID:    payload.TargetCharacterID,
		}, nil
	case "send_chat_message":
		var payload struct {
			Channel             string `json:"channel"`
			Text                string `json:"text"`
			TargetCharacterName string `json:"target_character_name"`
		}
		if err := decodeCommandPayloadStrict(command.Payload, &payload); err != nil {
			return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Chat payload is invalid.")
		}
		return &parsedCommand{
			commandType:    command.Type,
			chatChannel:    payload.Channel,
			chatText:       payload.Text,
			chatTargetName: payload.TargetCharacterName,
		}, nil
	default:
		return nil, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command.")
	}
}

func decodeCommandPayloadStrict(payload json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing payload tokens")
		}
		return err
	}
	return nil
}

func sequenceMessage(expected int) string {
	return fmt.Sprintf("Expected command_seq %d.", expected)
}

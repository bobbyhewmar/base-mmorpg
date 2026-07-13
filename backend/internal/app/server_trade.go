package app

import (
	"context"
	"time"
)

type tradeDirectNotification struct {
	send    func(map[string]any) bool
	payload map[string]any
}

func (s *Server) processTradeCommand(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) []map[string]any {
	if session == nil || runtime == nil || s == nil || s.store == nil || s.store.Items == nil || s.store.ActionLogs == nil {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Gameplay trade pipeline is unavailable.")}
	}

	runtime.mu.Lock()
	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		runtime.mu.Unlock()
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if runtime.isPlayerDead() {
		runtime.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}

	actorCharacterID := runtime.characterID
	actorSessionID := runtime.sessionID
	actorName := runtime.characterName
	actorRegionID := runtime.regionID
	actorPosition := runtime.position
	knownTarget, targetKnown := runtime.knownEntities[parsed.targetID]
	runtime.mu.Unlock()

	switch parsed.commandType {
	case "offer_trade_item":
		if !targetKnown || knownTarget.EntityType != "player" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.target_not_known", "Referenced player is not in the current known-set."))
		}

		targetAttached := s.attachedSessionByCharacterID(parsed.targetID)
		if targetAttached == nil || targetAttached.runtime == nil || targetAttached.send == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.target_not_known", "Referenced player is not available for trade."))
		}
		if targetAttached.runtime.regionIDValue() != actorRegionID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.target_not_known", "Referenced player is not available for trade."))
		}

		targetAttached.runtime.syncMovementTo(time.Now())
		targetPresence := targetAttached.runtime.playerPresenceEntity()
		if distance(actorPosition, targetPresence.Position) > playerTradeInteractionRange {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.target_out_of_range", "Referenced player is not in trade range."))
		}

		sourceItem, err := s.validateTradeOfferItem(ctx, actorCharacterID, parsed.itemID, parsed.quantity)
		if err != nil {
			switch err {
			case errItemNotFound:
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.item_not_found", "Referenced inventory item is not available for trade."))
			case errInvalidSplitQuantity:
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.invalid_quantity", "Trade quantity is invalid for the referenced item."))
			default:
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect trade offer inventory."))
			}
		}

		targetName, _ := targetPresence.State["name"].(string)
		if targetName == "" {
			targetName = targetAttached.runtime.characterID
		}

		s.attachedMu.Lock()
		if pending := s.pendingTradeForCharacterLocked(actorCharacterID); pending != nil {
			s.attachedMu.Unlock()
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.actor_busy", "Actor already has a pending trade offer."))
		}
		if pending := s.pendingTradeForCharacterLocked(parsed.targetID); pending != nil {
			s.attachedMu.Unlock()
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.target_busy", "Referenced player already has a pending trade offer."))
		}

		offer := &playerTradeOffer{
			ID:                randomID("trade"),
			SourceSessionID:   actorSessionID,
			SourceCharacterID: actorCharacterID,
			SourceName:        actorName,
			TargetSessionID:   targetAttached.sessionID,
			TargetCharacterID: parsed.targetID,
			TargetName:        targetName,
			ItemInstanceID:    parsed.itemID,
			TemplateID:        sourceItem.TemplateID,
			Quantity:          parsed.quantity,
			RegionID:          actorRegionID,
			CreatedAt:         time.Now(),
		}
		s.pendingTrades[offer.ID] = offer
		s.attachedMu.Unlock()

		if err := s.store.ActionLogs.Create(ctx, ActionLogRecord{
			ID:                 randomID("action"),
			CharacterID:        actorCharacterID,
			ActionType:         "player_trade_offer",
			ReferenceID:        offer.ID,
			CounterpartyEntity: parsed.targetID,
			ItemInstanceID:     parsed.itemID,
			TemplateID:         sourceItem.TemplateID,
			Quantity:           parsed.quantity,
			ItemQuantityBefore: sourceItem.Quantity,
			ItemQuantityAfter:  sourceItem.Quantity,
		}); err != nil {
			s.removePendingTrade(offer.ID)
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist trade offer audit."))
		}

		if targetAttached.send == nil || !targetAttached.sendSerialized(incomingTradeNotice(offer, tradeNoticeStatusPending, actorName+" sent you a trade offer.")) {
			s.removePendingTrade(offer.ID)
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.target_not_known", "Referenced player is not available for trade."))
		}

		notice := outgoingTradeNotice(offer, tradeNoticeStatusPending, "Trade offer sent to "+targetName+".")
		notice["command_id"] = command.CommandID
		notice["command_seq"] = command.CommandSeq
		return append(outbound, notice)
	case "accept_trade_offer":
		offer := s.pendingTradeByID(parsed.tradeOfferID)
		if offer == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.offer_not_found", "Trade offer is not available."))
		}
		if offer.TargetCharacterID != actorCharacterID || offer.TargetSessionID != actorSessionID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.offer_not_recipient", "Trade offer is not assigned to this actor."))
		}

		sourceAttached := s.attachedSessionByCharacterID(offer.SourceCharacterID)
		if sourceAttached == nil || sourceAttached.runtime == nil {
			s.removePendingTrade(offer.ID)
			notice := incomingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because the sender is no longer available.")
			notice["command_id"] = command.CommandID
			notice["command_seq"] = command.CommandSeq
			return append(outbound, notice)
		}
		if sourceAttached.runtime.regionIDValue() != actorRegionID {
			s.removePendingTrade(offer.ID)
			if sourceAttached.send != nil {
				sourceAttached.sendSerialized(outgoingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because both players are no longer nearby."))
			}
			notice := incomingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because both players are no longer nearby.")
			notice["command_id"] = command.CommandID
			notice["command_seq"] = command.CommandSeq
			return append(outbound, notice)
		}

		sourceAttached.runtime.syncMovementTo(time.Now())
		sourcePresence := sourceAttached.runtime.playerPresenceEntity()
		if distance(actorPosition, sourcePresence.Position) > playerTradeInteractionRange {
			s.removePendingTrade(offer.ID)
			if sourceAttached.send != nil {
				sourceAttached.sendSerialized(outgoingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because both players are no longer nearby."))
			}
			notice := incomingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because both players are no longer nearby.")
			notice["command_id"] = command.CommandID
			notice["command_seq"] = command.CommandSeq
			return append(outbound, notice)
		}

		sourceItems, targetItems, err := s.store.Items.TradeInventoryItem(
			ctx,
			offer.SourceCharacterID,
			offer.TargetCharacterID,
			offer.ItemInstanceID,
			offer.Quantity,
			offer.ID,
		)
		if err != nil {
			switch err {
			case errItemNotFound, errInvalidSplitQuantity:
				s.removePendingTrade(offer.ID)
				if sourceAttached.send != nil {
					sourceAttached.sendSerialized(outgoingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because the item is no longer available."))
				}
				notice := incomingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because the item is no longer available.")
				notice["command_id"] = command.CommandID
				notice["command_seq"] = command.CommandSeq
				return append(outbound, notice)
			default:
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist player trade."))
			}
		}

		s.removePendingTrade(offer.ID)
		if sourceAttached.send != nil {
			sourceAttached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
				return []map[string]any{
					runtime.inventoryDeltaMessage("", 0, sourceItems),
					outgoingTradeNotice(offer, tradeNoticeStatusAccepted, actorName+" accepted your trade offer."),
				}
			})
		}

		notice := incomingTradeNotice(offer, tradeNoticeStatusAccepted, "Trade accepted.")
		notice["command_id"] = command.CommandID
		notice["command_seq"] = command.CommandSeq
		return append(
			outbound,
			runtime.inventoryDeltaMessage(command.CommandID, command.CommandSeq, targetItems),
			notice,
		)
	case "decline_trade_offer":
		offer := s.pendingTradeByID(parsed.tradeOfferID)
		if offer == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.offer_not_found", "Trade offer is not available."))
		}
		if offer.TargetCharacterID != actorCharacterID || offer.TargetSessionID != actorSessionID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "trade.offer_not_recipient", "Trade offer is not assigned to this actor."))
		}

		if err := s.store.ActionLogs.Create(ctx, ActionLogRecord{
			ID:                 randomID("action"),
			CharacterID:        actorCharacterID,
			ActionType:         "player_trade_decline",
			ReferenceID:        offer.ID,
			CounterpartyEntity: offer.SourceCharacterID,
			ItemInstanceID:     offer.ItemInstanceID,
			TemplateID:         offer.TemplateID,
			Quantity:           offer.Quantity,
		}); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist trade decline audit."))
		}

		s.removePendingTrade(offer.ID)
		sourceAttached := s.attachedSessionByCharacterID(offer.SourceCharacterID)
		if sourceAttached != nil && sourceAttached.send != nil {
			sourceAttached.sendSerialized(outgoingTradeNotice(offer, tradeNoticeStatusDeclined, actorName+" declined your trade offer."))
		}

		notice := incomingTradeNotice(offer, tradeNoticeStatusDeclined, "Trade declined.")
		notice["command_id"] = command.CommandID
		notice["command_seq"] = command.CommandSeq
		return append(outbound, notice)
	default:
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}
}

func (s *Server) attachedSessionByCharacterID(characterID string) *attachedSession {
	if s == nil || characterID == "" {
		return nil
	}

	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()

	for _, attached := range s.attached {
		if attached == nil || attached.runtime == nil || attached.send == nil || !attached.ready {
			continue
		}
		if attached.runtime.characterID == characterID {
			return attached
		}
	}
	return nil
}

func (s *Server) pendingTradeByID(offerID string) *playerTradeOffer {
	if s == nil || offerID == "" {
		return nil
	}

	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()

	return s.pendingTrades[offerID]
}

func (s *Server) removePendingTrade(offerID string) {
	if s == nil || offerID == "" {
		return
	}

	s.attachedMu.Lock()
	delete(s.pendingTrades, offerID)
	s.attachedMu.Unlock()
}

func (s *Server) pendingTradeForCharacterLocked(characterID string) *playerTradeOffer {
	for _, offer := range s.pendingTrades {
		if offer == nil {
			continue
		}
		if offer.SourceCharacterID == characterID || offer.TargetCharacterID == characterID {
			return offer
		}
	}
	return nil
}

func (s *Server) clearPendingTradesForSession(sessionID string) []tradeDirectNotification {
	if s == nil || sessionID == "" {
		return nil
	}

	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()

	notifications := make([]tradeDirectNotification, 0)
	for offerID, offer := range s.pendingTrades {
		if offer == nil {
			delete(s.pendingTrades, offerID)
			continue
		}

		switch sessionID {
		case offer.SourceSessionID:
			delete(s.pendingTrades, offerID)
			targetAttached := s.attached[offer.TargetSessionID]
			if targetAttached != nil && targetAttached.send != nil && targetAttached.ready {
				notifications = append(notifications, tradeDirectNotification{
					send:    targetAttached.send,
					payload: incomingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because the sender disconnected."),
				})
			}
		case offer.TargetSessionID:
			delete(s.pendingTrades, offerID)
			sourceAttached := s.attached[offer.SourceSessionID]
			if sourceAttached != nil && sourceAttached.send != nil && sourceAttached.ready {
				notifications = append(notifications, tradeDirectNotification{
					send:    sourceAttached.send,
					payload: outgoingTradeNotice(offer, tradeNoticeStatusCancelled, "Trade offer expired because the recipient disconnected."),
				})
			}
		}
	}
	return notifications
}

func (s *Server) validateTradeOfferItem(ctx context.Context, characterID string, itemID string, quantity int) (CharacterItem, error) {
	items, err := s.store.Items.ListByCharacterID(ctx, characterID)
	if err != nil {
		return CharacterItem{}, err
	}

	for _, item := range items {
		if item.ID != itemID {
			continue
		}
		if item.ContainerKind != itemContainerInventory {
			return CharacterItem{}, errItemNotFound
		}
		if quantity <= 0 {
			return CharacterItem{}, errInvalidSplitQuantity
		}
		if !itemTemplateIsStackable(item.TemplateID) {
			if quantity != 1 || item.Quantity != 1 {
				return CharacterItem{}, errInvalidSplitQuantity
			}
			return item, nil
		}
		if quantity > item.Quantity {
			return CharacterItem{}, errInvalidSplitQuantity
		}
		return item, nil
	}

	return CharacterItem{}, errItemNotFound
}

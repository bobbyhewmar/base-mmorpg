package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"
)

func cloneOutboundMessages(messages []map[string]any) []map[string]any {
	if len(messages) == 0 {
		return nil
	}

	bytes, err := json.Marshal(messages)
	if err != nil {
		return nil
	}
	var cloned []map[string]any
	if err := json.Unmarshal(bytes, &cloned); err != nil {
		return nil
	}
	return cloned
}

func gameplayCommandRecordStatusFromOutbound(messages []map[string]any) GameplayCommandRecordStatus {
	for _, message := range messages {
		kind, _ := message["kind"].(string)
		switch kind {
		case "delta", "entity_appear", "entity_disappear", "position_correction", tradeNoticeKind, partyNoticeKind, chatMessageKind:
			return gameplayCommandRecordStatusApplied
		}
	}
	return gameplayCommandRecordStatusRejected
}

func (s *Server) processGameplayCommandWithDedup(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) ([]map[string]any, bool) {
	startedAt := time.Now()
	if session == nil || runtime == nil || s == nil || s.store == nil || s.store.GameplayCommands == nil {
		outboundMessages := []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Gameplay command pipeline is unavailable.")}
		s.recordCommandObservation("", command, outboundMessages, "rejected", time.Since(startedAt))
		return outboundMessages, false
	}

	if replay, resolved, shouldFanOut := s.resolveExistingGameplayCommandRecord(ctx, session.ID, command); resolved {
		result := "replayed"
		if extractRejectReason(replay) != "" {
			result = "rejected"
		}
		s.recordCommandObservation(session.ID, command, replay, result, time.Since(startedAt))
		return replay, shouldFanOut
	}

	shouldPersist := command.CommandID != "" && command.CommandSeq == runtime.expectedCommandSeqValue()
	if shouldPersist {
		record := &GameplayCommandRecord{
			SessionID:   session.ID,
			CommandSeq:  command.CommandSeq,
			CommandID:   command.CommandID,
			CommandType: command.Type,
			Status:      gameplayCommandRecordStatusPending,
		}
		if err := s.store.GameplayCommands.CreatePending(ctx, record); err != nil {
			if errors.Is(err, errRecordConflict) {
				if replay, resolved, shouldFanOut := s.resolveExistingGameplayCommandRecord(ctx, session.ID, command); resolved {
					result := "replayed"
					if extractRejectReason(replay) != "" {
						result = "rejected"
					}
					s.recordCommandObservation(session.ID, command, replay, result, time.Since(startedAt))
					return replay, shouldFanOut
				}
			}
			s.recordStoreError("gameplay_commands.create_pending", err, errRecordConflict)
			log.Printf("gameplay command dedup create failed session=%s seq=%d: %v", session.ID, command.CommandSeq, err)
			outboundMessages := []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist command dedup record.")}
			s.recordCommandObservation(session.ID, command, outboundMessages, "rejected", time.Since(startedAt))
			return outboundMessages, false
		}
	}

	auditCtx := withCommandAuditMetadata(ctx, commandAuditMetadata{
		SessionID:  session.ID,
		CommandID:  command.CommandID,
		CommandSeq: command.CommandSeq,
	})

	var outboundMessages []map[string]any
	if command.Type == "pick_up_loot" {
		outboundMessages = runtime.processLootPickup(auditCtx, s.store, command)
	} else if command.Type == "interact_npc" {
		outboundMessages = runtime.processNPCCommand(auditCtx, s.store, command)
	} else if command.Type == "tame_mob" || command.Type == "summon_pet" || command.Type == "dismiss_pet" || command.Type == "mount_pet" || command.Type == "dismount_pet" {
		outboundMessages = runtime.processPetCommand(auditCtx, s.store, command)
	} else if command.Type == "invite_party_member" || command.Type == "accept_party_invite" || command.Type == "decline_party_invite" || command.Type == "leave_party" || command.Type == "kick_party_member" {
		outboundMessages = s.processPartyCommand(auditCtx, session, runtime, command)
	} else if command.Type == "create_clan" || command.Type == "invite_clan_member" || command.Type == "accept_clan_invite" || command.Type == "decline_clan_invite" || command.Type == "leave_clan" || command.Type == "kick_clan_member" || command.Type == "dissolve_clan" {
		outboundMessages = s.processClanCommand(auditCtx, session, runtime, command)
	} else if command.Type == "basic_attack" || command.Type == "use_skill" {
		outboundMessages = s.processCombatCommand(auditCtx, session, runtime, command)
	} else if command.Type == "send_chat_message" {
		outboundMessages = s.processChatCommand(auditCtx, session, runtime, command)
	} else if command.Type == "set_hotbar_state" {
		outboundMessages = runtime.processHotbarCommand(auditCtx, s.store, command)
	} else if command.Type == "offer_trade_item" || command.Type == "accept_trade_offer" || command.Type == "decline_trade_offer" {
		outboundMessages = s.processTradeCommand(auditCtx, session, runtime, command)
	} else if command.Type == "buy_item" || command.Type == "sell_item" || command.Type == "exchange_item" {
		outboundMessages = runtime.processVendorCommand(auditCtx, s.store, command)
	} else if command.Type == "deposit_item" || command.Type == "withdraw_item" {
		outboundMessages = runtime.processWarehouseCommand(auditCtx, s.store, command)
	} else if command.Type == "equip_item" || command.Type == "unequip_item" || command.Type == "split_item_stack" || command.Type == "merge_item_stacks" || command.Type == "use_item" {
		outboundMessages = runtime.processItemCommand(auditCtx, s.store, command)
	} else {
		outboundMessages = runtime.processCommand(command)
	}

	outboundMessages = s.applyPartyRewardSharing(session, runtime, command, outboundMessages)

	if shouldPersist {
		status := gameplayCommandRecordStatusFromOutbound(outboundMessages)
		if err := s.store.GameplayCommands.UpdateOutcome(ctx, session.ID, command.CommandSeq, status, outboundMessages); err != nil {
			s.recordStoreError("gameplay_commands.update_outcome", err)
			log.Printf("gameplay command dedup finalize failed session=%s seq=%d: %v", session.ID, command.CommandSeq, err)
		}
	}

	if commandOutcomeFromOutbound(outboundMessages) == "applied" {
		if commandTouchesDurableProgression(command.Type) {
			s.persistCharacterProgression(session.CharacterID, runtime)
		}
		if commandTouchesDurableCooldowns(command.Type) {
			s.persistCharacterCooldownState(session.CharacterID, runtime)
		}
		if commandTouchesDurableQuests(command.Type) {
			s.persistCharacterQuestState(session.CharacterID, runtime)
		}
	}

	if reasonCode := extractRejectReason(outboundMessages); reasonCode == "system.persistence_failed" {
		switch command.Type {
		case "pick_up_loot":
			s.recordStoreError("loot.pick_up", errors.New("critical persistence failure"))
		case "equip_item":
			s.recordStoreError("inventory.equip", errors.New("critical persistence failure"))
		case "unequip_item":
			s.recordStoreError("inventory.unequip", errors.New("critical persistence failure"))
		case "split_item_stack":
			s.recordStoreError("inventory.split", errors.New("critical persistence failure"))
		case "merge_item_stacks":
			s.recordStoreError("inventory.merge", errors.New("critical persistence failure"))
		case "use_item":
			s.recordStoreError("inventory.use", errors.New("critical persistence failure"))
		case "set_hotbar_state":
			s.recordStoreError("character_hotbars.replace_by_character", errors.New("critical persistence failure"))
		case "interact_npc":
			s.recordStoreError("quests.interact_npc", errors.New("critical persistence failure"))
		case "tame_mob":
			s.recordStoreError("pets.tame", errors.New("critical persistence failure"))
		case "summon_pet":
			s.recordStoreError("pets.summon", errors.New("critical persistence failure"))
		case "dismiss_pet":
			s.recordStoreError("pets.dismiss", errors.New("critical persistence failure"))
		case "mount_pet":
			s.recordStoreError("pets.mount", errors.New("critical persistence failure"))
		case "dismount_pet":
			s.recordStoreError("pets.dismount", errors.New("critical persistence failure"))
		case "buy_item":
			s.recordStoreError("economy.buy", errors.New("critical persistence failure"))
		case "exchange_item":
			s.recordStoreError("economy.exchange", errors.New("critical persistence failure"))
		case "sell_item":
			s.recordStoreError("economy.sell", errors.New("critical persistence failure"))
		case "deposit_item":
			s.recordStoreError("economy.warehouse.deposit", errors.New("critical persistence failure"))
		case "withdraw_item":
			s.recordStoreError("economy.warehouse.withdraw", errors.New("critical persistence failure"))
		case "offer_trade_item":
			s.recordStoreError("economy.trade", errors.New("critical persistence failure"))
		case "accept_trade_offer":
			s.recordStoreError("economy.trade", errors.New("critical persistence failure"))
		case "invite_party_member":
			s.recordStoreError("parties.invite", errors.New("critical persistence failure"))
		case "accept_party_invite":
			s.recordStoreError("parties.accept", errors.New("critical persistence failure"))
		case "decline_party_invite":
			s.recordStoreError("parties.decline", errors.New("critical persistence failure"))
		case "leave_party":
			s.recordStoreError("parties.leave", errors.New("critical persistence failure"))
		case "kick_party_member":
			s.recordStoreError("parties.kick", errors.New("critical persistence failure"))
		case "create_clan":
			s.recordStoreError("clans.create", errors.New("critical persistence failure"))
		case "invite_clan_member":
			s.recordStoreError("clans.invite", errors.New("critical persistence failure"))
		case "accept_clan_invite":
			s.recordStoreError("clans.accept", errors.New("critical persistence failure"))
		case "decline_clan_invite":
			s.recordStoreError("clans.decline", errors.New("critical persistence failure"))
		case "leave_clan":
			s.recordStoreError("clans.leave", errors.New("critical persistence failure"))
		case "kick_clan_member":
			s.recordStoreError("clans.kick", errors.New("critical persistence failure"))
		case "dissolve_clan":
			s.recordStoreError("clans.dissolve", errors.New("critical persistence failure"))
		case "send_chat_message":
			s.recordStoreError("chat_messages.create", errors.New("critical persistence failure"))
		}
	}

	s.recordCommandObservation(session.ID, command, outboundMessages, "", time.Since(startedAt))

	return outboundMessages, true
}

func (s *Server) processAsyncMovementCommandWithDedup(ctx context.Context, session *Session, attached *attachedSession, runtime *attachedRuntime, command commandEnvelope) []map[string]any {
	startedAt := time.Now()
	if session == nil || runtime == nil || attached == nil || s == nil || s.store == nil || s.store.GameplayCommands == nil {
		outboundMessages := []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Gameplay command pipeline is unavailable.")}
		s.recordCommandObservation("", command, outboundMessages, "rejected", time.Since(startedAt))
		return outboundMessages
	}

	if replay, resolved, _ := s.resolveExistingGameplayCommandRecord(ctx, session.ID, command); resolved {
		result := "replayed"
		if isAckOnlyOutbound(replay) {
			result = "accepted"
		} else if extractRejectReason(replay) != "" {
			result = "rejected"
		}
		s.recordCommandObservation(session.ID, command, replay, result, time.Since(startedAt))
		return replay
	}

	shouldPersist := command.CommandID != "" && command.CommandSeq == runtime.expectedCommandSeqValue()
	if shouldPersist {
		record := &GameplayCommandRecord{
			SessionID:   session.ID,
			CommandSeq:  command.CommandSeq,
			CommandID:   command.CommandID,
			CommandType: command.Type,
			Status:      gameplayCommandRecordStatusPending,
		}
		if err := s.store.GameplayCommands.CreatePending(ctx, record); err != nil {
			if errors.Is(err, errRecordConflict) {
				if replay, resolved, _ := s.resolveExistingGameplayCommandRecord(ctx, session.ID, command); resolved {
					result := "replayed"
					if isAckOnlyOutbound(replay) {
						result = "accepted"
					} else if extractRejectReason(replay) != "" {
						result = "rejected"
					}
					s.recordCommandObservation(session.ID, command, replay, result, time.Since(startedAt))
					return replay
				}
			}
			s.recordStoreError("gameplay_commands.create_pending", err, errRecordConflict)
			log.Printf("gameplay command dedup create failed session=%s seq=%d: %v", session.ID, command.CommandSeq, err)
			outboundMessages := []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist command dedup record.")}
			s.recordCommandObservation(session.ID, command, outboundMessages, "rejected", time.Since(startedAt))
			return outboundMessages
		}
	}

	request, outboundMessages := runtime.prepareAsyncMoveIntent(command)
	if request == nil {
		if shouldPersist {
			s.updateGameplayCommandOutcome(session.ID, command.CommandSeq, gameplayCommandRecordStatusFromOutbound(outboundMessages), outboundMessages)
		}
		s.recordCommandObservation(session.ID, command, outboundMessages, "", time.Since(startedAt))
		return outboundMessages
	}

	moveCtx, cancel := context.WithCancel(ctx)
	replaced := attached.replacePendingMove(&pendingMovementDispatch{
		requestToken: request.RequestToken,
		commandID:    request.CommandID,
		commandSeq:   request.CommandSeq,
		cancel:       cancel,
	})
	if replaced != nil {
		replaced.cancel()
		s.finalizeSupersededMovementOutcome(session.ID, replaced)
	}

	go s.resolveAsyncMovementCommand(moveCtx, session, attached, runtime, request, shouldPersist)
	return outboundMessages
}

func (s *Server) resolveAsyncMovementCommand(
	ctx context.Context,
	session *Session,
	attached *attachedSession,
	runtime *attachedRuntime,
	request *preparedMovementIntent,
	shouldPersist bool,
) {
	if s == nil || session == nil || attached == nil || runtime == nil || request == nil || request.Planner == nil {
		return
	}

	resolution := request.Planner.Resolve(ctx, request.RegionID, request.Start, request.Destination, request.Profile)
	if resolution.Status == movementPlanStatusCanceled || ctx.Err() != nil {
		return
	}

	var outboundMessages []map[string]any
	if !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		outboundMessages = runtime.completeAsyncMoveIntent(request, resolution)
		return outboundMessages
	}) {
		return
	}
	attached.clearPendingMove(request.RequestToken)
	if len(outboundMessages) == 0 {
		return
	}

	if shouldPersist {
		s.updateGameplayCommandOutcome(
			session.ID,
			request.CommandSeq,
			gameplayCommandRecordStatusFromOutbound(outboundMessages),
			outboundMessages,
		)
	}
	s.recordCommandObservation(
		session.ID,
		commandEnvelope{
			CommandID:  request.CommandID,
			CommandSeq: request.CommandSeq,
			Type:       "move_intent",
		},
		outboundMessages,
		"",
		time.Since(request.StartedAt),
	)
	if commandOutcomeFromOutbound(outboundMessages) == "applied" {
		s.fanOutPresenceState(session.ID, runtime)
	}
}

func (s *Server) finalizeSupersededMovementOutcome(sessionID string, pending *pendingMovementDispatch) {
	if pending == nil || pending.commandID == "" || pending.commandSeq <= 0 {
		return
	}
	s.updateGameplayCommandOutcome(
		sessionID,
		pending.commandSeq,
		gameplayCommandRecordStatusRejected,
		[]map[string]any{ackMessage(pending.commandID, pending.commandSeq)},
	)
}

func (s *Server) updateGameplayCommandOutcome(
	sessionID string,
	commandSeq int,
	status GameplayCommandRecordStatus,
	outboundMessages []map[string]any,
) {
	if s == nil || s.store == nil || s.store.GameplayCommands == nil || sessionID == "" || commandSeq <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.store.GameplayCommands.UpdateOutcome(ctx, sessionID, commandSeq, status, outboundMessages); err != nil {
		s.recordStoreError("gameplay_commands.update_outcome", err)
		log.Printf("gameplay command dedup finalize failed session=%s seq=%d: %v", sessionID, commandSeq, err)
	}
}

func isAckOnlyOutbound(messages []map[string]any) bool {
	return len(messages) == 1 && messages[0]["kind"] == "ack"
}

func (s *Server) resolveExistingGameplayCommandRecord(ctx context.Context, sessionID string, command commandEnvelope) ([]map[string]any, bool, bool) {
	record, err := s.store.GameplayCommands.GetBySessionAndSeq(ctx, sessionID, command.CommandSeq)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return nil, false, false
		}
		s.recordStoreError("gameplay_commands.get_by_session_and_seq", err, errRecordNotFound)
		log.Printf("gameplay command dedup load failed session=%s seq=%d: %v", sessionID, command.CommandSeq, err)
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to load command dedup record.")}, true, false
	}

	if record.CommandID != command.CommandID {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "sequence.conflicting_replay", "Command sequence is already bound to a different command_id.")}, true, false
	}
	if record.Status == gameplayCommandRecordStatusPending || len(record.OutboundMessages) == 0 {
		return []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}, true, false
	}
	return cloneOutboundMessages(record.OutboundMessages), true, false
}

func commandTouchesDurableProgression(commandType string) bool {
	switch commandType {
	case "use_skill", "basic_attack", "equip_item", "unequip_item", "use_item":
		return true
	default:
		return false
	}
}

func commandTouchesDurableCooldowns(commandType string) bool {
	switch commandType {
	case "use_skill", "basic_attack":
		return true
	default:
		return false
	}
}

func commandTouchesDurableQuests(commandType string) bool {
	switch commandType {
	case "use_skill", "basic_attack", "interact_npc":
		return true
	default:
		return false
	}
}

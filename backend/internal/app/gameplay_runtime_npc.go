package app

import (
	"context"
	"time"
)

func (runtime *attachedRuntime) processNPCCommand(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
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
	if parsed.commandType != "interact_npc" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}

	npc, exists := runtime.knownEntities[parsed.npcID]
	if !exists || npc.EntityType != "npc" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced NPC is not in the current known-set."))
	}
	if distance(runtime.position, npc.Position) > npcInteractionRange {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "npc.interaction_out_of_range", "Referenced NPC is not within interaction range."))
	}

	now := time.Now()
	if parsed.npcActionID == "" {
		interaction := npcInteractionForQuest(parsed.npcID, runtime.questState)
		if interaction == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_interactable", "Referenced entity does not provide an interaction in this slice."))
		}
		runtime.revision++
		outbound = append(outbound, deltaMessage(
			runtime.revision,
			command.CommandID,
			command.CommandSeq,
			runtime.selfDelta(now, map[string]any{
				"npc_interaction": interaction,
			}),
			nil,
			nil,
		))
		return outbound
	}

	if store == nil || store.CharacterQuests == nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Quest persistence is unavailable."))
	}
	if parsed.npcID != "npc_wardkeeper" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "npc.action_not_supported", "Referenced NPC does not support that action."))
	}

	switch parsed.npcActionID {
	case "accept_task":
		if normalizeCharacterQuestState(runtime.questState).Status != questStatusAvailable {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "quest.action_unavailable", "Quest is not available for acceptance."))
		}
		nextQuest := normalizeCharacterQuestState(runtime.questState)
		nextQuest.CharacterID = runtime.characterID
		nextQuest.Status = questStatusActive
		nextQuest.Progress = 0
		if err := store.CharacterQuests.UpsertByCharacterID(ctx, nextQuest); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist quest acceptance."))
		}
		runtime.questState = nextQuest
		runtime.revision++
		outbound = append(outbound, deltaMessage(
			runtime.revision,
			command.CommandID,
			command.CommandSeq,
			runtime.selfDelta(now, map[string]any{
				"npc_interaction": nil,
			}),
			nil,
			nil,
		))
		return outbound
	case "turn_in_task":
		if normalizeCharacterQuestState(runtime.questState).Status != questStatusReadyToTurnIn {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "quest.action_unavailable", "Quest cannot be completed yet."))
		}
		nextQuest := normalizeCharacterQuestState(runtime.questState)
		nextQuest.CharacterID = runtime.characterID
		nextQuest.Status = questStatusCompleted
		nextQuest.Progress = keeperRequestQuestDefinition.Goal
		items, err := store.CharacterQuests.CompleteQuestWithItemReward(
			ctx,
			nextQuest,
			keeperRequestQuestDefinition.RewardTemplate,
			keeperRequestQuestDefinition.RewardQuantity,
		)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist quest completion."))
		}
		runtime.questState = nextQuest
		runtime.recalculateDerivedStatsLocked(items)
		runtime.revision++
		itemSnapshot := snapshotCharacterItems(items)
		outbound = append(outbound, deltaMessage(
			runtime.revision,
			command.CommandID,
			command.CommandSeq,
			runtime.selfDelta(now, map[string]any{
				"npc_interaction": nil,
			}),
			nil,
			&itemSnapshot,
		))
		return outbound
	default:
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "npc.action_not_supported", "Referenced NPC does not support that action."))
	}
}

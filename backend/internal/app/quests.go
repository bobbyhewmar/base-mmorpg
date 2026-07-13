package app

type questDefinition struct {
	ID             string
	Title          string
	Description    string
	Goal           int
	TargetTemplate string
	RewardTemplate string
	RewardQuantity int
}

const (
	keeperRequestQuestID          = "keeper_request"
	keeperRequestGoal             = 3
	npcInteractionRange           = 4.5
	npcInteractionMerchant        = "merchant_services"
	npcInteractionWarehouse       = "warehouse_services"
	npcInteractionWardkeeperNew   = "wardkeeper_available"
	npcInteractionWardkeeperHunt  = "wardkeeper_active"
	npcInteractionWardkeeperTurn  = "wardkeeper_ready_to_turn_in"
	npcInteractionWardkeeperAfter = "wardkeeper_completed"
)

var keeperRequestQuestDefinition = questDefinition{
	ID:             keeperRequestQuestID,
	Title:          "Keeper of the Gate",
	Description:    "Defeat 3 Mirelings beyond the gate, then return to the wardkeeper.",
	Goal:           keeperRequestGoal,
	TargetTemplate: "mireling",
	RewardTemplate: "wardkeeper_mantle",
	RewardQuantity: 1,
}

func defaultCharacterQuestState() CharacterQuestState {
	return CharacterQuestState{
		QuestID:  keeperRequestQuestDefinition.ID,
		Status:   questStatusAvailable,
		Progress: 0,
	}
}

func normalizeCharacterQuestState(state CharacterQuestState) CharacterQuestState {
	if state.QuestID == "" {
		state.QuestID = keeperRequestQuestDefinition.ID
	}
	if state.QuestID != keeperRequestQuestDefinition.ID {
		return defaultCharacterQuestState()
	}
	switch state.Status {
	case questStatusAvailable, questStatusActive, questStatusReadyToTurnIn, questStatusCompleted:
	default:
		state.Status = questStatusAvailable
	}
	if state.Progress < 0 {
		state.Progress = 0
	}
	if state.Progress > keeperRequestQuestDefinition.Goal {
		state.Progress = keeperRequestQuestDefinition.Goal
	}
	switch state.Status {
	case questStatusAvailable:
		state.Progress = 0
	case questStatusReadyToTurnIn, questStatusCompleted:
		state.Progress = keeperRequestQuestDefinition.Goal
	}
	return state
}

func primaryQuestState(records []CharacterQuestState, characterID string) CharacterQuestState {
	for _, record := range records {
		if record.QuestID == keeperRequestQuestDefinition.ID {
			record.CharacterID = characterID
			return normalizeCharacterQuestState(record)
		}
	}
	state := defaultCharacterQuestState()
	state.CharacterID = characterID
	return state
}

func questSnapshot(state CharacterQuestState) CharacterQuestSnapshot {
	normalized := normalizeCharacterQuestState(state)
	return CharacterQuestSnapshot{
		ID:          keeperRequestQuestDefinition.ID,
		Title:       keeperRequestQuestDefinition.Title,
		Description: keeperRequestQuestDefinition.Description,
		Status:      normalized.Status,
		Progress:    normalized.Progress,
		Goal:        keeperRequestQuestDefinition.Goal,
	}
}

func npcInteractionForQuest(npcID string, state CharacterQuestState) *CharacterNPCInteraction {
	switch npcID {
	case "npc_merchant":
		return &CharacterNPCInteraction{NPCID: npcID, Kind: npcInteractionMerchant}
	case warehouseNPCEntityID:
		return &CharacterNPCInteraction{NPCID: npcID, Kind: npcInteractionWarehouse}
	case "npc_wardkeeper":
		switch normalizeCharacterQuestState(state).Status {
		case questStatusActive:
			return &CharacterNPCInteraction{NPCID: npcID, Kind: npcInteractionWardkeeperHunt}
		case questStatusReadyToTurnIn:
			return &CharacterNPCInteraction{NPCID: npcID, Kind: npcInteractionWardkeeperTurn}
		case questStatusCompleted:
			return &CharacterNPCInteraction{NPCID: npcID, Kind: npcInteractionWardkeeperAfter}
		default:
			return &CharacterNPCInteraction{NPCID: npcID, Kind: npcInteractionWardkeeperNew}
		}
	default:
		return nil
	}
}

func questProgressedByMobKill(state CharacterQuestState, mobTemplateID string) (CharacterQuestState, bool) {
	normalized := normalizeCharacterQuestState(state)
	if normalized.Status != questStatusActive || mobTemplateID != keeperRequestQuestDefinition.TargetTemplate {
		return normalized, false
	}

	normalized.Progress++
	if normalized.Progress >= keeperRequestQuestDefinition.Goal {
		normalized.Progress = keeperRequestQuestDefinition.Goal
		normalized.Status = questStatusReadyToTurnIn
	}
	return normalized, true
}

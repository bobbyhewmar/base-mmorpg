package app

import (
	"context"
	"testing"
)

func sharedPartySnapshot(partyID string, leader *partyTestClient, members ...*partyTestClient) *CharacterPartySnapshot {
	allMembers := append([]*partyTestClient{leader}, members...)
	snapshots := make([]CharacterPartyMemberSnapshot, 0, len(allMembers))
	for _, member := range allMembers {
		snapshots = append(snapshots, member.runtime.partyRosterMemberSnapshot(member.session.CharacterID == leader.session.CharacterID))
	}
	return &CharacterPartySnapshot{
		PartyID:           partyID,
		LeaderCharacterID: leader.session.CharacterID,
		Members:           snapshots,
	}
}

func applySharedPartySnapshot(party *CharacterPartySnapshot, clients ...*partyTestClient) {
	for _, client := range clients {
		client.runtime.loadPartyState(party, nil)
	}
}

func deltaXPValue(message map[string]any) int {
	self, _ := message["self"].(map[string]any)
	if self == nil {
		return 0
	}
	switch value := self["xp"].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func TestPartyRewardSharingSplitsXPAcrossEligibleMembersOnly(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_party_reward_leader", &Character{
		ID:           "char_party_reward_leader",
		AccountID:    "acc_party_reward_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	nearby := stagePartyTestClient(t, server, store, "sess_party_reward_nearby", &Character{
		ID:           "char_party_reward_nearby",
		AccountID:    "acc_party_reward_nearby",
		Name:         "Near",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	otherRegion := stagePartyTestClient(t, server, store, "sess_party_reward_other_region", &Character{
		ID:           "char_party_reward_other_region",
		AccountID:    "acc_party_reward_other_region",
		Name:         "Far",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	deadMember := stagePartyTestClient(t, server, store, "sess_party_reward_dead", &Character{
		ID:           "char_party_reward_dead",
		AccountID:    "acc_party_reward_dead",
		Name:         "Down",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})

	party := sharedPartySnapshot("party_rewards_1", leader, nearby, otherRegion, deadMember)
	applySharedPartySnapshot(party, leader, nearby, otherRegion, deadMember)
	otherRegion.runtime.regionID = "gate_road"
	deadMember.runtime.currentHP = 0

	moveRuntimeNearMob(leader.runtime, "mob_1")
	entity := leader.runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	leader.runtime.knownEntities["mob_1"] = entity

	leader.resetMessages()
	nearby.resetMessages()
	otherRegion.resetMessages()
	deadMember.resetMessages()

	outbound := dispatchPartyCommand(t, server, leader, "cmd_party_reward_attack_1", 1, "basic_attack", map[string]any{
		"target_id": "mob_1",
	})

	if findOutboundMessage(outbound, "ack") == nil {
		t.Fatalf("expected shared reward kill to ack, got %+v", outbound)
	}
	delta := findOutboundMessage(outbound, "delta")
	if delta == nil {
		t.Fatalf("expected authoritative delta for killer, got %+v", outbound)
	}
	if xp := deltaXPValue(delta); xp != 11 {
		t.Fatalf("expected killer xp share 11, got %+v", delta)
	}
	lootAppear := findOutboundMessage(outbound, "entity_appear")
	if lootAppear == nil {
		t.Fatalf("expected loot entity appear on kill, got %+v", outbound)
	}
	lootEntity, ok := lootAppear["entity"].(runtimeEntity)
	if !ok {
		t.Fatalf("expected runtime loot entity, got %+v", lootAppear["entity"])
	}
	if lootEntity.State["party_id"] != "party_rewards_1" {
		t.Fatalf("expected party-owned loot, got %+v", lootEntity.State)
	}
	eligibleCharacterIDs := runtimeLootEligibleCharacterIDs(lootEntity)
	if len(eligibleCharacterIDs) != 2 || eligibleCharacterIDs[0] != leader.session.CharacterID || eligibleCharacterIDs[1] != nearby.session.CharacterID {
		t.Fatalf("expected loot eligibility only for alive same-region party members, got %+v", eligibleCharacterIDs)
	}

	nearbyDelta := findOutboundMessage(nearby.messages, "delta")
	if nearbyDelta == nil || deltaXPValue(nearbyDelta) != 11 {
		t.Fatalf("expected nearby party member to receive shared xp delta, got %+v", nearby.messages)
	}
	if findOutboundMessage(otherRegion.messages, "delta") != nil {
		t.Fatalf("expected other-region member to receive no shared xp delta, got %+v", otherRegion.messages)
	}
	if findOutboundMessage(deadMember.messages, "delta") != nil {
		t.Fatalf("expected dead member to receive no shared xp delta, got %+v", deadMember.messages)
	}

	persistedNearby, err := store.Characters.GetByID(context.Background(), nearby.session.CharacterID)
	if err != nil {
		t.Fatalf("Characters.GetByID(nearby) error = %v", err)
	}
	if persistedNearby.XP != 11 {
		t.Fatalf("expected persisted shared xp 11 for nearby member, got %+v", persistedNearby)
	}

	persistedLeader, err := store.Characters.GetByID(context.Background(), leader.session.CharacterID)
	if err != nil {
		t.Fatalf("Characters.GetByID(leader) error = %v", err)
	}
	if persistedLeader.XP != 11 {
		t.Fatalf("expected persisted shared xp 11 for leader, got %+v", persistedLeader)
	}
}

func TestPartyRewardSharingFallsBackToFullXPWhenNoOtherMemberIsEligible(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_party_reward_fallback_leader", &Character{
		ID:           "char_party_reward_fallback_leader",
		AccountID:    "acc_party_reward_fallback_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	otherRegion := stagePartyTestClient(t, server, store, "sess_party_reward_fallback_other", &Character{
		ID:           "char_party_reward_fallback_other",
		AccountID:    "acc_party_reward_fallback_other",
		Name:         "Far",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	deadMember := stagePartyTestClient(t, server, store, "sess_party_reward_fallback_dead", &Character{
		ID:           "char_party_reward_fallback_dead",
		AccountID:    "acc_party_reward_fallback_dead",
		Name:         "Dead",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})

	party := sharedPartySnapshot("party_rewards_2", leader, otherRegion, deadMember)
	applySharedPartySnapshot(party, leader, otherRegion, deadMember)
	otherRegion.runtime.regionID = "gate_road"
	deadMember.runtime.currentHP = 0

	moveRuntimeNearMob(leader.runtime, "mob_1")
	entity := leader.runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	leader.runtime.knownEntities["mob_1"] = entity

	outbound := dispatchPartyCommand(t, server, leader, "cmd_party_reward_attack_2", 1, "basic_attack", map[string]any{
		"target_id": "mob_1",
	})

	delta := findOutboundMessage(outbound, "delta")
	if delta == nil || deltaXPValue(delta) != 22 {
		t.Fatalf("expected killer to keep full xp when no other member is eligible, got %+v", outbound)
	}
	lootAppear := findOutboundMessage(outbound, "entity_appear")
	if lootAppear == nil {
		t.Fatalf("expected loot entity appear, got %+v", outbound)
	}
	lootEntity, ok := lootAppear["entity"].(runtimeEntity)
	if !ok {
		t.Fatalf("expected runtime loot entity, got %+v", lootAppear["entity"])
	}
	if _, exists := lootEntity.State["party_id"]; exists {
		t.Fatalf("expected solo fallback loot to stay unowned by party, got %+v", lootEntity.State)
	}
}

func TestPartyRewardSharingAppendsKillerProgressionDeltaWhenOutboundLacksDelta(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_party_reward_missing_delta_leader", &Character{
		ID:           "char_party_reward_missing_delta_leader",
		AccountID:    "acc_party_reward_missing_delta_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	nearby := stagePartyTestClient(t, server, store, "sess_party_reward_missing_delta_nearby", &Character{
		ID:           "char_party_reward_missing_delta_nearby",
		AccountID:    "acc_party_reward_missing_delta_nearby",
		Name:         "Near",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})

	party := sharedPartySnapshot("party_rewards_missing_delta", leader, nearby)
	applySharedPartySnapshot(party, leader, nearby)
	leader.runtime.pendingPartyRewards = append(leader.runtime.pendingPartyRewards, pendingPartyRewardEvent{XPAmount: 22})

	outbound := server.applyPartyRewardSharing(leader.session, leader.runtime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_party_reward_missing_delta",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	}, []map[string]any{
		ackMessage("cmd_party_reward_missing_delta", 1),
	})

	delta := findOutboundMessage(outbound, "delta")
	if delta == nil {
		t.Fatalf("expected appended authoritative killer delta, got %+v", outbound)
	}
	if xp := deltaXPValue(delta); xp != 11 {
		t.Fatalf("expected killer xp share 11 in appended delta, got %+v", delta)
	}

	persistedLeader, err := store.Characters.GetByID(context.Background(), leader.session.CharacterID)
	if err != nil {
		t.Fatalf("Characters.GetByID(leader) error = %v", err)
	}
	if persistedLeader.XP != 11 {
		t.Fatalf("expected persisted leader shared xp 11, got %+v", persistedLeader)
	}

	persistedNearby, err := store.Characters.GetByID(context.Background(), nearby.session.CharacterID)
	if err != nil {
		t.Fatalf("Characters.GetByID(nearby) error = %v", err)
	}
	if persistedNearby.XP != 11 {
		t.Fatalf("expected persisted nearby shared xp 11, got %+v", persistedNearby)
	}
}

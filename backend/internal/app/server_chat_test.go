package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func stageChatTestClient(t *testing.T, server *Server, store *Store, sessionID string, character *Character) *partyTestClient {
	t.Helper()

	if err := store.CreateAccountWithCredential(context.Background(), &Account{
		ID:          character.AccountID,
		Login:       "login_" + character.AccountID,
		DisplayName: character.Name,
		State:       accountStateActive,
	}, &CredentialRecord{
		AccountID:         character.AccountID,
		PasswordHash:      "test_hash",
		PasswordAlgorithm: "bcrypt",
	}); err != nil && !errors.Is(err, errRecordConflict) {
		t.Fatalf("CreateAccountWithCredential(%s) error = %v", character.AccountID, err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(%s) error = %v", character.ID, err)
	}

	session := &Session{
		ID:              sessionID,
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_" + sessionID,
		AttachExpiresAt: time.Now().Add(5 * time.Minute).UTC(),
		Status:          sessionStatusAttached,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create(%s) error = %v", sessionID, err)
	}

	client := &partyTestClient{
		session: session,
		runtime: newAttachedRuntime(sessionID, character),
	}
	server.stageAttachedSession(sessionID, client.runtime, func(message map[string]any) bool {
		client.messages = append(client.messages, message)
		return true
	})
	client.messages = append(client.messages, server.activateAttachedSession(sessionID)...)
	t.Cleanup(func() {
		server.unregisterAttachedSession(sessionID)
	})
	return client
}

func findChatMessage(messages []map[string]any, channel string) map[string]any {
	for _, message := range messages {
		if message["kind"] != chatMessageKind {
			continue
		}
		if channel == "" {
			return message
		}
		if messageChannel, _ := message["channel"].(string); messageChannel == channel {
			return message
		}
	}
	return nil
}

func countMessageKind(messages []map[string]any, kind string) int {
	count := 0
	for _, message := range messages {
		if message["kind"] == kind {
			count++
		}
	}
	return count
}

func TestServerChatRegionFanOutAndPersistence(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sender := stageChatTestClient(t, server, store, "sess_chat_region_sender", &Character{
		ID:           "char_chat_region_sender",
		AccountID:    "acc_chat_region_sender",
		Name:         "Arden",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	nearby := stageChatTestClient(t, server, store, "sess_chat_region_nearby", &Character{
		ID:           "char_chat_region_nearby",
		AccountID:    "acc_chat_region_nearby",
		Name:         "Selene",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -7,
		PositionZ:    1,
	})
	otherRegion := stageChatTestClient(t, server, store, "sess_chat_region_other", &Character{
		ID:           "char_chat_region_other",
		AccountID:    "acc_chat_region_other",
		Name:         "Bastion",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "gate_road",
		PositionX:    3,
		PositionZ:    0,
	})
	sender.resetMessages()
	nearby.resetMessages()
	otherRegion.resetMessages()

	outbound := dispatchPartyCommand(t, server, sender, "cmd_chat_region_1", 1, "send_chat_message", map[string]any{
		"channel": "region",
		"text":    "  <b>Hello</b>\nworld  ",
	})
	if findOutboundMessage(outbound, "ack") == nil {
		t.Fatalf("expected ack, got %+v", outbound)
	}
	senderMessage := findChatMessage(outbound, chatChannelRegion)
	if senderMessage == nil {
		t.Fatalf("expected sender chat_message, got %+v", outbound)
	}
	if senderMessage["text"] != "<b>Hello</b> world" {
		t.Fatalf("expected normalized text, got %+v", senderMessage)
	}
	if nearbyMessage := findChatMessage(nearby.messages, chatChannelRegion); nearbyMessage == nil {
		t.Fatalf("expected nearby chat delivery, got %+v", nearby.messages)
	}
	if otherMessage := findChatMessage(otherRegion.messages, chatChannelRegion); otherMessage != nil {
		t.Fatalf("expected no other-region delivery, got %+v", otherRegion.messages)
	}

	records, err := store.ChatMessages.ListByCharacterID(context.Background(), sender.session.CharacterID)
	if err != nil {
		t.Fatalf("ChatMessages.ListByCharacterID() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one persisted chat record, got %+v", records)
	}
	if records[0].AccountID != sender.session.AccountID {
		t.Fatalf("expected persisted account id %s, got %+v", sender.session.AccountID, records[0])
	}
	if records[0].Channel != chatChannelRegion || records[0].RegionID != "dawn_plaza" || records[0].Text != "<b>Hello</b> world" {
		t.Fatalf("unexpected persisted chat record %+v", records[0])
	}
}

func TestServerChatAllowsDeadActorsAndRejectsLegacyLocalChannel(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sender := stageChatTestClient(t, server, store, "sess_chat_dead_sender", &Character{
		ID:           "char_chat_dead_sender",
		AccountID:    "acc_chat_dead_sender",
		Name:         "Arden",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	nearby := stageChatTestClient(t, server, store, "sess_chat_dead_nearby", &Character{
		ID:           "char_chat_dead_nearby",
		AccountID:    "acc_chat_dead_nearby",
		Name:         "Selene",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	sender.resetMessages()
	nearby.resetMessages()

	sender.runtime.currentHP = 0

	allowedOutbound := dispatchPartyCommand(t, server, sender, "cmd_chat_dead_1", 1, "send_chat_message", map[string]any{
		"channel": "region",
		"text":    "Still online while dead.",
	})
	if findOutboundMessage(allowedOutbound, "ack") == nil {
		t.Fatalf("expected ack for dead actor region chat, got %+v", allowedOutbound)
	}
	if findOutboundMessage(allowedOutbound, "reject") != nil {
		t.Fatalf("expected dead actor region chat to stay allowed, got %+v", allowedOutbound)
	}
	if nearbyMessage := findChatMessage(nearby.messages, chatChannelRegion); nearbyMessage == nil {
		t.Fatalf("expected dead actor region chat fan-out, got %+v", nearby.messages)
	}

	rejectOutbound := dispatchPartyCommand(t, server, sender, "cmd_chat_dead_2", 2, "send_chat_message", map[string]any{
		"channel": "local",
		"text":    "Legacy local should reject.",
	})
	requireRejectReason(t, rejectOutbound, "chat.channel_unknown")
}

func TestServerChatWhisperFanOutAndTargetLookup(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sender := stageChatTestClient(t, server, store, "sess_chat_whisper_sender", &Character{
		ID:           "char_chat_whisper_sender",
		AccountID:    "acc_chat_whisper_sender",
		Name:         "Arden",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	target := stageChatTestClient(t, server, store, "sess_chat_whisper_target", &Character{
		ID:           "char_chat_whisper_target",
		AccountID:    "acc_chat_whisper_target",
		Name:         "Selene",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "gate_road",
	})
	bystander := stageChatTestClient(t, server, store, "sess_chat_whisper_bystander", &Character{
		ID:           "char_chat_whisper_bystander",
		AccountID:    "acc_chat_whisper_bystander",
		Name:         "Bastion",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	sender.resetMessages()
	target.resetMessages()
	bystander.resetMessages()

	outbound := dispatchPartyCommand(t, server, sender, "cmd_chat_whisper_1", 1, "send_chat_message", map[string]any{
		"channel":               "whisper",
		"text":                  "Meet at the gate.",
		"target_character_name": "selene",
	})
	if findOutboundMessage(outbound, "ack") == nil {
		t.Fatalf("expected ack, got %+v", outbound)
	}
	senderMessage := findChatMessage(outbound, chatChannelWhisper)
	if senderMessage == nil {
		t.Fatalf("expected sender whisper payload, got %+v", outbound)
	}
	if senderMessage["target_character_id"] != target.session.CharacterID || senderMessage["target_character_name"] != "Selene" {
		t.Fatalf("expected target metadata in sender whisper payload, got %+v", senderMessage)
	}
	if targetMessage := findChatMessage(target.messages, chatChannelWhisper); targetMessage == nil {
		t.Fatalf("expected whisper delivery to target, got %+v", target.messages)
	}
	if bystanderMessage := findChatMessage(bystander.messages, chatChannelWhisper); bystanderMessage != nil {
		t.Fatalf("expected no whisper delivery to bystander, got %+v", bystander.messages)
	}

	records, err := store.ChatMessages.ListByCharacterID(context.Background(), sender.session.CharacterID)
	if err != nil {
		t.Fatalf("ChatMessages.ListByCharacterID() error = %v", err)
	}
	if len(records) != 1 || records[0].TargetCharacterID != target.session.CharacterID {
		t.Fatalf("expected persisted whisper target metadata, got %+v", records)
	}
}

func TestServerChatPartyAuthorityAndDedup(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stageChatTestClient(t, server, store, "sess_chat_party_leader", &Character{
		ID:           "char_chat_party_leader",
		AccountID:    "acc_chat_party_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	recruit := stageChatTestClient(t, server, store, "sess_chat_party_recruit", &Character{
		ID:           "char_chat_party_recruit",
		AccountID:    "acc_chat_party_recruit",
		Name:         "Recruit",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	leader.resetMessages()
	recruit.resetMessages()

	rejectOutbound := dispatchPartyCommand(t, server, leader, "cmd_chat_party_reject_1", 1, "send_chat_message", map[string]any{
		"channel": "party",
		"text":    "Hello without party.",
	})
	requireRejectReason(t, rejectOutbound, "chat.party_required")

	aimPartyInviteTarget(leader, recruit)
	inviteOutbound := dispatchPartyCommand(t, server, leader, "cmd_party_chat_invite_1", 2, "invite_party_member", map[string]any{})
	receivedInvite := findPartyNotice(recruit.messages, partyNoticeStatusInviteReceived)
	if findOutboundMessage(inviteOutbound, "ack") == nil || receivedInvite == nil {
		t.Fatalf("expected invite flow before party chat, outbound=%+v recruit=%+v", inviteOutbound, recruit.messages)
	}
	inviteID, _ := receivedInvite["invite_id"].(string)
	recruit.resetMessages()
	leader.resetMessages()
	acceptOutbound := dispatchPartyCommand(t, server, recruit, "cmd_party_chat_accept_1", 1, "accept_party_invite", map[string]any{
		"invite_id": inviteID,
	})
	if findOutboundMessage(acceptOutbound, "ack") == nil {
		t.Fatalf("expected accept ack, got %+v", acceptOutbound)
	}

	recruit.resetMessages()
	leader.resetMessages()
	partyChatOutbound := dispatchPartyCommand(t, server, leader, "cmd_chat_party_1", 3, "send_chat_message", map[string]any{
		"channel": "party",
		"text":    "Party up at the plaza.",
	})
	if findChatMessage(partyChatOutbound, chatChannelParty) == nil {
		t.Fatalf("expected sender party chat payload, got %+v", partyChatOutbound)
	}
	if countMessageKind(recruit.messages, chatMessageKind) != 1 {
		t.Fatalf("expected one party chat fan-out to recruit, got %+v", recruit.messages)
	}

	replayOutbound := dispatchPartyCommand(t, server, leader, "cmd_chat_party_1", 3, "send_chat_message", map[string]any{
		"channel": "party",
		"text":    "Party up at the plaza.",
	})
	if findChatMessage(replayOutbound, chatChannelParty) == nil {
		t.Fatalf("expected replayed sender party chat payload, got %+v", replayOutbound)
	}
	if countMessageKind(recruit.messages, chatMessageKind) != 1 {
		t.Fatalf("expected replay dedup to avoid duplicate party fan-out, got %+v", recruit.messages)
	}
}

func TestServerChatRateLimitRejectsBurstSpam(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sender := stageChatTestClient(t, server, store, "sess_chat_rate_sender", &Character{
		ID:           "char_chat_rate_sender",
		AccountID:    "acc_chat_rate_sender",
		Name:         "Arden",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	sender.resetMessages()

	for index := 1; index <= chatRateLimitBurst; index++ {
		outbound := dispatchPartyCommand(t, server, sender, "cmd_chat_rate_"+time.Now().Add(time.Duration(index)*time.Nanosecond).Format("150405.000000000"), index, "send_chat_message", map[string]any{
			"channel": "region",
			"text":    "burst message",
		})
		if findChatMessage(outbound, chatChannelRegion) == nil {
			t.Fatalf("expected burst message %d to pass, got %+v", index, outbound)
		}
	}

	rejectOutbound := dispatchPartyCommand(t, server, sender, "cmd_chat_rate_limit_5", chatRateLimitBurst+1, "send_chat_message", map[string]any{
		"channel": "region",
		"text":    "one too many",
	})
	requireRejectReason(t, rejectOutbound, "chat.rate_limited")
}

func TestMemoryChatMessageRepoFilters(t *testing.T) {
	store := newMemoryStore()
	characterA := &Character{ID: "char_chat_repo_a", AccountID: "acc_chat_repo_a", Name: "Repo A", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	characterB := &Character{ID: "char_chat_repo_b", AccountID: "acc_chat_repo_b", Name: "Repo B", BaseClass: "Mage", LastRegionID: "gate_road"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), characterA, initialCharacterItemSeed(characterA)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(characterA) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), characterB, initialCharacterItemSeed(characterB)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(characterB) error = %v", err)
	}

	now := time.Now().UTC()
	createRecord := func(record ChatMessageRecord) {
		if err := store.ChatMessages.Create(context.Background(), record); err != nil {
			t.Fatalf("ChatMessages.Create(%s) error = %v", record.ID, err)
		}
	}
	createRecord(ChatMessageRecord{
		ID:          "chat_repo_1",
		CharacterID: characterA.ID,
		Channel:     chatChannelRegion,
		RegionID:    "dawn_plaza",
		Text:        "first",
		CreatedAt:   now.Add(-2 * time.Minute),
	})
	createRecord(ChatMessageRecord{
		ID:                "chat_repo_2",
		CharacterID:       characterA.ID,
		Channel:           chatChannelWhisper,
		TargetCharacterID: characterB.ID,
		Text:              "second",
		CreatedAt:         now.Add(-time.Minute),
	})
	createRecord(ChatMessageRecord{
		ID:          "chat_repo_3",
		CharacterID: characterB.ID,
		Channel:     chatChannelParty,
		Text:        "third",
		CreatedAt:   now,
	})

	records, err := store.ChatMessages.ListByCharacterID(context.Background(), characterA.ID)
	if err != nil {
		t.Fatalf("ChatMessages.ListByCharacterID(characterA) error = %v", err)
	}
	if len(records) != 2 || records[0].ID != "chat_repo_2" || records[1].AccountID != characterA.AccountID {
		t.Fatalf("unexpected character records %+v", records)
	}

	filtered, err := store.ChatMessages.ListByFilter(context.Background(), ChatMessageQuery{
		TargetCharacterID: characterB.ID,
		Channel:           chatChannelWhisper,
		OccurredAfter:     ptrTime(now.Add(-90 * time.Second)),
		OccurredBefore:    ptrTime(now.Add(10 * time.Second)),
		Limit:             10,
	})
	if err != nil {
		t.Fatalf("ChatMessages.ListByFilter() error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "chat_repo_2" {
		t.Fatalf("unexpected filtered chat records %+v", filtered)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

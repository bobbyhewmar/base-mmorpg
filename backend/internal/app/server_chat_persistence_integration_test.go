package app

import (
	"context"
	"os"
	"testing"
)

func TestServerChatPersistsAcrossPostgresRestart(t *testing.T) {
	env := newPersistenceTestEnv(t)
	senderAccountID, _ := registerAndLogin(t, env, "persist.chat.sender@test")
	targetAccountID, _ := registerAndLogin(t, env, "persist.chat.target@test")

	sender := stageChatTestClient(t, env.server, env.store, "sess_chat_pg_sender", &Character{
		ID:           "char_chat_pg_sender",
		AccountID:    senderAccountID,
		Name:         "Arden",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	target := stageChatTestClient(t, env.server, env.store, "sess_chat_pg_target", &Character{
		ID:           "char_chat_pg_target",
		AccountID:    targetAccountID,
		Name:         "Selene",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "gate_road",
	})
	sender.resetMessages()
	target.resetMessages()

	outbound := dispatchPartyCommand(t, env.server, sender, "cmd_chat_pg_1", 1, "send_chat_message", map[string]any{
		"channel":               "whisper",
		"text":                  "See you after restart.",
		"target_character_name": "Selene",
	})
	if findChatMessage(outbound, chatChannelWhisper) == nil {
		t.Fatalf("expected whisper chat_message, got %+v", outbound)
	}

	initialRecords, err := env.store.ChatMessages.ListByCharacterID(context.Background(), sender.session.CharacterID)
	if err != nil {
		t.Fatalf("ChatMessages.ListByCharacterID(initial) error = %v", err)
	}
	if len(initialRecords) != 1 || initialRecords[0].TargetCharacterID != target.session.CharacterID {
		t.Fatalf("unexpected initial chat records %+v", initialRecords)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	restartedRecords, err := restartedStore.ChatMessages.ListByCharacterID(context.Background(), sender.session.CharacterID)
	if err != nil {
		t.Fatalf("ChatMessages.ListByCharacterID(restart) error = %v", err)
	}
	if len(restartedRecords) != 1 {
		t.Fatalf("expected one restarted chat record, got %+v", restartedRecords)
	}
	if restartedRecords[0].Text != "See you after restart." || restartedRecords[0].TargetCharacterID != target.session.CharacterID {
		t.Fatalf("unexpected restarted chat record %+v", restartedRecords[0])
	}
}

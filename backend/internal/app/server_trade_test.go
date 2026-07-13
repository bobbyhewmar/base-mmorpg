package app

import (
	"context"
	"encoding/json"
	"testing"
)

func findActionLogByType(records []ActionLogRecord, actionType string) *ActionLogRecord {
	for index := range records {
		if records[index].ActionType == actionType {
			return &records[index]
		}
	}
	return nil
}

func TestServerTradeOfferAcceptTransfersInventoryAndRecordsAudit(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sourceCharacter := &Character{ID: "char_trade_source", AccountID: "acc_trade_source", Name: "Source", BaseClass: "Fighter", Sex: "Male", Level: 1, LastRegionID: "dawn_plaza", PositionX: -8, PositionZ: 0}
	targetCharacter := &Character{ID: "char_trade_target", AccountID: "acc_trade_target", Name: "Target", BaseClass: "Mage", Sex: "Female", Level: 1, LastRegionID: "dawn_plaza", PositionX: -8, PositionZ: 0}
	if err := store.CreateCharacterWithItemSeed(context.Background(), sourceCharacter, initialCharacterItemSeed(sourceCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(source) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), targetCharacter, initialCharacterItemSeed(targetCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(target) error = %v", err)
	}

	sourceRuntime := newAttachedRuntime("sess_trade_source", sourceCharacter)
	targetRuntime := newAttachedRuntime("sess_trade_target", targetCharacter)
	var sourceMessages []map[string]any
	var targetMessages []map[string]any
	server.stageAttachedSession("sess_trade_source", sourceRuntime, func(message map[string]any) bool {
		sourceMessages = append(sourceMessages, message)
		return true
	})
	_ = server.activateAttachedSession("sess_trade_source")
	server.stageAttachedSession("sess_trade_target", targetRuntime, func(message map[string]any) bool {
		targetMessages = append(targetMessages, message)
		return true
	})
	targetMessages = append(targetMessages, server.activateAttachedSession("sess_trade_target")...)
	defer server.unregisterAttachedSession("sess_trade_source")
	defer server.unregisterAttachedSession("sess_trade_target")
	sourceMessages = nil
	targetMessages = nil

	sourceItems, err := store.Items.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(source) error = %v", err)
	}
	sourceGoldID := ""
	for _, item := range sourceItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			sourceGoldID = item.ID
			break
		}
	}
	if sourceGoldID == "" {
		t.Fatal("expected source inventory duskgold stack")
	}

	offerPayload, err := json.Marshal(map[string]any{
		"target_character_id": targetCharacter.ID,
		"item_instance_id":    sourceGoldID,
		"quantity":            1,
	})
	if err != nil {
		t.Fatalf("json.Marshal(offer) error = %v", err)
	}
	offerOutbound := server.processTradeCommand(context.Background(), &Session{ID: "sess_trade_source", CharacterID: sourceCharacter.ID}, sourceRuntime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_trade_offer_1",
		CommandSeq:      1,
		Type:            "offer_trade_item",
		Payload:         offerPayload,
	})
	if len(offerOutbound) != 2 || offerOutbound[0]["kind"] != "ack" || offerOutbound[1]["kind"] != tradeNoticeKind {
		t.Fatalf("expected ack and outgoing trade notice, got %+v", offerOutbound)
	}
	if len(targetMessages) != 1 || targetMessages[0]["kind"] != tradeNoticeKind {
		t.Fatalf("expected one incoming trade notice for target, got %+v", targetMessages)
	}

	offerID, _ := targetMessages[0]["offer_id"].(string)
	if offerID == "" {
		t.Fatalf("expected incoming trade notice to include offer_id, got %+v", targetMessages[0])
	}

	acceptPayload, err := json.Marshal(map[string]any{
		"trade_offer_id": offerID,
	})
	if err != nil {
		t.Fatalf("json.Marshal(accept) error = %v", err)
	}
	targetMessages = nil
	sourceMessages = nil
	acceptOutbound := server.processTradeCommand(context.Background(), &Session{ID: "sess_trade_target", CharacterID: targetCharacter.ID}, targetRuntime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_trade_accept_1",
		CommandSeq:      1,
		Type:            "accept_trade_offer",
		Payload:         acceptPayload,
	})
	if len(acceptOutbound) != 3 || acceptOutbound[0]["kind"] != "ack" || acceptOutbound[1]["kind"] != "delta" || acceptOutbound[2]["kind"] != tradeNoticeKind {
		t.Fatalf("expected ack, target delta, and accepted trade notice, got %+v", acceptOutbound)
	}
	if len(sourceMessages) != 2 || sourceMessages[0]["kind"] != "delta" || sourceMessages[1]["kind"] != tradeNoticeKind {
		t.Fatalf("expected source to receive inventory delta and trade resolution notice, got %+v", sourceMessages)
	}

	sourceItems, err = store.Items.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(source) after accept error = %v", err)
	}
	targetItems, err := store.Items.ListByCharacterID(context.Background(), targetCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(target) after accept error = %v", err)
	}

	sourceGold := 0
	for _, item := range sourceItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			sourceGold += item.Quantity
		}
	}
	targetGold := 0
	for _, item := range targetItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			targetGold += item.Quantity
		}
	}
	if sourceGold != 11 || targetGold != 13 {
		t.Fatalf("expected post-trade gold to be 11 and 13, got source=%d target=%d", sourceGold, targetGold)
	}

	sourceLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID(source) error = %v", err)
	}
	targetLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), targetCharacter.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID(target) error = %v", err)
	}
	sourceOfferLog := findActionLogByType(sourceLogs, "player_trade_offer")
	if sourceOfferLog == nil {
		t.Fatalf("unexpected source trade offer audit log = %+v", sourceLogs)
	}
	sourceSendLog := findActionLogByType(sourceLogs, "player_trade_send")
	if sourceSendLog == nil {
		t.Fatalf("unexpected source trade send audit log = %+v", sourceLogs)
	}
	targetAcceptLog := findActionLogByType(targetLogs, "player_trade_accept")
	if targetAcceptLog == nil {
		t.Fatalf("unexpected target trade accept audit log = %+v", targetLogs)
	}
	targetReceiveLog := findActionLogByType(targetLogs, "player_trade_receive")
	if targetReceiveLog == nil {
		t.Fatalf("unexpected target trade receive audit log = %+v", targetLogs)
	}
}

func TestServerTradeOfferDeclineLeavesInventoryUnchanged(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sourceCharacter := &Character{ID: "char_trade_decline_source", AccountID: "acc_trade_decline_source", Name: "Source", BaseClass: "Fighter", Sex: "Male", Level: 1, LastRegionID: "dawn_plaza", PositionX: -8, PositionZ: 0}
	targetCharacter := &Character{ID: "char_trade_decline_target", AccountID: "acc_trade_decline_target", Name: "Target", BaseClass: "Mage", Sex: "Female", Level: 1, LastRegionID: "dawn_plaza", PositionX: -8, PositionZ: 0}
	if err := store.CreateCharacterWithItemSeed(context.Background(), sourceCharacter, initialCharacterItemSeed(sourceCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(source) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), targetCharacter, initialCharacterItemSeed(targetCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(target) error = %v", err)
	}

	sourceRuntime := newAttachedRuntime("sess_trade_decline_source", sourceCharacter)
	targetRuntime := newAttachedRuntime("sess_trade_decline_target", targetCharacter)
	var sourceMessages []map[string]any
	var targetMessages []map[string]any
	server.stageAttachedSession("sess_trade_decline_source", sourceRuntime, func(message map[string]any) bool {
		sourceMessages = append(sourceMessages, message)
		return true
	})
	_ = server.activateAttachedSession("sess_trade_decline_source")
	server.stageAttachedSession("sess_trade_decline_target", targetRuntime, func(message map[string]any) bool {
		targetMessages = append(targetMessages, message)
		return true
	})
	targetMessages = append(targetMessages, server.activateAttachedSession("sess_trade_decline_target")...)
	defer server.unregisterAttachedSession("sess_trade_decline_source")
	defer server.unregisterAttachedSession("sess_trade_decline_target")
	sourceMessages = nil
	targetMessages = nil

	sourceItems, err := store.Items.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(source) error = %v", err)
	}
	sourceGoldID := ""
	for _, item := range sourceItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			sourceGoldID = item.ID
			break
		}
	}
	if sourceGoldID == "" {
		t.Fatal("expected source inventory duskgold stack")
	}

	offerPayload, err := json.Marshal(map[string]any{
		"target_character_id": targetCharacter.ID,
		"item_instance_id":    sourceGoldID,
		"quantity":            1,
	})
	if err != nil {
		t.Fatalf("json.Marshal(offer) error = %v", err)
	}
	_ = server.processTradeCommand(context.Background(), &Session{ID: "sess_trade_decline_source", CharacterID: sourceCharacter.ID}, sourceRuntime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_trade_offer_decline",
		CommandSeq:      1,
		Type:            "offer_trade_item",
		Payload:         offerPayload,
	})
	if len(targetMessages) != 1 || targetMessages[0]["kind"] != tradeNoticeKind {
		t.Fatalf("expected incoming trade notice before decline, got %+v", targetMessages)
	}

	offerID, _ := targetMessages[0]["offer_id"].(string)
	declinePayload, err := json.Marshal(map[string]any{
		"trade_offer_id": offerID,
	})
	if err != nil {
		t.Fatalf("json.Marshal(decline) error = %v", err)
	}
	sourceMessages = nil
	targetMessages = nil
	declineOutbound := server.processTradeCommand(context.Background(), &Session{ID: "sess_trade_decline_target", CharacterID: targetCharacter.ID}, targetRuntime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_trade_decline_1",
		CommandSeq:      1,
		Type:            "decline_trade_offer",
		Payload:         declinePayload,
	})
	if len(declineOutbound) != 2 || declineOutbound[0]["kind"] != "ack" || declineOutbound[1]["kind"] != tradeNoticeKind {
		t.Fatalf("expected ack and declined trade notice, got %+v", declineOutbound)
	}
	if len(sourceMessages) != 1 || sourceMessages[0]["kind"] != tradeNoticeKind {
		t.Fatalf("expected source to receive declined trade notice, got %+v", sourceMessages)
	}

	sourceItems, err = store.Items.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(source) after decline error = %v", err)
	}
	targetItems, err := store.Items.ListByCharacterID(context.Background(), targetCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(target) after decline error = %v", err)
	}

	sourceGold := 0
	for _, item := range sourceItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			sourceGold += item.Quantity
		}
	}
	targetGold := 0
	for _, item := range targetItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			targetGold += item.Quantity
		}
	}
	if sourceGold != 12 || targetGold != 12 {
		t.Fatalf("expected declined trade to keep gold unchanged at 12 each, got source=%d target=%d", sourceGold, targetGold)
	}

	sourceLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID(source) error = %v", err)
	}
	targetLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), targetCharacter.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID(target) error = %v", err)
	}
	sourceOfferLog := findActionLogByType(sourceLogs, "player_trade_offer")
	if sourceOfferLog == nil {
		t.Fatalf("unexpected source decline trade audit log = %+v", sourceLogs)
	}
	targetDeclineLog := findActionLogByType(targetLogs, "player_trade_decline")
	if targetDeclineLog == nil {
		t.Fatalf("unexpected target decline trade audit log = %+v", targetLogs)
	}
}

package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEconomyAuditServiceFiltersEventsTransfersAndTrades(t *testing.T) {
	store := newMemoryStore()
	sourceCharacter := &Character{ID: "char_audit_source", AccountID: "acc_audit_source", Name: "Audit Source", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	targetCharacter := &Character{ID: "char_audit_target", AccountID: "acc_audit_target", Name: "Audit Target", BaseClass: "Mage", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), sourceCharacter, initialCharacterItemSeed(sourceCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(source) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), targetCharacter, initialCharacterItemSeed(targetCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(target) error = %v", err)
	}

	actionRepo, ok := store.ActionLogs.(memoryActionLogRepo)
	if !ok {
		t.Fatalf("expected memory action log repo, got %T", store.ActionLogs)
	}
	storageRepo, ok := store.StorageTransfers.(memoryStorageTransferRecordRepo)
	if !ok {
		t.Fatalf("expected memory storage transfer repo, got %T", store.StorageTransfers)
	}

	baseTime := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	actionRepo.backend.recordActionLog(context.Background(), sourceCharacter.ID, ActionLogRecord{
		ID:                 "action_vendor_buy",
		CharacterID:        sourceCharacter.ID,
		ActionType:         "vendor_buy",
		ReferenceID:        "merchant_spear_offer",
		CounterpartyEntity: "npc_merchant",
		ItemInstanceID:     "item_vendor_buy",
		TemplateID:         "ironwood_spear",
		Quantity:           1,
		CurrencyTemplateID: "duskgold",
		CurrencyAmount:     -8,
		CreatedAt:          baseTime,
	})
	actionRepo.backend.recordActionLog(context.Background(), sourceCharacter.ID, ActionLogRecord{
		ID:                 "action_trade_offer",
		CharacterID:        sourceCharacter.ID,
		ActionType:         "player_trade_offer",
		ReferenceID:        "trade_ref_1",
		CounterpartyEntity: targetCharacter.ID,
		ItemInstanceID:     "item_trade_offer",
		TemplateID:         "duskgold",
		Quantity:           1,
		CreatedAt:          baseTime.Add(2 * time.Minute),
	})
	actionRepo.backend.recordActionLog(context.Background(), targetCharacter.ID, ActionLogRecord{
		ID:                 "action_trade_receive",
		CharacterID:        targetCharacter.ID,
		ActionType:         "player_trade_receive",
		ReferenceID:        "trade_ref_1",
		CounterpartyEntity: sourceCharacter.ID,
		ItemInstanceID:     "item_trade_receive",
		TemplateID:         "duskgold",
		Quantity:           1,
		CreatedAt:          baseTime.Add(3 * time.Minute),
	})
	storageRepo.backend.recordStorageTransfer(context.Background(), sourceCharacter.ID, StorageTransferRecord{
		ID:                 "transfer_warehouse_1",
		CharacterID:        sourceCharacter.ID,
		SourceItemID:       "item_storage_gold",
		TemplateID:         "duskgold",
		Quantity:           2,
		FromContainerKind:  itemContainerInventory,
		ToContainerKind:    itemContainerWarehouse,
		TransferType:       "warehouse_deposit",
		CounterpartyEntity: warehouseNPCEntityID,
		CreatedAt:          baseTime.Add(time.Minute),
	})

	service := NewEconomyAuditService(store)

	characterEvents, err := service.ListEvents(context.Background(), ActionLogQuery{
		CharacterID: sourceCharacter.ID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListEvents(character) error = %v", err)
	}
	if len(characterEvents) != 2 || characterEvents[0].ActionType != "player_trade_offer" || characterEvents[1].ActionType != "vendor_buy" {
		t.Fatalf("unexpected character-scoped events = %+v", characterEvents)
	}

	itemEvents, err := service.ListEvents(context.Background(), ActionLogQuery{
		ItemInstanceID: "item_vendor_buy",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListEvents(item) error = %v", err)
	}
	if len(itemEvents) != 1 || itemEvents[0].ActionType != "vendor_buy" {
		t.Fatalf("unexpected item-scoped events = %+v", itemEvents)
	}

	from := baseTime.Add(90 * time.Second)
	to := baseTime.Add(150 * time.Second)
	timeWindowEvents, err := service.ListEvents(context.Background(), ActionLogQuery{
		OccurredAfter:  &from,
		OccurredBefore: &to,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListEvents(time-range) error = %v", err)
	}
	if len(timeWindowEvents) != 1 || timeWindowEvents[0].ActionType != "player_trade_offer" {
		t.Fatalf("unexpected time-scoped events = %+v", timeWindowEvents)
	}

	transfers, err := service.ListWarehouseTransfers(context.Background(), StorageTransferQuery{
		CharacterID: sourceCharacter.ID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListWarehouseTransfers() error = %v", err)
	}
	if len(transfers) != 1 || transfers[0].TransferType != "warehouse_deposit" {
		t.Fatalf("unexpected warehouse transfers = %+v", transfers)
	}

	trades, err := service.ListTrades(context.Background(), sourceCharacter.ID, 10, 0, nil, nil)
	if err != nil {
		t.Fatalf("ListTrades() error = %v", err)
	}
	if len(trades) != 2 {
		t.Fatalf("expected two trade events involving source character, got %+v", trades)
	}
	if !isTradeActionType(trades[0].ActionType) || !isTradeActionType(trades[1].ActionType) {
		t.Fatalf("expected only trade action types, got %+v", trades)
	}
}

func TestInternalEconomyAuditEndpointRequiresTokenAndFiltersResults(t *testing.T) {
	store := newMemoryStore()
	character := &Character{ID: "char_internal_audit", AccountID: "acc_internal_audit", Name: "Internal Audit Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	offer, exists := vendorOfferByID("merchant_spear_offer")
	if !exists {
		t.Fatal("expected merchant_spear_offer in setup")
	}
	auditCtx := withCommandAuditMetadata(context.Background(), commandAuditMetadata{
		SessionID:  "sess_internal_audit",
		CommandID:  "cmd_internal_buy",
		CommandSeq: 7,
	})
	if _, err := store.Items.BuyVendorOffer(auditCtx, character.ID, offer, 1); err != nil {
		t.Fatalf("Items.BuyVendorOffer() error = %v", err)
	}

	server := NewServerWithConfig(":0", "", store, ServerConfig{
		InternalAuditEnabled: true,
		InternalAuditToken:   "audit-secret",
	})
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	unauthorizedResponse, err := http.Get(httpServer.URL + "/internal/economy/events?action_type=vendor_buy")
	if err != nil {
		t.Fatalf("http.Get(unauthorized) error = %v", err)
	}
	if unauthorizedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized internal audit response, got %d", unauthorizedResponse.StatusCode)
	}
	_ = unauthorizedResponse.Body.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/internal/economy/events?action_type=vendor_buy", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("X-Internal-Audit-Token", "audit-secret")

	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected authorized internal audit response, got %d", response.StatusCode)
	}

	var payload struct {
		Events []ActionLogRecord `json:"events"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("json.Decode() error = %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected one filtered event, got %+v", payload.Events)
	}
	if payload.Events[0].ActionType != "vendor_buy" ||
		payload.Events[0].AccountID != character.AccountID ||
		payload.Events[0].SessionID != "sess_internal_audit" ||
		payload.Events[0].CommandID != "cmd_internal_buy" ||
		payload.Events[0].CommandSeq != 7 {
		t.Fatalf("unexpected internal audit event payload = %+v", payload.Events[0])
	}
}

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type persistenceTestEnv struct {
	store      *Store
	server     *Server
	httpServer *httptest.Server
}

func newPersistenceTestEnv(t *testing.T) *persistenceTestEnv {
	t.Helper()

	databaseURL := os.Getenv("L2BG_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("L2BG_TEST_DATABASE_URL not configured")
	}

	store, err := NewStore(databaseURL)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	sessionRepo, ok := store.GameplaySessions.(postgresGameplaySessionRepo)
	if !ok {
		t.Fatalf("expected postgres gameplay session repo, got %T", store.GameplaySessions)
	}
	if err := sessionRepo.backend.truncateAllTables(context.Background()); err != nil {
		t.Fatalf("truncateAllTables() error = %v", err)
	}

	server := NewServer(":0", "ws://example.test/v1/gameplay/ws", store)
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	t.Cleanup(httpServer.Close)

	return &persistenceTestEnv{
		store:      store,
		server:     server,
		httpServer: httpServer,
	}
}

func postJSON(t *testing.T, client *http.Client, url string, payload any, bearer string) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	return response
}

func decodeBody[T any](t *testing.T, response *http.Response) T {
	t.Helper()
	defer response.Body.Close()

	var payload T
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("json.Decode() error = %v", err)
	}
	return payload
}

func registerAndLogin(t *testing.T, env *persistenceTestEnv, login string) (string, string) {
	t.Helper()

	registerResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/auth/register", map[string]any{
		"login":        login,
		"password":     "hunter123",
		"display_name": "Tester",
	}, "")
	if registerResponse.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d", registerResponse.StatusCode)
	}
	registerPayload := decodeBody[map[string]any](t, registerResponse)

	loginResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/auth/login", map[string]any{
		"login":    login,
		"password": "hunter123",
	}, "")
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}
	loginPayload := decodeBody[map[string]any](t, loginResponse)

	return registerPayload["account_id"].(string), loginPayload["access_token"].(string)
}

func loginAgainstServer(t *testing.T, client *http.Client, baseURL string, login string, password string) string {
	t.Helper()

	loginResponse := postJSON(t, client, baseURL+"/v1/auth/login", map[string]any{
		"login":    login,
		"password": password,
	}, "")
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}
	loginPayload := decodeBody[map[string]any](t, loginResponse)
	accessToken, ok := loginPayload["access_token"].(string)
	if !ok || accessToken == "" {
		t.Fatalf("missing access_token in login payload: %+v", loginPayload)
	}
	return accessToken
}

func TestRegisterPersistsAccountAndCredential(t *testing.T) {
	env := newPersistenceTestEnv(t)

	response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/auth/register", map[string]any{
		"login":        "persist.register@test",
		"password":     "hunter123",
		"display_name": "Persist Register",
	}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d", response.StatusCode)
	}
	payload := decodeBody[map[string]any](t, response)
	accountID := payload["account_id"].(string)

	account, err := env.store.Accounts.GetByID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("Accounts.GetByID() error = %v", err)
	}
	credential, err := env.store.Credentials.GetByAccountID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("Credentials.GetByAccountID() error = %v", err)
	}

	if account.Login != "persist.register@test" {
		t.Fatalf("unexpected persisted login = %s", account.Login)
	}
	if credential.PasswordHash == "" || credential.PasswordAlgorithm != passwordAlgorithmBcryptV1 {
		t.Fatalf("unexpected persisted credential = %+v", credential)
	}
}

func TestLoginReadsPersistedAccount(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, accessToken := registerAndLogin(t, env, "persist.login@test")

	if accessToken == "" {
		t.Fatal("expected access token")
	}
	account, err := env.store.Accounts.GetByID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("Accounts.GetByID() error = %v", err)
	}
	if account.ID != accountID {
		t.Fatalf("unexpected account id = %s", account.ID)
	}
	accountSession, err := env.store.AccountSessions.GetActiveByToken(context.Background(), accessToken, time.Now())
	if err != nil {
		t.Fatalf("AccountSessions.GetActiveByToken() error = %v", err)
	}
	if accountSession.AccountID != accountID {
		t.Fatalf("unexpected account session = %+v", accountSession)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	restartedServer := NewServer(":0", "ws://example.test/v1/gameplay/ws", restartedStore)
	restartedHTTPServer := httptest.NewServer(restartedServer.withCORS(restartedServer.mux))
	defer restartedHTTPServer.Close()

	request, err := http.NewRequest(http.MethodGet, restartedHTTPServer.URL+"/v1/characters", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := restartedHTTPServer.Client().Do(request)
	if err != nil {
		t.Fatalf("restarted client.Do() error = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected persisted access session to survive restart, got %d", response.StatusCode)
	}
}

func TestCreateCharacterPersistsCharacter(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, accessToken := registerAndLogin(t, env, "persist.character@test")

	response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Persist Hero",
	}, accessToken)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", response.StatusCode)
	}

	characters, err := env.store.Characters.ListByAccountID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("Characters.ListByAccountID() error = %v", err)
	}
	if len(characters) != 1 || characters[0].Name != "Persist Hero" {
		t.Fatalf("unexpected persisted characters = %+v", characters)
	}
	if characters[0].HairStyle != 1 || characters[0].SkinType != 2 {
		t.Fatalf("unexpected persisted appearance = %+v", characters[0])
	}
	if characters[0].HairColor != "#6b4e37" {
		t.Fatalf("unexpected persisted hair color = %+v", characters[0])
	}
}

func TestWorldEnterPersistsGameplaySession(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.session@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Session Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	payload := decodeBody[map[string]any](t, characterResponse)
	character := payload["character"].(map[string]any)

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": character["character_id"],
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	sessionID := worldEnterPayload["session_id"].(string)

	session, err := env.store.GameplaySessions.GetByID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	if session.CharacterID != character["character_id"] || session.Status != sessionStatusPendingAttach {
		t.Fatalf("unexpected persisted session = %+v", session)
	}
}

func TestCreateCharacterInitializesPersistedWorldState(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, accessToken := registerAndLogin(t, env, "persist.worldstate-init@test")

	response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World State Init Hero",
	}, accessToken)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", response.StatusCode)
	}

	characters, err := env.store.Characters.ListByAccountID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("Characters.ListByAccountID() error = %v", err)
	}
	if len(characters) != 1 {
		t.Fatalf("expected one character, got %+v", characters)
	}
	if characters[0].LastRegionID != startingRegionID || characters[0].PositionX != startingPositionX || characters[0].PositionZ != startingPositionZ {
		t.Fatalf("unexpected initial persisted world state = %+v", characters[0])
	}
}

func TestCreateCharacterSeedsPersistedInventoryAndEquipment(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.items-seed@test")

	response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Items Seed Hero",
	}, accessToken)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", response.StatusCode)
	}
	payload := decodeBody[map[string]any](t, response)
	character := payload["character"].(map[string]any)

	items, err := env.store.Items.ListByCharacterID(context.Background(), character["character_id"].(string))
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("expected 6 seeded items, got %+v", items)
	}

	var hasCurrency bool
	var hasPotion bool
	var hasChest bool
	var hasGloves bool
	var hasBoots bool
	var hasWeapon bool
	for _, item := range items {
		switch {
		case item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory && item.Quantity == 12:
			hasCurrency = true
		case item.TemplateID == "healing_potion" && item.ContainerKind == itemContainerInventory && item.Quantity == 3:
			hasPotion = true
		case item.TemplateID == "wardkeeper_mantle" && item.ContainerKind == itemContainerEquipment && item.EquipSlot == equipSlotChest:
			hasChest = true
		case item.TemplateID == "watcher_gloves" && item.ContainerKind == itemContainerInventory && item.Quantity == 1:
			hasGloves = true
		case item.TemplateID == "pathrunner_boots" && item.ContainerKind == itemContainerInventory && item.Quantity == 1:
			hasBoots = true
		case item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerEquipment && item.EquipSlot == equipSlotWeapon:
			hasWeapon = true
		}
	}
	if !hasCurrency || !hasPotion || !hasChest || !hasGloves || !hasBoots || !hasWeapon {
		t.Fatalf("unexpected seeded item state = %+v", items)
	}
}

func TestEconomyAuditPersistsVendorAndWarehouseHistory(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.economy-audit@test")

	response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Economy Audit Hero",
	}, accessToken)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", response.StatusCode)
	}
	payload := decodeBody[map[string]any](t, response)
	characterID := payload["character"].(map[string]any)["character_id"].(string)

	offer, exists := vendorOfferByID("merchant_spear_offer")
	if !exists {
		t.Fatal("expected merchant_spear_offer in setup")
	}

	items, err := env.store.Items.BuyVendorOffer(context.Background(), characterID, offer, 1)
	if err != nil {
		t.Fatalf("Items.BuyVendorOffer() error = %v", err)
	}

	purchasedItemID := ""
	for _, item := range items {
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory {
			purchasedItemID = item.ID
			break
		}
	}
	if purchasedItemID == "" {
		t.Fatalf("expected purchased inventory spear after vendor buy, got %+v", items)
	}

	items, err = env.store.Items.SellVendorItem(context.Background(), characterID, purchasedItemID, 1)
	if err != nil {
		t.Fatalf("Items.SellVendorItem() error = %v", err)
	}

	duskgoldInventoryID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldInventoryID = item.ID
			break
		}
	}
	if duskgoldInventoryID == "" {
		t.Fatalf("expected inventory duskgold stack after vendor sell, got %+v", items)
	}

	items, err = env.store.Items.DepositWarehouseItem(context.Background(), characterID, duskgoldInventoryID, 2)
	if err != nil {
		t.Fatalf("Items.DepositWarehouseItem() error = %v", err)
	}

	warehouseItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerWarehouse {
			warehouseItemID = item.ID
			break
		}
	}
	if warehouseItemID == "" {
		t.Fatalf("expected warehouse duskgold stack after deposit, got %+v", items)
	}

	if _, err := env.store.Items.WithdrawWarehouseItem(context.Background(), characterID, warehouseItemID, 1); err != nil {
		t.Fatalf("Items.WithdrawWarehouseItem() error = %v", err)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	actionLogs, err := restartedStore.ActionLogs.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID() error = %v", err)
	}
	if len(actionLogs) != 4 {
		t.Fatalf("expected four persisted action logs, got %+v", actionLogs)
	}

	var buyLog *ActionLogRecord
	var sellLog *ActionLogRecord
	var depositLog *ActionLogRecord
	var withdrawLog *ActionLogRecord
	for index := range actionLogs {
		switch actionLogs[index].ActionType {
		case "vendor_buy":
			buyLog = &actionLogs[index]
		case "vendor_sell":
			sellLog = &actionLogs[index]
		case "warehouse_deposit":
			depositLog = &actionLogs[index]
		case "warehouse_withdraw":
			withdrawLog = &actionLogs[index]
		}
	}
	if buyLog == nil ||
		buyLog.ReferenceID != "merchant_spear_offer" ||
		buyLog.CounterpartyEntity != "npc_merchant" ||
		buyLog.TemplateID != "ironwood_spear" ||
		buyLog.Quantity != 1 ||
		buyLog.CurrencyTemplateID != "duskgold" ||
		buyLog.CurrencyAmount != -8 ||
		buyLog.CurrencyBalanceBefore != 12 ||
		buyLog.CurrencyBalanceAfter != 4 {
		t.Fatalf("unexpected persisted vendor buy action log = %+v", buyLog)
	}
	if sellLog == nil ||
		sellLog.CounterpartyEntity != "npc_merchant" ||
		sellLog.ItemInstanceID != purchasedItemID ||
		sellLog.TemplateID != "ironwood_spear" ||
		sellLog.Quantity != 1 ||
		sellLog.CurrencyTemplateID != "duskgold" ||
		sellLog.CurrencyAmount != 4 {
		t.Fatalf("unexpected persisted vendor sell action log = %+v", sellLog)
	}
	if depositLog == nil ||
		depositLog.ItemInstanceID != duskgoldInventoryID ||
		depositLog.TemplateID != "duskgold" ||
		depositLog.Quantity != 2 ||
		depositLog.FromContainerKind != itemContainerInventory ||
		depositLog.ToContainerKind != itemContainerWarehouse {
		t.Fatalf("unexpected persisted warehouse deposit action log = %+v", depositLog)
	}
	if withdrawLog == nil ||
		withdrawLog.ItemInstanceID != warehouseItemID ||
		withdrawLog.TemplateID != "duskgold" ||
		withdrawLog.Quantity != 1 ||
		withdrawLog.FromContainerKind != itemContainerWarehouse ||
		withdrawLog.ToContainerKind != itemContainerInventory {
		t.Fatalf("unexpected persisted warehouse withdraw action log = %+v", withdrawLog)
	}

	storageTransfers, err := restartedStore.StorageTransfers.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("StorageTransfers.ListByCharacterID() error = %v", err)
	}
	if len(storageTransfers) != 2 {
		t.Fatalf("expected two persisted storage transfer records, got %+v", storageTransfers)
	}

	var depositRecord *StorageTransferRecord
	var withdrawRecord *StorageTransferRecord
	for index := range storageTransfers {
		switch storageTransfers[index].TransferType {
		case "warehouse_deposit":
			depositRecord = &storageTransfers[index]
		case "warehouse_withdraw":
			withdrawRecord = &storageTransfers[index]
		}
	}
	if depositRecord == nil ||
		depositRecord.SourceItemID != duskgoldInventoryID ||
		depositRecord.TemplateID != "duskgold" ||
		depositRecord.Quantity != 2 ||
		depositRecord.FromContainerKind != itemContainerInventory ||
		depositRecord.ToContainerKind != itemContainerWarehouse ||
		depositRecord.CounterpartyEntity != warehouseNPCEntityID {
		t.Fatalf("unexpected persisted warehouse deposit record = %+v", depositRecord)
	}
	if withdrawRecord == nil ||
		withdrawRecord.SourceItemID != warehouseItemID ||
		withdrawRecord.TemplateID != "duskgold" ||
		withdrawRecord.Quantity != 1 ||
		withdrawRecord.FromContainerKind != itemContainerWarehouse ||
		withdrawRecord.ToContainerKind != itemContainerInventory ||
		withdrawRecord.CounterpartyEntity != warehouseNPCEntityID {
		t.Fatalf("unexpected persisted warehouse withdraw record = %+v", withdrawRecord)
	}
}

func TestExchangeOfferPersistsInventoryAndActionLog(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.exchange-offer@test")

	response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Exchange Offer Hero",
	}, accessToken)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", response.StatusCode)
	}
	payload := decodeBody[map[string]any](t, response)
	characterID := payload["character"].(map[string]any)["character_id"].(string)

	offer, exists := exchangeOfferByID("merchant_mantle_exchange")
	if !exists {
		t.Fatal("expected merchant_mantle_exchange in setup")
	}

	items, err := env.store.Items.ExchangeOffer(context.Background(), characterID, offer, 1)
	if err != nil {
		t.Fatalf("Items.ExchangeOffer() error = %v", err)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	persistedItems, err := restartedStore.Items.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	duskgoldQuantity := 0
	inventoryMantles := 0
	for _, item := range persistedItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldQuantity += item.Quantity
		}
		if item.TemplateID == "wardkeeper_mantle" && item.ContainerKind == itemContainerInventory {
			inventoryMantles++
		}
	}
	if duskgoldQuantity != 2 || inventoryMantles != 1 {
		t.Fatalf("unexpected persisted exchange inventory = %+v (from immediate result %+v)", persistedItems, items)
	}

	actionLogs, err := restartedStore.ActionLogs.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID() error = %v", err)
	}
	if len(actionLogs) != 1 {
		t.Fatalf("expected one persisted exchange action log, got %+v", actionLogs)
	}
	if actionLogs[0].ActionType != "vendor_exchange" ||
		actionLogs[0].ReferenceID != "merchant_mantle_exchange" ||
		actionLogs[0].CounterpartyEntity != "npc_merchant" ||
		actionLogs[0].TemplateID != "wardkeeper_mantle" ||
		actionLogs[0].Quantity != 1 ||
		actionLogs[0].CurrencyTemplateID != "duskgold" ||
		actionLogs[0].CurrencyAmount != -10 ||
		actionLogs[0].CurrencyBalanceBefore != 12 ||
		actionLogs[0].CurrencyBalanceAfter != 2 {
		t.Fatalf("unexpected persisted exchange action log = %+v", actionLogs[0])
	}
}

func TestEconomyAuditFilterQueriesReadPersistedVendorWarehouseAndTradeHistory(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.economy-filters@test")

	createCharacter := func(name string) string {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
			"race":       "Human",
			"base_class": "Fighter",
			"sex":        "Male",
			"hair_style": 1,
			"hair_color": "#6b4e37",
			"skin_type":  2,
			"name":       name,
		}, accessToken)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create character status = %d", response.StatusCode)
		}
		payload := decodeBody[map[string]any](t, response)
		return payload["character"].(map[string]any)["character_id"].(string)
	}

	sourceCharacterID := createCharacter("Economy Filter Source")
	targetCharacterID := createCharacter("Economy Filter Target")

	buyCtx := withCommandAuditMetadata(context.Background(), commandAuditMetadata{
		SessionID:  "sess_filter_buy",
		CommandID:  "cmd_filter_buy",
		CommandSeq: 11,
	})
	offer, exists := vendorOfferByID("merchant_spear_offer")
	if !exists {
		t.Fatal("expected merchant_spear_offer in setup")
	}
	items, err := env.store.Items.BuyVendorOffer(buyCtx, sourceCharacterID, offer, 1)
	if err != nil {
		t.Fatalf("Items.BuyVendorOffer() error = %v", err)
	}
	purchasedItemID := ""
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory {
			purchasedItemID = item.ID
		}
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldItemID = item.ID
		}
	}
	if purchasedItemID == "" || duskgoldItemID == "" {
		t.Fatalf("expected purchased spear and remaining duskgold, got %+v", items)
	}

	storageCtx := withCommandAuditMetadata(context.Background(), commandAuditMetadata{
		SessionID:  "sess_filter_storage",
		CommandID:  "cmd_filter_storage",
		CommandSeq: 12,
	})
	items, err = env.store.Items.DepositWarehouseItem(storageCtx, sourceCharacterID, duskgoldItemID, 2)
	if err != nil {
		t.Fatalf("Items.DepositWarehouseItem() error = %v", err)
	}

	tradeCtx := withCommandAuditMetadata(context.Background(), commandAuditMetadata{
		SessionID:  "sess_filter_trade",
		CommandID:  "cmd_filter_trade",
		CommandSeq: 13,
	})
	if _, _, err := env.store.Items.TradeInventoryItem(tradeCtx, sourceCharacterID, targetCharacterID, duskgoldItemID, 1, "trade_filter_ref"); err != nil {
		t.Fatalf("Items.TradeInventoryItem() error = %v", err)
	}

	itemEvents, err := env.store.ActionLogs.ListByFilter(context.Background(), ActionLogQuery{
		ItemInstanceID: purchasedItemID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ActionLogs.ListByFilter(item) error = %v", err)
	}
	if len(itemEvents) != 1 || itemEvents[0].ActionType != "vendor_buy" || itemEvents[0].CommandID != "cmd_filter_buy" {
		t.Fatalf("unexpected item-filtered audit events = %+v", itemEvents)
	}

	warehouseEvents, err := env.store.ActionLogs.ListByFilter(context.Background(), ActionLogQuery{
		CharacterID: sourceCharacterID,
		ActionType:  "warehouse_deposit",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ActionLogs.ListByFilter(action_type) error = %v", err)
	}
	if len(warehouseEvents) != 1 || warehouseEvents[0].FromContainerKind != itemContainerInventory || warehouseEvents[0].ToContainerKind != itemContainerWarehouse {
		t.Fatalf("unexpected warehouse-filtered audit events = %+v", warehouseEvents)
	}

	tradeEvents, err := env.store.ActionLogs.ListByFilter(context.Background(), ActionLogQuery{
		InvolvedCharacterID: sourceCharacterID,
		ActionTypes:         tradeActionTypes,
		Limit:               10,
	})
	if err != nil {
		t.Fatalf("ActionLogs.ListByFilter(trades) error = %v", err)
	}
	if len(tradeEvents) < 3 {
		t.Fatalf("expected at least three trade-related events involving source character, got %+v", tradeEvents)
	}

	transfers, err := env.store.StorageTransfers.ListByFilter(context.Background(), StorageTransferQuery{
		CharacterID: sourceCharacterID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("StorageTransfers.ListByFilter() error = %v", err)
	}
	if len(transfers) != 1 || transfers[0].TransferType != "warehouse_deposit" || transfers[0].CommandID != "cmd_filter_storage" {
		t.Fatalf("unexpected persisted storage transfer filter result = %+v", transfers)
	}
}

func TestPlayerTradePersistsInventoryAndActionLogs(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, _ := registerAndLogin(t, env, "persist.player-trade@test")

	sourceCharacter := &Character{
		ID:           "char_trade_persist_source",
		AccountID:    accountID,
		Name:         "Trade Persist Source",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
		IsEnterable:  true,
	}
	targetCharacter := &Character{
		ID:           "char_trade_persist_target",
		AccountID:    accountID,
		Name:         "Trade Persist Target",
		Race:         "Elf",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
		IsEnterable:  true,
	}
	if err := env.store.CreateCharacterWithItemSeed(context.Background(), sourceCharacter, initialCharacterItemSeed(sourceCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(source) error = %v", err)
	}
	if err := env.store.CreateCharacterWithItemSeed(context.Background(), targetCharacter, initialCharacterItemSeed(targetCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(target) error = %v", err)
	}

	sourceItems, err := env.store.Items.ListByCharacterID(context.Background(), sourceCharacter.ID)
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
		t.Fatal("expected source inventory duskgold stack during setup")
	}

	immediateSourceItems, immediateTargetItems, err := env.store.Items.TradeInventoryItem(
		context.Background(),
		sourceCharacter.ID,
		targetCharacter.ID,
		sourceGoldID,
		2,
		"trade_persist_ref",
	)
	if err != nil {
		t.Fatalf("Items.TradeInventoryItem() error = %v", err)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	persistedSourceItems, err := restartedStore.Items.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(source) restart error = %v", err)
	}
	persistedTargetItems, err := restartedStore.Items.ListByCharacterID(context.Background(), targetCharacter.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID(target) restart error = %v", err)
	}

	sourceGold := 0
	for _, item := range persistedSourceItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			sourceGold += item.Quantity
		}
	}
	targetGold := 0
	for _, item := range persistedTargetItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			targetGold += item.Quantity
		}
	}
	if sourceGold != 10 || targetGold != 14 {
		t.Fatalf("unexpected persisted trade inventory source=%+v target=%+v immediateSource=%+v immediateTarget=%+v", persistedSourceItems, persistedTargetItems, immediateSourceItems, immediateTargetItems)
	}

	sourceLogs, err := restartedStore.ActionLogs.ListByCharacterID(context.Background(), sourceCharacter.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID(source) error = %v", err)
	}
	targetLogs, err := restartedStore.ActionLogs.ListByCharacterID(context.Background(), targetCharacter.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID(target) error = %v", err)
	}
	if len(sourceLogs) != 1 || sourceLogs[0].ActionType != "player_trade_send" || sourceLogs[0].ReferenceID != "trade_persist_ref" || sourceLogs[0].CounterpartyEntity != targetCharacter.ID || sourceLogs[0].TemplateID != "duskgold" || sourceLogs[0].Quantity != 2 {
		t.Fatalf("unexpected persisted source trade action log = %+v", sourceLogs)
	}
	if len(targetLogs) != 2 {
		t.Fatalf("unexpected persisted target trade action log = %+v", targetLogs)
	}
	if targetLogs[0].ActionType != "player_trade_accept" || targetLogs[0].ReferenceID != "trade_persist_ref" || targetLogs[0].CounterpartyEntity != sourceCharacter.ID || targetLogs[0].TemplateID != "duskgold" || targetLogs[0].Quantity != 2 {
		t.Fatalf("unexpected persisted target trade accept log = %+v", targetLogs[0])
	}
	if targetLogs[1].ActionType != "player_trade_receive" || targetLogs[1].ReferenceID != "trade_persist_ref" || targetLogs[1].CounterpartyEntity != sourceCharacter.ID || targetLogs[1].TemplateID != "duskgold" || targetLogs[1].Quantity != 2 {
		t.Fatalf("unexpected persisted target trade receive log = %+v", targetLogs[1])
	}
}

func TestWorldEnterUsesPersistedCharacterWorldState(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, accessToken := registerAndLogin(t, env, "persist.worldstate-enter@test")

	character := &Character{
		ID:           "char_worldstate_enter",
		AccountID:    accountID,
		Name:         "World State Enter Hero",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "west_field",
		PositionX:    21,
		PositionZ:    -3,
		IsEnterable:  true,
	}
	if err := env.store.Characters.Create(context.Background(), character); err != nil {
		t.Fatalf("Characters.Create() error = %v", err)
	}

	response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": character.ID,
	}, accessToken)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", response.StatusCode)
	}
	payload := decodeBody[map[string]any](t, response)

	session, loadedCharacter, err := env.server.attachSession(payload["session_id"].(string), payload["attach_token"].(string))
	if err != nil {
		t.Fatalf("attachSession() error = %v", err)
	}
	runtime := newAttachedRuntime(session.ID, loadedCharacter)
	regionContext := runtime.regionContextMessage()
	selfPosition := regionContext["self_position"].(runtimePoint)

	if regionContext["region_id"] != "west_field" {
		t.Fatalf("expected persisted region_id west_field, got %+v", regionContext["region_id"])
	}
	if selfPosition.X != 21 || selfPosition.Z != -3 {
		t.Fatalf("expected persisted self_position (21,-3), got %+v", selfPosition)
	}
}

func TestWorldEnterReturnsPersistedInventoryAndEquipment(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-items@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World Enter Items Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	character := characterPayload["character"].(map[string]any)

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": character["character_id"],
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	itemState, ok := worldEnterPayload["item_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected item_state in world enter payload, got %+v", worldEnterPayload["item_state"])
	}

	inventory, ok := itemState["inventory"].([]any)
	if !ok || len(inventory) != 4 {
		t.Fatalf("expected four inventory items, got %+v", itemState["inventory"])
	}
	equipment, ok := itemState["equipment"].([]any)
	if !ok || len(equipment) != 2 {
		t.Fatalf("expected two equipped items, got %+v", itemState["equipment"])
	}
}

func TestWorldEnterReturnsLootPersistedFromOnlinePickup(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-loot@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World Enter Loot Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)

	loadedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	runtime := newAttachedRuntime("sess_loot_pickup", loadedCharacter)
	runtime.position = runtimePoint{X: -8, Z: 0}
	runtime.knownEntities["loot_1"] = runtimeEntity{
		EntityID:   "loot_1",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtime.position,
		State:      map[string]any{"quantity": 4},
	}
	outbound := runtime.processLootPickup(context.Background(), env.store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pickup",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_1"}`),
	})
	if len(outbound) != 3 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack, delta and entity_disappear from pickup, got %+v", outbound)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	itemState, ok := worldEnterPayload["item_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected item_state in world enter payload, got %+v", worldEnterPayload["item_state"])
	}
	inventory, ok := itemState["inventory"].([]any)
	if !ok {
		t.Fatalf("expected inventory array, got %+v", itemState["inventory"])
	}
	duskgoldQuantity := 0
	for _, entry := range inventory {
		item, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected inventory item object, got %+v", entry)
		}
		if item["template_id"] == "duskgold" {
			duskgoldQuantity += int(item["quantity"].(float64))
		}
	}
	if duskgoldQuantity != 16 {
		t.Fatalf("expected world enter to reflect persisted duskgold quantity 16, got %d", duskgoldQuantity)
	}
}

func TestWorldEnterReturnsPartyLootPersistedFromEligiblePickup(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-party-loot@test")

	createCharacter := func(name string, sex string) string {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
			"race":       "Human",
			"base_class": "Fighter",
			"sex":        sex,
			"hair_style": 1,
			"hair_color": "#6b4e37",
			"skin_type":  2,
			"name":       name,
		}, accessToken)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create character %s status = %d", name, response.StatusCode)
		}
		payload := decodeBody[map[string]any](t, response)
		return payload["character"].(map[string]any)["character_id"].(string)
	}

	leaderID := createCharacter("Party Loot Leader", "Male")
	memberID := createCharacter("Party Loot Member", "Female")

	loadedMember, err := env.store.Characters.GetByID(context.Background(), memberID)
	if err != nil {
		t.Fatalf("Characters.GetByID(member) error = %v", err)
	}
	runtime := newAttachedRuntime("sess_party_loot_pickup", loadedMember)
	runtime.position = runtimePoint{X: -8, Z: 0}
	runtime.loadPartyState(&CharacterPartySnapshot{
		PartyID:           "persist_party_loot_1",
		LeaderCharacterID: leaderID,
		Members: []CharacterPartyMemberSnapshot{
			{CharacterID: leaderID, Name: "Party Loot Leader", IsLeader: true, Online: true},
			{CharacterID: memberID, Name: "Party Loot Member", Online: true},
		},
	}, nil)
	runtime.knownEntities["loot_party_1"] = runtimeEntity{
		EntityID:   "loot_party_1",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtime.position,
		State: map[string]any{
			"quantity":               4,
			"party_id":               "persist_party_loot_1",
			"eligible_character_ids": []string{leaderID, memberID},
		},
	}
	outbound := runtime.processLootPickup(context.Background(), env.store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pickup_party",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_party_1"}`),
	})
	if len(outbound) != 3 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack, delta and entity_disappear from party pickup, got %+v", outbound)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": memberID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	itemState, ok := worldEnterPayload["item_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected item_state in world enter payload, got %+v", worldEnterPayload["item_state"])
	}
	inventory, ok := itemState["inventory"].([]any)
	if !ok {
		t.Fatalf("expected inventory array, got %+v", itemState["inventory"])
	}
	duskgoldQuantity := 0
	for _, entry := range inventory {
		item, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected inventory item object, got %+v", entry)
		}
		if item["template_id"] == "duskgold" {
			duskgoldQuantity += int(item["quantity"].(float64))
		}
	}
	if duskgoldQuantity != 16 {
		t.Fatalf("expected world enter to reflect persisted party duskgold quantity 16, got %d", duskgoldQuantity)
	}
}

func TestWorldEnterReflectsOnlyWinningLootPickupAfterContention(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-loot-contention@test")

	createCharacter := func(name string, sex string) string {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
			"race":       "Human",
			"base_class": "Fighter",
			"sex":        sex,
			"hair_style": 1,
			"hair_color": "#6b4e37",
			"skin_type":  2,
			"name":       name,
		}, accessToken)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create character status = %d", response.StatusCode)
		}
		payload := decodeBody[map[string]any](t, response)
		return payload["character"].(map[string]any)["character_id"].(string)
	}

	winnerCharacterID := createCharacter("Loot Winner Hero", "Female")
	loserCharacterID := createCharacter("Loot Loser Hero", "Male")

	winnerCharacter, err := env.store.Characters.GetByID(context.Background(), winnerCharacterID)
	if err != nil {
		t.Fatalf("Characters.GetByID(winner) error = %v", err)
	}
	loserCharacter, err := env.store.Characters.GetByID(context.Background(), loserCharacterID)
	if err != nil {
		t.Fatalf("Characters.GetByID(loser) error = %v", err)
	}

	winnerRuntime := newAttachedRuntime("sess_loot_winner", winnerCharacter)
	loserRuntime := newAttachedRuntime("sess_loot_loser", loserCharacter)
	sharedLoot := runtimeEntity{
		EntityID:   "loot_contended",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtimePoint{X: -8, Z: 0},
		State:      map[string]any{"quantity": 4},
	}
	winnerRuntime.position = sharedLoot.Position
	loserRuntime.position = sharedLoot.Position
	winnerRuntime.knownEntities[sharedLoot.EntityID] = sharedLoot
	loserRuntime.knownEntities[sharedLoot.EntityID] = sharedLoot

	winnerOutbound := winnerRuntime.processLootPickup(context.Background(), env.store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pickup_1",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_contended"}`),
	})
	if len(winnerOutbound) != 3 || winnerOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected winner pickup to succeed, got %+v", winnerOutbound)
	}

	loserOutbound := loserRuntime.processLootPickup(context.Background(), env.store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pickup_1",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_contended"}`),
	})
	if len(loserOutbound) != 2 || loserOutbound[1]["reason_code"] != "loot.already_collected" {
		t.Fatalf("expected loser pickup to reject with contention, got %+v", loserOutbound)
	}

	checkDuskgold := func(characterID string) int {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
			"character_id": characterID,
		}, accessToken)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("world enter status = %d", response.StatusCode)
		}
		payload := decodeBody[map[string]any](t, response)
		itemState := payload["item_state"].(map[string]any)
		inventory := itemState["inventory"].([]any)
		total := 0
		for _, entry := range inventory {
			item := entry.(map[string]any)
			if item["template_id"] == "duskgold" {
				total += int(item["quantity"].(float64))
			}
		}
		return total
	}

	if winnerDuskgold := checkDuskgold(winnerCharacterID); winnerDuskgold != 16 {
		t.Fatalf("expected winning character world enter to show duskgold 16, got %d", winnerDuskgold)
	}
	if loserDuskgold := checkDuskgold(loserCharacterID); loserDuskgold != 12 {
		t.Fatalf("expected losing character world enter to remain at duskgold 12, got %d", loserDuskgold)
	}
}

func TestWorldEnterReflectsPersistedEquipmentAfterOnlineEquipAndUnequip(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-equip@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World Enter Equip Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)

	if _, err := env.store.Items.UnequipItem(context.Background(), characterID, equipSlotWeapon); err != nil {
		t.Fatalf("Items.UnequipItem() setup error = %v", err)
	}
	items, err := env.store.Items.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() setup error = %v", err)
	}
	weaponItemID := ""
	for _, item := range items {
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory {
			weaponItemID = item.ID
			break
		}
	}
	if weaponItemID == "" {
		t.Fatalf("expected unequipped spear in inventory during setup")
	}

	loadedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	runtime := newAttachedRuntime("sess_equip_worldstate", loadedCharacter)

	equipOutbound := runtime.processItemCommand(context.Background(), env.store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_equip",
		CommandSeq:      1,
		Type:            "equip_item",
		Payload:         []byte(`{"item_instance_id":"` + weaponItemID + `"}`),
	})
	if len(equipOutbound) != 2 || equipOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected equip_item to succeed, got %+v", equipOutbound)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter after equip status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	itemState := worldEnterPayload["item_state"].(map[string]any)
	selfState := worldEnterPayload["self_state"].(map[string]any)
	selfStats := selfState["stats"].(map[string]any)
	equipment := itemState["equipment"].([]any)
	foundWeaponEquipped := false
	for _, entry := range equipment {
		item := entry.(map[string]any)
		if item["item_instance_id"] == weaponItemID && item["equip_slot"] == "weapon" {
			foundWeaponEquipped = true
		}
	}
	if !foundWeaponEquipped {
		t.Fatalf("expected world enter to reflect equipped spear, got %+v", equipment)
	}
	if selfStats["attack"] != float64(27) || selfStats["defense"] != float64(18) {
		t.Fatalf("expected world enter to reflect equipped derived stats, got %+v", selfStats)
	}

	unequipOutbound := runtime.processItemCommand(context.Background(), env.store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_unequip",
		CommandSeq:      2,
		Type:            "unequip_item",
		Payload:         []byte(`{"equip_slot":"weapon"}`),
	})
	if len(unequipOutbound) != 2 || unequipOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected unequip_item to succeed, got %+v", unequipOutbound)
	}

	worldEnterResponse = postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter after unequip status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload = decodeBody[map[string]any](t, worldEnterResponse)
	itemState = worldEnterPayload["item_state"].(map[string]any)
	selfState = worldEnterPayload["self_state"].(map[string]any)
	selfStats = selfState["stats"].(map[string]any)
	inventory := itemState["inventory"].([]any)
	foundWeaponInInventory := false
	for _, entry := range inventory {
		item := entry.(map[string]any)
		if item["item_instance_id"] == weaponItemID && item["template_id"] == "ironwood_spear" {
			foundWeaponInInventory = true
		}
	}
	if !foundWeaponInInventory {
		t.Fatalf("expected world enter to reflect unequipped spear in inventory, got %+v", inventory)
	}
	if selfStats["attack"] != float64(17) || selfStats["defense"] != float64(18) {
		t.Fatalf("expected world enter to reflect unequipped derived stats, got %+v", selfStats)
	}
}

func TestWorldEnterReflectsPersistedChestStatsUsedByMitigation(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-defense@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World Enter Defense Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter with chest status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	selfState := worldEnterPayload["self_state"].(map[string]any)
	selfStats := selfState["stats"].(map[string]any)
	if selfState["hp"] != float64(122) {
		t.Fatalf("expected world enter to expose authoritative hp 122, got %+v", selfState["hp"])
	}
	if selfStats["defense"] != float64(18) || selfStats["max_hp"] != float64(150) {
		t.Fatalf("expected world enter to reflect chest mitigation stats, got %+v", selfStats)
	}

	loadedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	runtime := newAttachedRuntime("sess_defense_worldstate", loadedCharacter)
	unequipOutbound := runtime.processItemCommand(context.Background(), env.store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_unequip_chest",
		CommandSeq:      1,
		Type:            "unequip_item",
		Payload:         []byte(`{"equip_slot":"chest"}`),
	})
	if len(unequipOutbound) != 2 || unequipOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected chest unequip to succeed, got %+v", unequipOutbound)
	}

	worldEnterResponse = postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter without chest status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload = decodeBody[map[string]any](t, worldEnterResponse)
	selfState = worldEnterPayload["self_state"].(map[string]any)
	selfStats = selfState["stats"].(map[string]any)
	if selfState["hp"] != float64(122) {
		t.Fatalf("expected world enter to preserve authoritative hp 122 after chest unequip, got %+v", selfState["hp"])
	}
	if selfStats["defense"] != float64(12) || selfStats["max_hp"] != float64(130) {
		t.Fatalf("expected world enter to reflect unequipped chest mitigation stats, got %+v", selfStats)
	}
}

func TestWorldEnterReflectsPersistedProgressionAfterAuthoritativeCombat(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-progression@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World Progress Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)
	if err := env.store.Characters.UpdateProgression(context.Background(), characterID, 2, 0, 92, 122, 58); err != nil {
		t.Fatalf("Characters.UpdateProgression() setup error = %v", err)
	}

	loadedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	if err := env.store.Characters.UpdateProgression(context.Background(), characterID, 1, 60, 80, 120, 58); err != nil {
		t.Fatalf("Characters.UpdateProgression() setup error = %v", err)
	}
	loadedCharacter, err = env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() after setup error = %v", err)
	}
	items, err := env.store.Items.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	session := &Session{
		ID:              "sess_progression_world_enter",
		AccountID:       loadedCharacter.AccountID,
		CharacterID:     loadedCharacter.ID,
		AttachToken:     "attach_progression_world_enter",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	runtime := newAttachedRuntime(session.ID, loadedCharacter)
	runtime.derivedStats = deriveCharacterStats(loadedCharacter, items)
	runtime.reconcileResourcePools()
	moveRuntimeNearMob(runtime, "mob_1")
	entity := runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	runtime.knownEntities["mob_1"] = entity

	outbound, _ := env.server.processGameplayCommandWithDedup(context.Background(), session, runtime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_progression_world_enter",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(outbound) != 3 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected authoritative combat to emit ack, delta and loot appear, got %+v", outbound)
	}

	persistedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() persisted error = %v", err)
	}
	if persistedCharacter.Level != 2 || persistedCharacter.XP != 12 || persistedCharacter.CurrentHP != 168 || persistedCharacter.CurrentMP != 65 {
		t.Fatalf("expected persisted progression level=2 xp=12 hp=168 mp=65, got %+v", persistedCharacter)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	selfState := worldEnterPayload["self_state"].(map[string]any)
	selfStats := selfState["stats"].(map[string]any)
	if selfState["level"] != float64(2) || selfState["xp"] != float64(12) {
		t.Fatalf("expected world enter to reflect persisted level/xp, got %+v", selfState)
	}
	if selfState["hp"] != float64(168) || selfState["mp"] != float64(65) {
		t.Fatalf("expected world enter to reflect persisted hp/mp, got %+v", selfState)
	}
	if selfStats["max_hp"] != float64(168) || selfStats["max_mp"] != float64(65) || selfStats["attack"] != float64(31) || selfStats["defense"] != float64(20) {
		t.Fatalf("expected world enter to reflect leveled stats, got %+v", selfStats)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	restartedCharacter, err := restartedStore.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("restarted Characters.GetByID() error = %v", err)
	}
	if restartedCharacter.Level != 2 || restartedCharacter.XP != 12 || restartedCharacter.CurrentHP != 168 || restartedCharacter.CurrentMP != 65 {
		t.Fatalf("expected restarted store to preserve progression, got %+v", restartedCharacter)
	}
}

func TestWorldEnterReflectsPersistedHotbarStateAfterOnlineReentry(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-hotbar@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World Enter Hotbar Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)

	loadedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	items, err := env.store.Items.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	itemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" {
			itemID = item.ID
			break
		}
	}
	if itemID == "" {
		t.Fatalf("expected seeded duskgold item, got %+v", items)
	}

	session := &Session{
		ID:              "sess_hotbar_world_enter",
		AccountID:       loadedCharacter.AccountID,
		CharacterID:     loadedCharacter.ID,
		AttachToken:     "attach_hotbar_world_enter",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusAttached,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	runtime := newAttachedRuntime(session.ID, loadedCharacter)
	outbound, _ := env.server.processGameplayCommandWithDedup(context.Background(), session, runtime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_hotbar_world_enter",
		CommandSeq:      1,
		Type:            "set_hotbar_state",
		Payload: hotbarSnapshotPayload(t, 2,
			CharacterHotbarSlot{SlotIndex: 0, EntryType: "action", ActionID: "basic_attack"},
			CharacterHotbarSlot{SlotIndex: 1, EntryType: "item", ItemInstanceID: itemID},
			CharacterHotbarSlot{SlotIndex: 2, EntryType: "skill", SkillID: "crescent_strike"},
		),
	})
	if len(outbound) != 2 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected hotbar command to emit ack and delta, got %+v", outbound)
	}
	if err := env.store.GameplaySessions.UpdateStatus(context.Background(), session.ID, sessionStatusClosed); err != nil {
		t.Fatalf("GameplaySessions.UpdateStatus() close error = %v", err)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	selfState := worldEnterPayload["self_state"].(map[string]any)
	hotbar := selfState["hotbar"].(map[string]any)
	if hotbar["open_bar_count"] != float64(2) {
		t.Fatalf("expected world enter hotbar open_bar_count 2, got %+v", hotbar)
	}
	slots, ok := hotbar["slots"].([]any)
	if !ok || len(slots) < 3 {
		t.Fatalf("expected world enter hotbar slots, got %+v", hotbar["slots"])
	}
	slot0 := slots[0].(map[string]any)
	slot1 := slots[1].(map[string]any)
	slot2 := slots[2].(map[string]any)
	if slot0["entry_type"] != "action" || slot0["action_id"] != "basic_attack" {
		t.Fatalf("expected slot 0 action binding, got %+v", slot0)
	}
	if slot1["entry_type"] != "item" || slot1["item_instance_id"] != itemID {
		t.Fatalf("expected slot 1 item binding, got %+v", slot1)
	}
	if slot2["entry_type"] != "skill" || slot2["skill_id"] != "crescent_strike" {
		t.Fatalf("expected slot 2 skill binding, got %+v", slot2)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()
	restartedHotbar, err := restartedStore.CharacterHotbars.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("restarted CharacterHotbars.ListByCharacterID() error = %v", err)
	}
	if restartedHotbar.OpenBarCount != 2 || restartedHotbar.Slots[0].ActionID != "basic_attack" || restartedHotbar.Slots[1].ItemInstanceID != itemID {
		t.Fatalf("expected restarted store to preserve hotbar state, got %+v", restartedHotbar)
	}
}

func TestWorldEnterAndReattachReflectPersistedCooldownsAfterAuthoritativeSkillUse(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.worldstate-cooldowns@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "World Cooldown Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)
	if err := env.store.Characters.UpdateProgression(context.Background(), characterID, 2, 0, 92, 122, 58); err != nil {
		t.Fatalf("Characters.UpdateProgression() setup error = %v", err)
	}

	loadedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	items, err := env.store.Items.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	session := &Session{
		ID:              "sess_cooldowns_world_enter",
		AccountID:       loadedCharacter.AccountID,
		CharacterID:     loadedCharacter.ID,
		AttachToken:     "attach_cooldowns_world_enter",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusAttached,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	runtime := newAttachedRuntime(session.ID, loadedCharacter)
	runtime.derivedStats = deriveCharacterStats(loadedCharacter, items)
	runtime.reconcileResourcePools()
	moveRuntimeNearMob(runtime, "mob_1")

	outbound, _ := env.server.processGameplayCommandWithDedup(context.Background(), session, runtime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_cooldowns_world_enter",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"grave_bloom","target_id":"mob_1"}`),
	})
	if len(outbound) < 2 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected authoritative skill use to emit ack and delta, got %+v", outbound)
	}

	persistedCooldowns, err := env.store.CharacterCooldowns.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("CharacterCooldowns.ListByCharacterID() error = %v", err)
	}
	if len(persistedCooldowns) != 1 || persistedCooldowns[0].SkillID != "grave_bloom" || !persistedCooldowns[0].EndsAt.After(time.Now()) {
		t.Fatalf("expected persisted future grave_bloom cooldown, got %+v", persistedCooldowns)
	}

	if err := env.store.GameplaySessions.UpdateStatus(context.Background(), session.ID, sessionStatusClosed); err != nil {
		t.Fatalf("GameplaySessions.UpdateStatus() close error = %v", err)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	selfState := worldEnterPayload["self_state"].(map[string]any)
	cooldowns, ok := selfState["cooldowns"].(map[string]any)
	if !ok {
		t.Fatalf("expected world enter self_state cooldowns map, got %+v", selfState["cooldowns"])
	}
	remaining, ok := cooldowns["grave_bloom"].(float64)
	maxExpectedRemaining := float64(supportedSkills["grave_bloom"].CooldownMS + 1500)
	if !ok || remaining <= 0 || remaining > maxExpectedRemaining {
		t.Fatalf("expected positive grave_bloom cooldown <= %.0fms, got %+v", maxExpectedRemaining, cooldowns["grave_bloom"])
	}

	reentrySessionID := worldEnterPayload["session_id"].(string)
	reentryAttachToken := worldEnterPayload["attach_token"].(string)

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	restartedServer := NewServer(":0", "ws://example.test/v1/gameplay/ws", restartedStore)
	restartedSession, restartedCharacter, err := restartedServer.attachSession(reentrySessionID, reentryAttachToken)
	if err != nil {
		t.Fatalf("attachSession() after restart error = %v", err)
	}
	reattachedRuntime, err := restartedServer.buildAttachedRuntime(context.Background(), restartedSession, restartedCharacter, time.Now())
	if err != nil {
		t.Fatalf("buildAttachedRuntime() after restart error = %v", err)
	}
	reattachedRuntime.seedKnownEntity(testRuntimeMob(
		"mob_1",
		"mireling",
		mobPersonalityPassive,
		runtimePoint{X: -108, Z: 0},
		54,
	))
	moveRuntimeNearMob(reattachedRuntime, "mob_1")
	reattachedRuntime.expectedCommandSeq = 2

	reentryOutbound, _ := restartedServer.processGameplayCommandWithDedup(context.Background(), restartedSession, reattachedRuntime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_cooldowns_reentry",
		CommandSeq:      2,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"grave_bloom","target_id":"mob_1"}`),
	})
	if len(reentryOutbound) != 2 || reentryOutbound[0]["kind"] != "ack" || reentryOutbound[1]["reason_code"] != "combat.cooldown_active" {
		t.Fatalf("expected reattached runtime to reject persisted grave_bloom cooldown, got %+v", reentryOutbound)
	}
}

func TestDisconnectDuringDeathPersistsExplicitRespawnCheckpoint(t *testing.T) {
	env := newPersistenceTestEnv(t)
	login := "persist.worldstate-death-checkpoint@test"
	password := "hunter123"
	_, accessToken := registerAndLogin(t, env, login)

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Death Checkpoint Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)

	if err := env.store.Characters.UpdateProgression(context.Background(), characterID, 1, 0, 80, 1, 58); err != nil {
		t.Fatalf("Characters.UpdateProgression() setup error = %v", err)
	}
	loadedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	items, err := env.store.Items.ListByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	session := &Session{
		ID:              "sess_death_checkpoint",
		AccountID:       loadedCharacter.AccountID,
		CharacterID:     loadedCharacter.ID,
		AttachToken:     "attach_death_checkpoint",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusAttached,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	runtime := newAttachedRuntime(session.ID, loadedCharacter)
	runtime.derivedStats = deriveCharacterStats(loadedCharacter, items)
	runtime.reconcileResourcePools()
	moveRuntimeNearMob(runtime, "mob_1")

	outbound, _ := env.server.processGameplayCommandWithDedup(context.Background(), session, runtime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_death_checkpoint",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(outbound) != 2 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected lethal skill use to emit ack and delta, got %+v", outbound)
	}
	if self, ok := outbound[1]["self"].(map[string]any); !ok || self["dead"] != true || self["hp"] != 0 {
		t.Fatalf("expected live runtime to remain dead before respawn tick, got %+v", outbound[1]["self"])
	}

	env.server.persistCharacterWorldState(characterID, runtime)
	env.server.persistCharacterProgression(characterID, runtime)
	env.server.persistCharacterCooldownState(characterID, runtime)
	if err := env.store.GameplaySessions.UpdateStatus(context.Background(), session.ID, sessionStatusClosed); err != nil {
		t.Fatalf("GameplaySessions.UpdateStatus() close error = %v", err)
	}

	persistedCharacter, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() persisted error = %v", err)
	}
	if persistedCharacter.PositionX != -8 || persistedCharacter.PositionZ != 0 {
		t.Fatalf("expected persisted respawn position (-8,0), got %+v", persistedCharacter)
	}
	if persistedCharacter.CurrentHP != 150 || persistedCharacter.CurrentMP != 58 {
		t.Fatalf("expected persisted respawn resources hp=150 mp=58, got %+v", persistedCharacter)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()

	restartedServer := NewServer(":0", "ws://example.test/v1/gameplay/ws", restartedStore)
	restartedHTTPServer := httptest.NewServer(restartedServer.withCORS(restartedServer.mux))
	defer restartedHTTPServer.Close()
	restartedToken := loginAgainstServer(t, restartedHTTPServer.Client(), restartedHTTPServer.URL, login, password)

	worldEnterResponse := postJSON(t, restartedHTTPServer.Client(), restartedHTTPServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, restartedToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	selfState := worldEnterPayload["self_state"].(map[string]any)
	if selfState["dead"] != false || selfState["hp"] != float64(150) || selfState["mp"] != float64(58) {
		t.Fatalf("expected world enter to expose explicit respawn checkpoint resources, got %+v", selfState)
	}

	reattachSessionID := worldEnterPayload["session_id"].(string)
	reattachToken := worldEnterPayload["attach_token"].(string)
	reattachSession, reattachCharacter, err := restartedServer.attachSession(reattachSessionID, reattachToken)
	if err != nil {
		t.Fatalf("attachSession() error = %v", err)
	}
	reattachedRuntime, err := restartedServer.buildAttachedRuntime(context.Background(), reattachSession, reattachCharacter, time.Now())
	if err != nil {
		t.Fatalf("buildAttachedRuntime() error = %v", err)
	}
	regionContext := reattachedRuntime.regionContextMessage()
	selfPosition := regionContext["self_position"].(runtimePoint)
	if selfPosition.X != -8 || selfPosition.Z != 0 {
		t.Fatalf("expected attach-time runtime to restore respawn checkpoint position, got %+v", selfPosition)
	}
}

func TestPersistCharacterWorldStateAtExplicitBoundary(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, _ := registerAndLogin(t, env, "persist.worldstate-save@test")

	character := &Character{
		ID:           "char_worldstate_save",
		AccountID:    accountID,
		Name:         "World State Save Hero",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
		IsEnterable:  true,
	}
	if err := env.store.Characters.Create(context.Background(), character); err != nil {
		t.Fatalf("Characters.Create() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_boundary_save", character)
	runtime.mu.Lock()
	runtime.position = runtimePoint{X: 12, Z: 6}
	runtime.clearActiveMovementLocked()
	runtime.mu.Unlock()

	env.server.persistCharacterWorldState(character.ID, runtime)

	persisted, err := env.store.Characters.GetByID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	if persisted.LastRegionID != "dawn_plaza" || persisted.PositionX != 12 || persisted.PositionZ != 6 {
		t.Fatalf("unexpected persisted boundary world state = %+v", persisted)
	}
}

func TestExpiredPendingAttachDoesNotBlockNewWorldEnter(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, accessToken := registerAndLogin(t, env, "persist.expire@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Expire Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	payload := decodeBody[map[string]any](t, characterResponse)
	character := payload["character"].(map[string]any)
	characterID := character["character_id"].(string)

	expiredSession := &Session{
		ID:              "sess_expired_pending",
		AccountID:       accountID,
		CharacterID:     characterID,
		AttachToken:     "attach_expired_pending",
		AttachExpiresAt: time.Now().Add(-1 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), expiredSession); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}

	staleSession, err := env.store.GameplaySessions.GetByID(context.Background(), expiredSession.ID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	if staleSession.Status != sessionStatusExpired {
		t.Fatalf("expected stale session to be expired, got %+v", staleSession)
	}
}

func TestPendingAttachIsReusedInsteadOfBlockingImmediateRetry(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.reuse@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Retry Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	payload := decodeBody[map[string]any](t, characterResponse)
	character := payload["character"].(map[string]any)
	characterID := character["character_id"].(string)

	firstWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if firstWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("first world enter status = %d", firstWorldEnterResponse.StatusCode)
	}
	firstWorldEnterPayload := decodeBody[map[string]any](t, firstWorldEnterResponse)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(env.httpServer.URL, "http") + "/v1/gameplay/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://malicious.test"},
		},
	})
	if err == nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "expected handshake rejection")
		t.Fatal("expected websocket handshake to fail for unauthorized origin")
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}

	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	if worldEnterPayload["session_id"] != firstWorldEnterPayload["session_id"] {
		t.Fatalf("expected world enter to reuse pending session %v, got %v", firstWorldEnterPayload["session_id"], worldEnterPayload["session_id"])
	}
	if worldEnterPayload["attach_token"] != firstWorldEnterPayload["attach_token"] {
		t.Fatalf("expected world enter to reuse pending attach token %v, got %v", firstWorldEnterPayload["attach_token"], worldEnterPayload["attach_token"])
	}
}

func TestOwnedSessionIsReissuedByWorldEnterWithoutCompetingAuthority(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.blocked@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Blocked Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	payload := decodeBody[map[string]any](t, characterResponse)
	character := payload["character"].(map[string]any)
	characterID := character["character_id"].(string)

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("first world enter status = %d", worldEnterResponse.StatusCode)
	}
	firstPayload := decodeBody[map[string]any](t, worldEnterResponse)
	sessionID := firstPayload["session_id"].(string)
	ownedSession, _, err := env.server.attachSession(sessionID, firstPayload["attach_token"].(string))
	if err != nil {
		t.Fatalf("attachSession() error = %v", err)
	}
	defer env.server.closeAttachedSession(ownedSession.ID, ownedSession.FencingToken)

	secondResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if secondResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected active-session reissue, got %d", secondResponse.StatusCode)
	}
	secondPayload := decodeBody[map[string]any](t, secondResponse)
	if secondPayload["session_id"] != sessionID {
		t.Fatalf("world enter created competing authority: first=%+v second=%+v", firstPayload, secondPayload)
	}
	if secondPayload["attach_token"] == firstPayload["attach_token"] {
		t.Fatalf("world enter did not return the credential rotated by ownership acquisition: first=%+v second=%+v", firstPayload, secondPayload)
	}
}

func TestAttachExpiredUpdatesPersistedSessionStatus(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, _ := registerAndLogin(t, env, "persist.attach-expired@test")

	character := &Character{
		ID:           "char_attach_expired",
		AccountID:    accountID,
		Name:         "Attach Expired Hero",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		IsEnterable:  true,
	}
	if err := env.store.Characters.Create(context.Background(), character); err != nil {
		t.Fatalf("Characters.Create() error = %v", err)
	}

	session := &Session{
		ID:              "sess_attach_expired",
		AccountID:       accountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_expired_token",
		AttachExpiresAt: time.Now().Add(-1 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	_, _, err := env.server.attachSession(session.ID, session.AttachToken)
	if err == nil || err.Error() != "session.expired" {
		t.Fatalf("expected session.expired, got %v", err)
	}

	persistedSession, err := env.store.GameplaySessions.GetByID(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	if persistedSession.Status != sessionStatusExpired {
		t.Fatalf("expected expired status after attach rejection, got %+v", persistedSession)
	}
}

func TestStartupSanitizationClosesAttachedSessionAndReleasesWorldEnter(t *testing.T) {
	env := newPersistenceTestEnv(t)
	login := "persist.startup-attached@test"
	password := "hunter123"
	_, accessToken := registerAndLogin(t, env, login)

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Startup Attached Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	payload := decodeBody[map[string]any](t, characterResponse)
	character := payload["character"].(map[string]any)
	characterID := character["character_id"].(string)

	firstWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if firstWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("first world enter status = %d", firstWorldEnterResponse.StatusCode)
	}
	firstWorldEnterPayload := decodeBody[map[string]any](t, firstWorldEnterResponse)
	sessionID := firstWorldEnterPayload["session_id"].(string)

	if err := env.store.GameplaySessions.UpdateStatus(context.Background(), sessionID, sessionStatusAttached); err != nil {
		t.Fatalf("GameplaySessions.UpdateStatus() error = %v", err)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore() restart error = %v", err)
	}
	defer restartedStore.Close()

	if err := restartedStore.SanitizeGameplaySessionLifecycle(context.Background(), time.Now()); err != nil {
		t.Fatalf("SanitizeGameplaySessionLifecycle() error = %v", err)
	}

	persistedSession, err := restartedStore.GameplaySessions.GetByID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	if persistedSession.Status != sessionStatusClosed {
		t.Fatalf("expected attached session to become closed on startup sanitization, got %+v", persistedSession)
	}

	restartedServer := NewServer(":0", "ws://example.test/v1/gameplay/ws", restartedStore)
	restartedHTTPServer := httptest.NewServer(restartedServer.withCORS(restartedServer.mux))
	defer restartedHTTPServer.Close()
	restartedToken := loginAgainstServer(t, restartedHTTPServer.Client(), restartedHTTPServer.URL, login, password)

	secondWorldEnterResponse := postJSON(t, restartedHTTPServer.Client(), restartedHTTPServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, restartedToken)
	if secondWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected world enter to be released after startup sanitization, got %d", secondWorldEnterResponse.StatusCode)
	}
}

func TestStartupSanitizationExpiresPendingAttachBeforeWorldEnter(t *testing.T) {
	env := newPersistenceTestEnv(t)
	login := "persist.startup-pending@test"
	password := "hunter123"
	accountID, _ := registerAndLogin(t, env, login)

	character := &Character{
		ID:           "char_startup_pending",
		AccountID:    accountID,
		Name:         "Startup Pending Hero",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
		IsEnterable:  true,
	}
	if err := env.store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	session := &Session{
		ID:              "sess_startup_pending",
		AccountID:       accountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_startup_pending",
		AttachExpiresAt: time.Now().Add(-1 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	restartedStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore() restart error = %v", err)
	}
	defer restartedStore.Close()

	if err := restartedStore.SanitizeGameplaySessionLifecycle(context.Background(), time.Now()); err != nil {
		t.Fatalf("SanitizeGameplaySessionLifecycle() error = %v", err)
	}

	persistedSession, err := restartedStore.GameplaySessions.GetByID(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	if persistedSession.Status != sessionStatusExpired {
		t.Fatalf("expected pending_attach to become expired on startup sanitization, got %+v", persistedSession)
	}

	restartedServer := NewServer(":0", "ws://example.test/v1/gameplay/ws", restartedStore)
	restartedHTTPServer := httptest.NewServer(restartedServer.withCORS(restartedServer.mux))
	defer restartedHTTPServer.Close()
	restartedToken := loginAgainstServer(t, restartedHTTPServer.Client(), restartedHTTPServer.URL, login, password)

	worldEnterResponse := postJSON(t, restartedHTTPServer.Client(), restartedHTTPServer.URL+"/v1/world/enter", map[string]any{
		"character_id": character.ID,
	}, restartedToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected world enter after startup expiration, got %d", worldEnterResponse.StatusCode)
	}
}

func TestWorldEnterRehydratesPersistedMountedPetState(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.pet-mounted@test")

	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Female",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Pet Mounted Hero",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)

	now := time.Now().UTC()
	if err := env.store.CharacterPets.Create(context.Background(), &CharacterPet{
		ID:            "pet_persisted_mount",
		CharacterID:   characterID,
		PetTemplateID: "mireling_strider",
		IsSummoned:    true,
		IsMounted:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("CharacterPets.Create() error = %v", err)
	}

	worldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": characterID,
	}, accessToken)
	if worldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("world enter status = %d", worldEnterResponse.StatusCode)
	}
	worldEnterPayload := decodeBody[map[string]any](t, worldEnterResponse)
	selfState, ok := worldEnterPayload["self_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected self_state payload, got %+v", worldEnterPayload["self_state"])
	}
	stats, ok := selfState["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected self_state.stats payload, got %+v", selfState["stats"])
	}
	if moveSpeed, ok := stats["move_speed"].(float64); !ok || moveSpeed != 4.05 {
		t.Fatalf("expected mounted move_speed 4.05 in world enter, got %+v", stats["move_speed"])
	}
	pets, ok := selfState["pets"].([]any)
	if !ok || len(pets) != 1 {
		t.Fatalf("expected one persisted pet snapshot in world enter, got %+v", selfState["pets"])
	}
	firstPet, ok := pets[0].(map[string]any)
	if !ok {
		t.Fatalf("expected pet snapshot map, got %+v", pets[0])
	}
	if mounted, ok := firstPet["mounted"].(bool); !ok || !mounted {
		t.Fatalf("expected mounted pet snapshot, got %+v", firstPet)
	}

	sessionID, _ := worldEnterPayload["session_id"].(string)
	session, err := env.store.GameplaySessions.GetByID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	character, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	runtime, err := env.server.buildAttachedRuntime(context.Background(), session, character, time.Now())
	if err != nil {
		t.Fatalf("buildAttachedRuntime() error = %v", err)
	}
	if len(runtime.pets) != 1 || !runtime.pets[0].IsMounted || !runtime.pets[0].IsSummoned {
		t.Fatalf("expected runtime to rehydrate mounted summoned pet, got %+v", runtime.pets)
	}
	if runtime.derivedStats.MoveSpeed != 4.05 {
		t.Fatalf("expected runtime mounted move speed 4.05, got %v", runtime.derivedStats.MoveSpeed)
	}
	if petEntity, exists := runtime.activePetEntity(); !exists || petEntity == nil || petEntity.EntityType != petEntityType {
		t.Fatalf("expected active pet entity after runtime rehydrate, got exists=%v entity=%+v", exists, petEntity)
	}
}

func TestWorldEnterReflectsPersistedPartyStateAndInvites(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, leaderToken := registerAndLogin(t, env, "persist.party.leader@test")
	_, inviteeToken := registerAndLogin(t, env, "persist.party.invitee@test")

	createCharacter := func(token string, name string) string {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
			"race":       "Human",
			"base_class": "Fighter",
			"sex":        "Male",
			"hair_style": 1,
			"hair_color": "#6b4e37",
			"skin_type":  2,
			"name":       name,
		}, token)
		if response.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			t.Fatalf("create character %s status = %d body = %s", name, response.StatusCode, strings.TrimSpace(string(body)))
		}
		payload := decodeBody[map[string]any](t, response)
		return payload["character"].(map[string]any)["character_id"].(string)
	}

	leaderCharacterID := createCharacter(leaderToken, "Party Leader Persist")
	inviteeCharacterID := createCharacter(inviteeToken, "Party Invitee Persist")

	now := time.Now().UTC()
	party := &Party{
		ID:                "persist_party_state_1",
		LeaderCharacterID: leaderCharacterID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := env.store.Parties.Create(context.Background(), party, PartyMember{}); err != nil {
		t.Fatalf("Parties.Create() error = %v", err)
	}
	if err := env.store.Parties.CreateInvite(context.Background(), &PartyInvite{
		ID:                 "persist_party_invite_1",
		PartyID:            party.ID,
		InviterCharacterID: leaderCharacterID,
		InviteeCharacterID: inviteeCharacterID,
		ExpiresAt:          now.Add(time.Minute),
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("Parties.CreateInvite() error = %v", err)
	}

	leaderWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": leaderCharacterID,
	}, leaderToken)
	if leaderWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("leader world enter status = %d", leaderWorldEnterResponse.StatusCode)
	}
	leaderWorldEnterPayload := decodeBody[map[string]any](t, leaderWorldEnterResponse)
	leaderSelfState := leaderWorldEnterPayload["self_state"].(map[string]any)
	if partyValue, exists := leaderSelfState["party"]; exists && partyValue != nil {
		t.Fatalf("expected leader to remain outside a joined party until invite acceptance, got %+v", partyValue)
	}

	inviteeWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": inviteeCharacterID,
	}, inviteeToken)
	if inviteeWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("invitee world enter status = %d", inviteeWorldEnterResponse.StatusCode)
	}
	inviteeWorldEnterPayload := decodeBody[map[string]any](t, inviteeWorldEnterResponse)
	inviteeSelfState := inviteeWorldEnterPayload["self_state"].(map[string]any)
	if partyValue, exists := inviteeSelfState["party"]; exists && partyValue != nil {
		t.Fatalf("expected invitee to have no joined party yet, got %+v", partyValue)
	}
	invites, ok := inviteeSelfState["party_invites"].([]any)
	if !ok || len(invites) != 1 {
		t.Fatalf("expected one persisted party invite, got %+v", inviteeSelfState["party_invites"])
	}
	firstInvite, ok := invites[0].(map[string]any)
	if !ok {
		t.Fatalf("expected invite snapshot map, got %+v", invites[0])
	}
	if firstInvite["invite_id"] != "persist_party_invite_1" || firstInvite["inviter_character_id"] != leaderCharacterID {
		t.Fatalf("unexpected invite snapshot = %+v", firstInvite)
	}
}

func TestWorldEnterReflectsPersistedClanStateAndInvites(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, leaderToken := registerAndLogin(t, env, "persist.clan.leader@test")
	_, inviteeToken := registerAndLogin(t, env, "persist.clan.invitee@test")

	createCharacter := func(token string, name string) string {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
			"race":       "Human",
			"base_class": "Fighter",
			"sex":        "Male",
			"hair_style": 1,
			"hair_color": "#6b4e37",
			"skin_type":  2,
			"name":       name,
		}, token)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create character %s status = %d", name, response.StatusCode)
		}
		payload := decodeBody[map[string]any](t, response)
		return payload["character"].(map[string]any)["character_id"].(string)
	}

	leaderCharacterID := createCharacter(leaderToken, "Clan Leader Persist")
	inviteeCharacterID := createCharacter(inviteeToken, "Clan Invitee Persist")

	now := time.Now().UTC()
	clan := &Clan{
		ID:                "persist_clan_state_1",
		Name:              "Nightfall",
		LeaderCharacterID: leaderCharacterID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	leaderMember := ClanMember{
		ClanID:      clan.ID,
		CharacterID: leaderCharacterID,
		JoinedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := env.store.Clans.Create(context.Background(), clan, leaderMember); err != nil {
		t.Fatalf("Clans.Create() error = %v", err)
	}
	if err := env.store.Clans.CreateInvite(context.Background(), &ClanInvite{
		ID:                 "persist_clan_invite_1",
		ClanID:             clan.ID,
		InviterCharacterID: leaderCharacterID,
		InviteeCharacterID: inviteeCharacterID,
		ExpiresAt:          now.Add(time.Minute),
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("Clans.CreateInvite() error = %v", err)
	}

	leaderWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": leaderCharacterID,
	}, leaderToken)
	if leaderWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("leader world enter status = %d", leaderWorldEnterResponse.StatusCode)
	}
	leaderWorldEnterPayload := decodeBody[map[string]any](t, leaderWorldEnterResponse)
	leaderSelfState := leaderWorldEnterPayload["self_state"].(map[string]any)
	leaderClan, ok := leaderSelfState["clan"].(map[string]any)
	if !ok {
		t.Fatalf("expected leader clan snapshot, got %+v", leaderSelfState["clan"])
	}
	if leaderClan["clan_id"] != clan.ID || leaderClan["leader_character_id"] != leaderCharacterID || leaderClan["name"] != "Nightfall" {
		t.Fatalf("unexpected leader clan snapshot = %+v", leaderClan)
	}
	if leaderInvites, exists := leaderSelfState["clan_invites"]; exists {
		if inviteSlice, ok := leaderInvites.([]any); ok && len(inviteSlice) > 0 {
			t.Fatalf("expected leader to have no inbound clan invites, got %+v", inviteSlice)
		}
	}

	inviteeWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": inviteeCharacterID,
	}, inviteeToken)
	if inviteeWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("invitee world enter status = %d", inviteeWorldEnterResponse.StatusCode)
	}
	inviteeWorldEnterPayload := decodeBody[map[string]any](t, inviteeWorldEnterResponse)
	inviteeSelfState := inviteeWorldEnterPayload["self_state"].(map[string]any)
	if clanValue, exists := inviteeSelfState["clan"]; exists && clanValue != nil {
		t.Fatalf("expected invitee to have no joined clan yet, got %+v", clanValue)
	}
	invites, ok := inviteeSelfState["clan_invites"].([]any)
	if !ok || len(invites) != 1 {
		t.Fatalf("expected one persisted clan invite, got %+v", inviteeSelfState["clan_invites"])
	}
	firstInvite, ok := invites[0].(map[string]any)
	if !ok {
		t.Fatalf("expected invite snapshot map, got %+v", invites[0])
	}
	if firstInvite["invite_id"] != "persist_clan_invite_1" || firstInvite["clan_id"] != clan.ID || firstInvite["clan_name"] != "Nightfall" {
		t.Fatalf("unexpected clan invite snapshot = %+v", firstInvite)
	}
}

func TestWorldEnterReflectsPersistedAllianceStateAndInvites(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, leaderToken := registerAndLogin(t, env, "persist.alliance.leader@test")
	_, targetLeaderToken := registerAndLogin(t, env, "persist.alliance.target@test")

	createCharacter := func(token string, name string) string {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
			"race":       "Human",
			"base_class": "Fighter",
			"sex":        "Male",
			"hair_style": 1,
			"hair_color": "#6b4e37",
			"skin_type":  2,
			"name":       name,
		}, token)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create character %s status = %d", name, response.StatusCode)
		}
		payload := decodeBody[map[string]any](t, response)
		return payload["character"].(map[string]any)["character_id"].(string)
	}

	leaderCharacterID := createCharacter(leaderToken, "Allred")
	targetLeaderCharacterID := createCharacter(targetLeaderToken, "Alltar")
	now := time.Now().UTC()
	founderClan := &Clan{
		ID:                "persist_alliance_founder_clan",
		Name:              "Nightfall",
		LeaderCharacterID: leaderCharacterID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := env.store.Clans.Create(context.Background(), founderClan, ClanMember{
		ClanID:      founderClan.ID,
		CharacterID: leaderCharacterID,
		JoinedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Clans.Create(founder) error = %v", err)
	}
	targetClan := &Clan{
		ID:                "persist_alliance_target_clan",
		Name:              "Moonrise",
		LeaderCharacterID: targetLeaderCharacterID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := env.store.Clans.Create(context.Background(), targetClan, ClanMember{
		ClanID:      targetClan.ID,
		CharacterID: targetLeaderCharacterID,
		JoinedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Clans.Create(target) error = %v", err)
	}
	alliance := &Alliance{
		ID:           "persist_alliance_1",
		Name:         "Eclipse",
		LeaderClanID: founderClan.ID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := env.store.Alliances.Create(context.Background(), alliance, AllianceMember{
		AllianceID: alliance.ID,
		ClanID:     founderClan.ID,
		JoinedAt:   now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("Alliances.Create() error = %v", err)
	}
	if err := env.store.Alliances.CreateInvite(context.Background(), &AllianceInvite{
		ID:                 "persist_alliance_invite_1",
		AllianceID:         alliance.ID,
		InviterClanID:      founderClan.ID,
		InviterCharacterID: leaderCharacterID,
		TargetClanID:       targetClan.ID,
		InviteeCharacterID: targetLeaderCharacterID,
		ExpiresAt:          now.Add(time.Minute),
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("Alliances.CreateInvite() error = %v", err)
	}

	leaderWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": leaderCharacterID,
	}, leaderToken)
	if leaderWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("leader world enter status = %d", leaderWorldEnterResponse.StatusCode)
	}
	leaderWorldEnterPayload := decodeBody[map[string]any](t, leaderWorldEnterResponse)
	leaderSelfState := leaderWorldEnterPayload["self_state"].(map[string]any)
	leaderAlliance, ok := leaderSelfState["alliance"].(map[string]any)
	if !ok {
		t.Fatalf("expected leader alliance snapshot, got %+v", leaderSelfState["alliance"])
	}
	if leaderAlliance["alliance_id"] != alliance.ID || leaderAlliance["leader_clan_id"] != founderClan.ID || leaderAlliance["name"] != "Eclipse" {
		t.Fatalf("unexpected leader alliance snapshot = %+v", leaderAlliance)
	}

	targetWorldEnterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
		"character_id": targetLeaderCharacterID,
	}, targetLeaderToken)
	if targetWorldEnterResponse.StatusCode != http.StatusOK {
		t.Fatalf("target world enter status = %d", targetWorldEnterResponse.StatusCode)
	}
	targetWorldEnterPayload := decodeBody[map[string]any](t, targetWorldEnterResponse)
	targetSelfState := targetWorldEnterPayload["self_state"].(map[string]any)
	if allianceValue, exists := targetSelfState["alliance"]; exists && allianceValue != nil {
		t.Fatalf("expected target clan leader to have no joined alliance yet, got %+v", allianceValue)
	}
	invites, ok := targetSelfState["alliance_invites"].([]any)
	if !ok || len(invites) != 1 {
		t.Fatalf("expected one persisted alliance invite, got %+v", targetSelfState["alliance_invites"])
	}
	firstInvite, ok := invites[0].(map[string]any)
	if !ok {
		t.Fatalf("expected alliance invite snapshot map, got %+v", invites[0])
	}
	if firstInvite["invite_id"] != "persist_alliance_invite_1" || firstInvite["alliance_id"] != alliance.ID || firstInvite["alliance_name"] != "Eclipse" {
		t.Fatalf("unexpected alliance invite snapshot = %+v", firstInvite)
	}
}

package app

import (
	"context"
	"testing"
	"time"
)

func pvpStoreTestEvent(id string, actor *Character, target *Character) PvPCombatEvent {
	return PvPCombatEvent{
		ID:                  id,
		AttackerCharacterID: actor.ID,
		AttackerAccountID:   actor.AccountID,
		VictimCharacterID:   target.ID,
		VictimAccountID:     target.AccountID,
		ActionType:          "basic_attack",
		Damage:              10,
		CPDamage:            10,
		Result:              "hit",
		CreatedAt:           time.Now().UTC(),
	}
}

func TestMemoryCharacterRepositoryAppliesPvPCombatStateAtomically(t *testing.T) {
	store := newMemoryStore()
	actor := &Character{ID: "char_store_pvp_actor", AccountID: "acct_store_pvp_actor", Name: "StorePvpActor", BaseClass: "Fighter", LastRegionID: startingRegionID}
	target := &Character{ID: "char_store_pvp_target", AccountID: "acct_store_pvp_target", Name: "StorePvpTarget", BaseClass: "Mage", LastRegionID: startingRegionID}
	if err := store.Characters.Create(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if err := store.Characters.Create(context.Background(), target); err != nil {
		t.Fatal(err)
	}

	flagUntil := time.Now().Add(time.Minute).UTC()
	err := store.Characters.ApplyPvPCombatState(context.Background(), CharacterPvPCombatState{
		CharacterID:  actor.ID,
		CurrentCP:    80,
		CurrentHP:    122,
		CurrentMP:    52,
		PvPKills:     3,
		PKCount:      2,
		Karma:        200,
		PvPFlagUntil: flagUntil,
	}, CharacterPvPCombatState{
		CharacterID: target.ID,
		CurrentCP:   0,
		CurrentHP:   17,
		CurrentMP:   58,
		PvPKills:    1,
		PKCount:     0,
		Karma:       0,
	}, pvpStoreTestEvent("pvp_event_memory", actor, target))
	if err != nil {
		t.Fatal(err)
	}

	loadedActor, _ := store.Characters.GetByID(context.Background(), actor.ID)
	loadedTarget, _ := store.Characters.GetByID(context.Background(), target.ID)
	if loadedActor.CurrentMP != 52 || loadedActor.PvPKills != 3 || loadedActor.PKCount != 2 || loadedActor.Karma != 200 || !loadedActor.PvPFlagUntil.Equal(flagUntil) {
		t.Fatalf("unexpected actor state: %+v", loadedActor)
	}
	if loadedTarget.CurrentCP != 0 || loadedTarget.CurrentHP != 17 {
		t.Fatalf("unexpected target state: %+v", loadedTarget)
	}

	beforeActor := *loadedActor
	if err := store.Characters.ApplyPvPCombatState(context.Background(), CharacterPvPCombatState{CharacterID: actor.ID}, CharacterPvPCombatState{CharacterID: "missing"}, pvpStoreTestEvent("pvp_event_missing", actor, target)); err != errRecordNotFound {
		t.Fatalf("missing target error = %v", err)
	}
	afterActor, _ := store.Characters.GetByID(context.Background(), actor.ID)
	if *afterActor != beforeActor {
		t.Fatalf("failed atomic update changed actor: before=%+v after=%+v", beforeActor, afterActor)
	}
	events, err := store.PvPCombatEvents.ListByFilter(context.Background(), PvPCombatEventQuery{InvolvedCharacterID: actor.ID})
	if err != nil || len(events) != 1 || events[0].ID != "pvp_event_memory" {
		t.Fatalf("unexpected atomic PvP audit events: events=%+v err=%v", events, err)
	}
}

func TestPostgresCharacterRepositoryPersistsPvPCombatState(t *testing.T) {
	env := newPersistenceTestEnv(t)
	actor := &Character{ID: "char_pg_pvp_actor", AccountID: "acct_pg_pvp_actor", Name: "PgPvpActor", BaseClass: "Fighter", LastRegionID: startingRegionID}
	target := &Character{ID: "char_pg_pvp_target", AccountID: "acct_pg_pvp_target", Name: "PgPvpTarget", BaseClass: "Mage", LastRegionID: startingRegionID}

	if err := env.store.Accounts.Create(context.Background(), &Account{ID: actor.AccountID, Login: "pg-pvp-actor@test", DisplayName: "actor", State: accountStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.Accounts.Create(context.Background(), &Account{ID: target.AccountID, Login: "pg-pvp-target@test", DisplayName: "target", State: accountStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.Characters.Create(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if err := env.store.Characters.Create(context.Background(), target); err != nil {
		t.Fatal(err)
	}

	flagUntil := time.Now().Add(time.Minute).UTC().Truncate(time.Microsecond)
	if err := env.store.Characters.ApplyPvPCombatState(context.Background(), CharacterPvPCombatState{
		CharacterID:  actor.ID,
		CurrentCP:    70,
		CurrentHP:    120,
		CurrentMP:    51,
		PvPKills:     4,
		PKCount:      2,
		Karma:        200,
		PvPFlagUntil: flagUntil,
	}, CharacterPvPCombatState{
		CharacterID: target.ID,
		CurrentCP:   0,
		CurrentHP:   9,
		CurrentMP:   58,
	}, pvpStoreTestEvent("pvp_event_postgres", actor, target)); err != nil {
		t.Fatal(err)
	}

	loadedActor, err := env.store.Characters.GetByID(context.Background(), actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedTarget, err := env.store.Characters.GetByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedActor.PvPKills != 4 || loadedActor.PKCount != 2 || loadedActor.Karma != 200 || loadedActor.CurrentMP != 51 || !loadedActor.PvPFlagUntil.Equal(flagUntil) {
		t.Fatalf("unexpected persisted actor: %+v", loadedActor)
	}
	if loadedTarget.CurrentCP != 0 || loadedTarget.CurrentHP != 9 {
		t.Fatalf("unexpected persisted target: %+v", loadedTarget)
	}
	events, err := env.store.PvPCombatEvents.ListByFilter(context.Background(), PvPCombatEventQuery{AttackerCharacterID: actor.ID})
	if err != nil || len(events) != 1 || events[0].ID != "pvp_event_postgres" {
		t.Fatalf("unexpected persisted PvP audit: events=%+v err=%v", events, err)
	}
}

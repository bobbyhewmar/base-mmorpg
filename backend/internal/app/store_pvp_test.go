package app

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

func pvpStoreMutation(id string, actor *Character, target *Character, damage int, occurredAt time.Time) PvPCombatMutation {
	return PvPCombatMutation{
		EventID:             id,
		AttackerCharacterID: actor.ID,
		VictimCharacterID:   target.ID,
		ActionType:          "basic_attack",
		Damage:              damage,
		SessionID:           "session_" + id,
		CommandID:           "command_" + id,
		CommandSeq:          1,
		OccurredAt:          occurredAt,
	}
}

func TestMemoryCharacterRepositoryAppliesPvPCombatAtomically(t *testing.T) {
	store := newMemoryStore()
	actor := &Character{ID: "char_store_pvp_actor", AccountID: "acct_store_pvp_actor", Name: "StorePvpActor", BaseClass: "Fighter", LastRegionID: startingRegionID}
	target := &Character{ID: "char_store_pvp_target", AccountID: "acct_store_pvp_target", Name: "StorePvpTarget", BaseClass: "Mage", LastRegionID: startingRegionID}
	if err := store.Characters.Create(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if err := store.Characters.Create(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	initialTarget, err := store.Characters.GetByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	mutation := pvpStoreMutation("pvp_event_memory", actor, target, 10, now)
	mutation.MPCost = 6
	mutation.CooldownID = "basic_attack"
	mutation.CooldownDuration = time.Second
	commit, err := store.Characters.ApplyPvPCombat(context.Background(), mutation)
	if err != nil {
		t.Fatal(err)
	}
	if commit.Event.CPDamage != 10 || commit.Event.HPDamage != 0 || commit.Event.Result != "hit" {
		t.Fatalf("unexpected combat commit: %+v", commit)
	}

	loadedActor, _ := store.Characters.GetByID(context.Background(), actor.ID)
	loadedTarget, _ := store.Characters.GetByID(context.Background(), target.ID)
	if loadedActor.CurrentMP != 52 || !loadedActor.PvPFlagUntil.After(now) {
		t.Fatalf("unexpected actor state: %+v", loadedActor)
	}
	if loadedTarget.CurrentCP != initialTarget.CurrentCP-10 || loadedTarget.CurrentHP != initialTarget.CurrentHP {
		t.Fatalf("unexpected target state: %+v", loadedTarget)
	}
	cooldowns, err := store.CharacterCooldowns.ListByCharacterID(context.Background(), actor.ID)
	if err != nil || len(cooldowns) != 1 || cooldowns[0].SkillID != "basic_attack" || !cooldowns[0].EndsAt.Equal(commit.CooldownEndsAt) {
		t.Fatalf("unexpected atomic attacker cooldown: cooldowns=%+v err=%v", cooldowns, err)
	}

	beforeActor := *loadedActor
	missingMutation := pvpStoreMutation("pvp_event_missing", actor, &Character{ID: "missing"}, 10, now.Add(2*time.Second))
	if _, err := store.Characters.ApplyPvPCombat(context.Background(), missingMutation); err != errRecordNotFound {
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

func TestMemoryPvPCombatPersistsAssistsAndRepeatedKillSignal(t *testing.T) {
	store := newMemoryStore()
	now := time.Now().UTC()
	firstAttacker := &Character{ID: "char_assist_first", AccountID: "acct_assist_first", Name: "AssistFirst", BaseClass: "Fighter", LastRegionID: startingRegionID}
	killer := &Character{ID: "char_assist_killer", AccountID: "acct_assist_killer", Name: "AssistKiller", BaseClass: "Fighter", LastRegionID: startingRegionID}
	victim := &Character{ID: "char_assist_victim", AccountID: "acct_assist_victim", Name: "AssistVictim", BaseClass: "Mage", LastRegionID: startingRegionID}
	for _, character := range []*Character{firstAttacker, killer, victim} {
		if err := store.Characters.Create(context.Background(), character); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Characters.UpdateProgression(context.Background(), victim.ID, 1, 0, 0, 50, 58); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Characters.ApplyPvPCombat(context.Background(), pvpStoreMutation("assist_hit", firstAttacker, victim, 10, now)); err != nil {
		t.Fatal(err)
	}
	kill, err := store.Characters.ApplyPvPCombat(context.Background(), pvpStoreMutation("assist_kill", killer, victim, 100, now.Add(time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if kill.Event.KillerCharacterID != killer.ID || len(kill.Event.AssistCharacterIDs) != 1 || kill.Event.AssistCharacterIDs[0] != firstAttacker.ID {
		t.Fatalf("unexpected kill attribution: %+v", kill.Event)
	}
	if kill.Event.Suspicious || kill.Event.RepeatedKillCount != 1 {
		t.Fatalf("first pair kill should not be suspicious: %+v", kill.Event)
	}

	if err := store.Characters.UpdateProgression(context.Background(), victim.ID, 1, 0, 0, 1, 58); err != nil {
		t.Fatal(err)
	}
	repeated, err := store.Characters.ApplyPvPCombat(context.Background(), pvpStoreMutation("assist_repeated_kill", killer, victim, 10, now.Add(2*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.Event.Suspicious || repeated.Event.RepeatedKillCount != 2 || len(repeated.Event.AssistCharacterIDs) != 0 {
		t.Fatalf("repeated kill signal or death-boundary attribution is wrong: %+v", repeated.Event)
	}
	suspicious := true
	events, err := store.PvPCombatEvents.ListByFilter(context.Background(), PvPCombatEventQuery{
		KillerCharacterID: killer.ID,
		VictimCharacterID: victim.ID,
		Suspicious:        &suspicious,
	})
	if err != nil || len(events) != 1 || events[0].ID != repeated.Event.ID {
		t.Fatalf("unexpected suspicious kill query: events=%+v err=%v", events, err)
	}
}

func TestMemoryPvPCombatSerializesConcurrentDamageOnSameVictim(t *testing.T) {
	store := newMemoryStore()
	assertConcurrentPvPCombatSerialization(t, store, store)
}

func TestPostgresPvPCombatPersistsAndSerializesAcrossStoreInstances(t *testing.T) {
	env := newPersistenceTestEnv(t)
	secondStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	assertConcurrentPvPCombatSerialization(t, env.store, secondStore)
}

func TestPostgresPvPCombatPersistsRepeatedKillSignal(t *testing.T) {
	env := newPersistenceTestEnv(t)
	ctx := context.Background()
	attacker := &Character{ID: "char_pg_repeated_attacker", AccountID: "acct_pg_repeated_attacker", Name: "PgRepeatedAttacker", BaseClass: "Fighter", LastRegionID: startingRegionID}
	victim := &Character{ID: "char_pg_repeated_victim", AccountID: "acct_pg_repeated_victim", Name: "PgRepeatedVictim", BaseClass: "Mage", LastRegionID: startingRegionID}
	for _, character := range []*Character{attacker, victim} {
		if err := env.store.Accounts.Create(ctx, &Account{ID: character.AccountID, Login: character.AccountID + "@test", DisplayName: character.Name, State: accountStateActive}); err != nil {
			t.Fatal(err)
		}
		if err := env.store.Characters.Create(ctx, character); err != nil {
			t.Fatal(err)
		}
	}
	for index, eventID := range []string{"pg_repeated_first", "pg_repeated_second"} {
		if err := env.store.Characters.UpdateProgression(ctx, victim.ID, 1, 0, 0, 1, 58); err != nil {
			t.Fatal(err)
		}
		commit, err := env.store.Characters.ApplyPvPCombat(ctx, pvpStoreMutation(eventID, attacker, victim, 10, time.Now().Add(time.Duration(index)*time.Second)))
		if err != nil {
			t.Fatal(err)
		}
		if commit.Event.Suspicious != (index == 1) || commit.Event.RepeatedKillCount != index+1 {
			t.Fatalf("unexpected repeated kill commit %d: %+v", index, commit.Event)
		}
	}
	suspicious := true
	events, err := env.store.PvPCombatEvents.ListByFilter(ctx, PvPCombatEventQuery{KillerCharacterID: attacker.ID, VictimCharacterID: victim.ID, Suspicious: &suspicious})
	if err != nil || len(events) != 1 || events[0].ID != "pg_repeated_second" || events[0].RepeatedKillCount != 2 {
		t.Fatalf("unexpected persisted PostgreSQL repeated-kill signal: events=%+v err=%v", events, err)
	}
}

func TestMemoryKarmaRecoveryAppliesOnceAndPersistsRecoveryAudit(t *testing.T) {
	store := newMemoryStore()
	now := time.Now().UTC()
	character := &Character{
		ID:                 "char_karma_recovery_memory",
		AccountID:          "acct_karma_recovery_memory",
		Name:               "KarmaRecoveryMemory",
		BaseClass:          "Fighter",
		LastRegionID:       startingRegionID,
		Karma:              60,
		KarmaRecoveryDueAt: now.Add(-time.Second),
		KarmaHighSince:     now.Add(-20 * time.Minute),
	}
	if err := store.Characters.Create(context.Background(), character); err != nil {
		t.Fatal(err)
	}
	commit, err := store.Characters.ApplyKarmaRecovery(context.Background(), character.ID, now, "tick")
	if err != nil {
		t.Fatal(err)
	}
	if commit.Event == nil || commit.State.Karma != 40 || commit.Event.RecoveredAmount != karmaRecoveryAmount {
		t.Fatalf("unexpected recovery commit: %+v", commit)
	}
	replayed, err := store.Characters.ApplyKarmaRecovery(context.Background(), character.ID, now, "tick")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Event != nil || replayed.State.Karma != 40 {
		t.Fatalf("duplicate recovery should be a no-op: %+v", replayed)
	}
	events, err := store.PvPCombatEvents.ListKarmaRecoveryEvents(context.Background(), PvPKarmaRecoveryEventQuery{CharacterID: character.ID})
	if err != nil || len(events) != 1 || events[0].Trigger != "tick" {
		t.Fatalf("unexpected recovery audit events: events=%+v err=%v", events, err)
	}
}

func TestPostgresKarmaRecoveryPersistsAcrossReload(t *testing.T) {
	env := newPersistenceTestEnv(t)
	ctx := context.Background()
	account := &Account{ID: "acct_pg_karma_recovery", Login: "acct_pg_karma_recovery@test", DisplayName: "PgKarmaRecovery", State: accountStateActive}
	if err := env.store.Accounts.Create(ctx, account); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	character := &Character{
		ID:                 "char_pg_karma_recovery",
		AccountID:          account.ID,
		Name:               "PgKarmaRecovery",
		BaseClass:          "Fighter",
		LastRegionID:       startingRegionID,
		Karma:              40,
		KarmaRecoveryDueAt: now.Add(-time.Second),
		KarmaHighSince:     now.Add(-20 * time.Minute),
	}
	if err := env.store.Characters.Create(ctx, character); err != nil {
		t.Fatal(err)
	}
	commit, err := env.store.Characters.ApplyKarmaRecovery(ctx, character.ID, now, "attach")
	if err != nil {
		t.Fatal(err)
	}
	if commit.Event == nil || commit.State.Karma != 20 {
		t.Fatalf("unexpected postgres recovery commit: %+v", commit)
	}
	loaded, err := env.store.Characters.GetByID(ctx, character.ID)
	if err != nil || loaded.Karma != 20 || loaded.KarmaRecoveryDueAt.IsZero() {
		t.Fatalf("postgres recovery was not persisted: character=%+v err=%v", loaded, err)
	}
	events, err := env.store.PvPCombatEvents.ListKarmaRecoveryEvents(ctx, PvPKarmaRecoveryEventQuery{CharacterID: character.ID})
	if err != nil || len(events) != 1 || events[0].Trigger != "attach" {
		t.Fatalf("unexpected postgres recovery audit events: events=%+v err=%v", events, err)
	}
}

func TestMemoryPvPCombatAccountCorrelationQuery(t *testing.T) {
	store := newMemoryStore()
	now := time.Now().UTC()
	attacker := &Character{ID: "char_account_corr_attacker", AccountID: "acct_account_corr_attacker", Name: "AccountCorrAttacker", BaseClass: "Fighter", LastRegionID: startingRegionID}
	victim := &Character{ID: "char_account_corr_victim", AccountID: "acct_account_corr_victim", Name: "AccountCorrVictim", BaseClass: "Mage", LastRegionID: startingRegionID}
	for _, character := range []*Character{attacker, victim} {
		if err := store.Characters.Create(context.Background(), character); err != nil {
			t.Fatal(err)
		}
	}
	for index, eventID := range []string{"corr_first", "corr_second"} {
		if err := store.Characters.UpdateProgression(context.Background(), victim.ID, 1, 0, 0, 1, 58); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Characters.ApplyPvPCombat(context.Background(), pvpStoreMutation(eventID, attacker, victim, 10, now.Add(time.Duration(index)*time.Second))); err != nil {
			t.Fatal(err)
		}
	}
	correlations, err := store.PvPCombatEvents.ListAccountCorrelations(context.Background(), PvPAccountCorrelationQuery{
		AccountID:            attacker.AccountID,
		MinRepeatedKillCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(correlations) != 1 || correlations[0].AttackerAccountID != attacker.AccountID || correlations[0].VictimAccountID != victim.AccountID || correlations[0].SuspiciousCount != 1 || correlations[0].MaxRepeatedKillCount != 2 {
		t.Fatalf("unexpected account correlation records: %+v", correlations)
	}
}

func assertConcurrentPvPCombatSerialization(t *testing.T, firstStore *Store, secondStore *Store) {
	t.Helper()
	ctx := context.Background()
	firstAttacker := &Character{ID: "char_concurrent_first", AccountID: "acct_concurrent_first", Name: "ConcurrentFirst", BaseClass: "Fighter", LastRegionID: startingRegionID}
	secondAttacker := &Character{ID: "char_concurrent_second", AccountID: "acct_concurrent_second", Name: "ConcurrentSecond", BaseClass: "Fighter", LastRegionID: startingRegionID}
	victim := &Character{ID: "char_concurrent_victim", AccountID: "acct_concurrent_victim", Name: "ConcurrentVictim", BaseClass: "Mage", LastRegionID: startingRegionID}
	for _, character := range []*Character{firstAttacker, secondAttacker, victim} {
		if firstStore.Mode == "postgres" {
			if err := firstStore.Accounts.Create(ctx, &Account{ID: character.AccountID, Login: character.AccountID + "@test", DisplayName: character.Name, State: accountStateActive}); err != nil {
				t.Fatal(err)
			}
		}
		if err := firstStore.Characters.Create(ctx, character); err != nil {
			t.Fatal(err)
		}
	}
	if err := firstStore.Characters.UpdateProgression(ctx, victim.ID, 1, 0, 0, 100, 58); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	results := make(chan *PvPCombatCommit, 2)
	errors := make(chan error, 2)
	now := time.Now().UTC()
	mutations := []struct {
		store    *Store
		mutation PvPCombatMutation
	}{
		{store: firstStore, mutation: pvpStoreMutation("concurrent_first", firstAttacker, victim, 60, now)},
		{store: secondStore, mutation: pvpStoreMutation("concurrent_second", secondAttacker, victim, 60, now)},
	}
	var wait sync.WaitGroup
	for _, item := range mutations {
		wait.Add(1)
		go func(store *Store, mutation PvPCombatMutation) {
			defer wait.Done()
			<-started
			commit, err := store.Characters.ApplyPvPCombat(ctx, mutation)
			if err != nil {
				errors <- err
				return
			}
			results <- commit
		}(item.store, item.mutation)
	}
	close(started)
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatalf("concurrent combat failed: %v", err)
	}
	commits := make([]*PvPCombatCommit, 0, 2)
	for commit := range results {
		commits = append(commits, commit)
	}
	if len(commits) != 2 {
		t.Fatalf("commit count = %d want 2", len(commits))
	}
	loadedVictim, err := firstStore.Characters.GetByID(ctx, victim.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedVictim.CurrentCP != 0 || loadedVictim.CurrentHP != 0 {
		t.Fatalf("concurrent damage was lost or over-applied: %+v", loadedVictim)
	}
	events, err := firstStore.PvPCombatEvents.ListByFilter(ctx, PvPCombatEventQuery{VictimCharacterID: victim.ID})
	if err != nil || len(events) != 2 {
		t.Fatalf("expected exactly two serialized audit events: events=%+v err=%v", events, err)
	}
	var hit, kill *PvPCombatEvent
	for index := range events {
		switch events[index].Result {
		case "hit":
			hit = &events[index]
		case "pk_kill", "pvp_kill":
			kill = &events[index]
		}
	}
	if hit == nil || kill == nil || hit.Damage != 60 || kill.Damage != 40 {
		t.Fatalf("unexpected serialized damage events: %+v", events)
	}
	if kill.KillerCharacterID == "" || len(kill.AssistCharacterIDs) != 1 || kill.AssistCharacterIDs[0] != hit.AttackerCharacterID {
		t.Fatalf("concurrent kill attribution is wrong: hit=%+v kill=%+v", hit, kill)
	}
}

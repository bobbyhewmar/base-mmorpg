package app

import (
	"context"
	"math"
	"reflect"
	"time"
)

func (runtime *attachedRuntime) primaryPetLocked() (CharacterPet, int, bool) {
	if len(runtime.pets) == 0 {
		return CharacterPet{}, -1, false
	}
	return runtime.pets[0], 0, true
}

func (runtime *attachedRuntime) activePetLocked() (CharacterPet, int, bool) {
	for index, pet := range runtime.pets {
		if pet.IsSummoned {
			return pet, index, true
		}
	}
	return CharacterPet{}, -1, false
}

func (runtime *attachedRuntime) mountedPetIDLocked() string {
	for _, pet := range runtime.pets {
		if pet.IsMounted {
			return pet.ID
		}
	}
	return ""
}

func (runtime *attachedRuntime) projectedMountedPetIDLocked() any {
	if petID := runtime.mountedPetIDLocked(); petID != "" {
		return petID
	}
	return nil
}

func (runtime *attachedRuntime) petSnapshotsLocked() []CharacterPetSnapshot {
	return petSnapshots(runtime.pets)
}

func (runtime *attachedRuntime) loadPetState(pets []CharacterPet) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.pets = normalizeCharacterPets(pets)
	runtime.syncLocalPetEntityLocked()
	runtime.setMovementRunSpeedLocked(applyMountedPetMoveSpeed(runtime.derivedStats, runtime.pets).MoveSpeed)
}

func (runtime *attachedRuntime) recalculateDerivedStatsLocked(items []CharacterItem) {
	runtime.characterItems = cloneCharacterItems(items)
	runtime.derivedStats = deriveCharacterStats(&Character{
		BaseClass: runtime.characterBaseClass,
		Level:     runtime.characterLevel,
	}, runtime.characterItems)
	runtime.setMovementRunSpeedLocked(applyMountedPetMoveSpeed(runtime.derivedStats, runtime.pets).MoveSpeed)
	runtime.reconcileResourcePools()
}

func (runtime *attachedRuntime) petRuntimePositionLocked(pet CharacterPet) runtimePoint {
	if pet.IsMounted {
		return runtime.position
	}
	return runtimePoint{
		X: runtime.position.X - math.Cos(runtime.facing)*defaultPetFollowX + math.Sin(runtime.facing)*defaultPetFollowSide,
		Z: runtime.position.Z - math.Sin(runtime.facing)*defaultPetFollowX - math.Cos(runtime.facing)*defaultPetFollowSide,
	}
}

func (runtime *attachedRuntime) petEntityLocked(pet CharacterPet) runtimeEntity {
	template, _ := petTemplateByID(pet.PetTemplateID)
	state := map[string]any{
		"name":            petDisplayName(pet),
		"owner_id":        runtime.characterID,
		"owner_name":      runtime.characterName,
		"pet_template_id": pet.PetTemplateID,
		"visual_template": template.VisualTemplateID,
		"kind":            string(template.Kind),
		"follow_owner_id": runtime.characterID,
		"mount_eligible":  isMountEligible(template.Kind),
		"mounted":         pet.IsMounted,
	}
	if pet.IsMounted {
		state["mounted_by_character_id"] = runtime.characterID
	}
	return runtimeEntity{
		EntityID:   pet.ID,
		EntityType: petEntityType,
		TemplateID: template.VisualTemplateID,
		Position:   runtime.petRuntimePositionLocked(pet),
		State:      state,
	}
}

func (runtime *attachedRuntime) activePetEntity() (*runtimeEntity, bool) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.activePetEntityLocked()
}

func (runtime *attachedRuntime) activePetEntityLocked() (*runtimeEntity, bool) {
	pet, _, exists := runtime.activePetLocked()
	if !exists {
		return nil, false
	}
	entity := runtime.petEntityLocked(pet)
	return &entity, true
}

func (runtime *attachedRuntime) syncLocalPetEntityLocked() {
	for _, pet := range runtime.pets {
		delete(runtime.knownEntities, pet.ID)
		if runtime.targetID == pet.ID {
			runtime.targetID = ""
		}
	}
	if pet, _, exists := runtime.activePetLocked(); exists {
		entity := runtime.petEntityLocked(pet)
		runtime.knownEntities[entity.EntityID] = entity
	}
}

func companionPresencePatchFromEntity(entity runtimeEntity) map[string]any {
	patch := map[string]any{
		"entity_id": entity.EntityID,
		"position":  entity.Position,
	}
	for _, key := range []string{"name", "owner_id", "owner_name", "pet_template_id", "visual_template", "kind", "mount_eligible", "mounted", "mounted_by_character_id", "follow_owner_id"} {
		if value, exists := entity.State[key]; exists {
			patch[key] = value
		}
	}
	return patch
}

func (runtime *attachedRuntime) findRemotePetEntityByOwnerLocked(ownerID string) (runtimeEntity, bool) {
	for _, entity := range runtime.knownEntities {
		if entity.EntityType != petEntityType {
			continue
		}
		entityOwnerID, _ := entity.State["owner_id"].(string)
		if entityOwnerID == ownerID {
			return entity, true
		}
	}
	return runtimeEntity{}, false
}

func (runtime *attachedRuntime) syncRemotePetPresence(ownerID string, next *runtimeEntity) []map[string]any {
	if ownerID == "" {
		return nil
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	existing, hasExisting := runtime.findRemotePetEntityByOwnerLocked(ownerID)
	if next == nil {
		if !hasExisting {
			return nil
		}
		delete(runtime.knownEntities, existing.EntityID)
		if runtime.targetID == existing.EntityID {
			runtime.targetID = ""
		}
		runtime.regionRevision++
		return []map[string]any{entityDisappearMessage(runtime.regionRevision, existing.EntityID, entityDisappearPlayer)}
	}

	if !hasExisting {
		runtime.knownEntities[next.EntityID] = cloneRuntimeEntity(*next)
		runtime.regionRevision++
		return []map[string]any{entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(*next))}
	}

	if existing.EntityID != next.EntityID {
		delete(runtime.knownEntities, existing.EntityID)
		runtime.regionRevision++
		outbound := []map[string]any{
			entityDisappearMessage(runtime.regionRevision, existing.EntityID, entityDisappearPlayer),
		}
		runtime.knownEntities[next.EntityID] = cloneRuntimeEntity(*next)
		runtime.regionRevision++
		outbound = append(outbound, entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(*next)))
		return outbound
	}

	existingPatch := companionPresencePatchFromEntity(existing)
	nextPatch := companionPresencePatchFromEntity(*next)
	if reflect.DeepEqual(existingPatch, nextPatch) {
		return nil
	}
	runtime.knownEntities[next.EntityID] = cloneRuntimeEntity(*next)
	runtime.revision++
	return []map[string]any{
		deltaMessage(runtime.revision, "", 0, nil, []map[string]any{nextPatch}, nil),
	}
}

func (runtime *attachedRuntime) scheduleTamedMobRespawnLocked(entityID string, tamedAt time.Time) {
	spawn, exists := runtime.spawnEntities[entityID]
	if !exists {
		return
	}
	filtered := runtime.scheduledLifecycle[:0]
	for _, event := range runtime.scheduledLifecycle {
		if event.entityID != entityID {
			filtered = append(filtered, event)
		}
	}
	runtime.scheduledLifecycle = append(filtered, scheduledLifecycleEvent{
		dueAt:    tamedAt.Add(mobRespawnDelay),
		kind:     "entity_appear",
		entityID: entityID,
		entity:   cloneRuntimeEntity(spawn),
	})
}

func (runtime *attachedRuntime) processPetCommand(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
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
	if store == nil || store.CharacterPets == nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Pet persistence is unavailable."))
	}

	switch parsed.commandType {
	case "tame_mob":
		return append(outbound, runtime.tameMobLocked(ctx, store, command, parsed)...)
	case "summon_pet":
		return append(outbound, runtime.summonPetLocked(ctx, store, command)...)
	case "dismiss_pet":
		return append(outbound, runtime.dismissPetLocked(ctx, store, command)...)
	case "mount_pet":
		return append(outbound, runtime.mountPetLocked(ctx, store, command)...)
	case "dismount_pet":
		return append(outbound, runtime.dismountPetLocked(ctx, store, command)...)
	default:
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}
}

func (runtime *attachedRuntime) tameMobLocked(ctx context.Context, store *Store, command commandEnvelope, parsed *parsedCommand) []map[string]any {
	target, exists := runtime.knownEntities[parsed.targetID]
	if !exists || target.EntityType != "mob" {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced tame target is not in the current known-set.")}
	}
	if !isRuntimeEntityAlive(target) {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.target_not_tameable", "Referenced tame target is no longer eligible.")}
	}
	template, tameable := tameablePetTemplateForMob(target.TemplateID)
	if !tameable {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.target_not_tameable", "Referenced tame target cannot become a companion in this slice.")}
	}
	if distance(runtime.position, target.Position) > template.TameRange {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.tame_out_of_range", "Referenced tame target is outside tame range.")}
	}
	if len(runtime.pets) > 0 {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.ownership_limit_reached", "This slice allows only one owned companion per character.")}
	}

	now := time.Now()
	pet := CharacterPet{
		ID:            randomID("pet"),
		CharacterID:   runtime.characterID,
		PetTemplateID: template.ID,
		IsSummoned:    true,
		IsMounted:     false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.CharacterPets.Create(ctx, &pet); err != nil {
		if err == errRecordConflict {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.ownership_limit_reached", "This companion is already owned by the character.")}
		}
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist tame ownership.")}
	}

	runtime.pets = append(runtime.pets, pet)
	runtime.pets = normalizeCharacterPets(runtime.pets)
	delete(runtime.knownEntities, target.EntityID)
	if runtime.targetID == target.EntityID {
		runtime.targetID = ""
	}
	if runtime.queuedSkill != nil && runtime.queuedSkill.TargetID == target.EntityID {
		runtime.queuedSkill = nil
	}
	if runtime.queuedBasicAttack != nil && runtime.queuedBasicAttack.TargetID == target.EntityID {
		runtime.queuedBasicAttack = nil
	}
	runtime.syncLocalPetEntityLocked()
	runtime.scheduleTamedMobRespawnLocked(target.EntityID, now)

	entity, _ := runtime.activePetEntityLocked()
	runtime.revision++
	outbound := []map[string]any{
		deltaMessage(runtime.revision, command.CommandID, command.CommandSeq, runtime.selfDelta(now, nil), nil, nil),
	}
	runtime.regionRevision++
	outbound = append(outbound, entityDisappearMessage(runtime.regionRevision, target.EntityID, entityDisappearTamed))
	if entity != nil {
		runtime.regionRevision++
		outbound = append(outbound, entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(*entity)))
	}
	return outbound
}

func (runtime *attachedRuntime) summonPetLocked(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	pet, index, exists := runtime.primaryPetLocked()
	if !exists {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own a companion yet.")}
	}
	if pet.IsSummoned {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.already_summoned", "Owned companion is already summoned.")}
	}
	if err := store.CharacterPets.UpdateState(ctx, runtime.characterID, pet.ID, true, false); err != nil {
		if err == errPetNotFound {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own that companion.")}
		}
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist summon state.")}
	}

	pet.IsSummoned = true
	pet.IsMounted = false
	pet.UpdatedAt = time.Now()
	runtime.pets[index] = pet
	runtime.syncLocalPetEntityLocked()
	entity, _ := runtime.activePetEntityLocked()
	runtime.revision++
	outbound := []map[string]any{
		deltaMessage(runtime.revision, command.CommandID, command.CommandSeq, runtime.selfDelta(time.Now(), nil), nil, nil),
	}
	if entity != nil {
		runtime.regionRevision++
		outbound = append(outbound, entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(*entity)))
	}
	return outbound
}

func (runtime *attachedRuntime) dismissPetLocked(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	pet, index, exists := runtime.primaryPetLocked()
	if !exists {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own a companion yet.")}
	}
	if pet.IsMounted {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "mount.dismount_required", "Dismount before dismissing the companion.")}
	}
	if !pet.IsSummoned {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_summoned", "Owned companion is not currently summoned.")}
	}
	if err := store.CharacterPets.UpdateState(ctx, runtime.characterID, pet.ID, false, false); err != nil {
		if err == errPetNotFound {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own that companion.")}
		}
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist dismiss state.")}
	}

	pet.IsSummoned = false
	pet.IsMounted = false
	pet.UpdatedAt = time.Now()
	runtime.pets[index] = pet
	runtime.syncLocalPetEntityLocked()
	runtime.revision++
	runtime.regionRevision++
	return []map[string]any{
		deltaMessage(runtime.revision, command.CommandID, command.CommandSeq, runtime.selfDelta(time.Now(), nil), nil, nil),
		entityDisappearMessage(runtime.regionRevision, pet.ID, entityDisappearDismiss),
	}
}

func (runtime *attachedRuntime) mountPetLocked(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	pet, index, exists := runtime.primaryPetLocked()
	if !exists {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own a companion yet.")}
	}
	template, templateExists := petTemplateByID(pet.PetTemplateID)
	if !templateExists || !isMountEligible(template.Kind) {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "mount.not_mountable", "Owned companion cannot be mounted in this slice.")}
	}
	if !pet.IsSummoned {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "mount.pet_not_ready", "Summon the companion before mounting it.")}
	}
	if pet.IsMounted {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "mount.already_mounted", "Character is already mounted.")}
	}
	if err := store.CharacterPets.UpdateState(ctx, runtime.characterID, pet.ID, true, true); err != nil {
		if err == errPetNotFound {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own that companion.")}
		}
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist mount state.")}
	}

	pet.IsMounted = true
	pet.IsSummoned = true
	pet.UpdatedAt = time.Now()
	runtime.pets[index] = pet
	runtime.syncLocalPetEntityLocked()
	runtime.recalculateDerivedStatsLocked(runtime.characterItems)
	runtime.revision++
	return []map[string]any{
		deltaMessage(runtime.revision, command.CommandID, command.CommandSeq, runtime.selfDelta(time.Now(), nil), nil, nil),
	}
}

func (runtime *attachedRuntime) dismountPetLocked(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	pet, index, exists := runtime.primaryPetLocked()
	if !exists {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own a companion yet.")}
	}
	if !pet.IsMounted {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "mount.not_mounted", "Character is not currently mounted.")}
	}
	if err := store.CharacterPets.UpdateState(ctx, runtime.characterID, pet.ID, true, false); err != nil {
		if err == errPetNotFound {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "pet.not_owned", "Character does not own that companion.")}
		}
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist dismount state.")}
	}

	pet.IsMounted = false
	pet.IsSummoned = true
	pet.UpdatedAt = time.Now()
	runtime.pets[index] = pet
	runtime.syncLocalPetEntityLocked()
	runtime.recalculateDerivedStatsLocked(runtime.characterItems)
	runtime.revision++
	return []map[string]any{
		deltaMessage(runtime.revision, command.CommandID, command.CommandSeq, runtime.selfDelta(time.Now(), nil), nil, nil),
	}
}

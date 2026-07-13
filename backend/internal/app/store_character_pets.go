package app

import (
	"context"
	"database/sql"
	"errors"
)

func (repo memoryCharacterPetRepo) ListByCharacterID(_ context.Context, characterID string) ([]CharacterPet, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	pets := normalizeCharacterPets(repo.backend.characterPets[characterID])
	return cloneCharacterPets(pets), nil
}

func (repo memoryCharacterPetRepo) Create(_ context.Context, pet *CharacterPet) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.characters[pet.CharacterID]; !exists {
		return errRecordNotFound
	}
	for _, existing := range repo.backend.characterPets[pet.CharacterID] {
		if existing.ID == pet.ID || existing.PetTemplateID == pet.PetTemplateID {
			return errRecordConflict
		}
	}
	repo.backend.characterPets[pet.CharacterID] = append(
		repo.backend.characterPets[pet.CharacterID],
		CharacterPet{
			ID:            pet.ID,
			CharacterID:   pet.CharacterID,
			PetTemplateID: pet.PetTemplateID,
			CustomName:    pet.CustomName,
			IsSummoned:    pet.IsSummoned,
			IsMounted:     pet.IsMounted,
			CreatedAt:     pet.CreatedAt,
			UpdatedAt:     pet.UpdatedAt,
		},
	)
	repo.backend.characterPets[pet.CharacterID] = normalizeCharacterPets(repo.backend.characterPets[pet.CharacterID])
	return nil
}

func (repo memoryCharacterPetRepo) UpdateState(_ context.Context, characterID string, petID string, summoned bool, mounted bool) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	pets := repo.backend.characterPets[characterID]
	for index := range pets {
		if pets[index].ID != petID {
			continue
		}
		pets[index].IsSummoned = summoned
		pets[index].IsMounted = mounted
		repo.backend.characterPets[characterID] = normalizeCharacterPets(pets)
		return nil
	}
	return errPetNotFound
}

func (p *postgresStoreBackend) ListCharacterPetsByCharacterID(ctx context.Context, characterID string) ([]CharacterPet, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT pet_instance_id, character_id, pet_template_id, COALESCE(custom_name, ''), is_summoned, is_mounted, created_at, updated_at
		 FROM character_pets
		 WHERE character_id = $1
		 ORDER BY created_at, pet_instance_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pets := make([]CharacterPet, 0)
	for rows.Next() {
		var pet CharacterPet
		if err := rows.Scan(
			&pet.ID,
			&pet.CharacterID,
			&pet.PetTemplateID,
			&pet.CustomName,
			&pet.IsSummoned,
			&pet.IsMounted,
			&pet.CreatedAt,
			&pet.UpdatedAt,
		); err != nil {
			return nil, err
		}
		pets = append(pets, pet)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pets, nil
}

func (p *postgresStoreBackend) CreateCharacterPet(ctx context.Context, pet *CharacterPet) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO character_pets (pet_instance_id, character_id, pet_template_id, custom_name, is_summoned, is_mounted, created_at, updated_at)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)`,
		pet.ID,
		pet.CharacterID,
		pet.PetTemplateID,
		pet.CustomName,
		pet.IsSummoned,
		pet.IsMounted,
		pet.CreatedAt,
		pet.UpdatedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) UpdateCharacterPetState(ctx context.Context, characterID string, petID string, summoned bool, mounted bool) error {
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE character_pets
		 SET is_summoned = $3,
		     is_mounted = $4,
		     updated_at = NOW()
		 WHERE character_id = $1
		   AND pet_instance_id = $2`,
		characterID,
		petID,
		summoned,
		mounted,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errPetNotFound
	}
	return nil
}

type postgresCharacterPetRepo struct{ backend *postgresStoreBackend }

func (repo postgresCharacterPetRepo) ListByCharacterID(ctx context.Context, characterID string) ([]CharacterPet, error) {
	pets, err := repo.backend.ListCharacterPetsByCharacterID(ctx, characterID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return pets, nil
}

func (repo postgresCharacterPetRepo) Create(ctx context.Context, pet *CharacterPet) error {
	return repo.backend.CreateCharacterPet(ctx, pet)
}

func (repo postgresCharacterPetRepo) UpdateState(ctx context.Context, characterID string, petID string, summoned bool, mounted bool) error {
	return repo.backend.UpdateCharacterPetState(ctx, characterID, petID, summoned, mounted)
}

package app

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

func (repo memoryPartyRepo) GetByID(_ context.Context, partyID string) (*Party, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	party, exists := repo.backend.parties[partyID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneParty(party), nil
}

func (repo memoryPartyRepo) GetByCharacterID(_ context.Context, characterID string) (*Party, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	partyID, exists := repo.backend.partyByCharacter[characterID]
	if !exists {
		return nil, errRecordNotFound
	}
	party, exists := repo.backend.parties[partyID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneParty(party), nil
}

func (repo memoryPartyRepo) ListMembers(_ context.Context, partyID string) ([]PartyMember, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	members := normalizePartyMembers(repo.backend.partyMembers[partyID])
	if len(members) == 0 {
		return nil, errRecordNotFound
	}
	return clonePartyMembers(members), nil
}

func (repo memoryPartyRepo) Create(_ context.Context, party *Party, leader PartyMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if party == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.parties[party.ID]; exists {
		return errRecordConflict
	}
	if _, exists := repo.backend.partyByCharacter[leader.CharacterID]; exists {
		return errRecordConflict
	}
	if _, exists := repo.backend.characters[leader.CharacterID]; !exists {
		return errRecordNotFound
	}
	partyCopy := *party
	repo.backend.parties[party.ID] = &partyCopy
	memberCopy := leader
	repo.backend.partyMembers[party.ID] = []PartyMember{memberCopy}
	repo.backend.partyByCharacter[leader.CharacterID] = party.ID
	return nil
}

func (repo memoryPartyRepo) AddMember(_ context.Context, member *PartyMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if member == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.parties[member.PartyID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.characters[member.CharacterID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.partyByCharacter[member.CharacterID]; exists {
		return errRecordConflict
	}
	for _, existing := range repo.backend.partyMembers[member.PartyID] {
		if existing.CharacterID == member.CharacterID {
			return errRecordConflict
		}
	}
	repo.backend.partyMembers[member.PartyID] = append(repo.backend.partyMembers[member.PartyID], *member)
	repo.backend.partyMembers[member.PartyID] = normalizePartyMembers(repo.backend.partyMembers[member.PartyID])
	repo.backend.partyByCharacter[member.CharacterID] = member.PartyID
	if party := repo.backend.parties[member.PartyID]; party != nil {
		party.UpdatedAt = member.UpdatedAt
	}
	return nil
}

func (repo memoryPartyRepo) RemoveMember(_ context.Context, partyID string, characterID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	members := repo.backend.partyMembers[partyID]
	filtered := members[:0]
	removed := false
	for _, member := range members {
		if member.CharacterID == characterID {
			removed = true
			continue
		}
		filtered = append(filtered, member)
	}
	if !removed {
		return errRecordNotFound
	}
	repo.backend.partyMembers[partyID] = filtered
	delete(repo.backend.partyByCharacter, characterID)
	return nil
}

func (repo memoryPartyRepo) UpdateLeader(_ context.Context, partyID string, leaderCharacterID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	party, exists := repo.backend.parties[partyID]
	if !exists {
		return errRecordNotFound
	}
	party.LeaderCharacterID = leaderCharacterID
	party.UpdatedAt = time.Now().UTC()
	return nil
}

func (repo memoryPartyRepo) Delete(_ context.Context, partyID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.parties[partyID]; !exists {
		return errRecordNotFound
	}
	for _, member := range repo.backend.partyMembers[partyID] {
		delete(repo.backend.partyByCharacter, member.CharacterID)
	}
	delete(repo.backend.partyMembers, partyID)
	delete(repo.backend.parties, partyID)
	for inviteID, invite := range repo.backend.partyInvites {
		if invite != nil && invite.PartyID == partyID {
			delete(repo.backend.partyInvites, inviteID)
		}
	}
	return nil
}

func (repo memoryPartyRepo) ListPendingInvitesByInvitee(_ context.Context, characterID string, now time.Time) ([]PartyInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invites := make([]PartyInvite, 0)
	for _, invite := range repo.backend.partyInvites {
		if invite == nil || invite.InviteeCharacterID != characterID || !invite.ExpiresAt.After(now) {
			continue
		}
		invites = append(invites, *invite)
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return normalizePartyInvites(invites), nil
}

func (repo memoryPartyRepo) ListPendingInvitesByParty(_ context.Context, partyID string, now time.Time) ([]PartyInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invites := make([]PartyInvite, 0)
	for _, invite := range repo.backend.partyInvites {
		if invite == nil || invite.PartyID != partyID || !invite.ExpiresAt.After(now) {
			continue
		}
		invites = append(invites, *invite)
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return normalizePartyInvites(invites), nil
}

func (repo memoryPartyRepo) GetInviteByID(_ context.Context, inviteID string) (*PartyInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invite, exists := repo.backend.partyInvites[inviteID]
	if !exists || invite == nil {
		return nil, errRecordNotFound
	}
	cloned := *invite
	return &cloned, nil
}

func (repo memoryPartyRepo) CreateInvite(_ context.Context, invite *PartyInvite) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if invite == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.partyInvites[invite.ID]; exists {
		return errRecordConflict
	}
	if _, exists := repo.backend.parties[invite.PartyID]; !exists {
		return errRecordNotFound
	}
	for _, existing := range repo.backend.partyInvites {
		if existing == nil {
			continue
		}
		if existing.InviteeCharacterID == invite.InviteeCharacterID && existing.ExpiresAt.After(time.Now().UTC()) {
			return errRecordConflict
		}
	}
	inviteCopy := *invite
	repo.backend.partyInvites[invite.ID] = &inviteCopy
	return nil
}

func (repo memoryPartyRepo) DeleteInvite(_ context.Context, inviteID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.partyInvites[inviteID]; !exists {
		return errRecordNotFound
	}
	delete(repo.backend.partyInvites, inviteID)
	return nil
}

func (repo memoryPartyRepo) DeleteInvitesByParty(_ context.Context, partyID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	for inviteID, invite := range repo.backend.partyInvites {
		if invite != nil && invite.PartyID == partyID {
			delete(repo.backend.partyInvites, inviteID)
		}
	}
	return nil
}

func (repo memoryPartyRepo) DeletePendingInviteForInvitee(_ context.Context, partyID string, inviteeCharacterID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	removed := false
	for inviteID, invite := range repo.backend.partyInvites {
		if invite == nil {
			continue
		}
		if invite.PartyID == partyID && invite.InviteeCharacterID == inviteeCharacterID {
			delete(repo.backend.partyInvites, inviteID)
			removed = true
		}
	}
	if !removed {
		return errRecordNotFound
	}
	return nil
}

func (repo memoryPartyRepo) ExpireInvites(_ context.Context, now time.Time) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	for inviteID, invite := range repo.backend.partyInvites {
		if invite == nil {
			delete(repo.backend.partyInvites, inviteID)
			continue
		}
		if !invite.ExpiresAt.After(now) {
			delete(repo.backend.partyInvites, inviteID)
		}
	}
	return nil
}

func (p *postgresStoreBackend) GetPartyByID(ctx context.Context, partyID string) (*Party, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT party_id, leader_character_id, created_at, updated_at
		 FROM parties
		 WHERE party_id = $1`,
		partyID,
	)
	party := &Party{}
	if err := row.Scan(&party.ID, &party.LeaderCharacterID, &party.CreatedAt, &party.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return party, nil
}

func (p *postgresStoreBackend) GetPartyByCharacterID(ctx context.Context, characterID string) (*Party, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT p.party_id, p.leader_character_id, p.created_at, p.updated_at
		 FROM parties p
		 JOIN party_members pm ON pm.party_id = p.party_id
		 WHERE pm.character_id = $1`,
		characterID,
	)
	party := &Party{}
	if err := row.Scan(&party.ID, &party.LeaderCharacterID, &party.CreatedAt, &party.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return party, nil
}

func (p *postgresStoreBackend) ListPartyMembers(ctx context.Context, partyID string) ([]PartyMember, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT party_id, character_id, joined_at, created_at, updated_at
		 FROM party_members
		 WHERE party_id = $1
		 ORDER BY joined_at, character_id`,
		partyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]PartyMember, 0)
	for rows.Next() {
		var member PartyMember
		if err := rows.Scan(&member.PartyID, &member.CharacterID, &member.JoinedAt, &member.CreatedAt, &member.UpdatedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, errRecordNotFound
	}
	return members, nil
}

func (p *postgresStoreBackend) CreateParty(ctx context.Context, party *Party, leader PartyMember) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO parties (party_id, leader_character_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4)`,
		party.ID,
		party.LeaderCharacterID,
		party.CreatedAt,
		party.UpdatedAt,
	); err != nil {
		return mapPostgresError(err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO party_members (party_id, character_id, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		leader.PartyID,
		leader.CharacterID,
		leader.JoinedAt,
		leader.CreatedAt,
		leader.UpdatedAt,
	); err != nil {
		return mapPostgresError(err)
	}

	return tx.Commit()
}

func (p *postgresStoreBackend) AddPartyMember(ctx context.Context, member *PartyMember) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO party_members (party_id, character_id, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		member.PartyID,
		member.CharacterID,
		member.JoinedAt,
		member.CreatedAt,
		member.UpdatedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) RemovePartyMember(ctx context.Context, partyID string, characterID string) error {
	result, err := p.db.ExecContext(
		ctx,
		`DELETE FROM party_members
		 WHERE party_id = $1
		   AND character_id = $2`,
		partyID,
		characterID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) UpdatePartyLeader(ctx context.Context, partyID string, leaderCharacterID string) error {
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE parties
		 SET leader_character_id = $2,
		     updated_at = NOW()
		 WHERE party_id = $1`,
		partyID,
		leaderCharacterID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) DeleteParty(ctx context.Context, partyID string) error {
	result, err := p.db.ExecContext(ctx, `DELETE FROM parties WHERE party_id = $1`, partyID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) listPartyInvites(ctx context.Context, query string, arg any, now time.Time) ([]PartyInvite, error) {
	rows, err := p.db.QueryContext(ctx, query, arg, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invites := make([]PartyInvite, 0)
	for rows.Next() {
		var invite PartyInvite
		if err := rows.Scan(&invite.ID, &invite.PartyID, &invite.InviterCharacterID, &invite.InviteeCharacterID, &invite.ExpiresAt, &invite.CreatedAt, &invite.UpdatedAt); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return invites, nil
}

func (p *postgresStoreBackend) ListPendingPartyInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]PartyInvite, error) {
	return p.listPartyInvites(
		ctx,
		`SELECT invite_id, party_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM party_invites
		 WHERE invitee_character_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		characterID,
		now,
	)
}

func (p *postgresStoreBackend) ListPendingPartyInvitesByParty(ctx context.Context, partyID string, now time.Time) ([]PartyInvite, error) {
	return p.listPartyInvites(
		ctx,
		`SELECT invite_id, party_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM party_invites
		 WHERE party_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		partyID,
		now,
	)
}

func (p *postgresStoreBackend) GetPartyInviteByID(ctx context.Context, inviteID string) (*PartyInvite, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT invite_id, party_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM party_invites
		 WHERE invite_id = $1`,
		inviteID,
	)
	invite := &PartyInvite{}
	if err := row.Scan(&invite.ID, &invite.PartyID, &invite.InviterCharacterID, &invite.InviteeCharacterID, &invite.ExpiresAt, &invite.CreatedAt, &invite.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return invite, nil
}

func (p *postgresStoreBackend) CreatePartyInvite(ctx context.Context, invite *PartyInvite) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO party_invites (invite_id, party_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		invite.ID,
		invite.PartyID,
		invite.InviterCharacterID,
		invite.InviteeCharacterID,
		invite.ExpiresAt,
		invite.CreatedAt,
		invite.UpdatedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) DeletePartyInvite(ctx context.Context, inviteID string) error {
	result, err := p.db.ExecContext(ctx, `DELETE FROM party_invites WHERE invite_id = $1`, inviteID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) DeletePartyInvitesByParty(ctx context.Context, partyID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM party_invites WHERE party_id = $1`, partyID)
	return err
}

func (p *postgresStoreBackend) DeletePendingPartyInviteForInvitee(ctx context.Context, partyID string, inviteeCharacterID string) error {
	result, err := p.db.ExecContext(
		ctx,
		`DELETE FROM party_invites
		 WHERE party_id = $1
		   AND invitee_character_id = $2`,
		partyID,
		inviteeCharacterID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) ExpirePartyInvites(ctx context.Context, now time.Time) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM party_invites WHERE expires_at <= $1`, now)
	return err
}

func (repo postgresPartyRepo) GetByID(ctx context.Context, partyID string) (*Party, error) {
	return repo.backend.GetPartyByID(ctx, partyID)
}

func (repo postgresPartyRepo) GetByCharacterID(ctx context.Context, characterID string) (*Party, error) {
	return repo.backend.GetPartyByCharacterID(ctx, characterID)
}

func (repo postgresPartyRepo) ListMembers(ctx context.Context, partyID string) ([]PartyMember, error) {
	return repo.backend.ListPartyMembers(ctx, partyID)
}

func (repo postgresPartyRepo) Create(ctx context.Context, party *Party, leader PartyMember) error {
	return repo.backend.CreateParty(ctx, party, leader)
}

func (repo postgresPartyRepo) AddMember(ctx context.Context, member *PartyMember) error {
	return repo.backend.AddPartyMember(ctx, member)
}

func (repo postgresPartyRepo) RemoveMember(ctx context.Context, partyID string, characterID string) error {
	return repo.backend.RemovePartyMember(ctx, partyID, characterID)
}

func (repo postgresPartyRepo) UpdateLeader(ctx context.Context, partyID string, leaderCharacterID string) error {
	return repo.backend.UpdatePartyLeader(ctx, partyID, leaderCharacterID)
}

func (repo postgresPartyRepo) Delete(ctx context.Context, partyID string) error {
	return repo.backend.DeleteParty(ctx, partyID)
}

func (repo postgresPartyRepo) ListPendingInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]PartyInvite, error) {
	return repo.backend.ListPendingPartyInvitesByInvitee(ctx, characterID, now)
}

func (repo postgresPartyRepo) ListPendingInvitesByParty(ctx context.Context, partyID string, now time.Time) ([]PartyInvite, error) {
	return repo.backend.ListPendingPartyInvitesByParty(ctx, partyID, now)
}

func (repo postgresPartyRepo) GetInviteByID(ctx context.Context, inviteID string) (*PartyInvite, error) {
	return repo.backend.GetPartyInviteByID(ctx, inviteID)
}

func (repo postgresPartyRepo) CreateInvite(ctx context.Context, invite *PartyInvite) error {
	return repo.backend.CreatePartyInvite(ctx, invite)
}

func (repo postgresPartyRepo) DeleteInvite(ctx context.Context, inviteID string) error {
	return repo.backend.DeletePartyInvite(ctx, inviteID)
}

func (repo postgresPartyRepo) DeleteInvitesByParty(ctx context.Context, partyID string) error {
	return repo.backend.DeletePartyInvitesByParty(ctx, partyID)
}

func (repo postgresPartyRepo) DeletePendingInviteForInvitee(ctx context.Context, partyID string, inviteeCharacterID string) error {
	return repo.backend.DeletePendingPartyInviteForInvitee(ctx, partyID, inviteeCharacterID)
}

func (repo postgresPartyRepo) ExpireInvites(ctx context.Context, now time.Time) error {
	return repo.backend.ExpirePartyInvites(ctx, now)
}

func sortPartyMemberSnapshots(members []CharacterPartyMemberSnapshot) {
	sort.Slice(members, func(i, j int) bool {
		if members[i].IsLeader != members[j].IsLeader {
			return members[i].IsLeader
		}
		if members[i].Online != members[j].Online {
			return members[i].Online
		}
		if members[i].Level == members[j].Level {
			return members[i].Name < members[j].Name
		}
		return members[i].Level > members[j].Level
	})
}

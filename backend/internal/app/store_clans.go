package app

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

func (repo memoryClanRepo) GetByID(_ context.Context, clanID string) (*Clan, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	clan, exists := repo.backend.clans[clanID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneClan(clan), nil
}

func (repo memoryClanRepo) GetByName(_ context.Context, name string) (*Clan, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	clanID, exists := repo.backend.clanByName[normalizedClanLookupKey(name)]
	if !exists {
		return nil, errRecordNotFound
	}
	clan, exists := repo.backend.clans[clanID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneClan(clan), nil
}

func (repo memoryClanRepo) GetByCharacterID(_ context.Context, characterID string) (*Clan, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	clanID, exists := repo.backend.clanByCharacter[characterID]
	if !exists {
		return nil, errRecordNotFound
	}
	clan, exists := repo.backend.clans[clanID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneClan(clan), nil
}

func (repo memoryClanRepo) ListMembers(_ context.Context, clanID string) ([]ClanMember, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	members, exists := repo.backend.clanMembers[clanID]
	if !exists || len(members) == 0 {
		return nil, errRecordNotFound
	}
	return normalizeClanMembers(members), nil
}

func (repo memoryClanRepo) Create(_ context.Context, clan *Clan, leader ClanMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if clan == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clans[clan.ID]; exists {
		return errRecordConflict
	}
	nameKey := normalizedClanLookupKey(clan.Name)
	if nameKey == "" {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clanByName[nameKey]; exists {
		return errRecordConflict
	}
	if leader.CharacterID != "" {
		if _, exists := repo.backend.clanByCharacter[leader.CharacterID]; exists {
			return errRecordConflict
		}
		if _, exists := repo.backend.characters[leader.CharacterID]; !exists {
			return errRecordNotFound
		}
	}

	clanCopy := *clan
	clanCopy.Name = normalizeClanName(clan.Name)
	repo.backend.clans[clan.ID] = &clanCopy
	repo.backend.clanByName[nameKey] = clan.ID
	if leader.CharacterID != "" {
		memberCopy := leader
		repo.backend.clanMembers[clan.ID] = []ClanMember{memberCopy}
		repo.backend.clanByCharacter[leader.CharacterID] = clan.ID
	}
	return nil
}

func (repo memoryClanRepo) AddMember(_ context.Context, member *ClanMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if member == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clans[member.ClanID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.characters[member.CharacterID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clanByCharacter[member.CharacterID]; exists {
		return errRecordConflict
	}

	memberCopy := *member
	repo.backend.clanMembers[member.ClanID] = append(repo.backend.clanMembers[member.ClanID], memberCopy)
	repo.backend.clanByCharacter[member.CharacterID] = member.ClanID
	return nil
}

func (repo memoryClanRepo) RemoveMember(_ context.Context, clanID string, characterID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	members, exists := repo.backend.clanMembers[clanID]
	if !exists || len(members) == 0 {
		return errRecordNotFound
	}

	updated := members[:0]
	removed := false
	for _, member := range members {
		if member.CharacterID == characterID {
			removed = true
			delete(repo.backend.clanByCharacter, characterID)
			continue
		}
		updated = append(updated, member)
	}
	if !removed {
		return errRecordNotFound
	}
	repo.backend.clanMembers[clanID] = append([]ClanMember(nil), updated...)
	return nil
}

func (repo memoryClanRepo) Delete(_ context.Context, clanID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	clan, exists := repo.backend.clans[clanID]
	if !exists {
		return errRecordNotFound
	}
	for _, member := range repo.backend.clanMembers[clanID] {
		delete(repo.backend.clanByCharacter, member.CharacterID)
	}
	for inviteID, invite := range repo.backend.clanInvites {
		if invite == nil || invite.ClanID != clanID {
			continue
		}
		delete(repo.backend.clanInvites, inviteID)
	}
	delete(repo.backend.clanMembers, clanID)
	delete(repo.backend.clanByName, normalizedClanLookupKey(clan.Name))
	delete(repo.backend.clans, clanID)
	return nil
}

func (repo memoryClanRepo) ListPendingInvitesByInvitee(_ context.Context, characterID string, now time.Time) ([]ClanInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invites := make([]ClanInvite, 0)
	for _, invite := range repo.backend.clanInvites {
		if invite == nil || invite.InviteeCharacterID != characterID || !invite.ExpiresAt.After(now) {
			continue
		}
		invites = append(invites, *invite)
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return normalizeClanInvites(invites), nil
}

func (repo memoryClanRepo) ListPendingInvitesByInviter(_ context.Context, characterID string, now time.Time) ([]ClanInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invites := make([]ClanInvite, 0)
	for _, invite := range repo.backend.clanInvites {
		if invite == nil || invite.InviterCharacterID != characterID || !invite.ExpiresAt.After(now) {
			continue
		}
		invites = append(invites, *invite)
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return normalizeClanInvites(invites), nil
}

func (repo memoryClanRepo) ListPendingInvitesByClan(_ context.Context, clanID string, now time.Time) ([]ClanInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invites := make([]ClanInvite, 0)
	for _, invite := range repo.backend.clanInvites {
		if invite == nil || invite.ClanID != clanID || !invite.ExpiresAt.After(now) {
			continue
		}
		invites = append(invites, *invite)
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return normalizeClanInvites(invites), nil
}

func (repo memoryClanRepo) GetInviteByID(_ context.Context, inviteID string) (*ClanInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invite, exists := repo.backend.clanInvites[inviteID]
	if !exists {
		return nil, errRecordNotFound
	}
	cloned := *invite
	return &cloned, nil
}

func (repo memoryClanRepo) CreateInvite(_ context.Context, invite *ClanInvite) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if invite == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clanInvites[invite.ID]; exists {
		return errRecordConflict
	}
	if _, exists := repo.backend.clans[invite.ClanID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.characters[invite.InviterCharacterID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.characters[invite.InviteeCharacterID]; !exists {
		return errRecordNotFound
	}
	for _, existing := range repo.backend.clanInvites {
		if existing == nil || !existing.ExpiresAt.After(invite.CreatedAt) {
			continue
		}
		if existing.ClanID == invite.ClanID || existing.InviteeCharacterID == invite.InviteeCharacterID {
			return errRecordConflict
		}
	}
	cloned := *invite
	repo.backend.clanInvites[invite.ID] = &cloned
	return nil
}

func (repo memoryClanRepo) AcceptInvite(_ context.Context, inviteID string, member *ClanMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if member == nil {
		return errRecordNotFound
	}
	invite, exists := repo.backend.clanInvites[inviteID]
	if !exists || invite == nil || invite.ClanID != member.ClanID || invite.InviteeCharacterID != member.CharacterID || !invite.ExpiresAt.After(member.JoinedAt) {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clans[member.ClanID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.characters[member.CharacterID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clanByCharacter[member.CharacterID]; exists {
		return errRecordConflict
	}

	memberCopy := *member
	repo.backend.clanMembers[member.ClanID] = append(repo.backend.clanMembers[member.ClanID], memberCopy)
	repo.backend.clanByCharacter[member.CharacterID] = member.ClanID
	delete(repo.backend.clanInvites, inviteID)
	return nil
}

func (repo memoryClanRepo) DeleteInvite(_ context.Context, inviteID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.clanInvites[inviteID]; !exists {
		return errRecordNotFound
	}
	delete(repo.backend.clanInvites, inviteID)
	return nil
}

func (repo memoryClanRepo) DeleteInvitesByClan(_ context.Context, clanID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	removed := false
	for inviteID, invite := range repo.backend.clanInvites {
		if invite == nil {
			delete(repo.backend.clanInvites, inviteID)
			continue
		}
		if invite.ClanID == clanID {
			delete(repo.backend.clanInvites, inviteID)
			removed = true
		}
	}
	if !removed {
		return errRecordNotFound
	}
	return nil
}

func (repo memoryClanRepo) DeletePendingInviteForInvitee(_ context.Context, clanID string, inviteeCharacterID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	removed := false
	for inviteID, invite := range repo.backend.clanInvites {
		if invite == nil {
			delete(repo.backend.clanInvites, inviteID)
			continue
		}
		if invite.ClanID == clanID && invite.InviteeCharacterID == inviteeCharacterID {
			delete(repo.backend.clanInvites, inviteID)
			removed = true
		}
	}
	if !removed {
		return errRecordNotFound
	}
	return nil
}

func (repo memoryClanRepo) ExpireInvites(_ context.Context, now time.Time) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	for inviteID, invite := range repo.backend.clanInvites {
		if invite == nil {
			delete(repo.backend.clanInvites, inviteID)
			continue
		}
		if !invite.ExpiresAt.After(now) {
			delete(repo.backend.clanInvites, inviteID)
		}
	}
	return nil
}

func (p *postgresStoreBackend) GetClanByID(ctx context.Context, clanID string) (*Clan, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT clan_id, name, leader_character_id, created_at, updated_at
		 FROM clans
		 WHERE clan_id = $1`,
		clanID,
	)
	clan := &Clan{}
	if err := row.Scan(&clan.ID, &clan.Name, &clan.LeaderCharacterID, &clan.CreatedAt, &clan.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return clan, nil
}

func (p *postgresStoreBackend) GetClanByName(ctx context.Context, name string) (*Clan, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT clan_id, name, leader_character_id, created_at, updated_at
		 FROM clans
		 WHERE LOWER(BTRIM(name)) = LOWER(BTRIM($1))`,
		normalizeClanName(name),
	)
	clan := &Clan{}
	if err := row.Scan(&clan.ID, &clan.Name, &clan.LeaderCharacterID, &clan.CreatedAt, &clan.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return clan, nil
}

func (p *postgresStoreBackend) GetClanByCharacterID(ctx context.Context, characterID string) (*Clan, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT c.clan_id, c.name, c.leader_character_id, c.created_at, c.updated_at
		 FROM clans c
		 JOIN clan_members cm ON cm.clan_id = c.clan_id
		 WHERE cm.character_id = $1`,
		characterID,
	)
	clan := &Clan{}
	if err := row.Scan(&clan.ID, &clan.Name, &clan.LeaderCharacterID, &clan.CreatedAt, &clan.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return clan, nil
}

func (p *postgresStoreBackend) ListClanMembers(ctx context.Context, clanID string) ([]ClanMember, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT clan_id, character_id, joined_at, created_at, updated_at
		 FROM clan_members
		 WHERE clan_id = $1
		 ORDER BY joined_at, character_id`,
		clanID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]ClanMember, 0)
	for rows.Next() {
		var member ClanMember
		if err := rows.Scan(&member.ClanID, &member.CharacterID, &member.JoinedAt, &member.CreatedAt, &member.UpdatedAt); err != nil {
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

func (p *postgresStoreBackend) CreateClan(ctx context.Context, clan *Clan, leader ClanMember) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO clans (clan_id, name, leader_character_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		clan.ID,
		normalizeClanName(clan.Name),
		clan.LeaderCharacterID,
		clan.CreatedAt,
		clan.UpdatedAt,
	); err != nil {
		return mapPostgresError(err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO clan_members (clan_id, character_id, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		leader.ClanID,
		leader.CharacterID,
		leader.JoinedAt,
		leader.CreatedAt,
		leader.UpdatedAt,
	); err != nil {
		return mapPostgresError(err)
	}

	return tx.Commit()
}

func (p *postgresStoreBackend) AddClanMember(ctx context.Context, member *ClanMember) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO clan_members (clan_id, character_id, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		member.ClanID,
		member.CharacterID,
		member.JoinedAt,
		member.CreatedAt,
		member.UpdatedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) RemoveClanMember(ctx context.Context, clanID string, characterID string) error {
	result, err := p.db.ExecContext(
		ctx,
		`DELETE FROM clan_members
		 WHERE clan_id = $1
		   AND character_id = $2`,
		clanID,
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

func (p *postgresStoreBackend) DeleteClan(ctx context.Context, clanID string) error {
	result, err := p.db.ExecContext(ctx, `DELETE FROM clans WHERE clan_id = $1`, clanID)
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

func (p *postgresStoreBackend) listClanInvites(ctx context.Context, query string, arg any, now time.Time) ([]ClanInvite, error) {
	rows, err := p.db.QueryContext(ctx, query, arg, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invites := make([]ClanInvite, 0)
	for rows.Next() {
		var invite ClanInvite
		if err := rows.Scan(&invite.ID, &invite.ClanID, &invite.InviterCharacterID, &invite.InviteeCharacterID, &invite.ExpiresAt, &invite.CreatedAt, &invite.UpdatedAt); err != nil {
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

func (p *postgresStoreBackend) ListPendingClanInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]ClanInvite, error) {
	return p.listClanInvites(
		ctx,
		`SELECT invite_id, clan_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM clan_invites
		 WHERE invitee_character_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		characterID,
		now,
	)
}

func (p *postgresStoreBackend) ListPendingClanInvitesByInviter(ctx context.Context, characterID string, now time.Time) ([]ClanInvite, error) {
	return p.listClanInvites(
		ctx,
		`SELECT invite_id, clan_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM clan_invites
		 WHERE inviter_character_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		characterID,
		now,
	)
}

func (p *postgresStoreBackend) ListPendingClanInvitesByClan(ctx context.Context, clanID string, now time.Time) ([]ClanInvite, error) {
	return p.listClanInvites(
		ctx,
		`SELECT invite_id, clan_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM clan_invites
		 WHERE clan_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		clanID,
		now,
	)
}

func (p *postgresStoreBackend) GetClanInviteByID(ctx context.Context, inviteID string) (*ClanInvite, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT invite_id, clan_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM clan_invites
		 WHERE invite_id = $1`,
		inviteID,
	)
	invite := &ClanInvite{}
	if err := row.Scan(&invite.ID, &invite.ClanID, &invite.InviterCharacterID, &invite.InviteeCharacterID, &invite.ExpiresAt, &invite.CreatedAt, &invite.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return invite, nil
}

func (p *postgresStoreBackend) CreateClanInvite(ctx context.Context, invite *ClanInvite) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO clan_invites (invite_id, clan_id, inviter_character_id, invitee_character_id, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		invite.ID,
		invite.ClanID,
		invite.InviterCharacterID,
		invite.InviteeCharacterID,
		invite.ExpiresAt,
		invite.CreatedAt,
		invite.UpdatedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) AcceptClanInvite(ctx context.Context, inviteID string, member *ClanMember) error {
	if member == nil {
		return errRecordNotFound
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var clanID string
	var inviteeCharacterID string
	var expiresAt time.Time
	if err := tx.QueryRowContext(
		ctx,
		`SELECT clan_id, invitee_character_id, expires_at
		 FROM clan_invites
		 WHERE invite_id = $1
		 FOR UPDATE`,
		inviteID,
	).Scan(&clanID, &inviteeCharacterID, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errRecordNotFound
		}
		return err
	}
	if clanID != member.ClanID || inviteeCharacterID != member.CharacterID || !expiresAt.After(member.JoinedAt) {
		return errRecordNotFound
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO clan_members (clan_id, character_id, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		member.ClanID,
		member.CharacterID,
		member.JoinedAt,
		member.CreatedAt,
		member.UpdatedAt,
	); err != nil {
		return mapPostgresError(err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM clan_invites WHERE invite_id = $1`, inviteID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return errRecordNotFound
	}
	return tx.Commit()
}

func (p *postgresStoreBackend) DeleteClanInvite(ctx context.Context, inviteID string) error {
	result, err := p.db.ExecContext(ctx, `DELETE FROM clan_invites WHERE invite_id = $1`, inviteID)
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

func (p *postgresStoreBackend) DeleteClanInvitesByClan(ctx context.Context, clanID string) error {
	result, err := p.db.ExecContext(ctx, `DELETE FROM clan_invites WHERE clan_id = $1`, clanID)
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

func (p *postgresStoreBackend) DeletePendingClanInviteForInvitee(ctx context.Context, clanID string, inviteeCharacterID string) error {
	result, err := p.db.ExecContext(
		ctx,
		`DELETE FROM clan_invites
		 WHERE clan_id = $1
		   AND invitee_character_id = $2`,
		clanID,
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

func (p *postgresStoreBackend) ExpireClanInvites(ctx context.Context, now time.Time) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM clan_invites WHERE expires_at <= $1`, now)
	return err
}

type postgresClanRepo struct{ backend *postgresStoreBackend }

func (repo postgresClanRepo) GetByID(ctx context.Context, clanID string) (*Clan, error) {
	return repo.backend.GetClanByID(ctx, clanID)
}

func (repo postgresClanRepo) GetByName(ctx context.Context, name string) (*Clan, error) {
	return repo.backend.GetClanByName(ctx, name)
}

func (repo postgresClanRepo) GetByCharacterID(ctx context.Context, characterID string) (*Clan, error) {
	return repo.backend.GetClanByCharacterID(ctx, characterID)
}

func (repo postgresClanRepo) ListMembers(ctx context.Context, clanID string) ([]ClanMember, error) {
	return repo.backend.ListClanMembers(ctx, clanID)
}

func (repo postgresClanRepo) Create(ctx context.Context, clan *Clan, leader ClanMember) error {
	return repo.backend.CreateClan(ctx, clan, leader)
}

func (repo postgresClanRepo) AddMember(ctx context.Context, member *ClanMember) error {
	return repo.backend.AddClanMember(ctx, member)
}

func (repo postgresClanRepo) RemoveMember(ctx context.Context, clanID string, characterID string) error {
	return repo.backend.RemoveClanMember(ctx, clanID, characterID)
}

func (repo postgresClanRepo) Delete(ctx context.Context, clanID string) error {
	return repo.backend.DeleteClan(ctx, clanID)
}

func (repo postgresClanRepo) ListPendingInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]ClanInvite, error) {
	return repo.backend.ListPendingClanInvitesByInvitee(ctx, characterID, now)
}

func (repo postgresClanRepo) ListPendingInvitesByInviter(ctx context.Context, characterID string, now time.Time) ([]ClanInvite, error) {
	return repo.backend.ListPendingClanInvitesByInviter(ctx, characterID, now)
}

func (repo postgresClanRepo) ListPendingInvitesByClan(ctx context.Context, clanID string, now time.Time) ([]ClanInvite, error) {
	return repo.backend.ListPendingClanInvitesByClan(ctx, clanID, now)
}

func (repo postgresClanRepo) GetInviteByID(ctx context.Context, inviteID string) (*ClanInvite, error) {
	return repo.backend.GetClanInviteByID(ctx, inviteID)
}

func (repo postgresClanRepo) CreateInvite(ctx context.Context, invite *ClanInvite) error {
	return repo.backend.CreateClanInvite(ctx, invite)
}

func (repo postgresClanRepo) AcceptInvite(ctx context.Context, inviteID string, member *ClanMember) error {
	return repo.backend.AcceptClanInvite(ctx, inviteID, member)
}

func (repo postgresClanRepo) DeleteInvite(ctx context.Context, inviteID string) error {
	return repo.backend.DeleteClanInvite(ctx, inviteID)
}

func (repo postgresClanRepo) DeleteInvitesByClan(ctx context.Context, clanID string) error {
	return repo.backend.DeleteClanInvitesByClan(ctx, clanID)
}

func (repo postgresClanRepo) DeletePendingInviteForInvitee(ctx context.Context, clanID string, inviteeCharacterID string) error {
	return repo.backend.DeletePendingClanInviteForInvitee(ctx, clanID, inviteeCharacterID)
}

func (repo postgresClanRepo) ExpireInvites(ctx context.Context, now time.Time) error {
	return repo.backend.ExpireClanInvites(ctx, now)
}

func sortClanMemberSnapshots(members []CharacterClanMemberSnapshot) {
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

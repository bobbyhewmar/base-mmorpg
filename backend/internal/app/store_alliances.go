package app

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

func (repo memoryAllianceRepo) GetByID(_ context.Context, allianceID string) (*Alliance, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	alliance, exists := repo.backend.alliances[allianceID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneAlliance(alliance), nil
}

func (repo memoryAllianceRepo) GetByName(_ context.Context, name string) (*Alliance, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	allianceID, exists := repo.backend.allianceByName[normalizedAllianceLookupKey(name)]
	if !exists {
		return nil, errRecordNotFound
	}
	alliance, exists := repo.backend.alliances[allianceID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneAlliance(alliance), nil
}

func (repo memoryAllianceRepo) GetByClanID(_ context.Context, clanID string) (*Alliance, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	allianceID, exists := repo.backend.allianceByClan[clanID]
	if !exists {
		return nil, errRecordNotFound
	}
	alliance, exists := repo.backend.alliances[allianceID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneAlliance(alliance), nil
}

func (repo memoryAllianceRepo) GetByCharacterID(_ context.Context, characterID string) (*Alliance, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	clanID, exists := repo.backend.clanByCharacter[characterID]
	if !exists {
		return nil, errRecordNotFound
	}
	allianceID, exists := repo.backend.allianceByClan[clanID]
	if !exists {
		return nil, errRecordNotFound
	}
	alliance, exists := repo.backend.alliances[allianceID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneAlliance(alliance), nil
}

func (repo memoryAllianceRepo) ListMembers(_ context.Context, allianceID string) ([]AllianceMember, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	members, exists := repo.backend.allianceMembers[allianceID]
	if !exists || len(members) == 0 {
		return nil, errRecordNotFound
	}
	return normalizeAllianceMembers(members), nil
}

func (repo memoryAllianceRepo) Create(_ context.Context, alliance *Alliance, founder AllianceMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if alliance == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.alliances[alliance.ID]; exists {
		return errRecordConflict
	}
	nameKey := normalizedAllianceLookupKey(alliance.Name)
	if nameKey == "" {
		return errRecordNotFound
	}
	if _, exists := repo.backend.allianceByName[nameKey]; exists {
		return errRecordConflict
	}
	if founder.ClanID != "" {
		if _, exists := repo.backend.clans[founder.ClanID]; !exists {
			return errRecordNotFound
		}
		if _, exists := repo.backend.allianceByClan[founder.ClanID]; exists {
			return errRecordConflict
		}
	}

	allianceCopy := *alliance
	allianceCopy.Name = normalizeAllianceName(alliance.Name)
	repo.backend.alliances[alliance.ID] = &allianceCopy
	repo.backend.allianceByName[nameKey] = alliance.ID
	if founder.ClanID != "" {
		memberCopy := founder
		repo.backend.allianceMembers[alliance.ID] = []AllianceMember{memberCopy}
		repo.backend.allianceByClan[founder.ClanID] = alliance.ID
	}
	return nil
}

func (repo memoryAllianceRepo) AddMember(_ context.Context, member *AllianceMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if member == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.alliances[member.AllianceID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clans[member.ClanID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.allianceByClan[member.ClanID]; exists {
		return errRecordConflict
	}

	memberCopy := *member
	repo.backend.allianceMembers[member.AllianceID] = append(repo.backend.allianceMembers[member.AllianceID], memberCopy)
	repo.backend.allianceByClan[member.ClanID] = member.AllianceID
	return nil
}

func (repo memoryAllianceRepo) AcceptInvite(_ context.Context, inviteID string, member *AllianceMember) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if member == nil {
		return errRecordNotFound
	}
	invite, exists := repo.backend.allianceInvites[inviteID]
	if !exists || invite == nil || invite.AllianceID != member.AllianceID || invite.TargetClanID != member.ClanID || !invite.ExpiresAt.After(member.JoinedAt) {
		return errRecordNotFound
	}
	if _, exists := repo.backend.alliances[member.AllianceID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clans[member.ClanID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.allianceByClan[member.ClanID]; exists {
		return errRecordConflict
	}

	memberCopy := *member
	repo.backend.allianceMembers[member.AllianceID] = append(repo.backend.allianceMembers[member.AllianceID], memberCopy)
	repo.backend.allianceByClan[member.ClanID] = member.AllianceID
	delete(repo.backend.allianceInvites, inviteID)
	return nil
}

func (repo memoryAllianceRepo) RemoveMember(_ context.Context, allianceID string, clanID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	members, exists := repo.backend.allianceMembers[allianceID]
	if !exists || len(members) == 0 {
		return errRecordNotFound
	}

	updated := members[:0]
	removed := false
	for _, member := range members {
		if member.ClanID == clanID {
			removed = true
			delete(repo.backend.allianceByClan, clanID)
			continue
		}
		updated = append(updated, member)
	}
	if !removed {
		return errRecordNotFound
	}
	repo.backend.allianceMembers[allianceID] = append([]AllianceMember(nil), updated...)
	return nil
}

func (repo memoryAllianceRepo) Delete(_ context.Context, allianceID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	alliance, exists := repo.backend.alliances[allianceID]
	if !exists {
		return errRecordNotFound
	}
	for _, member := range repo.backend.allianceMembers[allianceID] {
		delete(repo.backend.allianceByClan, member.ClanID)
	}
	for inviteID, invite := range repo.backend.allianceInvites {
		if invite == nil || invite.AllianceID != allianceID {
			continue
		}
		delete(repo.backend.allianceInvites, inviteID)
	}
	delete(repo.backend.allianceMembers, allianceID)
	delete(repo.backend.allianceByName, normalizedAllianceLookupKey(alliance.Name))
	delete(repo.backend.alliances, allianceID)
	return nil
}

func (repo memoryAllianceRepo) listPendingInvites(filter func(*AllianceInvite) bool, now time.Time) ([]AllianceInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invites := make([]AllianceInvite, 0)
	for _, invite := range repo.backend.allianceInvites {
		if invite == nil || !invite.ExpiresAt.After(now) || !filter(invite) {
			continue
		}
		invites = append(invites, *invite)
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return normalizeAllianceInvites(invites), nil
}

func (repo memoryAllianceRepo) ListPendingInvitesByInvitee(_ context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return repo.listPendingInvites(func(invite *AllianceInvite) bool {
		return invite.InviteeCharacterID == characterID
	}, now)
}

func (repo memoryAllianceRepo) ListPendingInvitesByInviter(_ context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return repo.listPendingInvites(func(invite *AllianceInvite) bool {
		return invite.InviterCharacterID == characterID
	}, now)
}

func (repo memoryAllianceRepo) ListPendingInvitesByAlliance(_ context.Context, allianceID string, now time.Time) ([]AllianceInvite, error) {
	return repo.listPendingInvites(func(invite *AllianceInvite) bool {
		return invite.AllianceID == allianceID
	}, now)
}

func (repo memoryAllianceRepo) ListPendingInvitesByTargetClan(_ context.Context, clanID string, now time.Time) ([]AllianceInvite, error) {
	return repo.listPendingInvites(func(invite *AllianceInvite) bool {
		return invite.TargetClanID == clanID
	}, now)
}

func (repo memoryAllianceRepo) ListExpiredInvitesByInvitee(_ context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invites := make([]AllianceInvite, 0)
	for _, invite := range repo.backend.allianceInvites {
		if invite == nil || invite.InviteeCharacterID != characterID || invite.ExpiresAt.After(now) {
			continue
		}
		invites = append(invites, *invite)
	}
	if len(invites) == 0 {
		return nil, errRecordNotFound
	}
	return normalizeAllianceInvites(invites), nil
}

func (repo memoryAllianceRepo) GetInviteByID(_ context.Context, inviteID string) (*AllianceInvite, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	invite, exists := repo.backend.allianceInvites[inviteID]
	if !exists {
		return nil, errRecordNotFound
	}
	cloned := *invite
	return &cloned, nil
}

func (repo memoryAllianceRepo) CreateInvite(_ context.Context, invite *AllianceInvite) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if invite == nil {
		return errRecordNotFound
	}
	if _, exists := repo.backend.allianceInvites[invite.ID]; exists {
		return errRecordConflict
	}
	if _, exists := repo.backend.alliances[invite.AllianceID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clans[invite.InviterClanID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.clans[invite.TargetClanID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.characters[invite.InviterCharacterID]; !exists {
		return errRecordNotFound
	}
	if _, exists := repo.backend.characters[invite.InviteeCharacterID]; !exists {
		return errRecordNotFound
	}
	for _, existing := range repo.backend.allianceInvites {
		if existing == nil || !existing.ExpiresAt.After(invite.CreatedAt) {
			continue
		}
		if existing.AllianceID == invite.AllianceID || existing.TargetClanID == invite.TargetClanID {
			return errRecordConflict
		}
	}
	cloned := *invite
	repo.backend.allianceInvites[invite.ID] = &cloned
	return nil
}

func (repo memoryAllianceRepo) DeleteInvite(_ context.Context, inviteID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.allianceInvites[inviteID]; !exists {
		return errRecordNotFound
	}
	delete(repo.backend.allianceInvites, inviteID)
	return nil
}

func (repo memoryAllianceRepo) DeleteInvitesByAlliance(_ context.Context, allianceID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	removed := false
	for inviteID, invite := range repo.backend.allianceInvites {
		if invite == nil {
			delete(repo.backend.allianceInvites, inviteID)
			continue
		}
		if invite.AllianceID == allianceID {
			delete(repo.backend.allianceInvites, inviteID)
			removed = true
		}
	}
	if !removed {
		return errRecordNotFound
	}
	return nil
}

func (repo memoryAllianceRepo) DeletePendingInviteForTargetClan(_ context.Context, allianceID string, targetClanID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	removed := false
	for inviteID, invite := range repo.backend.allianceInvites {
		if invite == nil {
			delete(repo.backend.allianceInvites, inviteID)
			continue
		}
		if invite.AllianceID == allianceID && invite.TargetClanID == targetClanID {
			delete(repo.backend.allianceInvites, inviteID)
			removed = true
		}
	}
	if !removed {
		return errRecordNotFound
	}
	return nil
}

func (repo memoryAllianceRepo) ExpireInvites(_ context.Context, now time.Time) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	for inviteID, invite := range repo.backend.allianceInvites {
		if invite == nil {
			delete(repo.backend.allianceInvites, inviteID)
			continue
		}
		if !invite.ExpiresAt.After(now) {
			delete(repo.backend.allianceInvites, inviteID)
		}
	}
	return nil
}

func (p *postgresStoreBackend) GetAllianceByID(ctx context.Context, allianceID string) (*Alliance, error) {
	row := postgresExecutorFromContext(ctx, p.db).QueryRowContext(
		ctx,
		`SELECT alliance_id, name, leader_clan_id, created_at, updated_at
		 FROM alliances
		 WHERE alliance_id = $1`,
		allianceID,
	)
	alliance := &Alliance{}
	if err := row.Scan(&alliance.ID, &alliance.Name, &alliance.LeaderClanID, &alliance.CreatedAt, &alliance.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return alliance, nil
}

func (p *postgresStoreBackend) GetAllianceByName(ctx context.Context, name string) (*Alliance, error) {
	row := postgresExecutorFromContext(ctx, p.db).QueryRowContext(
		ctx,
		`SELECT alliance_id, name, leader_clan_id, created_at, updated_at
		 FROM alliances
		 WHERE LOWER(BTRIM(name)) = LOWER(BTRIM($1))`,
		normalizeAllianceName(name),
	)
	alliance := &Alliance{}
	if err := row.Scan(&alliance.ID, &alliance.Name, &alliance.LeaderClanID, &alliance.CreatedAt, &alliance.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return alliance, nil
}

func (p *postgresStoreBackend) GetAllianceByClanID(ctx context.Context, clanID string) (*Alliance, error) {
	row := postgresExecutorFromContext(ctx, p.db).QueryRowContext(
		ctx,
		`SELECT a.alliance_id, a.name, a.leader_clan_id, a.created_at, a.updated_at
		 FROM alliances a
		 JOIN alliance_members am ON am.alliance_id = a.alliance_id
		 WHERE am.clan_id = $1`,
		clanID,
	)
	alliance := &Alliance{}
	if err := row.Scan(&alliance.ID, &alliance.Name, &alliance.LeaderClanID, &alliance.CreatedAt, &alliance.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return alliance, nil
}

func (p *postgresStoreBackend) GetAllianceByCharacterID(ctx context.Context, characterID string) (*Alliance, error) {
	row := postgresExecutorFromContext(ctx, p.db).QueryRowContext(
		ctx,
		`SELECT a.alliance_id, a.name, a.leader_clan_id, a.created_at, a.updated_at
		 FROM alliances a
		 JOIN alliance_members am ON am.alliance_id = a.alliance_id
		 JOIN clan_members cm ON cm.clan_id = am.clan_id
		 WHERE cm.character_id = $1`,
		characterID,
	)
	alliance := &Alliance{}
	if err := row.Scan(&alliance.ID, &alliance.Name, &alliance.LeaderClanID, &alliance.CreatedAt, &alliance.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return alliance, nil
}

func (p *postgresStoreBackend) ListAllianceMembers(ctx context.Context, allianceID string) ([]AllianceMember, error) {
	rows, err := postgresExecutorFromContext(ctx, p.db).QueryContext(
		ctx,
		`SELECT alliance_id, clan_id, joined_at, created_at, updated_at
		 FROM alliance_members
		 WHERE alliance_id = $1
		 ORDER BY joined_at, clan_id`,
		allianceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]AllianceMember, 0)
	for rows.Next() {
		var member AllianceMember
		if err := rows.Scan(&member.AllianceID, &member.ClanID, &member.JoinedAt, &member.CreatedAt, &member.UpdatedAt); err != nil {
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

func (p *postgresStoreBackend) CreateAlliance(ctx context.Context, alliance *Alliance, founder AllianceMember) error {
	return p.RunSocialCommandTransaction(ctx, func(txCtx context.Context) error {
		executor := postgresExecutorFromContext(txCtx, p.db)
		if _, err := executor.ExecContext(
			txCtx,
			`INSERT INTO alliances (alliance_id, name, leader_clan_id, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			alliance.ID,
			normalizeAllianceName(alliance.Name),
			alliance.LeaderClanID,
			alliance.CreatedAt,
			alliance.UpdatedAt,
		); err != nil {
			return mapPostgresError(err)
		}

		if _, err := executor.ExecContext(
			txCtx,
			`INSERT INTO alliance_members (alliance_id, clan_id, joined_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			founder.AllianceID,
			founder.ClanID,
			founder.JoinedAt,
			founder.CreatedAt,
			founder.UpdatedAt,
		); err != nil {
			return mapPostgresError(err)
		}
		return nil
	})
}

func (p *postgresStoreBackend) AddAllianceMember(ctx context.Context, member *AllianceMember) error {
	_, err := postgresExecutorFromContext(ctx, p.db).ExecContext(
		ctx,
		`INSERT INTO alliance_members (alliance_id, clan_id, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		member.AllianceID,
		member.ClanID,
		member.JoinedAt,
		member.CreatedAt,
		member.UpdatedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) AcceptAllianceInvite(ctx context.Context, inviteID string, member *AllianceMember) error {
	if member == nil {
		return errRecordNotFound
	}
	return p.RunSocialCommandTransaction(ctx, func(txCtx context.Context) error {
		executor := postgresExecutorFromContext(txCtx, p.db)
		var allianceID string
		var targetClanID string
		var expiresAt time.Time
		if err := executor.QueryRowContext(
			txCtx,
			`SELECT alliance_id, target_clan_id, expires_at
			 FROM alliance_invites
			 WHERE invite_id = $1
			 FOR UPDATE`,
			inviteID,
		).Scan(&allianceID, &targetClanID, &expiresAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errRecordNotFound
			}
			return err
		}
		if allianceID != member.AllianceID || targetClanID != member.ClanID || !expiresAt.After(member.JoinedAt) {
			return errRecordNotFound
		}

		if _, err := executor.ExecContext(
			txCtx,
			`INSERT INTO alliance_members (alliance_id, clan_id, joined_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			member.AllianceID,
			member.ClanID,
			member.JoinedAt,
			member.CreatedAt,
			member.UpdatedAt,
		); err != nil {
			return mapPostgresError(err)
		}

		result, err := executor.ExecContext(txCtx, `DELETE FROM alliance_invites WHERE invite_id = $1`, inviteID)
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
		return nil
	})
}

func (p *postgresStoreBackend) RemoveAllianceMember(ctx context.Context, allianceID string, clanID string) error {
	result, err := postgresExecutorFromContext(ctx, p.db).ExecContext(
		ctx,
		`DELETE FROM alliance_members
		 WHERE alliance_id = $1
		   AND clan_id = $2`,
		allianceID,
		clanID,
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

func (p *postgresStoreBackend) DeleteAlliance(ctx context.Context, allianceID string) error {
	result, err := postgresExecutorFromContext(ctx, p.db).ExecContext(ctx, `DELETE FROM alliances WHERE alliance_id = $1`, allianceID)
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

func (p *postgresStoreBackend) listAllianceInvites(ctx context.Context, query string, arg any, now time.Time) ([]AllianceInvite, error) {
	rows, err := postgresExecutorFromContext(ctx, p.db).QueryContext(ctx, query, arg, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invites := make([]AllianceInvite, 0)
	for rows.Next() {
		var invite AllianceInvite
		if err := rows.Scan(&invite.ID, &invite.AllianceID, &invite.InviterClanID, &invite.InviterCharacterID, &invite.TargetClanID, &invite.InviteeCharacterID, &invite.ExpiresAt, &invite.CreatedAt, &invite.UpdatedAt); err != nil {
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
	sort.Slice(invites, func(i, j int) bool {
		if invites[i].ExpiresAt.Equal(invites[j].ExpiresAt) {
			return invites[i].ID < invites[j].ID
		}
		return invites[i].ExpiresAt.Before(invites[j].ExpiresAt)
	})
	return invites, nil
}

func (p *postgresStoreBackend) ListPendingAllianceInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return p.listAllianceInvites(
		ctx,
		`SELECT invite_id, alliance_id, inviter_clan_id, inviter_character_id, target_clan_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM alliance_invites
		 WHERE invitee_character_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		characterID,
		now,
	)
}

func (p *postgresStoreBackend) ListPendingAllianceInvitesByInviter(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return p.listAllianceInvites(
		ctx,
		`SELECT invite_id, alliance_id, inviter_clan_id, inviter_character_id, target_clan_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM alliance_invites
		 WHERE inviter_character_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		characterID,
		now,
	)
}

func (p *postgresStoreBackend) ListPendingAllianceInvitesByAlliance(ctx context.Context, allianceID string, now time.Time) ([]AllianceInvite, error) {
	return p.listAllianceInvites(
		ctx,
		`SELECT invite_id, alliance_id, inviter_clan_id, inviter_character_id, target_clan_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM alliance_invites
		 WHERE alliance_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		allianceID,
		now,
	)
}

func (p *postgresStoreBackend) ListPendingAllianceInvitesByTargetClan(ctx context.Context, clanID string, now time.Time) ([]AllianceInvite, error) {
	return p.listAllianceInvites(
		ctx,
		`SELECT invite_id, alliance_id, inviter_clan_id, inviter_character_id, target_clan_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM alliance_invites
		 WHERE target_clan_id = $1
		   AND expires_at > $2
		 ORDER BY expires_at, invite_id`,
		clanID,
		now,
	)
}

func (p *postgresStoreBackend) ListExpiredAllianceInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return p.listAllianceInvites(
		ctx,
		`SELECT invite_id, alliance_id, inviter_clan_id, inviter_character_id, target_clan_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM alliance_invites
		 WHERE invitee_character_id = $1
		   AND expires_at <= $2
		 ORDER BY expires_at, invite_id`,
		characterID,
		now,
	)
}

func (p *postgresStoreBackend) GetAllianceInviteByID(ctx context.Context, inviteID string) (*AllianceInvite, error) {
	row := postgresExecutorFromContext(ctx, p.db).QueryRowContext(
		ctx,
		`SELECT invite_id, alliance_id, inviter_clan_id, inviter_character_id, target_clan_id, invitee_character_id, expires_at, created_at, updated_at
		 FROM alliance_invites
		 WHERE invite_id = $1`,
		inviteID,
	)
	invite := &AllianceInvite{}
	if err := row.Scan(&invite.ID, &invite.AllianceID, &invite.InviterClanID, &invite.InviterCharacterID, &invite.TargetClanID, &invite.InviteeCharacterID, &invite.ExpiresAt, &invite.CreatedAt, &invite.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return invite, nil
}

func (p *postgresStoreBackend) CreateAllianceInvite(ctx context.Context, invite *AllianceInvite) error {
	_, err := postgresExecutorFromContext(ctx, p.db).ExecContext(
		ctx,
		`INSERT INTO alliance_invites (invite_id, alliance_id, inviter_clan_id, inviter_character_id, target_clan_id, invitee_character_id, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		invite.ID,
		invite.AllianceID,
		invite.InviterClanID,
		invite.InviterCharacterID,
		invite.TargetClanID,
		invite.InviteeCharacterID,
		invite.ExpiresAt,
		invite.CreatedAt,
		invite.UpdatedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) DeleteAllianceInvite(ctx context.Context, inviteID string) error {
	result, err := postgresExecutorFromContext(ctx, p.db).ExecContext(ctx, `DELETE FROM alliance_invites WHERE invite_id = $1`, inviteID)
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

func (p *postgresStoreBackend) DeleteAllianceInvitesByAlliance(ctx context.Context, allianceID string) error {
	result, err := postgresExecutorFromContext(ctx, p.db).ExecContext(ctx, `DELETE FROM alliance_invites WHERE alliance_id = $1`, allianceID)
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

func (p *postgresStoreBackend) DeletePendingAllianceInviteForTargetClan(ctx context.Context, allianceID string, targetClanID string) error {
	result, err := postgresExecutorFromContext(ctx, p.db).ExecContext(
		ctx,
		`DELETE FROM alliance_invites
		 WHERE alliance_id = $1
		   AND target_clan_id = $2`,
		allianceID,
		targetClanID,
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

func (p *postgresStoreBackend) ExpireAllianceInvites(ctx context.Context, now time.Time) error {
	_, err := postgresExecutorFromContext(ctx, p.db).ExecContext(ctx, `DELETE FROM alliance_invites WHERE expires_at <= $1`, now)
	return err
}

type postgresAllianceRepo struct{ backend *postgresStoreBackend }

func (repo postgresAllianceRepo) GetByID(ctx context.Context, allianceID string) (*Alliance, error) {
	return repo.backend.GetAllianceByID(ctx, allianceID)
}

func (repo postgresAllianceRepo) GetByName(ctx context.Context, name string) (*Alliance, error) {
	return repo.backend.GetAllianceByName(ctx, name)
}

func (repo postgresAllianceRepo) GetByClanID(ctx context.Context, clanID string) (*Alliance, error) {
	return repo.backend.GetAllianceByClanID(ctx, clanID)
}

func (repo postgresAllianceRepo) GetByCharacterID(ctx context.Context, characterID string) (*Alliance, error) {
	return repo.backend.GetAllianceByCharacterID(ctx, characterID)
}

func (repo postgresAllianceRepo) ListMembers(ctx context.Context, allianceID string) ([]AllianceMember, error) {
	return repo.backend.ListAllianceMembers(ctx, allianceID)
}

func (repo postgresAllianceRepo) Create(ctx context.Context, alliance *Alliance, founder AllianceMember) error {
	return repo.backend.CreateAlliance(ctx, alliance, founder)
}

func (repo postgresAllianceRepo) AddMember(ctx context.Context, member *AllianceMember) error {
	return repo.backend.AddAllianceMember(ctx, member)
}

func (repo postgresAllianceRepo) AcceptInvite(ctx context.Context, inviteID string, member *AllianceMember) error {
	return repo.backend.AcceptAllianceInvite(ctx, inviteID, member)
}

func (repo postgresAllianceRepo) RemoveMember(ctx context.Context, allianceID string, clanID string) error {
	return repo.backend.RemoveAllianceMember(ctx, allianceID, clanID)
}

func (repo postgresAllianceRepo) Delete(ctx context.Context, allianceID string) error {
	return repo.backend.DeleteAlliance(ctx, allianceID)
}

func (repo postgresAllianceRepo) ListPendingInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return repo.backend.ListPendingAllianceInvitesByInvitee(ctx, characterID, now)
}

func (repo postgresAllianceRepo) ListPendingInvitesByInviter(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return repo.backend.ListPendingAllianceInvitesByInviter(ctx, characterID, now)
}

func (repo postgresAllianceRepo) ListPendingInvitesByAlliance(ctx context.Context, allianceID string, now time.Time) ([]AllianceInvite, error) {
	return repo.backend.ListPendingAllianceInvitesByAlliance(ctx, allianceID, now)
}

func (repo postgresAllianceRepo) ListPendingInvitesByTargetClan(ctx context.Context, clanID string, now time.Time) ([]AllianceInvite, error) {
	return repo.backend.ListPendingAllianceInvitesByTargetClan(ctx, clanID, now)
}

func (repo postgresAllianceRepo) ListExpiredInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error) {
	return repo.backend.ListExpiredAllianceInvitesByInvitee(ctx, characterID, now)
}

func (repo postgresAllianceRepo) GetInviteByID(ctx context.Context, inviteID string) (*AllianceInvite, error) {
	return repo.backend.GetAllianceInviteByID(ctx, inviteID)
}

func (repo postgresAllianceRepo) CreateInvite(ctx context.Context, invite *AllianceInvite) error {
	return repo.backend.CreateAllianceInvite(ctx, invite)
}

func (repo postgresAllianceRepo) DeleteInvite(ctx context.Context, inviteID string) error {
	return repo.backend.DeleteAllianceInvite(ctx, inviteID)
}

func (repo postgresAllianceRepo) DeleteInvitesByAlliance(ctx context.Context, allianceID string) error {
	return repo.backend.DeleteAllianceInvitesByAlliance(ctx, allianceID)
}

func (repo postgresAllianceRepo) DeletePendingInviteForTargetClan(ctx context.Context, allianceID string, targetClanID string) error {
	return repo.backend.DeletePendingAllianceInviteForTargetClan(ctx, allianceID, targetClanID)
}

func (repo postgresAllianceRepo) ExpireInvites(ctx context.Context, now time.Time) error {
	return repo.backend.ExpireAllianceInvites(ctx, now)
}

func sortAllianceMemberSnapshots(members []CharacterAllianceMemberSnapshot) {
	sort.Slice(members, func(i, j int) bool {
		if members[i].IsLeaderClan != members[j].IsLeaderClan {
			return members[i].IsLeaderClan
		}
		if members[i].MemberCount == members[j].MemberCount {
			return members[i].Name < members[j].Name
		}
		return members[i].MemberCount > members[j].MemberCount
	})
}

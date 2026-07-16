CREATE TABLE IF NOT EXISTS schema_bootstrap (
  bootstrap_key TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS accounts (
  account_id TEXT PRIMARY KEY,
  login TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  state TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS account_credentials (
  account_id TEXT PRIMARY KEY REFERENCES accounts(account_id) ON DELETE CASCADE,
  password_hash TEXT NOT NULL,
  password_algorithm TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS account_sessions (
  access_token TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(account_id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_sessions_account_id ON account_sessions(account_id);
CREATE INDEX IF NOT EXISTS idx_account_sessions_expires_at ON account_sessions(expires_at);

CREATE TABLE IF NOT EXISTS characters (
  character_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(account_id) ON DELETE CASCADE,
  name TEXT NOT NULL UNIQUE,
  race TEXT NOT NULL,
  base_class TEXT NOT NULL,
  sex TEXT NOT NULL,
  hair_style INTEGER NOT NULL DEFAULT 0,
  hair_color TEXT NOT NULL DEFAULT '#6b4e37',
  skin_type INTEGER NOT NULL DEFAULT 0,
  level INTEGER NOT NULL DEFAULT 1,
  xp INTEGER NOT NULL DEFAULT 0,
  current_cp INTEGER NOT NULL DEFAULT 80,
  current_hp INTEGER NOT NULL DEFAULT 122,
  current_mp INTEGER NOT NULL DEFAULT 58,
  pvp_kills INTEGER NOT NULL DEFAULT 0,
  pk_count INTEGER NOT NULL DEFAULT 0,
  karma INTEGER NOT NULL DEFAULT 0,
  pvp_flag_until TIMESTAMPTZ NULL,
  last_region_id TEXT NOT NULL,
  is_enterable BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE characters ADD COLUMN IF NOT EXISTS current_position_x DOUBLE PRECISION NOT NULL DEFAULT -8;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS current_position_z DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS xp INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS current_cp INTEGER NOT NULL DEFAULT 80;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS current_hp INTEGER NOT NULL DEFAULT 122;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS current_mp INTEGER NOT NULL DEFAULT 58;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS pvp_kills INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS pk_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS karma INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS pvp_flag_until TIMESTAMPTZ NULL;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS hair_style INTEGER NOT NULL DEFAULT 0;
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_name = 'characters'
      AND column_name = 'hair_color'
      AND data_type <> 'text'
  ) AND NOT EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_name = 'characters'
      AND column_name = 'legacy_hair_color_index'
  ) THEN
    ALTER TABLE characters RENAME COLUMN hair_color TO legacy_hair_color_index;
  END IF;
END $$;
ALTER TABLE characters ADD COLUMN IF NOT EXISTS hair_color TEXT NOT NULL DEFAULT '#6b4e37';
ALTER TABLE characters ADD COLUMN IF NOT EXISTS skin_type INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_characters_account_id ON characters(account_id);
CREATE INDEX IF NOT EXISTS idx_characters_pvp_flag_until ON characters(pvp_flag_until) WHERE pvp_flag_until IS NOT NULL;

CREATE TABLE IF NOT EXISTS pvp_combat_events (
  event_id TEXT PRIMARY KEY,
  attacker_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  attacker_account_id TEXT NULL REFERENCES accounts(account_id) ON DELETE SET NULL,
  victim_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  victim_account_id TEXT NULL REFERENCES accounts(account_id) ON DELETE SET NULL,
  action_type TEXT NOT NULL,
  skill_id TEXT NULL,
  damage INTEGER NOT NULL,
  cp_damage INTEGER NOT NULL,
  hp_damage INTEGER NOT NULL,
  result TEXT NOT NULL,
  killer_character_id TEXT NULL REFERENCES characters(character_id) ON DELETE SET NULL,
  assist_character_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  suspicious BOOLEAN NOT NULL DEFAULT FALSE,
  repeated_kill_count INTEGER NOT NULL DEFAULT 0,
  attacker_flagged_before BOOLEAN NOT NULL,
  attacker_flagged_after BOOLEAN NOT NULL,
  victim_flagged_before BOOLEAN NOT NULL,
  victim_flagged_after BOOLEAN NOT NULL,
  pvp_kills_before INTEGER NOT NULL,
  pvp_kills_after INTEGER NOT NULL,
  pk_count_before INTEGER NOT NULL,
  pk_count_after INTEGER NOT NULL,
  karma_before INTEGER NOT NULL,
  karma_after INTEGER NOT NULL,
  karma_delta INTEGER NOT NULL,
  session_id TEXT NULL,
  command_id TEXT NULL,
  command_seq INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE pvp_combat_events ADD COLUMN IF NOT EXISTS killer_character_id TEXT NULL REFERENCES characters(character_id) ON DELETE SET NULL;
ALTER TABLE pvp_combat_events ADD COLUMN IF NOT EXISTS assist_character_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE pvp_combat_events ADD COLUMN IF NOT EXISTS suspicious BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE pvp_combat_events ADD COLUMN IF NOT EXISTS repeated_kill_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_pvp_combat_events_attacker ON pvp_combat_events(attacker_character_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pvp_combat_events_victim ON pvp_combat_events(victim_character_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pvp_combat_events_result ON pvp_combat_events(result, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pvp_combat_events_killer ON pvp_combat_events(killer_character_id, created_at DESC) WHERE killer_character_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_pvp_combat_events_repeated_pair ON pvp_combat_events(attacker_character_id, victim_character_id, created_at DESC) WHERE result IN ('pvp_kill', 'pk_kill');
CREATE INDEX IF NOT EXISTS idx_pvp_combat_events_suspicious ON pvp_combat_events(created_at DESC) WHERE suspicious = TRUE;
CREATE UNIQUE INDEX IF NOT EXISTS idx_pvp_combat_events_command_once ON pvp_combat_events(session_id, command_seq) WHERE session_id IS NOT NULL AND command_seq > 0;

CREATE TABLE IF NOT EXISTS character_skill_cooldowns (
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  skill_id TEXT NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (character_id, skill_id)
);

CREATE INDEX IF NOT EXISTS idx_character_skill_cooldowns_character_id ON character_skill_cooldowns(character_id);
CREATE INDEX IF NOT EXISTS idx_character_skill_cooldowns_ends_at ON character_skill_cooldowns(ends_at);

CREATE TABLE IF NOT EXISTS character_hotbar_loadouts (
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  slot_index INTEGER NOT NULL,
  entry_type TEXT NULL,
  skill_id TEXT NULL,
  item_instance_id TEXT NULL,
  action_id TEXT NULL,
  open_bar_count INTEGER NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (character_id, slot_index),
  CONSTRAINT chk_character_hotbar_slot_index CHECK (slot_index >= 0 AND slot_index < 36),
  CONSTRAINT chk_character_hotbar_open_bar_count CHECK (open_bar_count >= 1 AND open_bar_count <= 3),
  CONSTRAINT chk_character_hotbar_binding_shape CHECK (
    (entry_type IS NULL AND skill_id IS NULL AND item_instance_id IS NULL AND action_id IS NULL)
    OR (entry_type = 'skill' AND skill_id IS NOT NULL AND item_instance_id IS NULL AND action_id IS NULL)
    OR (entry_type = 'item' AND skill_id IS NULL AND item_instance_id IS NOT NULL AND action_id IS NULL)
    OR (entry_type = 'action' AND skill_id IS NULL AND item_instance_id IS NULL AND action_id IS NOT NULL)
  )
);

ALTER TABLE character_hotbar_loadouts ADD COLUMN IF NOT EXISTS item_instance_id TEXT NULL;
ALTER TABLE character_hotbar_loadouts ADD COLUMN IF NOT EXISTS action_id TEXT NULL;
ALTER TABLE character_hotbar_loadouts DROP CONSTRAINT IF EXISTS chk_character_hotbar_binding_shape;
ALTER TABLE character_hotbar_loadouts ADD CONSTRAINT chk_character_hotbar_binding_shape CHECK (
  (entry_type IS NULL AND skill_id IS NULL AND item_instance_id IS NULL AND action_id IS NULL)
  OR (entry_type = 'skill' AND skill_id IS NOT NULL AND item_instance_id IS NULL AND action_id IS NULL)
  OR (entry_type = 'item' AND skill_id IS NULL AND item_instance_id IS NOT NULL AND action_id IS NULL)
  OR (entry_type = 'action' AND skill_id IS NULL AND item_instance_id IS NULL AND action_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_character_hotbar_loadouts_character_id ON character_hotbar_loadouts(character_id);

CREATE TABLE IF NOT EXISTS character_pets (
  pet_instance_id TEXT PRIMARY KEY,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  pet_template_id TEXT NOT NULL,
  custom_name TEXT NULL,
  is_summoned BOOLEAN NOT NULL DEFAULT FALSE,
  is_mounted BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_character_pets_mounted_requires_summoned CHECK (NOT is_mounted OR is_summoned)
);

CREATE INDEX IF NOT EXISTS idx_character_pets_character_id ON character_pets(character_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_character_pets_character_template ON character_pets(character_id, pet_template_id);

CREATE TABLE IF NOT EXISTS character_quests (
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  quest_id TEXT NOT NULL,
  status TEXT NOT NULL,
  progress INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (character_id, quest_id),
  CONSTRAINT chk_character_quests_progress_non_negative CHECK (progress >= 0)
);

CREATE INDEX IF NOT EXISTS idx_character_quests_character_id ON character_quests(character_id);

CREATE TABLE IF NOT EXISTS clans (
  clan_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  leader_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_clans_name_normalized ON clans((LOWER(BTRIM(name))));
CREATE INDEX IF NOT EXISTS idx_clans_leader_character_id ON clans(leader_character_id);

CREATE TABLE IF NOT EXISTS clan_members (
  clan_id TEXT NOT NULL REFERENCES clans(clan_id) ON DELETE CASCADE,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  joined_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (clan_id, character_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_clan_members_character_id ON clan_members(character_id);
CREATE INDEX IF NOT EXISTS idx_clan_members_clan_id ON clan_members(clan_id, joined_at, character_id);

CREATE TABLE IF NOT EXISTS clan_invites (
  invite_id TEXT PRIMARY KEY,
  clan_id TEXT NOT NULL REFERENCES clans(clan_id) ON DELETE CASCADE,
  inviter_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  invitee_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clan_invites_clan_id ON clan_invites(clan_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_clan_invites_invitee_character_id ON clan_invites(invitee_character_id, expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_clan_invites_clan_unique_pending ON clan_invites(clan_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_clan_invites_invitee_unique_pending ON clan_invites(invitee_character_id);

CREATE TABLE IF NOT EXISTS parties (
  party_id TEXT PRIMARY KEY,
  leader_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_parties_leader_character_id ON parties(leader_character_id);

CREATE TABLE IF NOT EXISTS party_members (
  party_id TEXT NOT NULL REFERENCES parties(party_id) ON DELETE CASCADE,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  joined_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (party_id, character_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_party_members_character_id ON party_members(character_id);
CREATE INDEX IF NOT EXISTS idx_party_members_party_id ON party_members(party_id, joined_at, character_id);

CREATE TABLE IF NOT EXISTS party_invites (
  invite_id TEXT PRIMARY KEY,
  party_id TEXT NOT NULL REFERENCES parties(party_id) ON DELETE CASCADE,
  inviter_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  invitee_character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_party_invites_party_id ON party_invites(party_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_party_invites_invitee_character_id ON party_invites(invitee_character_id, expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_party_invites_invitee_unique_pending ON party_invites(invitee_character_id);

CREATE TABLE IF NOT EXISTS chat_messages (
  chat_message_id TEXT PRIMARY KEY,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  account_id TEXT NULL REFERENCES accounts(account_id) ON DELETE CASCADE,
  channel TEXT NOT NULL,
  target_character_id TEXT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  region_id TEXT NULL,
  text TEXT NOT NULL,
  session_id TEXT NULL,
  command_id TEXT NULL,
  command_seq INTEGER NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_character_id ON chat_messages(character_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chat_messages_target_character_id ON chat_messages(target_character_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chat_messages_channel ON chat_messages(channel, created_at);
CREATE INDEX IF NOT EXISTS idx_chat_messages_region_id ON chat_messages(region_id, created_at);

CREATE TABLE IF NOT EXISTS character_items (
  item_instance_id TEXT PRIMARY KEY,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  template_id TEXT NOT NULL,
  quantity INTEGER NOT NULL,
  container_kind TEXT NOT NULL,
  equip_slot TEXT NULL,
  instance_attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_character_items_quantity_positive CHECK (quantity > 0)
);

ALTER TABLE character_items ADD COLUMN IF NOT EXISTS instance_attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.table_constraints
    WHERE table_name = 'character_items'
      AND constraint_name = 'chk_character_items_container_shape'
  ) THEN
    ALTER TABLE character_items DROP CONSTRAINT chk_character_items_container_shape;
  END IF;
END $$;

ALTER TABLE character_items
  ADD CONSTRAINT chk_character_items_container_shape CHECK (
    (container_kind = 'inventory' AND equip_slot IS NULL)
    OR (container_kind = 'warehouse' AND equip_slot IS NULL)
    OR (container_kind = 'equipment' AND equip_slot IS NOT NULL AND quantity = 1)
  );

CREATE INDEX IF NOT EXISTS idx_character_items_character_id ON character_items(character_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_character_items_equipment_slot
  ON character_items(character_id, equip_slot)
  WHERE container_kind = 'equipment' AND equip_slot IS NOT NULL;

CREATE TABLE IF NOT EXISTS storage_transfer_records (
  transfer_id TEXT PRIMARY KEY,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  account_id TEXT NULL REFERENCES accounts(account_id) ON DELETE CASCADE,
  source_item_instance_id TEXT NOT NULL,
  template_id TEXT NOT NULL,
  quantity INTEGER NOT NULL,
  item_quantity_before INTEGER NULL,
  item_quantity_after INTEGER NULL,
  from_container_kind TEXT NOT NULL,
  to_container_kind TEXT NOT NULL,
  transfer_type TEXT NOT NULL,
  counterparty_entity_id TEXT NULL,
  session_id TEXT NULL,
  command_id TEXT NULL,
  command_seq INTEGER NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_storage_transfer_quantity_positive CHECK (quantity > 0)
);

ALTER TABLE storage_transfer_records ADD COLUMN IF NOT EXISTS account_id TEXT NULL REFERENCES accounts(account_id) ON DELETE CASCADE;
ALTER TABLE storage_transfer_records ADD COLUMN IF NOT EXISTS item_quantity_before INTEGER NULL;
ALTER TABLE storage_transfer_records ADD COLUMN IF NOT EXISTS item_quantity_after INTEGER NULL;
ALTER TABLE storage_transfer_records ADD COLUMN IF NOT EXISTS session_id TEXT NULL;
ALTER TABLE storage_transfer_records ADD COLUMN IF NOT EXISTS command_id TEXT NULL;
ALTER TABLE storage_transfer_records ADD COLUMN IF NOT EXISTS command_seq INTEGER NULL;

CREATE INDEX IF NOT EXISTS idx_storage_transfer_records_character_id ON storage_transfer_records(character_id, created_at);
CREATE INDEX IF NOT EXISTS idx_storage_transfer_records_source_item_id ON storage_transfer_records(source_item_instance_id, created_at);
CREATE INDEX IF NOT EXISTS idx_storage_transfer_records_transfer_type ON storage_transfer_records(transfer_type, created_at);

CREATE TABLE IF NOT EXISTS action_logs (
  action_log_id TEXT PRIMARY KEY,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  account_id TEXT NULL REFERENCES accounts(account_id) ON DELETE CASCADE,
  action_type TEXT NOT NULL,
  reference_id TEXT NULL,
  counterparty_entity_id TEXT NULL,
  item_instance_id TEXT NULL,
  template_id TEXT NULL,
  quantity INTEGER NULL,
  item_quantity_before INTEGER NULL,
  item_quantity_after INTEGER NULL,
  currency_template_id TEXT NULL,
  currency_amount INTEGER NULL,
  currency_balance_before INTEGER NULL,
  currency_balance_after INTEGER NULL,
  from_container_kind TEXT NULL,
  to_container_kind TEXT NULL,
  session_id TEXT NULL,
  command_id TEXT NULL,
  command_seq INTEGER NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS account_id TEXT NULL REFERENCES accounts(account_id) ON DELETE CASCADE;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS item_quantity_before INTEGER NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS item_quantity_after INTEGER NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS currency_balance_before INTEGER NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS currency_balance_after INTEGER NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS from_container_kind TEXT NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS to_container_kind TEXT NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS session_id TEXT NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS command_id TEXT NULL;
ALTER TABLE action_logs ADD COLUMN IF NOT EXISTS command_seq INTEGER NULL;

CREATE INDEX IF NOT EXISTS idx_action_logs_character_id ON action_logs(character_id, created_at);
CREATE INDEX IF NOT EXISTS idx_action_logs_item_instance_id ON action_logs(item_instance_id, created_at);
CREATE INDEX IF NOT EXISTS idx_action_logs_action_type ON action_logs(action_type, created_at);
CREATE INDEX IF NOT EXISTS idx_action_logs_counterparty_entity_id ON action_logs(counterparty_entity_id, created_at);
CREATE INDEX IF NOT EXISTS idx_action_logs_reference_id ON action_logs(reference_id, created_at);

CREATE TABLE IF NOT EXISTS gameplay_sessions (
  session_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(account_id) ON DELETE CASCADE,
  character_id TEXT NOT NULL REFERENCES characters(character_id) ON DELETE CASCADE,
  attach_token TEXT NOT NULL,
  status TEXT NOT NULL,
  attach_expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gameplay_sessions_account_id ON gameplay_sessions(account_id);
CREATE INDEX IF NOT EXISTS idx_gameplay_sessions_character_id ON gameplay_sessions(character_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gameplay_sessions_attach_token ON gameplay_sessions(attach_token);

CREATE TABLE IF NOT EXISTS gameplay_session_ownerships (
  character_id TEXT PRIMARY KEY REFERENCES characters(character_id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES gameplay_sessions(session_id) ON DELETE CASCADE,
  server_instance_id TEXT NOT NULL,
  fencing_token BIGINT NOT NULL CHECK (fencing_token > 0),
  region_id TEXT NOT NULL,
  lease_expires_at TIMESTAMPTZ NOT NULL,
  acquired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  renewed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gameplay_session_ownerships_session_id ON gameplay_session_ownerships(session_id);
CREATE INDEX IF NOT EXISTS idx_gameplay_session_ownerships_lease ON gameplay_session_ownerships(lease_expires_at);
CREATE INDEX IF NOT EXISTS idx_gameplay_session_ownerships_instance ON gameplay_session_ownerships(server_instance_id, lease_expires_at);

CREATE TABLE IF NOT EXISTS gameplay_command_records (
  session_id TEXT NOT NULL REFERENCES gameplay_sessions(session_id) ON DELETE CASCADE,
  command_seq INTEGER NOT NULL,
  command_id TEXT NOT NULL,
  command_type TEXT NOT NULL,
  status TEXT NOT NULL,
  outcome_json JSONB NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (session_id, command_seq)
);

CREATE INDEX IF NOT EXISTS idx_gameplay_command_records_session_id ON gameplay_command_records(session_id);

CREATE TABLE IF NOT EXISTS gameplay_event_outbox (
  event_id BIGSERIAL PRIMARY KEY,
  idempotency_key TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload_json JSONB NOT NULL,
  target_server_instance_id TEXT NOT NULL,
  target_region_id TEXT NULL,
  target_session_id TEXT NULL,
  target_character_id TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  claimed_at TIMESTAMPTZ NULL,
  claim_owner_id TEXT NULL,
  claim_deadline_at TIMESTAMPTZ NULL,
  delivered_at TIMESTAMPTZ NULL,
  dead_lettered_at TIMESTAMPTZ NULL,
  retry_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  CONSTRAINT chk_gameplay_event_outbox_retry_count CHECK (retry_count >= 0),
  CONSTRAINT chk_gameplay_event_outbox_identity CHECK (
    BTRIM(idempotency_key) <> ''
    AND BTRIM(event_type) <> ''
    AND BTRIM(target_server_instance_id) <> ''
  ),
  CONSTRAINT chk_gameplay_event_outbox_terminal_shape CHECK (
    delivered_at IS NULL OR dead_lettered_at IS NULL
  ),
  CONSTRAINT chk_gameplay_event_outbox_claim_shape CHECK (
    (claim_owner_id IS NULL AND claim_deadline_at IS NULL)
    OR (claim_owner_id IS NOT NULL AND claim_deadline_at IS NOT NULL)
  )
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE table_name = 'gameplay_event_outbox'
      AND constraint_name = 'chk_gameplay_event_outbox_retry_count'
  ) THEN
    ALTER TABLE gameplay_event_outbox
      ADD CONSTRAINT chk_gameplay_event_outbox_retry_count CHECK (retry_count >= 0);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE table_name = 'gameplay_event_outbox'
      AND constraint_name = 'chk_gameplay_event_outbox_identity'
  ) THEN
    ALTER TABLE gameplay_event_outbox
      ADD CONSTRAINT chk_gameplay_event_outbox_identity CHECK (
        BTRIM(idempotency_key) <> ''
        AND BTRIM(event_type) <> ''
        AND BTRIM(target_server_instance_id) <> ''
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE table_name = 'gameplay_event_outbox'
      AND constraint_name = 'chk_gameplay_event_outbox_terminal_shape'
  ) THEN
    ALTER TABLE gameplay_event_outbox
      ADD CONSTRAINT chk_gameplay_event_outbox_terminal_shape CHECK (
        delivered_at IS NULL OR dead_lettered_at IS NULL
      );
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE table_name = 'gameplay_event_outbox'
      AND constraint_name = 'chk_gameplay_event_outbox_claim_shape'
  ) THEN
    ALTER TABLE gameplay_event_outbox
      ADD CONSTRAINT chk_gameplay_event_outbox_claim_shape CHECK (
        (claim_owner_id IS NULL AND claim_deadline_at IS NULL)
        OR (claim_owner_id IS NOT NULL AND claim_deadline_at IS NOT NULL)
      );
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_gameplay_event_outbox_idempotency_key
  ON gameplay_event_outbox(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_gameplay_event_outbox_claim
  ON gameplay_event_outbox(target_server_instance_id, available_at, event_id)
  WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_gameplay_event_outbox_delivered_at
  ON gameplay_event_outbox(delivered_at)
  WHERE delivered_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_gameplay_event_outbox_target_character
  ON gameplay_event_outbox(target_character_id, event_id)
  WHERE target_character_id IS NOT NULL;

INSERT INTO schema_bootstrap (bootstrap_key)
VALUES ('baseline_v1')
ON CONFLICT (bootstrap_key) DO UPDATE
SET applied_at = NOW();

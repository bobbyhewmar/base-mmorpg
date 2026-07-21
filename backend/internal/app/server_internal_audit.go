package app

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleInternalEconomyEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalAuditAccess(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}

	limit, offset, from, to, ok := parseInternalAuditQuery(w, r)
	if !ok {
		return
	}
	service := NewEconomyAuditService(s.store)
	events, err := service.ListEvents(r.Context(), ActionLogQuery{
		CharacterID:    strings.TrimSpace(r.URL.Query().Get("character_id")),
		ItemInstanceID: strings.TrimSpace(r.URL.Query().Get("item_instance_id")),
		ActionType:     strings.TrimSpace(r.URL.Query().Get("action_type")),
		OccurredAfter:  from,
		OccurredBefore: to,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to query economy events.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":  events,
		"limit":   limit,
		"offset":  offset,
		"filters": internalAuditFilters(r, from, to),
	})
}

func (s *Server) handleInternalWarehouseTransfers(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalAuditAccess(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}

	characterID := strings.TrimSpace(r.URL.Query().Get("character_id"))
	if characterID == "" {
		writeError(w, http.StatusBadRequest, "audit.character_required", "character_id is required.")
		return
	}
	limit, offset, from, to, ok := parseInternalAuditQuery(w, r)
	if !ok {
		return
	}
	service := NewEconomyAuditService(s.store)
	transfers, err := service.ListWarehouseTransfers(r.Context(), StorageTransferQuery{
		CharacterID:    characterID,
		SourceItemID:   strings.TrimSpace(r.URL.Query().Get("item_instance_id")),
		TransferType:   strings.TrimSpace(r.URL.Query().Get("transfer_type")),
		OccurredAfter:  from,
		OccurredBefore: to,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to query warehouse transfers.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transfers": transfers,
		"limit":     limit,
		"offset":    offset,
		"filters":   internalAuditFilters(r, from, to),
	})
}

func (s *Server) handleInternalTradeEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalAuditAccess(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}

	characterID := strings.TrimSpace(r.URL.Query().Get("character_id"))
	if characterID == "" {
		writeError(w, http.StatusBadRequest, "audit.character_required", "character_id is required.")
		return
	}
	limit, offset, from, to, ok := parseInternalAuditQuery(w, r)
	if !ok {
		return
	}
	service := NewEconomyAuditService(s.store)
	events, err := service.ListTrades(r.Context(), characterID, limit, offset, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to query trade events.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":  events,
		"limit":   limit,
		"offset":  offset,
		"filters": internalAuditFilters(r, from, to),
	})
}

func (s *Server) handleInternalPvPEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalAuditAccess(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}
	if s.store == nil || s.store.PvPCombatEvents == nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "PvP audit repository is unavailable.")
		return
	}
	limit, offset, from, to, ok := parseInternalAuditQuery(w, r)
	if !ok {
		return
	}
	suspicious, ok := parseOptionalBoolQuery(w, r, "suspicious")
	if !ok {
		return
	}
	actionType := strings.TrimSpace(r.URL.Query().Get("action"))
	if actionType == "" {
		actionType = strings.TrimSpace(r.URL.Query().Get("action_type"))
	}
	events, err := s.store.PvPCombatEvents.ListByFilter(r.Context(), PvPCombatEventQuery{
		AttackerCharacterID: strings.TrimSpace(r.URL.Query().Get("attacker_character_id")),
		AttackerAccountID:   strings.TrimSpace(r.URL.Query().Get("attacker_account_id")),
		VictimCharacterID:   strings.TrimSpace(r.URL.Query().Get("victim_character_id")),
		VictimAccountID:     strings.TrimSpace(r.URL.Query().Get("victim_account_id")),
		KillerCharacterID:   strings.TrimSpace(r.URL.Query().Get("killer_character_id")),
		InvolvedCharacterID: strings.TrimSpace(r.URL.Query().Get("character_id")),
		ActionType:          actionType,
		Result:              strings.TrimSpace(r.URL.Query().Get("result")),
		Suspicious:          suspicious,
		OccurredAfter:       from,
		OccurredBefore:      to,
		Limit:               limit,
		Offset:              offset,
	})
	if err != nil {
		s.recordStoreError("pvp_audit.list", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to query PvP combat events.")
		return
	}
	filters := map[string]any{
		"attacker_character_id": strings.TrimSpace(r.URL.Query().Get("attacker_character_id")),
		"attacker_account_id":   strings.TrimSpace(r.URL.Query().Get("attacker_account_id")),
		"victim_character_id":   strings.TrimSpace(r.URL.Query().Get("victim_character_id")),
		"victim_account_id":     strings.TrimSpace(r.URL.Query().Get("victim_account_id")),
		"killer_character_id":   strings.TrimSpace(r.URL.Query().Get("killer_character_id")),
		"character_id":          strings.TrimSpace(r.URL.Query().Get("character_id")),
		"action":                actionType,
		"result":                strings.TrimSpace(r.URL.Query().Get("result")),
	}
	if suspicious != nil {
		filters["suspicious"] = *suspicious
	}
	if from != nil {
		filters["from"] = from.Format(time.RFC3339)
	}
	if to != nil {
		filters["to"] = to.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":  events,
		"limit":   limit,
		"offset":  offset,
		"filters": filters,
	})
}

func (s *Server) handleInternalPvPKarmaRecoveryEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalAuditAccess(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}
	if s.store == nil || s.store.PvPCombatEvents == nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "PvP recovery repository is unavailable.")
		return
	}
	limit, offset, from, to, ok := parseInternalAuditQuery(w, r)
	if !ok {
		return
	}
	events, err := s.store.PvPCombatEvents.ListKarmaRecoveryEvents(r.Context(), PvPKarmaRecoveryEventQuery{
		CharacterID:    strings.TrimSpace(r.URL.Query().Get("character_id")),
		AccountID:      strings.TrimSpace(r.URL.Query().Get("account_id")),
		Trigger:        strings.TrimSpace(r.URL.Query().Get("trigger")),
		OccurredAfter:  from,
		OccurredBefore: to,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		s.recordStoreError("pvp_recovery.list", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to query PvP karma recovery events.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"limit":  limit,
		"offset": offset,
		"filters": map[string]any{
			"character_id": strings.TrimSpace(r.URL.Query().Get("character_id")),
			"account_id":   strings.TrimSpace(r.URL.Query().Get("account_id")),
			"trigger":      strings.TrimSpace(r.URL.Query().Get("trigger")),
			"from":         optionalAuditFilterTime(from),
			"to":           optionalAuditFilterTime(to),
		},
	})
}

func (s *Server) handleInternalPvPCorrelations(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalAuditAccess(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}
	if s.store == nil || s.store.PvPCombatEvents == nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "PvP correlation repository is unavailable.")
		return
	}
	limit, offset, from, to, ok := parseInternalAuditQuery(w, r)
	if !ok {
		return
	}
	suspiciousOnly, ok := parseOptionalBoolQuery(w, r, "suspicious")
	if !ok {
		return
	}
	minRepeatedKillCount, ok := parseOptionalIntQuery(w, r, "min_repeated_kill_count")
	if !ok {
		return
	}
	records, err := s.store.PvPCombatEvents.ListAccountCorrelations(r.Context(), PvPAccountCorrelationQuery{
		AccountID:            strings.TrimSpace(r.URL.Query().Get("account_id")),
		SuspiciousOnly:       suspiciousOnly,
		MinRepeatedKillCount: max(0, minRepeatedKillCount),
		OccurredAfter:        from,
		OccurredBefore:       to,
		Limit:                limit,
		Offset:               offset,
	})
	if err != nil {
		s.recordStoreError("pvp_correlation.list", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to query PvP account correlations.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"correlations": records,
		"limit":        limit,
		"offset":       offset,
		"filters": map[string]any{
			"account_id":              strings.TrimSpace(r.URL.Query().Get("account_id")),
			"min_repeated_kill_count": max(0, minRepeatedKillCount),
			"suspicious":              optionalAuditFilterBool(suspiciousOnly),
			"from":                    optionalAuditFilterTime(from),
			"to":                      optionalAuditFilterTime(to),
			"device_correlation":      "unavailable",
		},
	})
}

func (s *Server) handleInternalPvPHighKarma(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalAuditAccess(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "protocol.method_not_allowed", "Method not allowed.")
		return
	}
	if s.store == nil || s.store.Characters == nil {
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "PvP high-karma repository is unavailable.")
		return
	}
	limit, offset, _, _, ok := parseInternalAuditQuery(w, r)
	if !ok {
		return
	}
	persistentOnly, ok := parseOptionalBoolQuery(w, r, "persistent")
	if !ok {
		return
	}
	minimumKarma, ok := parseOptionalIntQuery(w, r, "minimum_karma")
	if !ok {
		return
	}
	persistent := persistentOnly != nil && *persistentOnly
	records, err := s.store.Characters.ListHighKarma(r.Context(), PvPHighKarmaQuery{
		CharacterID:    strings.TrimSpace(r.URL.Query().Get("character_id")),
		AccountID:      strings.TrimSpace(r.URL.Query().Get("account_id")),
		MinimumKarma:   max(1, minimumKarma),
		PersistentOnly: persistent,
		ObservedAt:     time.Now().UTC(),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		s.recordStoreError("pvp_high_karma.list", err)
		writeError(w, http.StatusInternalServerError, "system.persistence_failed", "Unable to query PvP high-karma characters.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"characters": records,
		"limit":      limit,
		"offset":     offset,
		"filters": map[string]any{
			"character_id":  strings.TrimSpace(r.URL.Query().Get("character_id")),
			"account_id":    strings.TrimSpace(r.URL.Query().Get("account_id")),
			"minimum_karma": max(1, minimumKarma),
			"persistent":    persistent,
		},
	})
}

func (s *Server) requireInternalAuditAccess(w http.ResponseWriter, r *http.Request) bool {
	if s == nil || !s.config.InternalAuditEnabled {
		http.NotFound(w, r)
		return false
	}
	if r == nil {
		writeError(w, http.StatusUnauthorized, "audit.not_authorized", "Missing internal audit token.")
		return false
	}
	providedToken := strings.TrimSpace(r.Header.Get("X-Internal-Audit-Token"))
	if providedToken == "" || subtle.ConstantTimeCompare([]byte(providedToken), []byte(s.config.InternalAuditToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "audit.not_authorized", "Missing or invalid internal audit token.")
		return false
	}
	return true
}

func parseInternalAuditQuery(w http.ResponseWriter, r *http.Request) (int, int, *time.Time, *time.Time, bool) {
	limit, ok := parseOptionalIntQuery(w, r, "limit")
	if !ok {
		return 0, 0, nil, nil, false
	}
	offset, ok := parseOptionalIntQuery(w, r, "offset")
	if !ok {
		return 0, 0, nil, nil, false
	}
	from, ok := parseOptionalTimeQuery(w, r, "from")
	if !ok {
		return 0, 0, nil, nil, false
	}
	to, ok := parseOptionalTimeQuery(w, r, "to")
	if !ok {
		return 0, 0, nil, nil, false
	}
	if from != nil && to != nil && from.After(*to) {
		writeError(w, http.StatusBadRequest, "audit.invalid_time_range", "from must be earlier than or equal to to.")
		return 0, 0, nil, nil, false
	}
	limit, offset = normalizeAuditPagination(limit, offset)
	return limit, offset, from, to, true
}

func parseOptionalIntQuery(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit.invalid_query", key+" must be an integer.")
		return 0, false
	}
	return parsed, true
}

func parseOptionalBoolQuery(w http.ResponseWriter, r *http.Request, key string) (*bool, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil, true
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit.invalid_query", key+" must be true or false.")
		return nil, false
	}
	return &parsed, true
}

func parseOptionalTimeQuery(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil, true
	}
	if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		parsed := time.UnixMilli(milliseconds).UTC()
		return &parsed, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit.invalid_query", key+" must be RFC3339 or unix milliseconds.")
		return nil, false
	}
	parsed = parsed.UTC()
	return &parsed, true
}

func internalAuditFilters(r *http.Request, from *time.Time, to *time.Time) map[string]any {
	filters := map[string]any{
		"character_id":     strings.TrimSpace(r.URL.Query().Get("character_id")),
		"item_instance_id": strings.TrimSpace(r.URL.Query().Get("item_instance_id")),
		"action_type":      strings.TrimSpace(r.URL.Query().Get("action_type")),
		"transfer_type":    strings.TrimSpace(r.URL.Query().Get("transfer_type")),
	}
	if from != nil {
		filters["from"] = from.Format(time.RFC3339)
	}
	if to != nil {
		filters["to"] = to.Format(time.RFC3339)
	}
	return filters
}

func optionalAuditFilterTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339)
}

func optionalAuditFilterBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

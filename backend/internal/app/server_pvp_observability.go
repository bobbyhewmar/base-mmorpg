package app

import "time"

func (s *Server) recordPvPCombatAuditEvent(event PvPCombatEvent) {
	if s == nil || s.observer == nil {
		return
	}
	if event.Suspicious {
		s.observer.incCounter("l2bg_pvp_audit_events_total", "Total PvP operational audit signals.", map[string]string{
			"result": "suspicious_kill",
		}, 1)
	}
	if event.RepeatedKillCount > 1 {
		s.observer.incCounter("l2bg_pvp_audit_events_total", "Total PvP operational audit signals.", map[string]string{
			"result": "repeated_pair",
		}, 1)
	}
	if event.AttackerAccountID != "" && event.VictimAccountID != "" && event.RepeatedKillCount > 1 {
		reason := "account_pair_correlation"
		if event.AttackerAccountID == event.VictimAccountID {
			reason = "same_account_pair"
		}
		s.observer.incCounter("l2bg_pvp_audit_events_total", "Total PvP operational audit signals.", map[string]string{
			"result": reason,
		}, 1)
		s.observer.log("warn", "pvp_account_correlation", map[string]any{
			"attacker_character_id": event.AttackerCharacterID,
			"victim_character_id":   event.VictimCharacterID,
			"repeated_kill_count":   event.RepeatedKillCount,
			"correlation_scope":     "account",
			"same_account":          event.AttackerAccountID == event.VictimAccountID,
			"created_at":            event.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
}

func (s *Server) recordPvPKarmaRecoveryEvent(event PvPKarmaRecoveryEvent, state CharacterPvPCombatState) {
	if s == nil || s.observer == nil {
		return
	}
	s.observer.incCounter("l2bg_pvp_karma_events_total", "Total PvP karma recovery signals.", map[string]string{
		"result":  "karma_recovered",
		"trigger": event.Trigger,
	}, 1)
	fields := map[string]any{
		"character_id":     event.CharacterID,
		"trigger":          event.Trigger,
		"karma_before":     event.KarmaBefore,
		"karma_after":      event.KarmaAfter,
		"recovered_amount": event.RecoveredAmount,
		"created_at":       event.CreatedAt.UTC().Format(time.RFC3339),
	}
	if persistentHighKarmaAt(state, event.CreatedAt) {
		s.observer.incCounter("l2bg_pvp_karma_events_total", "Total PvP karma recovery signals.", map[string]string{
			"result": "persistent_high_karma",
		}, 1)
		fields["persistent_high_karma"] = true
	}
	s.observer.log("info", "pvp_karma_recovery", fields)
}

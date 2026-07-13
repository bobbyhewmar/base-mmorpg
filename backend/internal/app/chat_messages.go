package app

import "time"

const (
	chatChannelRegion  = "region"
	chatChannelParty   = "party"
	chatChannelWhisper = "whisper"
)

type ChatMessageRecord struct {
	ID                string
	CharacterID       string
	AccountID         string
	Channel           string
	TargetCharacterID string
	RegionID          string
	Text              string
	SessionID         string
	CommandID         string
	CommandSeq        int
	CreatedAt         time.Time
}

type ChatMessageQuery struct {
	CharacterID       string
	TargetCharacterID string
	Channel           string
	RegionID          string
	OccurredAfter     *time.Time
	OccurredBefore    *time.Time
	Limit             int
	Offset            int
}

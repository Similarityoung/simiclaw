package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func ComputeKey(tenantID string, conv model.Conversation, dmScope string) (string, error) {
	if tenantID == "" {
		return "", errors.New("tenant_id is required")
	}
	if conv.ConversationID == "" {
		return "", errors.New("conversation_id is required")
	}
	if conv.ChannelType == "" {
		return "", errors.New("channel_type is required")
	}
	if dmScope == "" {
		dmScope = "default"
	}
	participant := "-"
	if conv.ChannelType == "dm" {
		if conv.ParticipantID == "" {
			return "", errors.New("participant_id is required for dm")
		}
		participant = conv.ParticipantID
	}
	raw := fmt.Sprintf("%s|%s|%s|%s|%s", tenantID, conv.ConversationID, conv.ChannelType, participant, dmScope)
	h := sha256.Sum256([]byte(raw))
	return "sk:" + hex.EncodeToString(h[:]), nil
}

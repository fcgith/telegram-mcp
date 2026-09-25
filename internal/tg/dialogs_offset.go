package tg

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
)

type DialogsOffset struct {
	MsgID int `json:"msg_id"`
	Date  int `json:"offset_date"`
	Peer  tg.InputPeerClass
}

func getInputPeerID(p tg.PeerClass) tg.InputPeerClass {
	switch v := p.(type) {
	case *tg.PeerUser:
		return &tg.InputPeerUser{UserID: v.UserID}
	case *tg.PeerChannel:
		return &tg.InputPeerChannel{ChannelID: v.ChannelID}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: v.ChatID}
	default:
		return &tg.InputPeerEmpty{}
	}
}

func (o DialogsOffset) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

func (o *DialogsOffset) String() string {

	var id, hash int64
	var peerType string
	switch p := o.Peer.(type) {
	case *tg.InputPeerUser:
		peerType = "user"
		id, hash = p.UserID, p.AccessHash
	case *tg.InputPeerChannel:
		peerType = "chan"
		id, hash = p.ChannelID, p.AccessHash
	case *tg.InputPeerChat:
		peerType = "chat"
		id = p.ChatID
	default:
		peerType = "unknown"
	}

	if id == 0 {
		return "end"
	}

	if hash != 0 {
		// gpcv: the access hash rides along (as uint64: no '-' inside a part).
		return fmt.Sprintf("%s-%d-%d-%d-%d", peerType, id, o.MsgID, o.Date, uint64(hash))
	}

	return fmt.Sprintf("%s-%d-%d-%d", peerType, id, o.MsgID, o.Date)
}

func (o *DialogsOffset) UnmarshalJSON(data []byte) error {
	parts := strings.Split(string(data), "-")
	if len(parts) != 4 && len(parts) != 5 {
		return fmt.Errorf("invalid format")
	}

	var hash uint64
	if len(parts) == 5 {
		var err error
		if hash, err = strconv.ParseUint(parts[4], 10, 64); err != nil {
			return fmt.Errorf("invalid access hash: %w", err)
		}
	}

	var err error
	switch parts[0] {
	case "user":
		var userID int64
		userID, err = strconv.ParseInt(parts[1], 10, 64)
		o.Peer = &tg.InputPeerUser{UserID: userID, AccessHash: int64(hash)}
	case "chan":
		var channelID int64
		channelID, err = strconv.ParseInt(parts[1], 10, 64)
		o.Peer = &tg.InputPeerChannel{ChannelID: channelID, AccessHash: int64(hash)}
	case "chat":
		var chatID int64
		chatID, err = strconv.ParseInt(parts[1], 10, 64)
		o.Peer = &tg.InputPeerChat{ChatID: chatID}
	default:
		return fmt.Errorf("unknown peer type")
	}

	if err != nil {
		return fmt.Errorf("invalid peer: %w", err)
	}

	o.MsgID, err = strconv.Atoi(parts[2])
	if err != nil {
		return fmt.Errorf("invalid message ID: %w", err)
	}

	o.Date, err = strconv.Atoi(parts[3])
	if err != nil {
		return fmt.Errorf("invalid date: %w", err)
	}

	return nil
}

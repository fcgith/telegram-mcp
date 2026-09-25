package tg

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/pkg/errors"
)

type HistoryArguments struct {
	Name   string `json:"name" jsonschema:"required,description=Name of the dialog"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=Offset for continuation"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Max messages to return (1-100; default 50)"`
	MinID  int    `json:"min_id,omitempty" jsonschema:"description=Return only messages with id greater than this"`
}

const (
	DefaultHistoryLimit = 50
	MaxHistoryLimit     = 100
)

// request builds the messages.getHistory request with a clamped limit.
func (a HistoryArguments) request(peer tg.InputPeerClass) *tg.MessagesGetHistoryRequest {
	limit := a.Limit
	if limit <= 0 {
		limit = DefaultHistoryLimit
	}
	if limit > MaxHistoryLimit {
		limit = MaxHistoryLimit
	}
	return &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		OffsetID: a.Offset,
		Limit:    limit,
		MinID:    a.MinID,
	}
}

type HistoryResponse struct {
	Messages []MessageInfo `json:"messages"`
	Offset   int           `json:"offset,omitempty"`
}

func (c *Client) GetHistory(args HistoryArguments) (*mcp.ToolResponse, error) {
	var messagesClass tg.MessagesMessagesClass
	client := c.T()
	if err := client.Run(context.Background(), func(ctx context.Context) (err error) {
		api := client.API()

		inputPeer, err := getInputPeerFromName(ctx, api, args.Name)
		if err != nil {
			return fmt.Errorf("get inputPeer from name: %w", err)
		}

		messagesClass, err = api.MessagesGetHistory(ctx, args.request(inputPeer))
		if err != nil {
			return fmt.Errorf("failed to get history: %w", err)
		}

		//Debug
		//jsonData, _ := json.Marshal(messagesClass)
		//log.Info().RawJSON("history", cleanJSON(jsonData)).Msg("history")

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to get history")
	}

	h, err := newHistory(messagesClass)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process history")
	}

	rsp := HistoryResponse{
		Messages: h.Info(),
		Offset:   h.Offset(),
	}

	jsonData, err := json.Marshal(rsp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}

	return mcp.NewToolResponse(mcp.NewTextContent(string(jsonData))), nil
}

var (
	peerIDHashRe = regexp.MustCompile(`^(usr|chn)\[(-?\d+):(-?\d+)\]$`)
	peerChatRe   = regexp.MustCompile(`^cht\[(\d+)\]$`)
)

// parsePeerHandle turns a stable handle (me, usr[id:hash], chn[id:hash],
// cht[id]) into an InputPeer without a network call. ok=false means the name
// is not a handle (e.g. @username) and must be resolved via the API.
func parsePeerHandle(name string) (peer tg.InputPeerClass, ok bool, err error) {
	if name == "me" {
		return &tg.InputPeerSelf{}, true, nil
	}
	if m := peerChatRe.FindStringSubmatch(name); m != nil {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return nil, true, errors.Wrapf(err, "chat peer(%q)", name)
		}
		return &tg.InputPeerChat{ChatID: id}, true, nil
	}
	if m := peerIDHashRe.FindStringSubmatch(name); m != nil {
		id, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return nil, true, errors.Wrapf(err, "peer id(%q)", name)
		}
		hash, err := strconv.ParseInt(m[3], 10, 64)
		if err != nil {
			return nil, true, errors.Wrapf(err, "peer hash(%q)", name)
		}
		if m[1] == "usr" {
			return &tg.InputPeerUser{UserID: id, AccessHash: hash}, true, nil
		}
		return &tg.InputPeerChannel{ChannelID: id, AccessHash: hash}, true, nil
	}
	if strings.ContainsAny(name, "[]") {
		return nil, true, errors.Errorf("malformed peer handle(%q)", name)
	}
	return nil, false, nil
}

func getInputPeerFromName(ctx context.Context, api *tg.Client, name string) (tg.InputPeerClass, error) {
	peer, ok, err := parsePeerHandle(name)
	if ok {
		return peer, err
	}

	sender := message.NewSender(api)
	inputPeer, err := sender.Resolve(name).AsInputPeer(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve name: %w", err)
	}

	return inputPeer, nil
}

type history struct {
	tg.MessagesMessages
	users map[int64]*tg.User
}

func newHistory(raw tg.MessagesMessagesClass) (*history, error) {
	var h history
	switch m := raw.(type) {
	case *tg.MessagesMessages:
		h = history{MessagesMessages: *m}
	case *tg.MessagesMessagesSlice:
		h = history{MessagesMessages: tg.MessagesMessages{
			Messages: m.Messages,
			Users:    m.Users,
			Chats:    m.Chats,
		}}
	case *tg.MessagesChannelMessages:
		h = history{MessagesMessages: tg.MessagesMessages{
			Messages: m.Messages,
			Users:    m.Users,
			Chats:    m.Chats,
		}}
	default:
		return nil, fmt.Errorf("unexpected type: %T", raw)
	}

	h.users = make(map[int64]*tg.User)
	for _, u := range h.Users {
		if user, ok := u.(*tg.User); ok {
			h.users[user.ID] = user
		}
	}

	return &h, nil
}

func (h *history) Info() []MessageInfo {
	messages := make([]MessageInfo, 0, len(h.Messages))

	for _, msg := range h.Messages {
		m, ok := msg.(*tg.Message)
		if !ok {
			continue
		}

		fromID := senderID(m)
		var who string
		if user, ok := h.users[fromID]; ok {
			who = getUsername(user)
		}

		var replyTo int
		if r, ok := m.ReplyTo.(*tg.MessageReplyHeader); ok {
			replyTo = r.ReplyToMsgID
		}

		messages = append(messages, MessageInfo{
			ID:        m.ID,
			FromID:    fromID,
			ReplyToID: replyTo,
			Out:       m.Out,
			Date:      m.Date,
			Who:       who,
			When:      time.Unix(int64(m.Date), 0).Format(time.DateTime),
			Text:      m.Message,
			ts:        m.Date,
		})
	}

	return messages
}

// senderID is the user id of the author. Private-chat messages carry no
// from_id; the author is then the peer (incoming) or unknown here (outgoing).
func senderID(m *tg.Message) int64 {
	if from, ok := m.FromID.(*tg.PeerUser); ok {
		return from.UserID
	}
	if m.FromID == nil && !m.Out {
		if p, ok := m.PeerID.(*tg.PeerUser); ok {
			return p.UserID
		}
	}
	return 0
}

// Offset is the id of the oldest message in the page (history is newest
// first); pass it back as offset to continue. Service messages count too, so
// pagination never stalls on a page that ends with one.
func (h *history) Offset() int {
	if n := len(h.Messages); n > 0 {
		return h.Messages[n-1].GetID()
	}

	return 0
}

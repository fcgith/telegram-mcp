package tg

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/gotd/td/tg"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/pkg/errors"
)

// SendArguments are the parameters for the Send tool (tg_send_message).
type SendArguments struct {
	Name string `json:"name" jsonschema:"required,description=Name of the dialog (user, group, or channel) to deliver the message to, exactly as returned by tg_dialogs"`
	Text string `json:"text" jsonschema:"required,description=Plain text of the message to send"`
	// RandomID lets the caller retry without duplicates: Telegram drops a second
	// message with the same random_id. Zero means "generate one".
	RandomID int64 `json:"random_id,omitempty" jsonschema:"description=Caller-chosen non-zero int64 for retry dedupe (optional)"`
}

// SendResponse is returned by the Send tool.
type SendResponse struct {
	Success   bool  `json:"success"`
	MessageID int   `json:"message_id,omitempty"`
	Date      int   `json:"date,omitempty"`
	RandomID  int64 `json:"random_id"`
}

// sendReceipt extracts the id and date of the message created by
// messages.sendMessage. Private chats answer with UpdateShortSentMessage;
// groups/channels with Updates carrying UpdateMessageID (matched by randomID)
// and the new message itself (for the date).
func sendReceipt(u tg.UpdatesClass, randomID int64) (id int, date int, err error) {
	switch v := u.(type) {
	case *tg.UpdateShortSentMessage:
		return v.ID, v.Date, nil
	case *tg.UpdateShort:
		return receiptFrom([]tg.UpdateClass{v.Update}, randomID, v.Date)
	case *tg.Updates:
		return receiptFrom(v.Updates, randomID, v.Date)
	case *tg.UpdatesCombined:
		return receiptFrom(v.Updates, randomID, v.Date)
	}
	return 0, 0, errors.Errorf("unexpected send result %T", u)
}

func receiptFrom(updates []tg.UpdateClass, randomID int64, fallbackDate int) (int, int, error) {
	id := 0
	for _, up := range updates {
		if m, ok := up.(*tg.UpdateMessageID); ok && m.RandomID == randomID {
			id = m.ID
		}
	}
	for _, up := range updates {
		var mc tg.MessageClass
		switch n := up.(type) {
		case *tg.UpdateNewMessage:
			mc = n.Message
		case *tg.UpdateNewChannelMessage:
			mc = n.Message
		}
		if m, ok := mc.(*tg.Message); ok && m.Out && (id == 0 || m.ID == id) {
			return m.ID, m.Date, nil
		}
	}
	if id != 0 {
		return id, fallbackDate, nil
	}
	return 0, 0, errors.New("send result has no message id")
}

// newRandomID returns a non-zero random int64 for use as the RandomID required
// by messages.sendMessage. Telegram uses this value to deduplicate a message if
// the request is retried, so it must be unique per logical message.
func newRandomID() (int64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	id := int64(binary.LittleEndian.Uint64(buf[:]))
	if id == 0 {
		id = 1
	}
	return id, nil
}

// Send delivers a text message to the given dialog via messages.sendMessage.
// Unlike SendDraft, which only stages an unsent draft in the input box, Send
// actually delivers the message to the recipient.
func (c *Client) Send(args SendArguments) (*mcp.ToolResponse, error) {
	rsp := SendResponse{RandomID: args.RandomID}
	if rsp.RandomID == 0 {
		var err error
		if rsp.RandomID, err = newRandomID(); err != nil {
			return nil, errors.Wrap(err, "generate random id")
		}
	}

	client := c.T()
	if err := client.Run(context.Background(), func(ctx context.Context) (err error) {
		api := client.API()

		inputPeer, err := getInputPeerFromName(ctx, api, args.Name)
		if err != nil {
			return fmt.Errorf("get inputPeer from name: %w", err)
		}

		updates, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     inputPeer,
			Message:  args.Text,
			RandomID: rsp.RandomID,
		})
		if err != nil {
			return fmt.Errorf("send message: %w", err)
		}

		// Delivered even if the receipt cannot be parsed; report what we have.
		rsp.Success = true
		rsp.MessageID, rsp.Date, _ = sendReceipt(updates, rsp.RandomID)

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to send message")
	}

	jsonData, err := json.Marshal(rsp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}

	return mcp.NewToolResponse(mcp.NewTextContent(string(jsonData))), nil
}

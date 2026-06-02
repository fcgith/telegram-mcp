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
}

// SendResponse is returned by the Send tool.
type SendResponse struct {
	Success bool `json:"success"`
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
	var ok bool
	client := c.T()
	if err := client.Run(context.Background(), func(ctx context.Context) (err error) {
		api := client.API()

		inputPeer, err := getInputPeerFromName(ctx, api, args.Name)
		if err != nil {
			return fmt.Errorf("get inputPeer from name: %w", err)
		}

		randomID, err := newRandomID()
		if err != nil {
			return fmt.Errorf("generate random id: %w", err)
		}

		_, err = api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     inputPeer,
			Message:  args.Text,
			RandomID: randomID,
		})
		if err != nil {
			return fmt.Errorf("send message: %w", err)
		}

		ok = true

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to send message")
	}

	jsonData, err := json.Marshal(SendResponse{Success: ok})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}

	return mcp.NewToolResponse(mcp.NewTextContent(string(jsonData))), nil
}

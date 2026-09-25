package tg

import (
	"encoding/json"
	"testing"

	"github.com/gotd/td/tg"
)

func TestParsePeerHandle(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
		err  bool
		want tg.InputPeerClass
	}{
		{"me", true, false, &tg.InputPeerSelf{}},
		{"usr[123:-456]", true, false, &tg.InputPeerUser{UserID: 123, AccessHash: -456}},
		{"chn[2225853048:8934705438195741763]", true, false, &tg.InputPeerChannel{ChannelID: 2225853048, AccessHash: 8934705438195741763}},
		{"cht[4626931529]", true, false, &tg.InputPeerChat{ChatID: 4626931529}},
		{"@user", false, false, nil},
		{"someuser", false, false, nil},
		{"usr[12]", true, true, nil},
		{"cht[1]x", true, true, nil},
		{"usr[1:99999999999999999999]", true, true, nil},
	}
	for _, c := range cases {
		got, ok, err := parsePeerHandle(c.name)
		if ok != c.ok || (err != nil) != c.err {
			t.Fatalf("%q: ok=%v err=%v", c.name, ok, err)
		}
		if c.want != nil && got.String() != c.want.String() {
			t.Fatalf("%q: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestPeerHandleRoundTrip(t *testing.T) {
	for _, src := range []any{
		&tg.User{ID: 42, AccessHash: -7},
		&tg.Chat{ID: 9},
		&tg.Channel{ID: 5, AccessHash: 11},
	} {
		h := peerHandle(src)
		if _, ok, err := parsePeerHandle(h); !ok || err != nil {
			t.Fatalf("%s does not parse back: %v", h, err)
		}
	}
	if got := getUsername(&tg.User{ID: 42, AccessHash: 3}); got != "usr[42:3]" {
		t.Fatalf("username-less user handle = %q", got)
	}
	if got := getUsername(&tg.User{ID: 42, Username: "bob"}); got != "bob" {
		t.Fatalf("username user = %q", got)
	}
}

func TestHistoryRequestArgs(t *testing.T) {
	peer := &tg.InputPeerSelf{}
	if r := (HistoryArguments{}).request(peer); r.Limit != 50 || r.MinID != 0 || r.OffsetID != 0 {
		t.Fatalf("defaults: %+v", r)
	}
	if r := (HistoryArguments{Limit: 500, MinID: 7, Offset: 90}).request(peer); r.Limit != 100 || r.MinID != 7 || r.OffsetID != 90 {
		t.Fatalf("clamp/min_id: %+v", r)
	}
	var a HistoryArguments
	if err := json.Unmarshal([]byte(`{"name":"me","limit":10,"min_id":3,"offset":20}`), &a); err != nil || a.Limit != 10 || a.MinID != 3 || a.Offset != 20 {
		t.Fatalf("json args: %+v %v", a, err)
	}
}

func TestHistoryInfoIDs(t *testing.T) {
	h, err := newHistory(&tg.MessagesMessagesSlice{
		Messages: []tg.MessageClass{
			&tg.Message{ID: 12, Out: true, PeerID: &tg.PeerUser{UserID: 5}, Date: 200, Message: "mine",
				ReplyTo: &tg.MessageReplyHeader{ReplyToMsgID: 11}},
			&tg.Message{ID: 11, PeerID: &tg.PeerUser{UserID: 5}, Date: 100, Message: "theirs"},
			&tg.Message{ID: 10, FromID: &tg.PeerUser{UserID: 8}, PeerID: &tg.PeerChat{ChatID: 1}, Date: 50},
			&tg.MessageService{ID: 9},
		},
		Users: []tg.UserClass{&tg.User{ID: 5, AccessHash: 77}},
	})
	if err != nil {
		t.Fatal(err)
	}
	info := h.Info()
	if len(info) != 3 {
		t.Fatalf("len %d", len(info))
	}
	if m := info[0]; m.ID != 12 || !m.Out || m.ReplyToID != 11 || m.FromID != 0 || m.Date != 200 {
		t.Fatalf("out msg: %+v", m)
	}
	if m := info[1]; m.ID != 11 || m.Out || m.FromID != 5 || m.Who != "usr[5:77]" {
		t.Fatalf("in msg: %+v", m)
	}
	if m := info[2]; m.FromID != 8 {
		t.Fatalf("group msg: %+v", m)
	}
	if h.Offset() != 9 {
		t.Fatalf("offset = %d, want oldest id 9 (service msg included)", h.Offset())
	}
	b, _ := json.Marshal(info[1])
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	for _, k := range []string{"id", "from_id", "date"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("json lacks %s: %s", k, b)
		}
	}
}

func TestSendReceipt(t *testing.T) {
	if id, d, err := sendReceipt(&tg.UpdateShortSentMessage{ID: 55, Date: 1000}, 1); err != nil || id != 55 || d != 1000 {
		t.Fatalf("short: %d %d %v", id, d, err)
	}
	u := &tg.Updates{Date: 999, Updates: []tg.UpdateClass{
		&tg.UpdateMessageID{ID: 70, RandomID: 3},
		&tg.UpdateMessageID{ID: 71, RandomID: 4},
		&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 71, Out: true, Date: 1234}},
	}}
	if id, d, err := sendReceipt(u, 4); err != nil || id != 71 || d != 1234 {
		t.Fatalf("updates: %d %d %v", id, d, err)
	}
	if id, d, err := sendReceipt(&tg.Updates{Date: 5, Updates: []tg.UpdateClass{&tg.UpdateMessageID{ID: 8, RandomID: 2}}}, 2); err != nil || id != 8 || d != 5 {
		t.Fatalf("id only: %d %d %v", id, d, err)
	}
	if _, _, err := sendReceipt(&tg.Updates{}, 1); err == nil {
		t.Fatal("empty updates must error")
	}
	var a SendArguments
	if err := json.Unmarshal([]byte(`{"name":"me","text":"x","random_id":9223372036854775807}`), &a); err != nil || a.RandomID != 9223372036854775807 {
		t.Fatalf("random_id precision: %d %v", a.RandomID, err)
	}
}

package pubsub

import (
	"strings"
	"testing"
)

// TestPushFramingPerProtocol covers the RESP3 push type. A delivery is
// out-of-band, so RESP3 frames it with '>' rather than '*'; a RESP3 client
// handed the array form treats it as the reply to whatever it last sent and
// desynchronises its pipeline.
func TestPushFramingPerProtocol(t *testing.T) {
	hub := NewShardedHub()

	sub2 := NewSubscriber(hub.NextSubscriberID())
	sub3 := NewSubscriber(hub.NextSubscriberID())
	sub3.SetProto(3)

	if sub2.Proto() != 2 {
		t.Fatalf("default protocol = %d, want 2", sub2.Proto())
	}

	hub.Subscribe(sub2, "ch")
	hub.Subscribe(sub3, "ch")

	if n := hub.Publish("ch", []byte("hi")); n != 2 {
		t.Fatalf("Publish reached %d subscribers, want 2", n)
	}

	got2 := string(<-sub2.MsgCh)
	got3 := string(<-sub3.MsgCh)

	if !strings.HasPrefix(got2, "*3\r\n") {
		t.Errorf("RESP2 delivery = %q, want an array", got2)
	}
	if !strings.HasPrefix(got3, ">3\r\n") {
		t.Errorf("RESP3 delivery = %q, want a push", got3)
	}
	// Only the framing differs; the payload must be identical.
	if strings.TrimPrefix(got2, "*3\r\n") != strings.TrimPrefix(got3, ">3\r\n") {
		t.Errorf("payloads differ beyond the frame:\n %q\n %q", got2, got3)
	}
}

func TestPatternPushFraming(t *testing.T) {
	hub := NewShardedHub()
	sub := NewSubscriber(hub.NextSubscriberID())
	sub.SetProto(3)
	hub.PSubscribe(sub, "news.*")

	if n := hub.Publish("news.tech", []byte("x")); n != 1 {
		t.Fatalf("pattern publish reached %d subscribers, want 1", n)
	}
	if got := string(<-sub.MsgCh); !strings.HasPrefix(got, ">4\r\n$8\r\npmessage") {
		t.Errorf("RESP3 pmessage = %q, want a push", got)
	}
}

func TestSubscribeConfirmationFraming(t *testing.T) {
	if got := string(FormatSubscribeReply("subscribe", "ch", 1, 2)); !strings.HasPrefix(got, "*3\r\n") {
		t.Errorf("RESP2 confirmation = %q", got)
	}
	if got := string(FormatSubscribeReply("subscribe", "ch", 1, 3)); !strings.HasPrefix(got, ">3\r\n") {
		t.Errorf("RESP3 confirmation = %q", got)
	}
}

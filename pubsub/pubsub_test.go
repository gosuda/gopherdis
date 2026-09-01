package pubsub

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPubSub_ExactChannelDelivery(t *testing.T) {
	hub := NewShardedHub()
	subA := NewSubscriber(hub.NextSubscriberID())
	subB := NewSubscriber(hub.NextSubscriberID())

	countA := hub.Subscribe(subA, "chat.general")
	if countA != 1 {
		t.Fatalf("expected count 1, got %d", countA)
	}

	countB := hub.Subscribe(subB, "chat.general")
	if countB != 1 {
		t.Fatalf("expected count 1, got %d", countB)
	}

	receivers := hub.Publish("chat.general", []byte("hello all"))
	if receivers != 2 {
		t.Fatalf("expected 2 receivers, got %d", receivers)
	}

	select {
	case msg := <-subA.MsgCh:
		expected := "*3\r\n$7\r\nmessage\r\n$12\r\nchat.general\r\n$9\r\nhello all\r\n"
		if string(msg) != expected {
			t.Fatalf("expected %q, got %q", expected, string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timeout waiting for subA message")
	}

	select {
	case msg := <-subB.MsgCh:
		expected := "*3\r\n$7\r\nmessage\r\n$12\r\nchat.general\r\n$9\r\nhello all\r\n"
		if string(msg) != expected {
			t.Fatalf("expected %q, got %q", expected, string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timeout waiting for subB message")
	}
}

func TestPubSub_PatternMatching(t *testing.T) {
	hub := NewShardedHub()
	sub := NewSubscriber(hub.NextSubscriberID())

	count := hub.PSubscribe(sub, "news.*")
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	receivers := hub.Publish("news.sports", []byte("goal scored"))
	if receivers != 1 {
		t.Fatalf("expected 1 receiver, got %d", receivers)
	}

	select {
	case msg := <-sub.MsgCh:
		expected := "*4\r\n$8\r\npmessage\r\n$6\r\nnews.*\r\n$11\r\nnews.sports\r\n$11\r\ngoal scored\r\n"
		if string(msg) != expected {
			t.Fatalf("expected %q, got %q", expected, string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timeout waiting for pattern message")
	}
}

func TestPubSub_UnsubscribeAndCleanUp(t *testing.T) {
	hub := NewShardedHub()
	sub := NewSubscriber(hub.NextSubscriberID())

	hub.Subscribe(sub, "c1")
	hub.Subscribe(sub, "c2")
	hub.PSubscribe(sub, "p*")

	if sub.SubCount() != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", sub.SubCount())
	}

	hub.Unsubscribe(sub, "c1")
	if sub.SubCount() != 2 {
		t.Fatalf("expected 2 subscriptions after c1 unsub, got %d", sub.SubCount())
	}

	hub.UnsubscribeAll(sub)
	if sub.SubCount() != 0 {
		t.Fatalf("expected 0 subscriptions after UnsubscribeAll, got %d", sub.SubCount())
	}

	receivers := hub.Publish("c2", []byte("msg"))
	if receivers != 0 {
		t.Fatalf("expected 0 receivers after cleanup, got %d", receivers)
	}
}

func TestPubSub_HighConcurrencyCrossShards(t *testing.T) {
	hub := NewShardedHub()
	const numSubs = 50
	const numPubs = 20
	const msgsPerPub = 50

	var subs []*Subscriber
	for i := 0; i < numSubs; i++ {
		s := NewSubscriber(hub.NextSubscriberID())
		hub.Subscribe(s, fmt.Sprintf("ch:%d", i%10))
		subs = append(subs, s)
	}

	var wg sync.WaitGroup
	wg.Add(numPubs)

	for p := 0; p < numPubs; p++ {
		go func(pubID int) {
			defer wg.Done()
			for m := 0; m < msgsPerPub; m++ {
				ch := fmt.Sprintf("ch:%d", (pubID+m)%10)
				_ = hub.Publish(ch, []byte("concurrent_data"))
			}
		}(p)
	}

	wg.Wait()

	// Drain messages
	for _, s := range subs {
		s.Close()
	}
}

package web

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func TestPresenceCountsTabsAndSeparatesDrafts(t *testing.T) {
	p := newDraftPresence()
	read := func(body []byte, want ...string) {
		t.Helper()
		var message struct {
			Online []string `json:"online_franchise_ids"`
		}
		if err := json.Unmarshal(body, &message); err != nil {
			t.Fatal(err)
		}
		if want == nil {
			want = []string{}
		}
		if !reflect.DeepEqual(message.Online, want) {
			t.Fatalf("online=%v, want %v", message.Online, want)
		}
	}
	messages, unsubscribe := p.events.Subscribe("draft")
	defer unsubscribe()
	body, firstTab := p.join("draft", "ann")
	read(body, "ann")
	read(<-messages, "ann")
	body, secondTab := p.join("draft", "ann")
	read(body, "ann")
	_, otherDraft := p.join("other-draft", "cy")
	defer otherDraft()
	_, bob := p.join("draft", "bob")
	read(<-messages, "ann", "bob")
	firstTab()
	firstTab() // cleanup must be idempotent
	select {
	case <-messages:
		t.Fatal("closing one tab changed presence while another remained")
	default:
	}
	bob()
	read(<-messages, "ann")
	secondTab()
	read(<-messages)
	if len(p.rooms["draft"]) != 0 {
		t.Fatal("last disconnect leaked presence")
	}
}

func TestPresenceConcurrentDisconnects(t *testing.T) {
	p := newDraftPresence()
	var group sync.WaitGroup
	for range 100 {
		_, leave := p.join("draft", "ann")
		group.Go(leave)
	}
	group.Wait()
	if len(p.rooms) != 0 {
		t.Fatal("concurrent connections leaked presence")
	}
}

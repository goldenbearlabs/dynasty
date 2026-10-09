package ingest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"crossover/internal/ingest"
)

func TestGetJSONRetriesOnce(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	saved := 0
	client := ingest.NewClient(0)
	client.Save = func(context.Context, string, []byte) { saved++ }

	var got struct{ OK bool }
	if err := client.GetJSON(context.Background(), server.URL, &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || requests != 2 || saved != 1 {
		t.Errorf("ok=%v requests=%d saved=%d, want true, 2, 1", got.OK, requests, saved)
	}

	// A feed that stays down is reported after the second attempt.
	down := httptest.NewServer(http.NotFoundHandler())
	defer down.Close()
	if err := client.GetJSON(context.Background(), down.URL, &got); err == nil {
		t.Error("GetJSON on a 404 returned no error")
	}
}

// A host that refuses is not asked again, by a retry or by the next
// request, until the cool-off has passed.
func TestClientLeavesARefusingHostAlone(t *testing.T) {
	requests := 0
	refuse := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if refuse {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := ingest.NewClient(0)
	client.CoolOff = 50 * time.Millisecond
	var v struct{}
	for range 3 {
		if err := client.GetJSON(context.Background(), server.URL, &v); !ingest.Refused(err) {
			t.Fatalf("err = %v, want a refusal", err)
		}
	}
	if requests != 1 {
		t.Fatalf("made %d requests to a host that refused, want 1", requests)
	}

	refuse = false
	time.Sleep(60 * time.Millisecond)
	if err := client.GetJSON(context.Background(), server.URL, &v); err != nil || requests != 2 {
		t.Fatalf("after the cool-off: err = %v after %d requests, want success on the second", err, requests)
	}
}

func TestClientSendsAHostThroughItsRelay(t *testing.T) {
	var asked string
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RequestURI()
		w.Write([]byte(`{}`))
	}))
	defer relay.Close()

	client := ingest.NewClient(0)
	client.Via = map[string]string{"feed.invalid": relay.URL}
	var v struct{}
	if err := client.GetJSON(context.Background(), "https://feed.invalid/api/v1/teams?sportId=1", &v); err != nil {
		t.Fatal(err)
	}
	if asked != "/api/v1/teams?sportId=1" {
		t.Errorf("relay was asked for %q, want the feed's path and query", asked)
	}
}

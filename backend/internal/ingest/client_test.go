package ingest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

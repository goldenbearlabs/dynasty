package web

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// stream upgrades the request to a websocket, sends the snapshot, and then
// forwards every message until the client leaves. The browser only listens:
// anything it wants to change goes through the ordinary HTTP endpoints.
// Callers subscribe before building the snapshot, so nothing is missed in
// between.
func (s *Server) stream(w http.ResponseWriter, r *http.Request, snapshot []byte, messages <-chan []byte) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept has already answered
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context()) // ends when the client goes away

	send := func(message []byte) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return conn.Write(ctx, websocket.MessageText, message)
	}
	if send(snapshot) != nil {
		return
	}
	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.Background.Done():
			conn.Close(websocket.StatusGoingAway, "server shutting down")
			return
		case message, open := <-messages:
			if !open { // fell behind; the client reconnects for a fresh snapshot
				conn.Close(websocket.StatusTryAgainLater, "reconnect")
				return
			}
			if send(message) != nil {
				return
			}
		case <-keepAlive.C:
			if conn.Ping(ctx) != nil {
				return
			}
		}
	}
}

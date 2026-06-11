package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 25 * time.Second
	outQueueSize = 256
)

// wsMsg is a buffered outgoing WebSocket frame.
type wsMsg struct {
	t    int
	data []byte
}

// Conn represents a single connected client.
type Conn struct {
	id  string
	ws  *websocket.Conn
	out chan wsMsg
	hub *Hub
}

func (c *Conn) sendText(text string) {
	select {
	case c.out <- wsMsg{websocket.TextMessage, []byte(text)}:
	default:
	}
}

func (c *Conn) emit(event string, args ...any) {
	data, _ := json.Marshal(append([]any{event}, args...))
	c.sendText("42" + string(data))
}

// Hub manages all connections and rooms.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*Conn
	rooms map[string]map[string]*Conn // roomID → set of Conn
}

func newHub() *Hub {
	return &Hub{
		conns: make(map[string]*Conn),
		rooms: make(map[string]map[string]*Conn),
	}
}

func (h *Hub) addConn(c *Conn) {
	h.mu.Lock()
	h.conns[c.id] = c
	h.mu.Unlock()
}

func (h *Hub) removeConn(c *Conn) {
	h.mu.Lock()
	delete(h.conns, c.id)
	for roomID, m := range h.rooms {
		delete(m, c.id)
		if len(m) == 0 {
			delete(h.rooms, roomID)
		}
	}
	h.mu.Unlock()
}

func (h *Hub) join(c *Conn, roomID string) {
	h.mu.Lock()
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[string]*Conn)
	}
	h.rooms[roomID][c.id] = c
	h.mu.Unlock()
}

func (h *Hub) leave(c *Conn, roomID string) {
	h.mu.Lock()
	if m := h.rooms[roomID]; m != nil {
		delete(m, c.id)
		if len(m) == 0 {
			delete(h.rooms, roomID)
		}
	}
	h.mu.Unlock()
}

// snapshot returns members of a room along with their IDs (locked together for consistency).
func (h *Hub) snapshot(roomID string) ([]*Conn, []string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	m := h.rooms[roomID]
	conns := make([]*Conn, 0, len(m))
	ids := make([]string, 0, len(m))
	for id, c := range m {
		conns = append(conns, c)
		ids = append(ids, id)
	}
	return conns, ids
}

// connRooms returns all room IDs the given connection is in.
func (h *Hub) connRooms(c *Conn) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var rooms []string
	for roomID, m := range h.rooms {
		if _, ok := m[c.id]; ok {
			rooms = append(rooms, roomID)
		}
	}
	return rooms
}

func (h *Hub) connByID(id string) (*Conn, bool) {
	h.mu.RLock()
	c, ok := h.conns[id]
	h.mu.RUnlock()
	return c, ok
}

// broadcast helpers

func emitAll(conns []*Conn, event string, args ...any) {
	data, _ := json.Marshal(append([]any{event}, args...))
	text := "42" + string(data)
	for _, c := range conns {
		c.sendText(text)
	}
}

func emitExcept(conns []*Conn, exceptID string, event string, args ...any) {
	data, _ := json.Marshal(append([]any{event}, args...))
	text := "42" + string(data)
	for _, c := range conns {
		if c.id != exceptID {
			c.sendText(text)
		}
	}
}

// relayBinary forwards a Socket.IO binary event to all conns in the snapshot except the sender.
// textFrame is the already-formatted "45N-[...]" string (without the EIO "4" prefix).
func relayBinary(conns []*Conn, exceptID string, textFrame string, bins [][]byte) {
	for _, c := range conns {
		if c.id == exceptID {
			continue
		}
		select {
		case c.out <- wsMsg{websocket.TextMessage, []byte(textFrame)}:
		default:
			continue
		}
		for _, b := range bins {
			select {
			case c.out <- wsMsg{websocket.BinaryMessage, b}:
			default:
			}
		}
	}
}

// event handlers

type followPayload struct {
	UserToFollow struct {
		SocketID string `json:"socketId"`
	} `json:"userToFollow"`
	Action string `json:"action"`
}

func (h *Hub) handleTextEvent(c *Conn, rawJSON string) {
	var args []json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &args); err != nil || len(args) == 0 {
		return
	}
	var event string
	json.Unmarshal(args[0], &event) //nolint

	switch event {
	case "join-room":
		if len(args) < 2 {
			return
		}
		var roomID string
		json.Unmarshal(args[1], &roomID) //nolint
		if roomID == "" {
			return
		}
		log.Printf("%s joined %s", c.id, roomID)
		h.join(c, roomID)
		members, ids := h.snapshot(roomID)
		if len(members) <= 1 {
			c.emit("first-in-room")
		} else {
			emitExcept(members, c.id, "new-user", c.id)
		}
		emitAll(members, "room-user-change", ids)

	case "user-follow":
		if len(args) < 2 {
			return
		}
		var p followPayload
		if err := json.Unmarshal(args[1], &p); err != nil {
			return
		}
		followRoom := "follow@" + p.UserToFollow.SocketID
		switch p.Action {
		case "FOLLOW":
			h.join(c, followRoom)
		case "UNFOLLOW":
			h.leave(c, followRoom)
		}
		_, ids := h.snapshot(followRoom)
		if target, ok := h.connByID(p.UserToFollow.SocketID); ok {
			target.emit("user-follow-room-change", ids)
		}
	}
}

func (h *Hub) handleBinaryEvent(c *Conn, jsonPart string, bins [][]byte) {
	var args []json.RawMessage
	if err := json.Unmarshal([]byte(jsonPart), &args); err != nil || len(args) < 2 {
		return
	}
	var event string
	json.Unmarshal(args[0], &event) //nolint

	switch event {
	case "server-broadcast", "server-volatile-broadcast":
		var roomID string
		json.Unmarshal(args[1], &roomID) //nolint
		if roomID == "" || len(bins) == 0 {
			return
		}
		// Rebuild as: 45N-["client-broadcast", {_placeholder,num:0}, ...]
		placeholders := make([]any, 1+len(bins))
		placeholders[0] = "client-broadcast"
		for i := range bins {
			placeholders[i+1] = map[string]any{"_placeholder": true, "num": i}
		}
		relayJSON, _ := json.Marshal(placeholders)
		textFrame := fmt.Sprintf("45%d-%s", len(bins), relayJSON)
		members, _ := h.snapshot(roomID)
		relayBinary(members, c.id, textFrame, bins)
	}
}

func (h *Hub) handleDisconnect(c *Conn) {
	log.Printf("%s disconnected", c.id)
	for _, roomID := range h.connRooms(c) {
		members, _ := h.snapshot(roomID)
		others := make([]*Conn, 0, len(members))
		otherIDs := make([]string, 0, len(members))
		for _, m := range members {
			if m.id != c.id {
				others = append(others, m)
				otherIDs = append(otherIDs, m.id)
			}
		}
		if strings.HasPrefix(roomID, "follow@") {
			if len(others) == 0 {
				targetID := strings.TrimPrefix(roomID, "follow@")
				if target, ok := h.connByID(targetID); ok {
					target.emit("broadcast-unfollow")
				}
			}
		} else if len(others) > 0 {
			emitAll(others, "room-user-change", otherIDs)
		}
	}
}

// WebSocket

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  65536,
	WriteBufferSize: 65536,
}

func newSID() string {
	b := make([]byte, 15)
	rand.Read(b) //nolint
	return base64.RawURLEncoding.EncodeToString(b)
}

func serve(hub *Hub, w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	defer ws.Close()

	sid := newSID()
	c := &Conn{id: sid, ws: ws, out: make(chan wsMsg, outQueueSize), hub: hub}
	hub.addConn(c)
	defer func() {
		hub.handleDisconnect(c)
		hub.removeConn(c)
		close(c.out)
	}()

	// Engine.IO OPEN packet
	openData, _ := json.Marshal(map[string]any{
		"sid":          sid,
		"upgrades":     []string{},
		"pingInterval": int(pingInterval / time.Millisecond),
		"pingTimeout":  20000,
		"maxPayload":   1_000_000,
	})
	ws.WriteMessage(websocket.TextMessage, append([]byte("0"), openData...))

	// Writer goroutine
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-c.out:
				if !ok {
					return
				}
				if err := ws.WriteMessage(msg.t, msg.data); err != nil {
					return
				}
			case <-ticker.C:
				if err := ws.WriteMessage(websocket.TextMessage, []byte("2")); err != nil {
					return
				}
			}
		}
	}()

	// Binary attachment accumulator
	var (
		pendingJSON  string
		pendingBins  [][]byte
		pendingCount int
		connected    bool
	)

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			break
		}

		// Binary frame: attachment for current binary event
		if mt == websocket.BinaryMessage {
			if pendingCount > 0 {
				pendingBins = append(pendingBins, data)
				if len(pendingBins) == pendingCount {
					hub.handleBinaryEvent(c, pendingJSON, pendingBins)
					pendingJSON, pendingBins, pendingCount = "", nil, 0
				}
			}
			continue
		}

		text := string(data)
		if len(text) < 2 {
			continue
		}

		switch text[0] {
		case '2': // EIO PING → PONG
			c.sendText("3")

		case '4': // EIO MESSAGE
			switch text[1] {
			case '0': // Socket.IO CONNECT
				if !connected {
					// Confirm namespace connection, then announce init-room
					ws.WriteMessage(websocket.TextMessage, []byte(`40{"sid":"`+sid+`"}`))
					c.emit("init-room")
					connected = true
				}
			case '1': // Socket.IO DISCONNECT
				return
			case '2': // Socket.IO EVENT
				if connected {
					hub.handleTextEvent(c, text[2:])
				}
			case '5': // Socket.IO BINARY_EVENT: "45N-[json]"
				if connected {
					rest := text[2:]
					dash := strings.IndexByte(rest, '-')
					if dash < 0 {
						continue
					}
					count := 0
					for _, ch := range rest[:dash] {
						if ch >= '0' && ch <= '9' {
							count = count*10 + int(ch-'0')
						}
					}
					pendingJSON = rest[dash+1:]
					pendingBins = nil
					pendingCount = count
				}
			}
		}
	}
}

func corsMiddleware(origin string, next http.Handler) http.Handler {
	if origin == "" {
		origin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if origin != "*" {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3002"
	}

	hub := newHub()
	mux := http.NewServeMux()

	mux.HandleFunc("/socket.io/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("EIO") != "4" || q.Get("transport") != "websocket" {
			http.Error(w, "only EIO=4 websocket transport is supported", http.StatusBadRequest)
			return
		}
		serve(hub, w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Excalidraw collaboration server is up :)")
	})

	handler := corsMiddleware(os.Getenv("CORS_ORIGIN"), mux)
	log.Printf("Listening on :%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

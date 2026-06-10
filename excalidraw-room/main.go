package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/rs/cors"
	"github.com/zishang520/socket.io/v2/socket"
)

type UserToFollow struct {
	SocketID string `json:"socketId"`
	Username string `json:"username"`
}

type OnUserFollowedPayload struct {
	UserToFollow UserToFollow `json:"userToFollow"`
	Action       string       `json:"action"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3002"
	}

	corsOrigin := os.Getenv("CORS_ORIGIN")

	io := socket.NewServer(nil, nil)

	io.On("connection", func(clients ...any) {
		s := clients[0].(*socket.Socket)
		log.Printf("connection established: %s", s.Id())
		s.Emit("init-room")

		s.On("join-room", func(args ...any) {
			if len(args) == 0 {
				return
			}
			roomID, ok := args[0].(string)
			if !ok {
				return
			}
			log.Printf("%s has joined %s", s.Id(), roomID)
			s.Join(socket.Room(roomID))

			io.In(socket.Room(roomID)).FetchSockets()(func(sockets []*socket.RemoteSocket, err error) {
				if err != nil {
					log.Printf("FetchSockets error: %v", err)
					return
				}
				if len(sockets) <= 1 {
					s.Emit("first-in-room")
				} else {
					log.Printf("%s new-user emitted to room %s", s.Id(), roomID)
					s.To(socket.Room(roomID)).Emit("new-user", string(s.Id()))
				}
				io.In(socket.Room(roomID)).Emit("room-user-change", remoteSocketIDs(sockets))
			})
		})

		s.On("server-broadcast", func(args ...any) {
			if len(args) < 3 {
				return
			}
			roomID, ok := args[0].(string)
			if !ok {
				return
			}
			log.Printf("%s sends update to %s", s.Id(), roomID)
			s.To(socket.Room(roomID)).Emit("client-broadcast", args[1], args[2])
		})

		s.On("server-volatile-broadcast", func(args ...any) {
			if len(args) < 3 {
				return
			}
			roomID, ok := args[0].(string)
			if !ok {
				return
			}
			log.Printf("%s sends volatile update to %s", s.Id(), roomID)
			s.To(socket.Room(roomID)).Volatile().Emit("client-broadcast", args[1], args[2])
		})

		s.On("user-follow", func(args ...any) {
			if len(args) == 0 {
				return
			}
			var payload OnUserFollowedPayload
			switch v := args[0].(type) {
			case string:
				if err := json.Unmarshal([]byte(v), &payload); err != nil {
					log.Printf("user-follow parse error: %v", err)
					return
				}
			case map[string]any:
				data, _ := json.Marshal(v)
				if err := json.Unmarshal(data, &payload); err != nil {
					log.Printf("user-follow parse error: %v", err)
					return
				}
			default:
				return
			}

			followRoom := socket.Room("follow@" + payload.UserToFollow.SocketID)

			switch payload.Action {
			case "FOLLOW":
				s.Join(followRoom)
				io.In(followRoom).FetchSockets()(func(sockets []*socket.RemoteSocket, err error) {
					if err == nil {
						io.To(socket.Room(payload.UserToFollow.SocketID)).Emit("user-follow-room-change", remoteSocketIDs(sockets))
					}
				})
			case "UNFOLLOW":
				s.Leave(followRoom)
				io.In(followRoom).FetchSockets()(func(sockets []*socket.RemoteSocket, err error) {
					if err == nil {
						io.To(socket.Room(payload.UserToFollow.SocketID)).Emit("user-follow-room-change", remoteSocketIDs(sockets))
					}
				})
			}
		})

		s.On("disconnecting", func(args ...any) {
			log.Printf("%s has disconnected", s.Id())
			for _, roomID := range s.Rooms().Keys() {
				room := socket.Room(roomID)
				io.In(room).FetchSockets()(func(sockets []*socket.RemoteSocket, err error) {
					if err != nil {
						return
					}
					others := make([]*socket.RemoteSocket, 0, len(sockets))
					for _, sock := range sockets {
						if sock.Id() != s.Id() {
							others = append(others, sock)
						}
					}
					isFollow := strings.HasPrefix(string(roomID), "follow@")
					if !isFollow && len(others) > 0 {
						s.To(room).Emit("room-user-change", remoteSocketIDs(others))
					}
					if isFollow && len(others) == 0 {
						targetID := strings.TrimPrefix(string(roomID), "follow@")
						io.To(socket.Room(targetID)).Emit("broadcast-unfollow")
					}
				})
			}
		})

		s.On("disconnect", func(args ...any) {
			s.RemoveAllListeners("join-room")
			s.RemoveAllListeners("server-broadcast")
			s.RemoveAllListeners("server-volatile-broadcast")
			s.RemoveAllListeners("user-follow")
			s.RemoveAllListeners("disconnecting")
		})
	})

	mux := http.NewServeMux()
	mux.Handle("/socket.io/", io.ServeHandler(nil))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Excalidraw collaboration server is up :)"))
	})

	var handler http.Handler
	if corsOrigin == "" || corsOrigin == "*" {
		handler = cors.AllowAll().Handler(mux)
	} else {
		handler = cors.New(cors.Options{
			AllowedOrigins:   []string{corsOrigin},
			AllowedHeaders:   []string{"Content-Type", "Authorization"},
			AllowCredentials: true,
		}).Handler(mux)
	}

	log.Printf("Listening on :%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func remoteSocketIDs(sockets []*socket.RemoteSocket) []string {
	ids := make([]string, len(sockets))
	for i, s := range sockets {
		ids[i] = string(s.Id())
	}
	return ids
}

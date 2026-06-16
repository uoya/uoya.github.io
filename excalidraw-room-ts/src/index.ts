import http from "http";
import { Server as SocketIO } from "socket.io";
import { initStorage, loadRoomState, scheduleSave } from "./storage.js";

type UserToFollow = {
  socketId: string;
  username: string;
};
type OnUserFollowedPayload = {
  userToFollow: UserToFollow;
  action: "FOLLOW" | "UNFOLLOW";
};

const port = process.env.PORT ?? 3002;

const httpServer = http.createServer((_req, res) => {
  res.end("Excalidraw collaboration server is up :)");
});

httpServer.listen(port, () => {
  console.log(`listening on port: ${port}`);
});

const io = new SocketIO(httpServer, {
  transports: ["websocket", "polling"],
  cors: {
    allowedHeaders: ["Content-Type", "Authorization"],
    origin: process.env.CORS_ORIGIN ?? "*",
    credentials: true,
  },
  allowEIO3: true,
});

initStorage().catch(console.error);

io.on("connection", (socket) => {
  io.to(socket.id).emit("init-room");

  socket.on("join-room", async (roomID) => {
    await socket.join(roomID);
    const sockets = await io.in(roomID).fetchSockets();
    if (sockets.length <= 1) {
      io.to(socket.id).emit("first-in-room");
      // Restore persisted room state for the first client in an empty room
      loadRoomState(roomID)
        .then((state) => {
          if (state) {
            io.to(socket.id).emit("client-broadcast", state.encryptedData, state.iv);
          }
        })
        .catch(console.error);
    } else {
      socket.broadcast.to(roomID).emit("new-user", socket.id);
    }
    io.in(roomID).emit(
      "room-user-change",
      sockets.map((s) => s.id),
    );
  });

  socket.on("server-broadcast", (roomID: string, encryptedData: ArrayBuffer, iv: Uint8Array) => {
    socket.broadcast.to(roomID).emit("client-broadcast", encryptedData, iv);
    scheduleSave(roomID, encryptedData, iv);
  });

  socket.on(
    "server-volatile-broadcast",
    (roomID: string, encryptedData: ArrayBuffer, iv: Uint8Array) => {
      socket.volatile.broadcast.to(roomID).emit("client-broadcast", encryptedData, iv);
      // Volatile broadcasts carry cursor/pointer positions — skip persistence
    },
  );

  socket.on("user-follow", async (payload: OnUserFollowedPayload) => {
    const roomID = `follow@${payload.userToFollow.socketId}`;

    switch (payload.action) {
      case "FOLLOW": {
        await socket.join(roomID);
        const sockets = await io.in(roomID).fetchSockets();
        io.to(payload.userToFollow.socketId).emit(
          "user-follow-room-change",
          sockets.map((s) => s.id),
        );
        break;
      }
      case "UNFOLLOW": {
        await socket.leave(roomID);
        const sockets = await io.in(roomID).fetchSockets();
        io.to(payload.userToFollow.socketId).emit(
          "user-follow-room-change",
          sockets.map((s) => s.id),
        );
        break;
      }
    }
  });

  socket.on("disconnecting", async () => {
    for (const roomID of socket.rooms) {
      const otherClients = (await io.in(roomID).fetchSockets()).filter((s) => s.id !== socket.id);
      const isFollowRoom = roomID.startsWith("follow@");

      if (!isFollowRoom && otherClients.length > 0) {
        socket.broadcast.to(roomID).emit(
          "room-user-change",
          otherClients.map((s) => s.id),
        );
      }

      if (isFollowRoom && otherClients.length === 0) {
        io.to(roomID.replace("follow@", "")).emit("broadcast-unfollow");
      }
    }
  });

  socket.on("disconnect", () => {
    socket.removeAllListeners();
    socket.disconnect();
  });
});

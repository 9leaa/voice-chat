package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Client struct {
	conn *websocket.Conn
	room string
	name string
}
type Message struct {
	Type    string          `json:"type"`
	From    string          `json:"from"`
	To      string          `json:"to"`
	Room    string          `json:"room"`
	Content json.RawMessage `json:"content"`
}

var (
	clients  = make(map[*Client]bool)
	userList = make(map[string]*Client)
	mu       sync.Mutex
)

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}
	defer ws.Close()
	client := &Client{conn: ws}
	mu.Lock()
	clients[client] = true
	mu.Unlock()
	log.Printf("新连接: %s", r.RemoteAddr)
	for {
		var msg Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("读取错误: %v", err)
			} else {
				log.Printf("连接正常关闭: %v", err)
			}
			break
		}
		switch msg.Type {
		case "join":
			client.room = msg.Room
			client.name = msg.From
			mu.Lock()
			if oldClient, exists := userList[msg.From]; exists {
				oldClient.conn.Close()
				delete(clients, oldClient)
			}
			userList[msg.From] = client
			broadcastUserList()
			mu.Unlock()
			log.Printf("%s 加入房间 %s", msg.From, msg.Room)
		case "offer", "answer", "candidate":
			mu.Lock()
			if target, ok := userList[msg.To]; ok {
				err := target.conn.WriteJSON(msg)
				if err != nil {
					log.Printf("转发信令错误: %v", err)
				}
			}
			mu.Unlock()
		}
	}
	mu.Lock()
	delete(clients, client)
	if client.name != "" {
		delete(userList, client.name)
		broadcastUserList()
	}
	mu.Unlock()
}
func broadcastUserList() {
	users := make([]string, 0, len(userList))
	for username := range userList {
		users = append(users, username)
	}
	userListMsg := struct {
		Type  string   `json:"type"`
		Users []string `json:"users"`
	}{
		Type:  "userlist",
		Users: users,
	}
	for client := range clients {
		err := client.conn.WriteJSON(userListMsg)
		if err != nil {
			log.Printf("广播用户列表错误: %v", err)
		}
	}
}
func StartSignalingServer(port string) {
	http.HandleFunc("/ws", handleConnections)
	log.Printf("信令服务器运行在端口 %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

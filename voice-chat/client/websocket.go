package main

import (
	"encoding/json"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketClient struct {
	Conn   *websocket.Conn
	UserID string
}

func NewWebSocketClient(serverAddr, userID string) *WebSocketClient {
	u := url.URL{Scheme: "ws", Host: serverAddr, Path: "/ws"}
	wsURL := u.String()
	log.Printf("正在连接到服务器: %s", wsURL)
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		log.Printf("连接服务器失败: %v", err)
		return nil
	}
	joinMsg := map[string]interface{}{
		"type": "join",
		"from": userID,
		"room": "main",
	}
	if err := conn.WriteJSON(joinMsg); err != nil {
		log.Printf("发送加入消息失败: %v", err)
		conn.Close()
		return nil
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Printf("心跳检测失败: %v", err)
					conn.Close()
					return
				}
			}
		}
	}()
	return &WebSocketClient{
		Conn:   conn,
		UserID: userID,
	}
}
func (c *WebSocketClient) ReadMessages(handler func(msgType string, data json.RawMessage)) {
	for {
		_, p, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket读取错误: %v", err)
			} else {
				log.Printf("WebSocket连接关闭: %v", err)
			}
			return
		}
		var msg struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
			Users   []string        `json:"users"`
		}
		if err := json.Unmarshal(p, &msg); err != nil {
			log.Printf("解析消息错误: %v", err)
			continue
		}
		switch msg.Type {
		case "userlist":
			usersData, _ := json.Marshal(msg.Users)
			handler(msg.Type, usersData)
		default:
			handler(msg.Type, msg.Content)
		}
	}
}
func (c *WebSocketClient) SendSignal(signalType, to string, data interface{}) {
	if c.Conn == nil {
		return
	}
	msg := map[string]interface{}{
		"type": signalType,
		"from": c.UserID,
		"to":   to,
		"data": data,
	}
	c.Conn.WriteJSON(msg)
}

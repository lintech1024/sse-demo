package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

type ClientManager struct {
	clients map[string]chan []byte // 用户ID -> 消息通道
	mutex   sync.RWMutex
}

var manager = ClientManager{
	clients: make(map[string]chan []byte),
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", sseHandler)
	mux.HandleFunc("/send", sendHandler)
	log.Println("SSE server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	message := r.URL.Query().Get("message")
	log.Printf("Sending message to %s: %s", uid, message)

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	if msgChan, ok := manager.clients[uid]; ok {
		msgChan <- []byte(message)
	} else {
		http.Error(w, "Client not found", http.StatusNotFound)
	}
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	log.Printf("Client connected: %s", uid)
	// 设置 SSE 必要的响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// 获取 Flusher 接口用于手动刷新缓冲区
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}
	msgChan := make(chan []byte)
	manager.mutex.Lock()
	manager.clients[uid] = msgChan
	manager.mutex.Unlock()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			close(msgChan)
			delete(manager.clients, uid)
			log.Println("Client disconnected")
			return
		case msg := <-msgChan:
			message := fmt.Sprintf("data: %s\n\n", msg)
			if _, err := w.Write([]byte(message)); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
			flusher.Flush()
		}
	}
}

package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

type ClientManager struct {
	clients    map[string]chan []byte // 用户ID -> 消息通道
	mutex      sync.RWMutex
	groups     map[string]map[string]struct{} // 群组ID -> 用户ID集合
	groupMutex sync.RWMutex
}

var manager = ClientManager{
	clients: make(map[string]chan []byte),
	groups:  make(map[string]map[string]struct{}),
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", sseHandler)
	mux.HandleFunc("/send", sendHandler)
	mux.HandleFunc("/group/in", groupInHandler)
	mux.HandleFunc("/group/out", groupOutHandler)
	log.Println("SSE server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func groupInHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Header.Get("X-User-Id")
	gid := r.URL.Query().Get("gid")
	log.Printf("Group in: %s, %s", uid, gid)
	if uid == "" || gid == "" {
		http.Error(w, "Missing user ID or group ID", http.StatusBadRequest)
		return
	}
	manager.groupMutex.Lock()
	defer manager.groupMutex.Unlock()
	if _, ok := manager.groups[gid]; !ok {
		manager.groups[gid] = make(map[string]struct{})
	}
	manager.groups[gid][uid] = struct{}{}
	log.Printf("User %s joined group %s", uid, gid)
	w.WriteHeader(http.StatusOK)
}

func groupOutHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Header.Get("X-User-Id")
	gid := r.URL.Query().Get("gid")
	log.Printf("Group in: %s, %s", uid, gid)
	if uid == "" || gid == "" {
		http.Error(w, "Missing user ID or group ID", http.StatusBadRequest)
		return
	}
	manager.groupMutex.Lock()
	defer manager.groupMutex.Unlock()
	if _, ok := manager.groups[gid]; !ok {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}
	if _, ok := manager.groups[gid][uid]; !ok {
		http.Error(w, "User not in group", http.StatusNotFound)
		return
	}
	delete(manager.groups[gid], uid)
	if len(manager.groups[gid]) == 0 {
		delete(manager.groups, gid)
	}
	log.Printf("User %s left group %s", uid, gid)
	w.WriteHeader(http.StatusOK)
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Header.Get("X-User-Id")
	gid := r.URL.Query().Get("gid")
	if uid == "" || gid == "" {
		http.Error(w, "Missing user ID or group ID", http.StatusBadRequest)
		return
	}
	msg := r.URL.Query().Get("msg")
	if msg == "" {
		http.Error(w, "Missing message", http.StatusBadRequest)
		return
	}
	manager.groupMutex.RLock()
	defer manager.groupMutex.RUnlock()
	if _, ok := manager.groups[gid]; !ok {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}
	if len(manager.groups[gid]) == 0 {
		http.Error(w, "Group is empty", http.StatusNotFound)
		return
	}
	log.Printf("Send message to group %s: %s", gid, msg)
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	for userID := range manager.groups[gid] {
		msgChan, ok := manager.clients[userID]
		if !ok {
			continue // 用户不在线，跳过
		}
		select {
		case msgChan <- []byte(msg):
			log.Printf("Message sent to %s", userID)
		default:
			log.Printf("System busy: %s", userID)
			http.Error(w, "System busy", http.StatusTooManyRequests)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Header.Get("X-User-Id")
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

	defer func() {
		manager.mutex.Lock()
		delete(manager.clients, uid)
		manager.mutex.Unlock()
		close(msgChan)
		log.Printf("Client disconnected: %s", uid)
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-msgChan:
			message := fmt.Sprintf("data: %s\n\n", msg)
			log.Printf("receive message to %s: %s", uid, message)
			if _, err := w.Write([]byte(message)); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
			flusher.Flush()
		}
	}
}

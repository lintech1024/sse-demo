package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	http.HandleFunc("/sse", sseHandler)
	log.Println("SSE server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	// 设置 SSE 必要的响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // 允许跨域访问

	// 获取 Flusher 接口用于手动刷新缓冲区
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			log.Println("Client disconnected")
			return
		default:
			// 构造事件数据（使用 RFC3339 时间格式作为示例）
			message := fmt.Sprintf("data: %s\n\n", time.Now().Format(time.RFC3339))

			// 写入响应并立即刷新
			if _, err := w.Write([]byte(message)); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
			flusher.Flush()

			// 每秒发送一次事件
			time.Sleep(1 * time.Second)
		}
	}
}

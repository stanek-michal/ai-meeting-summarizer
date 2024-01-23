package main

import (
    "log"
    "net/http"
    "github.com/stanek-michal/go-ai-summarizer/internal/transport"
    "github.com/stanek-michal/go-ai-summarizer/pkg/queue"
)

func main() {
    // Initialize the queue
    taskQueue := queue.NewQueue()
    go taskQueue.StartProcessing()

    // Initialize the HTTP server
    httpHandler := transport.NewHTTPHandler(taskQueue)
    http.HandleFunc("/", httpHandler.ServeHTTP)

    // Serve static files
    fs := http.FileServer(http.Dir("./web/static"))
    http.Handle("/static/", http.StripPrefix("/static/", fs))

    // Start the server
    log.Println("Starting server on :9001")
    if err := http.ListenAndServe(":9001", nil); err != nil {
        log.Fatal("ListenAndServe: ", err)
    }
}

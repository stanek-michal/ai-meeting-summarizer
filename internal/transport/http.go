package transport

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "github.com/stanek-michal/go-ai-summarizer/pkg/queue"
)

type HTTPHandler struct {
    Queue *queue.Queue
}

func NewHTTPHandler(q *queue.Queue) *HTTPHandler {
    return &HTTPHandler{Queue: q}
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/upload":
        h.handleFileUpload(w, r)
    case "/status":
        h.handleStatus(w, r)
    default:
        http.NotFound(w, r)
    }
}

func (h *HTTPHandler) handleFileUpload(w http.ResponseWriter, r *http.Request) {
    // Parse the multipart form
    const maxUploadSize = 10 << 20 // 10 MB
    if err := r.ParseMultipartForm(maxUploadSize); err != nil {
        http.Error(w, "File too large", http.StatusBadRequest)
        return
    }

    file, _, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "Invalid file", http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Enqueue the file processing task
    taskID, err := h.Queue.Enqueue(file)
    if err != nil {
        http.Error(w, "Error processing file", http.StatusInternalServerError)
        return
    }

    // Respond with the task ID
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{"task_id": taskID})
}

func (h *HTTPHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
    // Extract task ID from query parameters
    taskID := r.URL.Query().Get("id")
    if taskID == "" {
        http.Error(w, "Missing task ID", http.StatusBadRequest)
        return
    }

    // Get the task status
    status, err := h.Queue.Status(taskID)
    if err != nil {
        http.Error(w, "Invalid task ID", http.StatusBadRequest)
        return
    }

    // Respond with the task status
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(status)
}

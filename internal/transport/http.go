package transport

import (
    "encoding/json"
//    "fmt"
 //   "io"
    "net/http"
    "github.com/stanek-michal/go-ai-summarizer/pkg/queue"
)

type HTTPHandler struct {
    Queue *queue.Queue
}

func NewHTTPHandler(q *queue.Queue) *HTTPHandler {
    return &HTTPHandler{Queue: q}
}

func (h *HTTPHandler) HandleFileUpload(w http.ResponseWriter, r *http.Request) {
    // Set the maximum allowed file size to 10GB
    const maxUploadSize = 10 << 30 // 10 GB

    // Check the size of the request body (optional but recommended)
    if r.ContentLength > maxUploadSize {
        http.Error(w, "The uploaded file is too large", http.StatusRequestEntityTooLarge)
        return
    }

    // Wrap the request body with a MaxBytesReader to enforce the size limit
    r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

    // Parse the multipart form
    if err := r.ParseMultipartForm(maxUploadSize); err != nil {
        // Handle the case where the file is too large to fit in memory
        if err == http.ErrHandlerTimeout {
            http.Error(w, "The uploaded file is too large", http.StatusRequestEntityTooLarge)
        } else {
            http.Error(w, "Error parsing multipart form", http.StatusInternalServerError)
        }
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

func (h *HTTPHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
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

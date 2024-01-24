package transport

import (
    "encoding/json"
//    "fmt"
    "io"
    "os"
    "strings"
    "strconv"
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
        if err == http.ErrHandlerTimeout { // TODO this is probably wrong
            http.Error(w, "The uploaded file is too large", http.StatusRequestEntityTooLarge)
        } else {
            http.Error(w, "Error parsing multipart form", http.StatusInternalServerError)
        }
        return
    }

    file, header, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "Invalid file", http.StatusBadRequest)
        return
    }
    defer file.Close()

    var suffix string
    // Check file type (must be .wav or .mp4)
    if strings.HasSuffix(header.Filename, ".wav") {
       suffix = "wav"
    } else if strings.HasSuffix(header.Filename, ".mp4") {
       suffix = "mp4"
    } else {
        http.Error(w, "File must be a .wav or .mp4", http.StatusBadRequest)
        return
    }

    // Create a temporary file on the disk to save the uploaded content
    tempFile, err := os.CreateTemp("", "upload-*." + suffix)
    if err != nil {
        http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
        return
    }
    defer tempFile.Close()

    // Copy the contents of the uploaded file to the temp file
    _, err = io.Copy(tempFile, file)
    if err != nil {
        http.Error(w, "Failed to save file", http.StatusInternalServerError)
        return
    }

    // The path to the temp file
    filePath := tempFile.Name()

    // Enqueue the file path for processing
    taskID, err := h.Queue.Enqueue(filePath)
    if err != nil {
        http.Error(w, "Error processing file", http.StatusInternalServerError)
        return
    }

    // Respond with the task ID
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{"task_id": strconv.Itoa(taskID)})
}

func (h *HTTPHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
    // Extract task ID from query parameters
    taskID := r.URL.Query().Get("id")
    if taskID == "" {
        http.Error(w, "Missing task ID", http.StatusBadRequest)
        return
    }

    // Get the task status
    idNum, err := strconv.Atoi(taskID)
    if err != nil {
        http.Error(w, "Invalid task ID", http.StatusBadRequest)
        return
    }
    taskInfo, err := h.Queue.GetTaskInfo(idNum)
    if err != nil {
        http.Error(w, "Invalid task ID", http.StatusBadRequest)
        return
    }
    if taskInfo.Status == "completed" || taskInfo.Status == "failed" {
	// Can be removed from the queue now - client is getting result
        h.Queue.Cleanup(idNum)
    }

    // Respond with the task status
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(taskInfo)
}

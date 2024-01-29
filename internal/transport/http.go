package transport

import (
    "encoding/json"
//    "fmt"
    "io"
    "fmt"
    "io/ioutil"
    "os"
    "strings"
    "strconv"
    "sync"
    "net/http"
    "github.com/stanek-michal/go-ai-summarizer/pkg/queue"
)

// Mutex for counter.txt (visitor counter)
var counterMutex sync.Mutex
// Mutex for testimonials.txt (visitors book)
var testimonialsMutex sync.Mutex

const counterPath = "web/counter.txt"

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

func getCounter() (int, error) {
    // Read the current counter value from the file
    data, err := ioutil.ReadFile(counterPath)
    if err != nil {
        // Check if the error is because the file does not exist
        if os.IsNotExist(err) {
            // Create the file with an initial count of 0
            initialCount := []byte("0\n")
            err = ioutil.WriteFile(counterPath, initialCount, 0644) // 0644 is a common permission setting allowing read/write for owner and read for others
            if err != nil {
                return 0, err
            }
            return 0, nil // Successfully created the file with initial count of 0
        } else {
            // Return the original error if it was not because the file does not exist
            return 0, err
        }
    }

    // Convert the counter from string to int
    count, err := strconv.Atoi(strings.TrimSpace(string(data)))
    if err != nil {
        return 0, err
    }

    return count, nil
}

func incrementCounter() (int, error) {
    counterMutex.Lock()
    defer counterMutex.Unlock()

    count, err := getCounter()
    if err != nil {
        return 0, err
    }

    count++
    err = ioutil.WriteFile(counterPath, []byte(strconv.Itoa(count)), 0644)
    if err != nil {
        return 0, err
    }

    return count, nil
}

func (h *HTTPHandler) HandleCounter(w http.ResponseWriter, r *http.Request) {
    count, err := incrementCounter()
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
    fmt.Fprintf(w, "%d", count)
}

func (h *HTTPHandler) HandleTasksInQueue(w http.ResponseWriter, r *http.Request) {
    queueLength, err := h.Queue.GetQueueLength()
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // Respond with the number of tasks in the queue
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]int{"tasks_in_queue": queueLength})
}

func (h *HTTPHandler) GetTestimonials(w http.ResponseWriter, r *http.Request) {
    testimonialsMutex.Lock()
    defer testimonialsMutex.Unlock()

    testimonials, err := ioutil.ReadFile("web/testimonials.txt")
    if err != nil {
        http.Error(w, "File reading error", 500)
        return
    }
    w.Write(testimonials)
}

func (h *HTTPHandler) SubmitTestimonial(w http.ResponseWriter, r *http.Request) {
    testimonialsMutex.Lock()
    defer testimonialsMutex.Unlock()

    if r.Method != "POST" {
        http.Error(w, "Invalid request method", 405)
        return
    }

    testimonial, err := ioutil.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Invalid request", 400)
        return
    }

    trimmedTestimonial := strings.TrimSpace(string(testimonial))
    if len(trimmedTestimonial) == 0 {
        http.Error(w, "Testimonial cannot be empty", 400)
        return
    }

    // Open the file with append and write-only mode
    f, err := os.OpenFile("web/testimonials.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
    if err != nil {
        http.Error(w, "File opening error", 500)
        return
    }
    defer f.Close()

    // Append the new testimonial with a newline
    _, err = f.WriteString(trimmedTestimonial + "\n")
    if err != nil {
        http.Error(w, "File writing error", 500)
        return
    }
}


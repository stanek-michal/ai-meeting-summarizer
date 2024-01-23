package queue

import (
	"fmt"
	"io"
	"github.com/stanek-michal/go-ai-summarizer/internal/processing"
	"sync"
)

// Result contains a full transcript and a text summary of it
type Result struct {
	Transcript string
	Summary    string
}
// Task represents a processing task
type Task struct {
	ID     string
	Status string
	Result Result
}

// Queue represents a queue of tasks to be processed
type Queue struct {
	tasks      map[string]*Task
	processing chan struct{}
	mu         sync.Mutex
	processor  *processing.Processor
}

func NewQueue() *Queue {
	return &Queue{
		tasks:      make(map[string]*Task),
		processing: make(chan struct{}, 1),
		processor:  processing.NewProcessor(),
	}
}

// StartProcessing starts the processing of tasks in the queue
func (q *Queue) StartProcessing() {
	for {
		q.mu.Lock()
		var taskToProcess *Task
		for _, task := range q.tasks {
			if task.Status == "waiting" {
				taskToProcess = task
				task.Status = "processing"
				break
			}
		}
		q.mu.Unlock()

		if taskToProcess != nil {
			// Process the task
			result, err := q.processor.Process(nil) // Replace nil with the actual file reader
			q.mu.Lock()
			if err != nil {
				taskToProcess.Status = "failed"
				taskToProcess.Result = err.Error()
			} else {
				taskToProcess.Status = "completed"
				taskToProcess.Result = result
			}
			q.mu.Unlock()
		} else {
			// No tasks to process, wait for a new task
			<-q.processing
		}
	}
}

// Enqueue adds a new task to the queue
func (q *Queue) Enqueue(file io.Reader) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Generate a unique identifier for the task
	taskID := fmt.Sprintf("task-%d", len(q.tasks)+1)
	q.tasks[taskID] = &Task{ID: taskID, Status: "waiting"}

	// Notify the processing loop that there is a new task
	select {
		case q.processing <- struct{}{}: // Notify if the channel is empty
		default: // Do nothing if there's already a notification pending
	}

	return taskID, nil
}

// Status returns the status of a task by its ID
func (q *Queue) Status(taskID string) (*Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	return task, nil
}

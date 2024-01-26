package queue

import (
	"fmt"
	"strconv"
	"sync"
	"github.com/stanek-michal/go-ai-summarizer/internal/processing"
	"github.com/stanek-michal/go-ai-summarizer/pkg/types"
)

// Queue represents a queue of tasks to be processed
type Queue struct {
	taskLookup map[int]*types.Task    // For task status lookup
	taskQueue []*types.Task           // FIFO queue to maintain order and garbage collect old unclaimed tasks
	processing chan *types.Task       // enqueue tasks for processing
	lastID     int                    // last ID that was used for a task - ever incrementing counter
	mu         sync.Mutex
	processor  *processing.Processor
}

func NewQueue() *Queue {
	return &Queue{
		taskLookup: make(map[int]*types.Task),
		taskQueue:  make([]*types.Task, 0),
		processing: make(chan *types.Task, 1), // Limit to 1 as AI engine can process one thing at a time
		lastID:     0,
		processor:  processing.NewProcessor(),
	}
}

// StartProcessing processes one task at a time in infinite loop
func (q *Queue) StartProcessing() {
	for task := range q.processing {
		filename := ""
		q.mu.Lock()
		filename = task.FileName
		task.Status = "processing"
		q.mu.Unlock()

		result, err := q.processor.Process(filename)

		q.mu.Lock()
		task.Result = result
		if err != nil {
			task.Status = "failed"
		} else {
			task.Status = "completed"
		}
		q.mu.Unlock()
	}
}

// Enqueue adds a new task to the queue
func (q *Queue) Enqueue(fileName string) (int, error) {
	// Garbage collect old completed entries if we accumulated too many
	q.GarbageCollectOldEntries()

	q.mu.Lock()

	// Generate a unique identifier for the task
        q.lastID++
	taskID := q.lastID
	task := &types.Task{
		ID:       strconv.Itoa(taskID),
		FileName: fileName,
		Status:   "waiting",
	}
	q.taskQueue = append(q.taskQueue, task)
	q.taskLookup[taskID] = task
	q.mu.Unlock()

	// Send task to processor channel
	go func() {
		q.processing <- task
	}()

	return taskID, nil
}

// Cleanup a task by ID (only if its status is "completed" or "failed")
func (q *Queue) Cleanup(taskID int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.taskLookup[taskID]
	if !ok {
		return // Task not found
	}

	// Check if the task is completed before removing it
	if task.Status == "completed" || task.Status == "failed" {
		// Remove from taskLookup
		delete(q.taskLookup, taskID)
		// Remove from taskQueue
		for i, t := range q.taskQueue {
			if t.ID == strconv.Itoa(taskID) {
				q.taskQueue = append(q.taskQueue[:i], q.taskQueue[i+1:]...)
				break
			}
		}
	}
}

// Garbage collect old completed/failed entries if unclaimed by clients
func (q *Queue) GarbageCollectOldEntries() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.taskQueue) < 50 {
		return // Not enough to garbage collect yet
	}
	if q.taskQueue[0].Status == "waiting" || q.taskQueue[0].Status == "processing" {
		return // No completed entries
	}
	var completedEntries []int
	for _, entry := range q.taskQueue {
		if entry.Status == "waiting" || entry.Status == "processing" {
			break
		} else {
			idNum, err := strconv.Atoi(entry.ID)
			if err != nil {
				panic("error converting ID to integer")
			}
			completedEntries = append(completedEntries, idNum)
		}
	}
	if len(completedEntries) < 10 {
		return // Not enough accumulated yet
	}
	entriesToCleanup := completedEntries[:len(completedEntries)/2]
	for _, entryID := range entriesToCleanup {
		// Remove from taskLookup
		delete(q.taskLookup, entryID)
	}
	// Truncate entries from front of queue
	q.taskQueue = q.taskQueue[len(entriesToCleanup):]
}

// Returns the task info (status and result) by its ID
func (q *Queue) GetTaskInfo(taskID int) (*types.Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	task, exists := q.taskLookup[taskID]

	if !exists {
		return nil, fmt.Errorf("task not found")
	}
	localTask := *task

	return &localTask, nil
}

func (q *Queue) GetQueueLength() (int, error) {
    q.mu.Lock()
    defer q.mu.Unlock()

    return len(q.taskQueue), nil
}

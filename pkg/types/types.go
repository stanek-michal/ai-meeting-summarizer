package types

// Result contains a full transcript and a text summary of it
type Result struct {
        Transcript  string
        Summary     string
        ErrorMsg    string
}

// Task represents a processing task
type Task struct {
        ID       string
        FileName string
        Status   string
        Result   Result
}

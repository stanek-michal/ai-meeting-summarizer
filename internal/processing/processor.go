package processing

import (
	"log"
//	"os/exec"
	"time"
        "github.com/stanek-michal/go-ai-summarizer/pkg/types"
)

type Processor struct{}

func NewProcessor() *Processor {
	return &Processor{}
}

// Do processing on input file
func (p *Processor) Process(fileName string) (types.Result, error) {
	// Replace this with actual processing logic
	// For now, it just simulates processing time
	time.Sleep(15 * time.Second)

	// Example of running a system command:
	// cmd := exec.Command("system_command", "arg1", "arg2")
	// output, err := cmd.CombinedOutput()

	// Simulate system command output and error
	output := []byte("Simulated processing result")
	err := error(nil) // Should be replaced by the actual error from the command, if any

	if err != nil {
		log.Printf("Error processing file: %v", err)
		return types.Result{}, err
	}

	// TODO fill in Transcript, Summary, ErrorMsg
	result := types.Result{Summary: string(output) + ": summary", Transcript: string(output) + ": transcript", ErrorMsg: "errormsg"}
	return result, nil
}

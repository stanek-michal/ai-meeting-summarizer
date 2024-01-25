package processing

import (
	"log"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"errors"
	"time"
	"bytes"
	"net/http"
        "github.com/stanek-michal/go-ai-summarizer/pkg/types"
)

type Processor struct{}

func NewProcessor() *Processor {
	return &Processor{}
}

const hf_token_path = "/home/ubuntu/hf_token.txt"

// convertToWav takes an absolute path to an MP4 file and converts it to a WAV file in the same path.
func convertToWav(mp4FilePath string) (string, error) {
	// Get basename
	filename := filepath.Base(mp4FilePath)
	// Determine the output WAV file path by changing the extension
	wavFilePath := changeFileExtension(mp4FilePath, ".wav")

	// Prepare the ffmpeg command
	cmd := exec.Command("ffmpeg", "-i", mp4FilePath, "-vn", "-acodec", "pcm_s16le", "-ar", "32000", "-ac", "2", wavFilePath)

	// Create a log file to capture stdout and stderr
	logFile, err := os.Create(truncateFileExtension(filename) + "_ffmpeg_output.log")
	if err != nil {
		return "", err
	}
	defer logFile.Close()

	// Redirect stdout and stderr to the log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Execute the ffmpeg command
	err = cmd.Run()

	// Grab the result code of the command
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		}
		if exitCode != 0 {
			log.Printf("Error executing ffmpeg command: %v, errorcode: %v", err, exitCode)
		} else {
			log.Printf("Error executing ffmpeg command: %v", err)
		}
		return "", err
	}

	log.Printf("ffmpeg conversion finished successfully, output file: %s", wavFilePath)
	return wavFilePath, nil
}

func truncateFileExtension(filePath string) string {
	return strings.TrimSuffix(filePath, filepath.Ext(filePath))
}
func changeFileExtension(filePath string, newExt string) string {
	return truncateFileExtension(filePath) + newExt
}

// Generate diarized transcript from .wav with whisperx tool - may take many minutes
func generateTranscript(filePath string) (string, string, error) {
	// Get basename and extension
	inputFilename := filepath.Base(filePath)

	// Read the hf_token from a local file
	hfTokenBytes, err := ioutil.ReadFile(hf_token_path)
	if err != nil {
		log.Printf("Error reading HF token file: %v", err)
		return "", "", err
	}
	hfToken := strings.TrimSpace(string(hfTokenBytes))

	// Prepare the command
	cmd := exec.Command("whisperx", filePath, "--model", "large-v3", "--diarize", "--hf_token", hfToken)

	// Create output.log file to tee the output
	outputFile, err := os.Create(truncateFileExtension(inputFilename) + "_whisperx_output.log")
	if err != nil {
		log.Printf("Error creating output.log file: %v", err)
		return "", "", err
	}
	defer outputFile.Close()

	// Redirect stdout and stderr to the log file
	cmd.Stdout = outputFile
	cmd.Stderr = outputFile

	// Execute the whisperx command
	err = cmd.Run()

	// Grab the result code of the command
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		}
		if exitCode != 0 {
			log.Printf("Error executing whisperx command: %v, errorcode: %v", err, exitCode)
		} else {
			log.Printf("Error executing whisperx command: %v", err)
		}
		return "", "", err
	}

	vttFilepath := changeFileExtension(inputFilename, ".vtt")
	vttBytes, err := ioutil.ReadFile(vttFilepath)
	if err != nil {
		log.Printf("Error: %v not generated", vttFilepath)
		return "", "", err
	} else {
		return string(vttBytes), vttFilepath, nil
	}
}

func killKoboldcpp(koboldProcess *os.Process) error {
	// Kill -9 koboldcpp to reclaim VRAM
	if err := koboldProcess.Kill(); err != nil {
		log.Fatalf("Failed to kill process: %v", err)
		return errors.New("Could not kill koboldcpp")
	}

	// Wait for the process to finish to avoid zombies
	if _, err := koboldProcess.Wait(); err != nil {
		log.Printf("Process exited with error: %v", err)
	}
	return nil
}

// Run koboldcpp in the background, wait until it loads model then run summarization script
func generateSummary(transcript string, transcriptFilepath string) (string, error) {
	// Open the os.DevNull device
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()

	// Run koboldcpp as daemon and redirect output to /dev/null
	koboldcmd := exec.Command("koboldcpp",
		"--usecublas",
		"--gpulayers", "18",
		"--threads", "7",
		"--contextsize", "32768",
		"--noshift",
		"--quiet",
		"--skiplauncher",
		"--multiuser", "5",
		"--model", "/home/ubuntu/koboldcpp/models/mixtral-8x7b-instruct-v0.1.Q4_K_M.gguf")
	koboldcmd.Stdout = devNull // Redirect stdout to /dev/null
	koboldcmd.Stderr = devNull // Redirect stderr to /dev/null

	if err := koboldcmd.Start(); err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}

	// Periodically call koboldcpp HTTP API to check if server is initialized
	apiURL := "http://localhost:5001/api/v1/model"
	counter := 0
	for {
		resp, err := http.Get(apiURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			// Successfully received 200 OK
			log.Println("Koboldcpp daemon initialized.")
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		counter++
		if counter > 1200 {
			errorStr := "Error: koboldcpp did not initialize in 20minutes, exiting.."
			log.Println(errorStr)
			// Kill koboldcpp
			killKoboldcpp(koboldcmd.Process)
			return "", errors.New("errorStr")
		}

		// Wait before trying again
		time.Sleep(1 * time.Second)
	}

	// Run the Python summarizer script which will:
	// - preprocess, condense and chunk .vtt transcript
	// - call koboldcpp API to generate a summary with the LLM
	// - return the summary on stdout
	// Generation may take many minutes
	pythonCmd := exec.Command("python", "python/generate_ai_summary.py", transcriptFilepath)

	var summaryBuf bytes.Buffer
	pythonCmd.Stdout = &summaryBuf

	// Run summarization script and wait for it to finish
	if err := pythonCmd.Run(); err != nil {
		log.Printf("Error running python summarizer: %v\n", err)
		// Kill koboldcpp
		killKoboldcpp(koboldcmd.Process)
		return "", err
	}

	// Kill -9 koboldcpp - cannot keep it running as it will take VRAM away from whisperx
	killKoboldcpp(koboldcmd.Process)

	// Return summary
	return summaryBuf.String(), nil
}

// Do processing on input file (usually /tmp/upload-<randomhexstring>.wav or .mp4 or .vtt)
func (p *Processor) Process(filePath string) (types.Result, error) {
	// Get basename and extension
	baseFilename := filepath.Base(filePath)
	extension := filepath.Ext(baseFilename)

	// Make sure input file exists
	if _, err := os.Stat(filePath); err != nil {
		log.Printf("Input file does not exist: %v", err)
		return types.Result{}, err
	}

	if extension == ".mp4" {
		convertedFilePath, err := convertToWav(filePath)
		if err != nil {
			log.Printf("Conversion to .wav failed, error: %v", err)
			return types.Result{ErrorMsg: err.Error()}, err
		}
		filePath = convertedFilePath
		extension = ".wav"
	}
	if extension == ".wav" {
		transcript, transcriptFilepath, err := generateTranscript(filePath)
		if err != nil {
			log.Printf("Transcript generation failed, error: %v", err)
			return types.Result{ErrorMsg: err.Error()}, err
		}
		summary, err := generateSummary(transcript, transcriptFilepath)
		if err != nil {
			log.Printf("Summary generation failed, error: %v", err)
			return types.Result{Transcript: transcript, ErrorMsg: err.Error()}, err
		}
		// Populate the result object
		result := types.Result{
			Transcript: transcript,
			Summary:    summary,
			ErrorMsg:   "",
		}
		return result, nil
	} else {
		log.Printf("Unknown extension: %v", extension);
		return types.Result{}, nil
	}
	return types.Result{}, nil
}

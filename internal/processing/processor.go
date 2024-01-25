package processing

import (
	"log"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
func generateTranscript(filePath string) (string, error) {
	// Get basename and extension
	inputFilename := filepath.Base(filePath)

	// Read the hf_token from a local file
	hfTokenBytes, err := ioutil.ReadFile(hf_token_path)
	if err != nil {
		log.Printf("Error reading HF token file: %v", err)
		return "", err
	}
	hfToken := strings.TrimSpace(string(hfTokenBytes))
	log.Printf("Token: %q", hfToken)

	// Prepare the command
	cmd := exec.Command("whisperx", filePath, "--model", "large-v3", "--diarize", "--hf_token", hfToken)

	// Create output.log file to tee the output
	outputFile, err := os.Create(truncateFileExtension(inputFilename) + "_whisperx_output.log")
	if err != nil {
		log.Printf("Error creating output.log file: %v", err)
		return "", err
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
		return "", err
	}

	vttFilepath := changeFileExtension(inputFilename, ".vtt")
	vttBytes, err := ioutil.ReadFile(vttFilepath)
	if err != nil {
		log.Printf("Error: %v not generated", vttFilepath)
		return "", err
	} else {
		return string(vttBytes), nil
	}
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
		transcript, err := generateTranscript(filePath)
		if err != nil {
			log.Printf("Transcript generation failed, error: %v", err)
			return types.Result{ErrorMsg: err.Error()}, err
		}
		// Populate the result object
		result := types.Result{
			Transcript: transcript,
			Summary:    "",
			ErrorMsg:   "",
		}
		return result, nil
	} else {
		log.Printf("Unknown extension: %v", extension);
		return types.Result{}, nil
	}
	return types.Result{}, nil
}

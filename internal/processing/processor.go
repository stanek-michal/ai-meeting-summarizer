package processing

import (
    "bytes"
    "errors"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "syscall"
    "time"

    "github.com/stanek-michal/go-ai-summarizer/pkg/types"
)

type Processor struct{}

func NewProcessor() *Processor {
    return &Processor{}
}

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

    // Prepare the command
    cmd := exec.Command("whisperx", filePath, "--model", "large-v3", "--compute_type", "int8")

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

func killLocalLlamaProcess(p *os.Process) error {
    // Get the process group ID (PGID)
    pgid, err := syscall.Getpgid(p.Pid)
    if err != nil {
        log.Printf("syscall.Getpgid() failed with %s\n", err)
        return errors.New("could not get pgid of llama-cpp-python process")
    }
    log.Printf("Attempting to kill local llama process with PID: %d and PGID: %d", p.Pid, pgid)

    // Kill the process group
    if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
        log.Printf("syscall.Kill() failed with %s\n", err)
        return errors.New("could not kill llama-cpp-python server")
    }
    log.Println("Kill signal sent successfully")

    // Wait for the process to finish
    if _, err := p.Wait(); err != nil {
        log.Printf("Process exited with error: %v", err)
    } else {
        log.Println("Process wait completed without error")
    }

    return nil
}

// Start the llama-cpp-python server, wait for it to load model, then run summarization script
func generateSummary(transcript string, transcriptFilepath string) (string, error) {
    // Start llama-cpp-python in server mode with an OpenAI-compatible API
    llamaCmd := exec.Command("python",
        "-m", "llama_cpp.server",
        "--model", "./Qwen2.5-32B-Instruct-Q4_K_M.gguf",
        "--host", "127.0.0.1",
        "--port", "8000",
        "--n_ctx", "32768",
        // Add any additional flags you need, e.g. '--num_threads', etc.
    )

    // Set the process to run in its own new process group
    llamaCmd.SysProcAttr = &syscall.SysProcAttr{
        Setpgid: true,
    }

    // For logging
    devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
    if err != nil {
        log.Printf("Failed to open %s: %v", os.DevNull, err)
        return "", err
    }
    defer devNull.Close()

    llamaCmd.Stdout = devNull // Redirect stdout to /dev/null
    llamaCmd.Stderr = devNull // Redirect stderr to /dev/null

    if err := llamaCmd.Start(); err != nil {
        log.Printf("Failed to start llama-cpp-python server: %v", err)
        return "", err
    }
    log.Printf("Started llama-cpp-python with PID: %d", llamaCmd.Process.Pid)

    // Periodically call the health-check to see if server is up
    // The default OpenAI-compatible endpoint for llama-cpp-python is:
    //  http://127.0.0.1:8000/v1/models
    apiURL := "http://127.0.0.1:8000/v1/models"
    counter := 0
    for {
        resp, err := http.Get(apiURL)
        if err == nil && resp.StatusCode == 200 {
            log.Println("llama-cpp-python server initialized.")
            resp.Body.Close()
            break
        }
        if resp != nil {
            resp.Body.Close()
        }

        counter++
        if counter > 1200 {
            errorStr := "Error: llama-cpp-python did not initialize in 20 minutes, exiting.."
            log.Println(errorStr)
            killLocalLlamaProcess(llamaCmd.Process)
            return "", errors.New(errorStr)
        }

        time.Sleep(1 * time.Second)
    }

    // Open or create the log file for appending
    pythonLogFile, err := os.OpenFile("python_summarizer_log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        log.Println("Failed to open error log file:", err)
        killLocalLlamaProcess(llamaCmd.Process)
        return "", err
    }
    defer pythonLogFile.Close()

    // Run the Python summarizer script which will:
    // - preprocess, condense, chunk the .vtt transcript
    // - call the local llama-cpp (OpenAI-compatible) API to generate a summary
    pythonCmd := exec.Command("python", "python/generate_ai_summary.py", transcriptFilepath)
    var summaryBuf bytes.Buffer
    pythonCmd.Stdout = &summaryBuf
    pythonCmd.Stderr = pythonLogFile

    // Run summarization script
    if err := pythonCmd.Run(); err != nil {
        log.Printf("Error running python summarizer: %v\n", err)
        killLocalLlamaProcess(llamaCmd.Process)
        return "", err
    }

    // Kill the llama-cpp-python process
    killLocalLlamaProcess(llamaCmd.Process)

    // Return summary
    return summaryBuf.String(), nil
}

// Remove all files starting with filePath after truncating extension
func RemoveFilesByPrefixAllExtensions(filePath string) error {
    dir := filepath.Dir(filePath)
    baseFilename := filepath.Base(filePath)
    prefix := truncateFileExtension(baseFilename)

    return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        // Check if the current file's name starts with our prefix
        if strings.HasPrefix(filepath.Base(path), prefix) {
            err := os.Remove(path)
            if err != nil {
                return err
            }
            log.Printf("Removed: %s\n", path)
        }
        return nil
    })
}

func CleanUpUserFiles(filePath string, transcriptFilepath string) error {
    // Remove all files starting with filePath and transcriptFilepath without extension
    // (.mp4/.wav/.vtt, all of it)
    if err := RemoveFilesByPrefixAllExtensions(filePath); err != nil {
        return err
    }
    if transcriptFilepath != "" {
        if err := RemoveFilesByPrefixAllExtensions(transcriptFilepath); err != nil {
            return err
        }
    }
    return nil
}

// Do processing on input file (usually /tmp/upload-<randomhexstring>.wav or .mp4 or .vtt)
func (p *Processor) Process(filePath string) (types.Result, error) {
    log.Printf("Running Process() for: %v", filePath)
    // Get basename and extension
    baseFilename := filepath.Base(filePath)
    extension := filepath.Ext(baseFilename)

    // Make sure input file exists
    if _, err := os.Stat(filePath); err != nil {
        log.Printf("Input file does not exist: %v", err)
        return types.Result{}, err
    }

    if extension == ".mp4" {
        log.Printf("Converting %v to .wav", filePath)
        convertedFilePath, err := convertToWav(filePath)
        if err != nil {
            log.Printf("Conversion to .wav failed, error: %v", err)
            CleanUpUserFiles(filePath, "")
            return types.Result{ErrorMsg: err.Error()}, err
        }
        filePath = convertedFilePath
        extension = ".wav"
    }

    if extension == ".wav" {
        log.Printf("Generating transcript for %v", filePath)
        transcript, transcriptFilepath, err := generateTranscript(filePath)
        if err != nil {
            log.Printf("Transcript generation failed, error: %v", err)
            CleanUpUserFiles(filePath, "")
            return types.Result{ErrorMsg: err.Error()}, err
        }
        log.Printf("Generating summary for %v", transcriptFilepath)
        summary, err := generateSummary(transcript, transcriptFilepath)
        if err != nil {
            log.Printf("Summary generation failed, error: %v", err)
            CleanUpUserFiles(filePath, transcriptFilepath)
            return types.Result{Transcript: transcript, ErrorMsg: err.Error()}, err
        }
        // Populate the result object
        result := types.Result{
            Transcript: transcript,
            Summary:    summary,
            ErrorMsg:   "",
        }
        log.Printf("Generated summary for %v", transcriptFilepath)
        CleanUpUserFiles(filePath, transcriptFilepath)
        return result, nil
    } else {
        log.Printf("Unknown extension: %v", extension)
        CleanUpUserFiles(filePath, "")
        return types.Result{}, nil
    }
}


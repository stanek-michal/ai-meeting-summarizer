# AI Meeting Summarizer

An AI-powered app for summarizing meeting videos working fully locally.

## Prerequisites

- macOS (Apple Silicon, at least 32GB)

## Installation

1. Clone this repository:
   ```bash
   git clone <your-repository-url>
   cd <repository-directory>
   ```

2. Make the installation script executable:
   ```bash
   chmod +x install.sh
   ```

3. Run the installation script:
   ```bash
   ./install.sh
   ```

   This script will:
   - Install Homebrew (if not already installed)
   - Install Python 3.10, Go, and ffmpeg
   - Create a Python virtual environment
   - Install all required Python packages
   - Build the Go application

## Running the Application

1. Make the run script executable (only needed once):
   ```bash
   chmod +x run.sh
   ```

2. Start the application:
   ```bash
   ./run.sh
   ```

## Troubleshooting

If you encounter any issues:

1. Make sure all prerequisites are installed:
   ```bash
   brew install python@3.10 go ffmpeg
   ```

2. Try rebuilding the application:
   ```bash
   source venv/bin/activate
   go build -o summarizer_server cmd/server/main.go
   ```

3. Check if the Python virtual environment is activated:
   ```bash
   source venv/bin/activate
   ```

## License

MIT

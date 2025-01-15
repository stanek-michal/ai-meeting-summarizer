#!/bin/bash

echo "Starting installation of AI Meeting Summarizer..."

# Check for Homebrew
if ! command -v brew &> /dev/null; then
    echo "Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

# Install system dependencies
echo "Installing system dependencies..."
brew install python@3.10 go ffmpeg

# Create and activate virtual environment
echo "Setting up Python virtual environment..."
python3 -m venv venv
source venv/bin/activate

# Install Python dependencies
echo "Installing Python dependencies..."
pip install --upgrade pip
pip install -r requirements.txt

# Build Go binary
echo "Building Go application..."
go build -o summarizer_server cmd/server/main.go

echo "Installation complete!"
echo "You can now run the application using: ./run.sh"

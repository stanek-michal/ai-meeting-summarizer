#!/bin/bash

echo "Starting installation of AI Meeting Summarizer..."

# Check for Homebrew
if ! command -v brew &> /dev/null; then
    echo "Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

# Install system dependencies
echo "Installing system dependencies..."
brew install python@3.12 go ffmpeg wget

# Create and activate virtual environment
echo "Setting up Python virtual environment..."
/opt/homebrew/opt/python@3.12/bin/python3.12 -m venv venv
source venv/bin/activate

# Install Python dependencies
echo "Installing Python dependencies..."
pip install --upgrade pip

# Install PyTorch first
echo "Installing PyTorch..."
pip install torch torchvision torchaudio

echo "Installing other Python packages..."
pip install -r requirements.txt

# Create models directory if it doesn't exist
mkdir -p models

# Define model path and URL
MODEL_PATH="models/Qwen2.5-14B-Instruct-Q4_K_M.gguf"
MODEL_URL="https://huggingface.co/bartowski/Qwen2.5-14B-Instruct-GGUF/resolve/main/Qwen2.5-14B-Instruct-Q4_K_M.gguf?download=true"

# Check if model exists
if [ -f "$MODEL_PATH" ]; then
    echo "Model file already exists, skipping download..."
else
    echo "Downloading Qwen model (this may take a while)..."
    wget -O "$MODEL_PATH" "$MODEL_URL"
    
    # Verify download was successful
    if [ $? -ne 0 ]; then
        echo "Error downloading model file!"
        echo "You can try downloading it manually:"
        echo "wget -O $MODEL_PATH $MODEL_URL"
        exit 1
    fi
fi

# Build Go binary
echo "Building Go application..."
go build -o summarizer_server cmd/server/main.go

echo "Installation complete!"
echo "You can now run the application using: ./run.sh"

# Start with the NVIDIA CUDA base image with CUDA 11.8 and cuDNN
FROM nvidia/cuda:12.1.1-cudnn8-devel-ubuntu20.04

# Do not prompt for anything
ARG DEBIAN_FRONTEND=noninteractive

# Create ubuntu user and its home directory
RUN useradd -m ubuntu

# Set the working directory
WORKDIR /home/ubuntu

# Copy HuggingFace secret token
COPY hf_token.txt /home/ubuntu

# Install necessary packages
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    curl \
    build-essential \
    wget \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install Go
ENV GOLANG_VERSION=1.21.6
RUN wget -q https://go.dev/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go${GOLANG_VERSION}.linux-amd64.tar.gz \
    && rm go${GOLANG_VERSION}.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin

# Create the models directory
RUN mkdir -p /home/ubuntu/koboldcpp/models

# Download the model file into the models directory
RUN wget -q -O /home/ubuntu/koboldcpp/models/mixtral-8x7b-instruct-v0.1.Q4_K_M.gguf https://huggingface.co/TheBloke/Mixtral-8x7B-Instruct-v0.1-GGUF/resolve/main/mixtral-8x7b-instruct-v0.1.Q4_K_M.gguf?download=true

# Install koboldcpp
RUN curl -fLo /usr/bin/koboldcpp https://koboldai.org/cpplinux && chmod +x /usr/bin/koboldcpp

# Install Conda
#RUN wget \
#    https://repo.anaconda.com/miniconda/Miniconda3-latest-Linux-x86_64.sh \
#    && bash Miniconda3-latest-Linux-x86_64.sh -b -p /miniconda \
#    && rm -f Miniconda3-latest-Linux-x86_64.sh
#ENV PATH=/miniconda/bin:${PATH}
#RUN conda init

# Install whisperx
#RUN conda create --name whisperx python=3.10
#RUN conda init
#RUN conda activate whisperx
#RUN conda install pytorch==2.0.0 torchaudio==2.0.0 pytorch-cuda=11.8 -c pytorch -c nvidia
#RUN pip install git+https://github.com/m-bain/whisperx.git

# Set up the Python environment and install PyTorch and whisperx
RUN apt-get update && apt-get install -y python3.10 python3-pip
RUN ln -s /usr/bin/python3 /usr/bin/python

#RUN python3.10 -m pip install --upgrade pip \
#RUN pip install torch==2.0.0+cu118 torchaudio==2.0.0 -f https://download.pytorch.org/whl/cu113/torch_stable.html \
RUN pip install git+https://github.com/m-bain/whisperx.git

# Assume that ai_meeting_summarizer.tar.gz is present in the context directory
# and its directory structure is appropriate for building the Go application
#ADD ai_meeting_summarizer.tar.gz /ai_meeting_summarizer
#WORKDIR /ai_meeting_summarizer

# Set the working directory inside the container
WORKDIR /summarizer

# Copy the current directory contents into the container at /app
#COPY . /summarizer

# Build the Go application
#RUN go build -o summarizer_server cmd/server/main.go

# Expose the port the server will run on
EXPOSE 9001

# Set up a volume for logs
VOLUME ["/var/log/summarizer"]

# Run the server and redirect stdout and stderr to a log file
#CMD ./summarizer_server >> /var/log/summarizer/summarizer.log 2>&1

ENTRYPOINT ["/summarizer/entrypoint.sh"]


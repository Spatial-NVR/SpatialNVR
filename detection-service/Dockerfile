# Multi-stage Dockerfile for Detection Service
# Supports CPU, GPU (NVIDIA), OpenVINO, and Coral backends

# ===== Base Stage =====
FROM python:3.11-slim AS base

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1-mesa-glx \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender1 \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY src/ ./src/

# ===== CPU Stage =====
FROM base AS cpu

ENV DETECTION_BACKEND=cpu
EXPOSE 50051
CMD ["python", "src/server.py", "--port", "50051"]


# ===== GPU (NVIDIA) Stage =====
FROM nvidia/cuda:12.3.1-cudnn9-runtime-ubuntu22.04 AS gpu-base

WORKDIR /app

# Install Python and system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3.11 \
    python3-pip \
    python3.11-dev \
    libgl1-mesa-glx \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Create symlinks
RUN ln -sf /usr/bin/python3.11 /usr/bin/python3 && \
    ln -sf /usr/bin/python3 /usr/bin/python

# Install Python dependencies
COPY requirements.txt requirements-gpu.txt ./
RUN pip install --no-cache-dir -r requirements.txt -r requirements-gpu.txt

# Copy source code
COPY src/ ./src/

FROM gpu-base AS gpu

ENV DETECTION_BACKEND=nvidia
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility
EXPOSE 50051
CMD ["python", "src/server.py", "--port", "50051"]


# ===== OpenVINO Stage =====
FROM base AS openvino-base

# Install OpenVINO
COPY requirements-openvino.txt .
RUN pip install --no-cache-dir -r requirements-openvino.txt

FROM openvino-base AS openvino

ENV DETECTION_BACKEND=openvino
EXPOSE 50051
CMD ["python", "src/server.py", "--port", "50051"]


# ===== Coral Edge TPU Stage =====
FROM base AS coral-base

# Install Coral dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gnupg \
    curl \
    && echo "deb https://packages.cloud.google.com/apt coral-edgetpu-stable main" | \
       tee /etc/apt/sources.list.d/coral-edgetpu.list \
    && curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add - \
    && apt-get update && apt-get install -y --no-install-recommends \
    libedgetpu1-std \
    && rm -rf /var/lib/apt/lists/*

COPY requirements-coral.txt .
RUN pip install --no-cache-dir -r requirements-coral.txt

FROM coral-base AS coral

ENV DETECTION_BACKEND=coral
EXPOSE 50051
CMD ["python", "src/server.py", "--port", "50051"]


# ===== Default (CPU) =====
FROM cpu AS default

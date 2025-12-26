#!/bin/bash
# Run the Detection Service

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Create venv if it doesn't exist
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
    ./venv/bin/pip install --upgrade pip
    ./venv/bin/pip install -r requirements.txt
fi

# Default values
PORT=${DETECTION_PORT:-50051}
MODEL=${DETECTION_MODEL:-}
MODEL_TYPE=${DETECTION_MODEL_TYPE:-yolov8}

echo "Starting Detection Service on port $PORT..."
exec ./venv/bin/python src/main.py --port "$PORT" ${MODEL:+--model "$MODEL"} --model-type "$MODEL_TYPE"

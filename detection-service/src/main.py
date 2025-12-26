#!/usr/bin/env python3
"""
Detection Service Entry Point

Runs the detection service HTTP server for YOLO object detection.
Integrates with the NVR Go backend via HTTP API.

Usage:
    python main.py [--port PORT] [--model MODEL_PATH] [--model-type TYPE]

Example:
    python main.py --port 50051 --model /data/models/yolo11n.onnx --model-type yolo11
"""

import argparse
import logging
import os
import sys

# Add src directory to path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from http_server import serve

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def main():
    parser = argparse.ArgumentParser(
        description='NVR Detection Service - YOLO Object Detection',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Supported model types:
  yolo12    - YOLO v12 (fastest)
  yolo11    - YOLO v11
  yolov8    - YOLOv8
  yolov9    - YOLOv9
  mobilenet - MobileNet SSD

Environment variables:
  DETECTION_PORT      - HTTP port (default: 50051)
  DETECTION_MODEL     - Path to model file
  DETECTION_MODEL_TYPE - Model type (default: yolov8)
  CUDA_VISIBLE_DEVICES - GPU device(s) to use
        """
    )

    parser.add_argument(
        '--port', '-p',
        type=int,
        default=int(os.environ.get('DETECTION_PORT', 50051)),
        help='HTTP port to listen on (default: 50051)'
    )

    parser.add_argument(
        '--model', '-m',
        type=str,
        default=os.environ.get('DETECTION_MODEL'),
        help='Path to model file to auto-load'
    )

    parser.add_argument(
        '--model-type', '-t',
        type=str,
        default=os.environ.get('DETECTION_MODEL_TYPE', 'yolov8'),
        choices=['yolo12', 'yolo11', 'yolov8', 'yolov9', 'mobilenet'],
        help='Type of model (default: yolov8)'
    )

    parser.add_argument(
        '--models-dir',
        type=str,
        default=os.environ.get('MODELS_DIR', '/data/models'),
        help='Directory containing model files'
    )

    args = parser.parse_args()

    # Log startup info
    logger.info("=" * 60)
    logger.info("NVR Detection Service")
    logger.info("=" * 60)
    logger.info(f"Port: {args.port}")

    if args.model:
        logger.info(f"Model: {args.model}")
        logger.info(f"Model Type: {args.model_type}")

    # Check for GPU
    try:
        import onnxruntime as ort
        providers = ort.get_available_providers()
        if 'CUDAExecutionProvider' in providers:
            logger.info("GPU: CUDA available")
        elif 'CoreMLExecutionProvider' in providers:
            logger.info("GPU: CoreML available (Apple Silicon)")
        else:
            logger.info("GPU: None (using CPU)")
    except ImportError:
        logger.warning("ONNX Runtime not installed")

    logger.info("=" * 60)

    # Start server
    try:
        serve(port=args.port, model_path=args.model)
    except KeyboardInterrupt:
        logger.info("Shutting down...")
    except Exception as e:
        logger.exception(f"Server error: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()

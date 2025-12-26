"""
AI Detection Service - YOLOv12 Object Detection
FastAPI service for real-time object detection in video frames.
"""

import io
import logging
import os
from contextlib import asynccontextmanager
from typing import List, Optional

import numpy as np
from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from PIL import Image
from pydantic import BaseModel

# Configure logging
logging.basicConfig(
    level=getattr(logging, os.getenv("LOG_LEVEL", "INFO").upper()),
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)

# Global model variable
model = None


class Detection(BaseModel):
    """Single detection result."""

    class_name: str
    confidence: float
    bbox: List[float]  # [x1, y1, x2, y2]
    class_id: int


class DetectionResponse(BaseModel):
    """Response containing all detections."""

    detections: List[Detection]
    inference_time_ms: float
    image_size: List[int]  # [width, height]


class HealthResponse(BaseModel):
    """Health check response."""

    status: str
    model_loaded: bool
    model_name: Optional[str]
    device: str


def load_model():
    """Load the YOLO model."""
    global model

    model_name = os.getenv("MODEL_NAME", "yolov12n")
    device = os.getenv("DEVICE", "cpu")

    logger.info(f"Loading model: {model_name} on device: {device}")

    try:
        # Try to import and load YOLOv12
        # First try the official sunsmarterjie repo, then fall back to ultralytics
        try:
            from ultralytics import YOLO

            model_path = f"{model_name}.pt"
            model = YOLO(model_path)
            model.to(device)
            logger.info(f"Model {model_name} loaded successfully on {device}")
        except Exception as e:
            logger.warning(f"Failed to load model: {e}")
            logger.info("Running in mock mode - no actual detection")
            model = None

    except Exception as e:
        logger.error(f"Failed to load model: {e}")
        model = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan handler."""
    # Startup
    load_model()
    yield
    # Shutdown
    logger.info("Shutting down AI Detection service")


app = FastAPI(
    title="NVR AI Detection Service",
    description="YOLOv12 object detection for NVR system",
    version="0.1.0",
    lifespan=lifespan,
)

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint."""
    return HealthResponse(
        status="healthy",
        model_loaded=model is not None,
        model_name=os.getenv("MODEL_NAME", "yolov12n") if model else None,
        device=os.getenv("DEVICE", "cpu"),
    )


@app.post("/detect", response_model=DetectionResponse)
async def detect_objects(
    image: UploadFile = File(...),
    confidence: float = 0.5,
    iou: float = 0.45,
    classes: Optional[str] = None,  # Comma-separated class IDs
):
    """
    Detect objects in an uploaded image.

    Args:
        image: Image file (JPEG, PNG, etc.)
        confidence: Minimum confidence threshold (0.0-1.0)
        iou: IoU threshold for NMS (0.0-1.0)
        classes: Optional comma-separated list of class IDs to detect

    Returns:
        DetectionResponse with list of detections
    """
    import time

    start_time = time.time()

    # Validate confidence
    if not 0.0 <= confidence <= 1.0:
        raise HTTPException(status_code=400, detail="Confidence must be between 0.0 and 1.0")

    # Read and decode image
    try:
        img_bytes = await image.read()
        img = Image.open(io.BytesIO(img_bytes))
        img_array = np.array(img)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Invalid image: {str(e)}")

    # Get image dimensions
    width, height = img.size

    # Parse classes filter if provided
    class_filter = None
    if classes:
        try:
            class_filter = [int(c.strip()) for c in classes.split(",")]
        except ValueError:
            raise HTTPException(status_code=400, detail="Invalid classes format")

    detections = []

    if model is not None:
        try:
            # Run inference
            results = model(
                img_array,
                conf=confidence,
                iou=iou,
                classes=class_filter,
                verbose=False,
            )

            # Process results
            for r in results:
                boxes = r.boxes
                for box in boxes:
                    class_id = int(box.cls[0])
                    detections.append(
                        Detection(
                            class_name=model.names[class_id],
                            confidence=float(box.conf[0]),
                            bbox=box.xyxy[0].tolist(),
                            class_id=class_id,
                        )
                    )
        except Exception as e:
            logger.error(f"Detection error: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")
    else:
        # Mock mode - return empty detections
        logger.debug("Running in mock mode - no model loaded")

    inference_time = (time.time() - start_time) * 1000  # Convert to ms

    return DetectionResponse(
        detections=detections,
        inference_time_ms=round(inference_time, 2),
        image_size=[width, height],
    )


@app.get("/models")
async def list_models():
    """List available models."""
    return {
        "available_models": [
            {"name": "yolov12n", "description": "YOLOv12 Nano - fastest, lower accuracy"},
            {"name": "yolov12s", "description": "YOLOv12 Small - balanced"},
            {"name": "yolov12m", "description": "YOLOv12 Medium - better accuracy"},
            {"name": "yolov12l", "description": "YOLOv12 Large - high accuracy"},
            {"name": "yolov12x", "description": "YOLOv12 XLarge - best accuracy"},
            {"name": "yolo11n", "description": "YOLO11 Nano - fastest alternative"},
        ],
        "current_model": os.getenv("MODEL_NAME", "yolov12n"),
    }


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=int(os.getenv("PORT", "8001")),
        reload=os.getenv("MODE", "production") == "development",
        log_level=os.getenv("LOG_LEVEL", "info").lower(),
    )

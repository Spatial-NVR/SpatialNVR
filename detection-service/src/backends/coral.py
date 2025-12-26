"""Coral Edge TPU detector."""

import logging
from typing import Dict, List, Optional, Tuple
import numpy as np
import uuid

try:
    from pycoral.adapters import common, detect
    from pycoral.utils.edgetpu import make_interpreter
    from pycoral.utils.dataset import read_label_file
    CORAL_AVAILABLE = True
except ImportError:
    CORAL_AVAILABLE = False

from .base import (
    Detector, DetectionResult, BoundingBox, BackendType, ModelType,
    ModelInfo, BackendInfo, COCO_CLASSES
)

logger = logging.getLogger(__name__)


class CoralDetector(Detector):
    """Coral Edge TPU detector."""

    def __init__(self, device: str = ""):
        """
        Initialize Coral detector.

        Args:
            device: Edge TPU device path (empty for default)
        """
        self.device = device
        self.interpreters: Dict[str, any] = {}
        self.model_info: Dict[str, ModelInfo] = {}

    def name(self) -> str:
        return "Coral Edge TPU"

    def backend_type(self) -> BackendType:
        return BackendType.CORAL

    def is_available(self) -> bool:
        if not CORAL_AVAILABLE:
            return False
        try:
            # Try to list Edge TPU devices
            from pycoral.utils.edgetpu import list_edge_tpus
            devices = list_edge_tpus()
            return len(devices) > 0
        except Exception:
            return False

    def get_info(self) -> BackendInfo:
        if not CORAL_AVAILABLE:
            return BackendInfo(
                type=BackendType.CORAL,
                available=False
            )

        try:
            from pycoral.utils.edgetpu import list_edge_tpus
            devices = list_edge_tpus()
            device_name = devices[0]['type'] if devices else "Not found"
        except Exception:
            device_name = "Unknown"

        return BackendInfo(
            type=BackendType.CORAL,
            available=self.is_available(),
            version="2.0",
            device=device_name
        )

    def load_model(self, path: str, model_type: ModelType,
                   model_id: Optional[str] = None) -> str:
        """Load a TFLite model for Edge TPU."""
        if not CORAL_AVAILABLE:
            raise RuntimeError("Coral libraries not available")

        model_id = model_id or str(uuid.uuid4())[:8]

        try:
            # Create interpreter
            if self.device:
                interpreter = make_interpreter(path, device=self.device)
            else:
                interpreter = make_interpreter(path)

            interpreter.allocate_tensors()

            # Get input details
            input_details = interpreter.get_input_details()[0]
            input_shape = input_details['shape']
            input_size = (input_shape[2], input_shape[1])  # NHWC format

            self.interpreters[model_id] = interpreter
            self.model_info[model_id] = ModelInfo(
                id=model_id,
                name=path.split('/')[-1],
                type=model_type,
                backend=BackendType.CORAL,
                path=path,
                input_size=input_size,
                input_format="nhwc",
                pixel_format="rgb",
                classes=COCO_CLASSES,
                loaded=True
            )

            logger.info(f"Loaded Coral model: {model_id} from {path}")
            return model_id

        except Exception as e:
            logger.error(f"Failed to load Coral model: {e}")
            raise

    def unload_model(self, model_id: str) -> bool:
        """Unload a model."""
        if model_id in self.interpreters:
            del self.interpreters[model_id]
            del self.model_info[model_id]
            logger.info(f"Unloaded model: {model_id}")
            return True
        return False

    def detect(self, image: np.ndarray, model_id: str,
               min_confidence: float = 0.5,
               classes: Optional[List[str]] = None) -> List[DetectionResult]:
        """Perform detection using Edge TPU."""
        if model_id not in self.interpreters:
            raise ValueError(f"Model not loaded: {model_id}")

        interpreter = self.interpreters[model_id]
        info = self.model_info[model_id]

        # Preprocess - Edge TPU needs uint8
        import cv2
        resized = cv2.resize(image, info.input_size)
        input_data = np.expand_dims(resized, axis=0).astype(np.uint8)

        # Set input tensor
        common.set_input(interpreter, input_data)

        # Run inference
        interpreter.invoke()

        # Get detections using pycoral detection adapter
        detections = detect.get_objects(interpreter, min_confidence)

        # Convert to our format
        results = []
        img_height, img_width = image.shape[:2]

        for det in detections:
            bbox = det.bbox
            class_id = det.id

            label = info.classes[class_id] if class_id < len(info.classes) else f"class_{class_id}"
            object_type = self._classify_object_type(label)

            results.append(DetectionResult(
                object_type=object_type,
                label=label,
                confidence=det.score,
                bbox=BoundingBox.from_pixels(
                    bbox.xmin, bbox.ymin, bbox.xmax, bbox.ymax,
                    img_width, img_height
                )
            ))

        # Filter by classes if specified
        if classes:
            results = [r for r in results if r.label in classes]

        return results

    def get_models(self) -> List[ModelInfo]:
        """Get loaded models."""
        return list(self.model_info.values())

    def close(self) -> None:
        """Close all interpreters."""
        for model_id in list(self.interpreters.keys()):
            self.unload_model(model_id)

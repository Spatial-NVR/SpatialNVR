"""CoreML detector for Apple Silicon (macOS)."""

import logging
from typing import Dict, List, Optional, Tuple
import numpy as np
import uuid
import platform

try:
    import coremltools as ct
    COREML_AVAILABLE = platform.system() == "Darwin"
except ImportError:
    COREML_AVAILABLE = False

from .base import (
    Detector, DetectionResult, BoundingBox, BackendType, ModelType,
    ModelInfo, BackendInfo, COCO_CLASSES
)

logger = logging.getLogger(__name__)


class CoreMLDetector(Detector):
    """CoreML detector for Apple Silicon acceleration."""

    def __init__(self, compute_units: str = "ALL"):
        """
        Initialize CoreML detector.

        Args:
            compute_units: "ALL", "CPU_AND_NE", "CPU_ONLY"
                           (NE = Neural Engine / ANE)
        """
        self.compute_units = compute_units
        self.models: Dict[str, any] = {}
        self.model_info: Dict[str, ModelInfo] = {}

    def name(self) -> str:
        return "Apple CoreML"

    def backend_type(self) -> BackendType:
        return BackendType.COREML

    def is_available(self) -> bool:
        return COREML_AVAILABLE

    def get_info(self) -> BackendInfo:
        if not COREML_AVAILABLE:
            return BackendInfo(
                type=BackendType.COREML,
                available=False
            )

        # Detect Apple Silicon
        import subprocess
        try:
            result = subprocess.run(
                ['sysctl', '-n', 'machdep.cpu.brand_string'],
                capture_output=True, text=True
            )
            device = result.stdout.strip()
        except Exception:
            device = "Apple Silicon"

        return BackendInfo(
            type=BackendType.COREML,
            available=True,
            version=ct.__version__ if hasattr(ct, '__version__') else "unknown",
            device=device,
            compute=self.compute_units
        )

    def load_model(self, path: str, model_type: ModelType,
                   model_id: Optional[str] = None) -> str:
        """Load a CoreML model (.mlpackage or .mlmodel)."""
        if not COREML_AVAILABLE:
            raise RuntimeError("CoreML not available (requires macOS)")

        model_id = model_id or str(uuid.uuid4())[:8]

        try:
            # Determine compute units
            if self.compute_units == "ALL":
                compute = ct.ComputeUnit.ALL
            elif self.compute_units == "CPU_AND_NE":
                compute = ct.ComputeUnit.CPU_AND_NE
            else:
                compute = ct.ComputeUnit.CPU_ONLY

            # Load model
            model = ct.models.MLModel(path, compute_units=compute)

            # Get input details
            spec = model.get_spec()
            input_desc = spec.description.input[0]

            # Parse input shape
            if hasattr(input_desc.type, 'imageType'):
                input_size = (
                    input_desc.type.imageType.width,
                    input_desc.type.imageType.height
                )
            else:
                # Default for YOLO models
                input_size = (640, 640)

            self.models[model_id] = model
            self.model_info[model_id] = ModelInfo(
                id=model_id,
                name=path.split('/')[-1],
                type=model_type,
                backend=BackendType.COREML,
                path=path,
                input_size=input_size,
                input_format="nhwc",
                pixel_format="rgb",
                classes=COCO_CLASSES,
                loaded=True
            )

            logger.info(f"Loaded CoreML model: {model_id} from {path}")
            return model_id

        except Exception as e:
            logger.error(f"Failed to load CoreML model: {e}")
            raise

    def unload_model(self, model_id: str) -> bool:
        """Unload a model."""
        if model_id in self.models:
            del self.models[model_id]
            del self.model_info[model_id]
            logger.info(f"Unloaded model: {model_id}")
            return True
        return False

    def detect(self, image: np.ndarray, model_id: str,
               min_confidence: float = 0.5,
               classes: Optional[List[str]] = None) -> List[DetectionResult]:
        """Perform detection using CoreML."""
        if model_id not in self.models:
            raise ValueError(f"Model not loaded: {model_id}")

        from PIL import Image
        import cv2

        model = self.models[model_id]
        info = self.model_info[model_id]

        # Preprocess - CoreML typically uses PIL images
        resized = cv2.resize(image, info.input_size)
        pil_image = Image.fromarray(resized)

        # Run inference
        predictions = model.predict({'image': pil_image})

        # Get output (varies by model)
        # Handle common YOLO output formats
        output = None
        for key in predictions:
            if 'output' in key.lower() or 'boxes' in key.lower():
                output = predictions[key]
                break

        if output is None:
            output = list(predictions.values())[0]

        # Postprocess
        if isinstance(output, np.ndarray):
            results = self.postprocess_yolo(
                output,
                image.shape[1],
                image.shape[0],
                info.input_size,
                min_confidence,
                info.classes
            )
        else:
            # Handle dictionary-style output (some CoreML models)
            results = self._postprocess_coreml_dict(
                predictions,
                image.shape[1],
                image.shape[0],
                min_confidence,
                info.classes
            )

        # Filter by classes if specified
        if classes:
            results = [r for r in results if r.label in classes]

        return results

    def _postprocess_coreml_dict(self, predictions: dict,
                                  img_width: int, img_height: int,
                                  min_confidence: float,
                                  class_names: List[str]) -> List[DetectionResult]:
        """Postprocess dictionary-style CoreML output."""
        results = []

        # Look for bounding box arrays
        boxes = predictions.get('boxes', predictions.get('coordinates', []))
        scores = predictions.get('confidence', predictions.get('scores', []))
        labels = predictions.get('classLabels', predictions.get('labels', []))

        if not isinstance(boxes, np.ndarray):
            boxes = np.array(boxes)
        if not isinstance(scores, np.ndarray):
            scores = np.array(scores)

        for i in range(len(boxes)):
            confidence = float(scores[i]) if i < len(scores) else 0.5
            if confidence < min_confidence:
                continue

            box = boxes[i]
            # Handle different box formats
            if len(box) == 4:
                x1, y1, x2, y2 = box
                # Normalize if needed
                if x2 > 1 or y2 > 1:
                    x1, x2 = x1 / img_width, x2 / img_width
                    y1, y2 = y1 / img_height, y2 / img_height
            else:
                continue

            label_idx = int(labels[i]) if i < len(labels) else 0
            label = class_names[label_idx] if label_idx < len(class_names) else f"class_{label_idx}"
            object_type = self._classify_object_type(label)

            results.append(DetectionResult(
                object_type=object_type,
                label=label,
                confidence=confidence,
                bbox=BoundingBox(
                    x=float(x1),
                    y=float(y1),
                    width=float(x2 - x1),
                    height=float(y2 - y1)
                )
            ))

        return results

    def get_models(self) -> List[ModelInfo]:
        """Get loaded models."""
        return list(self.model_info.values())

    def close(self) -> None:
        """Close all models."""
        for model_id in list(self.models.keys()):
            self.unload_model(model_id)

"""ONNX Runtime detector - baseline/fallback backend."""

import logging
from typing import Dict, List, Optional, Tuple
import numpy as np
import uuid

try:
    import onnxruntime as ort
    ONNX_AVAILABLE = True
except ImportError:
    ONNX_AVAILABLE = False

from .base import (
    Detector, DetectionResult, BackendType, ModelType,
    ModelInfo, BackendInfo, COCO_CLASSES
)

logger = logging.getLogger(__name__)


class ONNXDetector(Detector):
    """ONNX Runtime detector (CPU/GPU baseline)."""

    def __init__(self, device: str = "cpu", device_index: int = 0):
        """
        Initialize ONNX Runtime detector.

        Args:
            device: "cpu" or "cuda"
            device_index: GPU device index (if using CUDA)
        """
        self.device = device
        self.device_index = device_index
        self.sessions: Dict[str, ort.InferenceSession] = {}
        self.model_info: Dict[str, ModelInfo] = {}

        # Configure session options
        self.session_options = ort.SessionOptions()
        self.session_options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL

        # Configure providers
        if device == "cuda" and 'CUDAExecutionProvider' in ort.get_available_providers():
            self.providers = [
                ('CUDAExecutionProvider', {'device_id': device_index}),
                'CPUExecutionProvider'
            ]
        else:
            self.providers = ['CPUExecutionProvider']

    def name(self) -> str:
        return "ONNX Runtime"

    def backend_type(self) -> BackendType:
        return BackendType.ONNX

    def is_available(self) -> bool:
        return ONNX_AVAILABLE

    def get_info(self) -> BackendInfo:
        if not ONNX_AVAILABLE:
            return BackendInfo(
                type=BackendType.ONNX,
                available=False
            )

        providers = ort.get_available_providers()
        device = "CUDA" if 'CUDAExecutionProvider' in providers else "CPU"

        return BackendInfo(
            type=BackendType.ONNX,
            available=True,
            version=ort.__version__,
            device=device,
            device_index=self.device_index,
            compute=", ".join(providers)
        )

    def load_model(self, path: str, model_type: ModelType,
                   model_id: Optional[str] = None) -> str:
        """Load an ONNX model."""
        if not ONNX_AVAILABLE:
            raise RuntimeError("ONNX Runtime not available")

        model_id = model_id or str(uuid.uuid4())[:8]

        try:
            session = ort.InferenceSession(
                path,
                sess_options=self.session_options,
                providers=self.providers
            )

            # Get input/output info
            inputs = session.get_inputs()
            input_shape = inputs[0].shape

            # Determine input size (assuming NCHW or NHWC)
            if len(input_shape) == 4:
                if input_shape[1] == 3:  # NCHW
                    input_size = (input_shape[3], input_shape[2])
                    input_format = "nchw"
                else:  # NHWC
                    input_size = (input_shape[2], input_shape[1])
                    input_format = "nhwc"
            else:
                input_size = (640, 640)
                input_format = "nchw"

            self.sessions[model_id] = session
            self.model_info[model_id] = ModelInfo(
                id=model_id,
                name=path.split('/')[-1],
                type=model_type,
                backend=BackendType.ONNX,
                path=path,
                input_size=input_size,
                input_format=input_format,
                pixel_format="rgb",
                classes=COCO_CLASSES,
                loaded=True
            )

            logger.info(f"Loaded ONNX model: {model_id} from {path}")
            return model_id

        except Exception as e:
            logger.error(f"Failed to load ONNX model: {e}")
            raise

    def unload_model(self, model_id: str) -> bool:
        """Unload a model."""
        if model_id in self.sessions:
            del self.sessions[model_id]
            del self.model_info[model_id]
            logger.info(f"Unloaded model: {model_id}")
            return True
        return False

    def detect(self, image: np.ndarray, model_id: str,
               min_confidence: float = 0.5,
               classes: Optional[List[str]] = None) -> List[DetectionResult]:
        """Perform detection using ONNX Runtime."""
        if model_id not in self.sessions:
            raise ValueError(f"Model not loaded: {model_id}")

        session = self.sessions[model_id]
        info = self.model_info[model_id]

        # Preprocess
        input_data = self.preprocess(
            image,
            info.input_size,
            info.input_format,
            info.pixel_format
        )

        # Run inference
        input_name = session.get_inputs()[0].name
        outputs = session.run(None, {input_name: input_data})

        # Postprocess based on model type
        if info.type in [ModelType.YOLOv8, ModelType.YOLO11, ModelType.YOLO12]:
            results = self._postprocess_yolov8(
                outputs[0],
                image.shape[1],
                image.shape[0],
                info.input_size,
                min_confidence,
                info.classes
            )
        else:
            results = self.postprocess_yolo(
                outputs[0],
                image.shape[1],
                image.shape[0],
                info.input_size,
                min_confidence,
                info.classes
            )

        # Filter by classes if specified
        if classes:
            results = [r for r in results if r.label in classes]

        return results

    def _postprocess_yolov8(self, outputs: np.ndarray, img_width: int, img_height: int,
                            input_size: Tuple[int, int], min_confidence: float,
                            class_names: List[str]) -> List[DetectionResult]:
        """
        Postprocess YOLOv8/v11/v12 outputs.

        YOLOv8 output format is [batch, 84, num_detections] where:
        - First 4 values are x_center, y_center, width, height
        - Next 80 values are class probabilities (COCO)
        """
        from .base import BoundingBox, DetectionResult

        results = []

        # Handle batch dimension
        if len(outputs.shape) == 3:
            outputs = outputs[0]  # [84, num_detections]

        # YOLOv8 outputs are transposed: [84, N] -> transpose to [N, 84]
        if outputs.shape[0] < outputs.shape[1]:
            outputs = outputs.T  # Now [N, 84]

        for detection in outputs:
            # Extract bounding box (first 4 values)
            x_center, y_center, width, height = detection[:4]

            # Get class probabilities (remaining values)
            class_probs = detection[4:]

            # Find best class
            class_id = np.argmax(class_probs)
            confidence = float(class_probs[class_id])

            if confidence < min_confidence:
                continue

            # Convert from center format to normalized coordinates
            x = (x_center - width / 2) / input_size[0]
            y = (y_center - height / 2) / input_size[1]
            w = width / input_size[0]
            h = height / input_size[1]

            # Clamp to valid range
            x = max(0, min(1, x))
            y = max(0, min(1, y))
            w = max(0, min(1 - x, w))
            h = max(0, min(1 - y, h))

            # Skip invalid boxes
            if w <= 0 or h <= 0:
                continue

            label = class_names[class_id] if class_id < len(class_names) else f"class_{class_id}"
            object_type = self._classify_object_type(label)

            results.append(DetectionResult(
                object_type=object_type,
                label=label,
                confidence=confidence,
                bbox=BoundingBox(x=x, y=y, width=w, height=h)
            ))

        return self._apply_nms(results, iou_threshold=0.45)

    def get_models(self) -> List[ModelInfo]:
        """Get loaded models."""
        return list(self.model_info.values())

    def close(self) -> None:
        """Close all sessions."""
        for model_id in list(self.sessions.keys()):
            self.unload_model(model_id)

"""Base detector interface and common types."""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Tuple
import numpy as np
from enum import Enum


class BackendType(str, Enum):
    """Detection backend types."""
    NVIDIA = "nvidia"
    OPENVINO = "openvino"
    CORAL = "coral"
    COREML = "coreml"
    ONNX = "onnx"
    CPU = "cpu"


class ModelType(str, Enum):
    """Detection model types."""
    YOLO12 = "yolo12"
    YOLO11 = "yolo11"
    YOLOv8 = "yolov8"
    YOLOv9 = "yolov9"
    YOLONAS = "yolonas"
    MOBILENET = "mobilenet"
    MOBILEDET = "mobiledet"
    FRIGATE_PLUS = "frigate_plus"


@dataclass
class BoundingBox:
    """Detection bounding box (normalized 0-1 coordinates)."""
    x: float  # Top-left X
    y: float  # Top-left Y
    width: float
    height: float

    @property
    def center(self) -> Tuple[float, float]:
        """Get center point."""
        return (self.x + self.width / 2, self.y + self.height / 2)

    @property
    def area(self) -> float:
        """Get area."""
        return self.width * self.height

    def to_pixels(self, img_width: int, img_height: int) -> Tuple[int, int, int, int]:
        """Convert to pixel coordinates (x1, y1, x2, y2)."""
        x1 = int(self.x * img_width)
        y1 = int(self.y * img_height)
        x2 = int((self.x + self.width) * img_width)
        y2 = int((self.y + self.height) * img_height)
        return (x1, y1, x2, y2)

    @classmethod
    def from_pixels(cls, x1: int, y1: int, x2: int, y2: int,
                    img_width: int, img_height: int) -> 'BoundingBox':
        """Create from pixel coordinates."""
        return cls(
            x=x1 / img_width,
            y=y1 / img_height,
            width=(x2 - x1) / img_width,
            height=(y2 - y1) / img_height
        )


@dataclass
class DetectionResult:
    """Single detection result."""
    object_type: str
    label: str
    confidence: float
    bbox: BoundingBox
    track_id: Optional[str] = None
    attributes: Dict[str, str] = field(default_factory=dict)


@dataclass
class ModelInfo:
    """Model information."""
    id: str
    name: str
    type: ModelType
    backend: BackendType
    path: str
    version: str = ""
    input_size: Tuple[int, int] = (640, 640)
    input_format: str = "nchw"  # nchw or nhwc
    pixel_format: str = "rgb"  # rgb or bgr
    classes: List[str] = field(default_factory=list)
    loaded: bool = False


@dataclass
class BackendInfo:
    """Backend information."""
    type: BackendType
    available: bool
    version: str = ""
    device: str = ""
    device_index: int = 0
    memory: int = 0  # bytes
    compute: str = ""


class Detector(ABC):
    """Abstract base class for detection backends."""

    @abstractmethod
    def name(self) -> str:
        """Return detector name."""
        pass

    @abstractmethod
    def backend_type(self) -> BackendType:
        """Return backend type."""
        pass

    @abstractmethod
    def is_available(self) -> bool:
        """Check if backend is available."""
        pass

    @abstractmethod
    def get_info(self) -> BackendInfo:
        """Get backend information."""
        pass

    @abstractmethod
    def load_model(self, path: str, model_type: ModelType,
                   model_id: Optional[str] = None) -> str:
        """Load a model. Returns model ID."""
        pass

    @abstractmethod
    def unload_model(self, model_id: str) -> bool:
        """Unload a model."""
        pass

    @abstractmethod
    def detect(self, image: np.ndarray, model_id: str,
               min_confidence: float = 0.5,
               classes: Optional[List[str]] = None) -> List[DetectionResult]:
        """Perform detection. Image should be in RGB format."""
        pass

    @abstractmethod
    def get_models(self) -> List[ModelInfo]:
        """Get loaded models."""
        pass

    def close(self) -> None:
        """Close the detector and release resources."""
        pass

    def preprocess(self, image: np.ndarray, target_size: Tuple[int, int],
                   input_format: str = "nchw",
                   pixel_format: str = "rgb") -> np.ndarray:
        """Preprocess image for inference."""
        import cv2

        # Resize
        resized = cv2.resize(image, target_size)

        # Convert pixel format if needed
        if pixel_format == "bgr":
            resized = cv2.cvtColor(resized, cv2.COLOR_RGB2BGR)

        # Normalize to 0-1
        normalized = resized.astype(np.float32) / 255.0

        # Convert to NCHW if needed
        if input_format == "nchw":
            normalized = np.transpose(normalized, (2, 0, 1))

        # Add batch dimension
        return np.expand_dims(normalized, axis=0)

    def postprocess_yolo(self, outputs: np.ndarray, img_width: int, img_height: int,
                         input_size: Tuple[int, int], min_confidence: float,
                         classes: List[str]) -> List[DetectionResult]:
        """Postprocess YOLO-style outputs."""
        results = []

        # Handle different YOLO output formats
        if len(outputs.shape) == 3:
            # [batch, num_detections, 5+num_classes] format
            outputs = outputs[0]  # Remove batch dimension
        elif len(outputs.shape) == 2:
            pass  # Already [num_detections, 5+num_classes]

        for detection in outputs:
            if len(detection) < 5:
                continue

            # Extract box and confidence
            x_center, y_center, width, height = detection[:4]

            # Get class scores
            if len(detection) > 5:
                class_scores = detection[5:]
                class_id = np.argmax(class_scores)
                confidence = float(class_scores[class_id])
            else:
                confidence = float(detection[4])
                class_id = 0

            if confidence < min_confidence:
                continue

            # Convert from center format to corner format
            x = (x_center - width / 2) / input_size[0]
            y = (y_center - height / 2) / input_size[1]
            w = width / input_size[0]
            h = height / input_size[1]

            # Clamp to valid range
            x = max(0, min(1, x))
            y = max(0, min(1, y))
            w = max(0, min(1 - x, w))
            h = max(0, min(1 - y, h))

            label = classes[class_id] if class_id < len(classes) else f"class_{class_id}"
            object_type = self._classify_object_type(label)

            results.append(DetectionResult(
                object_type=object_type,
                label=label,
                confidence=confidence,
                bbox=BoundingBox(x=x, y=y, width=w, height=h)
            ))

        return self._apply_nms(results, iou_threshold=0.45)

    def _classify_object_type(self, label: str) -> str:
        """Classify label into object type."""
        label_lower = label.lower()

        if label_lower in ['person', 'pedestrian', 'man', 'woman', 'child']:
            return 'person'
        elif label_lower in ['car', 'truck', 'bus', 'motorcycle', 'bicycle', 'vehicle']:
            return 'vehicle'
        elif label_lower in ['dog', 'cat', 'bird', 'horse', 'cow', 'sheep', 'animal']:
            return 'animal'
        elif label_lower in ['face']:
            return 'face'
        elif label_lower in ['license plate', 'plate']:
            return 'license_plate'
        elif label_lower in ['package', 'box']:
            return 'package'
        else:
            return 'unknown'

    def _apply_nms(self, detections: List[DetectionResult],
                   iou_threshold: float = 0.45) -> List[DetectionResult]:
        """Apply Non-Maximum Suppression."""
        if len(detections) <= 1:
            return detections

        # Sort by confidence
        detections = sorted(detections, key=lambda d: d.confidence, reverse=True)

        keep = []
        while detections:
            best = detections.pop(0)
            keep.append(best)

            detections = [
                d for d in detections
                if self._compute_iou(best.bbox, d.bbox) < iou_threshold
            ]

        return keep

    def _compute_iou(self, box1: BoundingBox, box2: BoundingBox) -> float:
        """Compute Intersection over Union."""
        x1 = max(box1.x, box2.x)
        y1 = max(box1.y, box2.y)
        x2 = min(box1.x + box1.width, box2.x + box2.width)
        y2 = min(box1.y + box1.height, box2.y + box2.height)

        if x2 <= x1 or y2 <= y1:
            return 0.0

        intersection = (x2 - x1) * (y2 - y1)
        union = box1.area + box2.area - intersection

        return intersection / union if union > 0 else 0.0


# COCO class names (common across YOLO models)
COCO_CLASSES = [
    'person', 'bicycle', 'car', 'motorcycle', 'airplane', 'bus', 'train', 'truck',
    'boat', 'traffic light', 'fire hydrant', 'stop sign', 'parking meter', 'bench',
    'bird', 'cat', 'dog', 'horse', 'sheep', 'cow', 'elephant', 'bear', 'zebra',
    'giraffe', 'backpack', 'umbrella', 'handbag', 'tie', 'suitcase', 'frisbee',
    'skis', 'snowboard', 'sports ball', 'kite', 'baseball bat', 'baseball glove',
    'skateboard', 'surfboard', 'tennis racket', 'bottle', 'wine glass', 'cup',
    'fork', 'knife', 'spoon', 'bowl', 'banana', 'apple', 'sandwich', 'orange',
    'broccoli', 'carrot', 'hot dog', 'pizza', 'donut', 'cake', 'chair', 'couch',
    'potted plant', 'bed', 'dining table', 'toilet', 'tv', 'laptop', 'mouse',
    'remote', 'keyboard', 'cell phone', 'microwave', 'oven', 'toaster', 'sink',
    'refrigerator', 'book', 'clock', 'vase', 'scissors', 'teddy bear', 'hair drier',
    'toothbrush'
]

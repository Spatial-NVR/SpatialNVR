"""Detection service implementation."""

import logging
import time
import uuid
from typing import Dict, List, Optional, Tuple
from dataclasses import dataclass, field
from concurrent.futures import ThreadPoolExecutor
import threading
import numpy as np
import cv2

from backends.base import (
    Detector, DetectionResult, BackendType, ModelType,
    ModelInfo, BackendInfo
)
from backends.onnx_runtime import ONNXDetector

# Motion detection
from motion_detection import MotionDetector, MotionConfig, MotionRegion

# Optional imports
try:
    from backends.nvidia import NVIDIADetector
except ImportError:
    NVIDIADetector = None

try:
    from backends.openvino import OpenVINODetector
except ImportError:
    OpenVINODetector = None

try:
    from backends.coral import CoralDetector
except ImportError:
    CoralDetector = None

try:
    from backends.coreml import CoreMLDetector
except ImportError:
    CoreMLDetector = None

logger = logging.getLogger(__name__)


@dataclass
class ServiceStats:
    """Service statistics."""
    processed_count: int = 0
    error_count: int = 0
    total_latency_ms: float = 0.0
    start_time: float = field(default_factory=time.time)

    @property
    def avg_latency_ms(self) -> float:
        if self.processed_count == 0:
            return 0.0
        return self.total_latency_ms / self.processed_count

    @property
    def uptime(self) -> float:
        return time.time() - self.start_time


class DetectionService:
    """Main detection service."""

    def __init__(self, config: Optional[dict] = None):
        """
        Initialize detection service.

        Args:
            config: Optional configuration dictionary
        """
        self.config = config or {}
        self.detectors: Dict[BackendType, Detector] = {}
        self.default_backend: Optional[BackendType] = None
        self.stats = ServiceStats()
        self.lock = threading.RLock()
        self.executor = ThreadPoolExecutor(max_workers=4)

        # Initialize motion detection
        motion_config = MotionConfig(
            enabled=self.config.get('motion_detection', {}).get('enabled', True),
            threshold=self.config.get('motion_detection', {}).get('threshold', 0.02),
            pixel_threshold=self.config.get('motion_detection', {}).get('pixel_threshold', 25),
        )
        self.motion_detector = MotionDetector(motion_config)

        # Initialize backends
        self._init_backends()

    def _init_backends(self) -> None:
        """Initialize available detection backends."""
        # Always try ONNX Runtime first (baseline)
        try:
            onnx = ONNXDetector()
            if onnx.is_available():
                self.detectors[BackendType.ONNX] = onnx
                self.default_backend = BackendType.ONNX
                logger.info("ONNX Runtime backend available")
        except Exception as e:
            logger.warning(f"Failed to initialize ONNX Runtime: {e}")

        # Try NVIDIA TensorRT
        if NVIDIADetector is not None:
            try:
                nvidia = NVIDIADetector()
                if nvidia.is_available():
                    self.detectors[BackendType.NVIDIA] = nvidia
                    self.default_backend = BackendType.NVIDIA
                    logger.info("NVIDIA TensorRT backend available")
            except Exception as e:
                logger.warning(f"Failed to initialize NVIDIA TensorRT: {e}")

        # Try OpenVINO
        if OpenVINODetector is not None:
            try:
                openvino = OpenVINODetector()
                if openvino.is_available():
                    self.detectors[BackendType.OPENVINO] = openvino
                    if self.default_backend is None:
                        self.default_backend = BackendType.OPENVINO
                    logger.info("OpenVINO backend available")
            except Exception as e:
                logger.warning(f"Failed to initialize OpenVINO: {e}")

        # Try Coral Edge TPU
        if CoralDetector is not None:
            try:
                coral = CoralDetector()
                if coral.is_available():
                    self.detectors[BackendType.CORAL] = coral
                    logger.info("Coral Edge TPU backend available")
            except Exception as e:
                logger.warning(f"Failed to initialize Coral: {e}")

        # Try CoreML (macOS)
        if CoreMLDetector is not None:
            try:
                coreml = CoreMLDetector()
                if coreml.is_available():
                    self.detectors[BackendType.COREML] = coreml
                    if self.default_backend is None:
                        self.default_backend = BackendType.COREML
                    logger.info("CoreML backend available")
            except Exception as e:
                logger.warning(f"Failed to initialize CoreML: {e}")

        if not self.detectors:
            logger.error("No detection backends available!")

    def get_backends(self) -> List[BackendInfo]:
        """Get information about available backends."""
        result = []
        for backend_type, detector in self.detectors.items():
            result.append(detector.get_info())
        return result

    def get_models(self) -> List[ModelInfo]:
        """Get all loaded models."""
        result = []
        for detector in self.detectors.values():
            result.extend(detector.get_models())
        return result

    def load_model(self, path: str, model_type: ModelType,
                   backend: Optional[BackendType] = None,
                   model_id: Optional[str] = None) -> str:
        """
        Load a detection model.

        Args:
            path: Path to model file
            model_type: Type of model
            backend: Preferred backend (None for auto)
            model_id: Custom model ID

        Returns:
            Model ID
        """
        # Select backend
        if backend is not None:
            if backend not in self.detectors:
                raise ValueError(f"Backend not available: {backend}")
            detector = self.detectors[backend]
        else:
            # Auto-select based on file extension
            if path.endswith('.engine'):
                detector = self.detectors.get(BackendType.NVIDIA)
            elif path.endswith('_edgetpu.tflite'):
                detector = self.detectors.get(BackendType.CORAL)
            elif path.endswith('.mlpackage') or path.endswith('.mlmodel'):
                detector = self.detectors.get(BackendType.COREML)
            elif path.endswith('.xml') or path.endswith('.bin'):
                detector = self.detectors.get(BackendType.OPENVINO)
            else:
                # Default to ONNX for .onnx files
                detector = self.detectors.get(self.default_backend)

            if detector is None:
                raise ValueError("No suitable backend available for model")

        return detector.load_model(path, model_type, model_id)

    def unload_model(self, model_id: str) -> bool:
        """Unload a model."""
        for detector in self.detectors.values():
            for model in detector.get_models():
                if model.id == model_id:
                    return detector.unload_model(model_id)
        return False

    def detect(self, image: np.ndarray, model_id: str,
               min_confidence: float = 0.5,
               classes: Optional[List[str]] = None,
               camera_id: str = "default",
               skip_motion_check: bool = False) -> Tuple[List[DetectionResult], bool]:
        """
        Perform object detection with optional motion pre-filtering.

        Args:
            image: RGB image as numpy array
            model_id: Model ID to use
            min_confidence: Minimum confidence threshold
            classes: Optional list of classes to filter
            camera_id: Camera identifier for per-camera motion tracking
            skip_motion_check: If True, skip motion detection and always run inference

        Returns:
            Tuple of (detection results, motion_detected)
        """
        start_time = time.time()

        try:
            # Check for motion first (unless skipped)
            motion_detected = True
            motion_regions = []

            if not skip_motion_check:
                motion_detected, motion_regions = self.motion_detector.detect(image, camera_id)

                if not motion_detected:
                    # No motion - skip expensive YOLO inference
                    logger.debug(f"No motion detected for camera {camera_id}, skipping inference")
                    return [], False

            # Find the detector that has this model
            detector = None
            for det in self.detectors.values():
                for model in det.get_models():
                    if model.id == model_id:
                        detector = det
                        break
                if detector:
                    break

            if detector is None:
                raise ValueError(f"Model not found: {model_id}")

            # Run detection
            results = detector.detect(image, model_id, min_confidence, classes)

            # Update stats
            latency = (time.time() - start_time) * 1000
            with self.lock:
                self.stats.processed_count += 1
                self.stats.total_latency_ms += latency

            return results, motion_detected

        except Exception as e:
            with self.lock:
                self.stats.error_count += 1
            logger.error(f"Detection failed: {e}")
            raise

    def detect_motion_only(self, image: np.ndarray,
                          camera_id: str = "default") -> Tuple[bool, List[MotionRegion]]:
        """
        Perform motion detection only (no YOLO inference).

        Args:
            image: RGB image as numpy array
            camera_id: Camera identifier

        Returns:
            Tuple of (motion_detected, motion_regions)
        """
        return self.motion_detector.detect(image, camera_id)

    def configure_motion(self, **kwargs) -> None:
        """
        Configure motion detection parameters.

        Args:
            enabled: Enable/disable motion detection
            method: Detection method ('frame_diff', 'mog2', 'knn')
            threshold: Motion threshold (0-1)
            pixel_threshold: Per-pixel difference threshold
        """
        self.motion_detector.set_config(**kwargs)

    def reset_motion(self, camera_id: Optional[str] = None) -> None:
        """Reset motion detection state for a camera or all cameras."""
        self.motion_detector.reset(camera_id)

    def detect_from_bytes(self, image_bytes: bytes, model_id: str,
                          min_confidence: float = 0.5,
                          classes: Optional[List[str]] = None,
                          camera_id: str = "default",
                          skip_motion_check: bool = False) -> Tuple[List[DetectionResult], bool]:
        """
        Perform detection from image bytes.

        Args:
            image_bytes: JPEG or PNG encoded image
            model_id: Model ID to use
            min_confidence: Minimum confidence threshold
            classes: Optional list of classes to filter
            camera_id: Camera identifier for motion tracking
            skip_motion_check: Skip motion detection

        Returns:
            Tuple of (detection results, motion_detected)
        """
        # Decode image
        nparr = np.frombuffer(image_bytes, np.uint8)
        image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
        if image is None:
            raise ValueError("Failed to decode image")

        # Convert BGR to RGB
        image = cv2.cvtColor(image, cv2.COLOR_BGR2RGB)

        return self.detect(image, model_id, min_confidence, classes, camera_id, skip_motion_check)

    def get_status(self) -> dict:
        """Get service status."""
        return {
            'connected': True,
            'backends': [b.__dict__ for b in self.get_backends()],
            'models': [m.__dict__ for m in self.get_models()],
            'queue_size': 0,
            'processed_count': self.stats.processed_count,
            'error_count': self.stats.error_count,
            'avg_latency_ms': self.stats.avg_latency_ms,
            'uptime': self.stats.uptime,
            'motion_detection': self.motion_detector.get_stats()
        }

    def close(self) -> None:
        """Close the service and release resources."""
        for detector in self.detectors.values():
            detector.close()
        self.executor.shutdown(wait=True)

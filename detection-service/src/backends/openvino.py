"""OpenVINO detector for Intel CPUs and NPUs (2025.x)."""

import logging
from typing import Dict, List, Optional, Tuple
import numpy as np
import uuid

try:
    from openvino import Core, Type, Layout
    from openvino.preprocess import PrePostProcessor
    OPENVINO_AVAILABLE = True
except ImportError:
    OPENVINO_AVAILABLE = False

from .base import (
    Detector, DetectionResult, BackendType, ModelType,
    ModelInfo, BackendInfo, COCO_CLASSES
)

logger = logging.getLogger(__name__)


class OpenVINODetector(Detector):
    """OpenVINO detector for Intel CPUs and NPUs."""

    def __init__(self, device: str = "AUTO"):
        """
        Initialize OpenVINO detector.

        Args:
            device: Device to use - "CPU", "GPU", "NPU", or "AUTO"
        """
        self.device = device
        self.models: Dict[str, any] = {}  # Compiled models
        self.model_info: Dict[str, ModelInfo] = {}

        if OPENVINO_AVAILABLE:
            self.core = Core()
            self._available_devices = self.core.available_devices
        else:
            self.core = None
            self._available_devices = []

    def name(self) -> str:
        return "OpenVINO"

    def backend_type(self) -> BackendType:
        return BackendType.OPENVINO

    def is_available(self) -> bool:
        return OPENVINO_AVAILABLE

    def get_info(self) -> BackendInfo:
        if not OPENVINO_AVAILABLE:
            return BackendInfo(
                type=BackendType.OPENVINO,
                available=False
            )

        # Get OpenVINO version
        try:
            from openvino import get_version
            version = get_version()
        except Exception:
            version = "unknown"

        # Determine device info
        device_name = self.device
        if self.device == "AUTO":
            # Auto-select best device
            if "NPU" in self._available_devices:
                device_name = "NPU"
            elif "GPU" in self._available_devices:
                device_name = "GPU"
            else:
                device_name = "CPU"

        return BackendInfo(
            type=BackendType.OPENVINO,
            available=True,
            version=version,
            device=device_name,
            compute=", ".join(self._available_devices)
        )

    def load_model(self, path: str, model_type: ModelType,
                   model_id: Optional[str] = None) -> str:
        """Load an OpenVINO model (ONNX or IR format)."""
        if not OPENVINO_AVAILABLE:
            raise RuntimeError("OpenVINO not available")

        model_id = model_id or str(uuid.uuid4())[:8]

        try:
            # Read model
            model = self.core.read_model(path)

            # Get input info
            input_layer = model.input(0)
            input_shape = input_layer.shape

            # Determine input format
            if len(input_shape) == 4:
                if input_shape[1] == 3:  # NCHW
                    input_size = (int(input_shape[3]), int(input_shape[2]))
                    input_format = "nchw"
                else:  # NHWC
                    input_size = (int(input_shape[2]), int(input_shape[1]))
                    input_format = "nhwc"
            else:
                input_size = (640, 640)
                input_format = "nchw"

            # Add preprocessing
            ppp = PrePostProcessor(model)
            ppp.input().tensor() \
                .set_element_type(Type.f32) \
                .set_layout(Layout("NCHW"))
            ppp.input().model().set_layout(Layout("NCHW"))
            model = ppp.build()

            # Compile model
            compiled_model = self.core.compile_model(model, self.device)

            self.models[model_id] = compiled_model
            self.model_info[model_id] = ModelInfo(
                id=model_id,
                name=path.split('/')[-1],
                type=model_type,
                backend=BackendType.OPENVINO,
                path=path,
                input_size=input_size,
                input_format=input_format,
                pixel_format="rgb",
                classes=COCO_CLASSES,
                loaded=True
            )

            logger.info(f"Loaded OpenVINO model: {model_id} from {path}")
            return model_id

        except Exception as e:
            logger.error(f"Failed to load OpenVINO model: {e}")
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
        """Perform detection using OpenVINO."""
        if model_id not in self.models:
            raise ValueError(f"Model not loaded: {model_id}")

        compiled_model = self.models[model_id]
        info = self.model_info[model_id]

        # Preprocess
        input_data = self.preprocess(
            image,
            info.input_size,
            info.input_format,
            info.pixel_format
        )

        # Run inference
        infer_request = compiled_model.create_infer_request()
        infer_request.infer({0: input_data})

        # Get output
        output = infer_request.get_output_tensor(0).data

        # Postprocess
        results = self.postprocess_yolo(
            output,
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

    def get_models(self) -> List[ModelInfo]:
        """Get loaded models."""
        return list(self.model_info.values())

    def close(self) -> None:
        """Close all models."""
        for model_id in list(self.models.keys()):
            self.unload_model(model_id)

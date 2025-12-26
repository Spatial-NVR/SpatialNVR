"""NVIDIA TensorRT detector for GPU acceleration."""

import logging
from typing import Dict, List, Optional, Tuple
import numpy as np
import uuid

try:
    import tensorrt as trt
    import pycuda.driver as cuda
    import pycuda.autoinit
    TENSORRT_AVAILABLE = True
except ImportError:
    TENSORRT_AVAILABLE = False

from .base import (
    Detector, DetectionResult, BackendType, ModelType,
    ModelInfo, BackendInfo, COCO_CLASSES
)

logger = logging.getLogger(__name__)


class TensorRTEngine:
    """TensorRT engine wrapper."""

    def __init__(self, engine_path: str):
        """Load TensorRT engine."""
        if not TENSORRT_AVAILABLE:
            raise RuntimeError("TensorRT not available")

        self.logger = trt.Logger(trt.Logger.WARNING)
        self.runtime = trt.Runtime(self.logger)

        with open(engine_path, 'rb') as f:
            self.engine = self.runtime.deserialize_cuda_engine(f.read())

        self.context = self.engine.create_execution_context()

        # Allocate buffers
        self.inputs = []
        self.outputs = []
        self.bindings = []
        self.stream = cuda.Stream()

        for i in range(self.engine.num_io_tensors):
            name = self.engine.get_tensor_name(i)
            shape = self.engine.get_tensor_shape(name)
            dtype = trt.nptype(self.engine.get_tensor_dtype(name))
            size = trt.volume(shape)

            # Allocate device memory
            device_mem = cuda.mem_alloc(size * dtype().itemsize)
            self.bindings.append(int(device_mem))

            if self.engine.get_tensor_mode(name) == trt.TensorIOMode.INPUT:
                self.inputs.append({
                    'name': name,
                    'shape': shape,
                    'dtype': dtype,
                    'device': device_mem,
                    'host': np.zeros(shape, dtype=dtype)
                })
            else:
                self.outputs.append({
                    'name': name,
                    'shape': shape,
                    'dtype': dtype,
                    'device': device_mem,
                    'host': np.zeros(shape, dtype=dtype)
                })

    def infer(self, input_data: np.ndarray) -> List[np.ndarray]:
        """Run inference."""
        # Copy input to device
        self.inputs[0]['host'] = input_data.astype(self.inputs[0]['dtype'])
        cuda.memcpy_htod_async(
            self.inputs[0]['device'],
            self.inputs[0]['host'],
            self.stream
        )

        # Set tensor addresses
        for inp in self.inputs:
            self.context.set_tensor_address(inp['name'], int(inp['device']))
        for out in self.outputs:
            self.context.set_tensor_address(out['name'], int(out['device']))

        # Run inference
        self.context.execute_async_v3(stream_handle=self.stream.handle)

        # Copy outputs from device
        results = []
        for out in self.outputs:
            cuda.memcpy_dtoh_async(out['host'], out['device'], self.stream)
            results.append(out['host'].copy())

        self.stream.synchronize()
        return results

    def __del__(self):
        """Cleanup."""
        pass  # CUDA resources are automatically freed


class NVIDIADetector(Detector):
    """NVIDIA TensorRT detector for GPU acceleration."""

    def __init__(self, device_index: int = 0):
        """
        Initialize NVIDIA detector.

        Args:
            device_index: CUDA device index
        """
        self.device_index = device_index
        self.engines: Dict[str, TensorRTEngine] = {}
        self.model_info: Dict[str, ModelInfo] = {}

        if TENSORRT_AVAILABLE:
            cuda.init()
            self.device = cuda.Device(device_index)
            self.cuda_context = self.device.make_context()
        else:
            self.device = None
            self.cuda_context = None

    def name(self) -> str:
        return "NVIDIA TensorRT"

    def backend_type(self) -> BackendType:
        return BackendType.NVIDIA

    def is_available(self) -> bool:
        if not TENSORRT_AVAILABLE:
            return False
        try:
            cuda.init()
            return cuda.Device.count() > 0
        except Exception:
            return False

    def get_info(self) -> BackendInfo:
        if not self.is_available():
            return BackendInfo(
                type=BackendType.NVIDIA,
                available=False
            )

        device = cuda.Device(self.device_index)
        return BackendInfo(
            type=BackendType.NVIDIA,
            available=True,
            version=trt.__version__,
            device=device.name(),
            device_index=self.device_index,
            memory=device.total_memory(),
            compute=f"SM {device.compute_capability()[0]}.{device.compute_capability()[1]}"
        )

    def load_model(self, path: str, model_type: ModelType,
                   model_id: Optional[str] = None) -> str:
        """Load a TensorRT engine."""
        if not TENSORRT_AVAILABLE:
            raise RuntimeError("TensorRT not available")

        model_id = model_id or str(uuid.uuid4())[:8]

        try:
            self.cuda_context.push()
            engine = TensorRTEngine(path)
            self.cuda_context.pop()

            # Get input shape
            input_shape = engine.inputs[0]['shape']
            input_size = (input_shape[3], input_shape[2])  # Assuming NCHW

            self.engines[model_id] = engine
            self.model_info[model_id] = ModelInfo(
                id=model_id,
                name=path.split('/')[-1],
                type=model_type,
                backend=BackendType.NVIDIA,
                path=path,
                input_size=input_size,
                input_format="nchw",
                pixel_format="rgb",
                classes=COCO_CLASSES,
                loaded=True
            )

            logger.info(f"Loaded TensorRT engine: {model_id} from {path}")
            return model_id

        except Exception as e:
            logger.error(f"Failed to load TensorRT engine: {e}")
            raise

    def unload_model(self, model_id: str) -> bool:
        """Unload a model."""
        if model_id in self.engines:
            del self.engines[model_id]
            del self.model_info[model_id]
            logger.info(f"Unloaded model: {model_id}")
            return True
        return False

    def detect(self, image: np.ndarray, model_id: str,
               min_confidence: float = 0.5,
               classes: Optional[List[str]] = None) -> List[DetectionResult]:
        """Perform detection using TensorRT."""
        if model_id not in self.engines:
            raise ValueError(f"Model not loaded: {model_id}")

        engine = self.engines[model_id]
        info = self.model_info[model_id]

        # Preprocess
        input_data = self.preprocess(
            image,
            info.input_size,
            info.input_format,
            info.pixel_format
        )

        # Run inference
        self.cuda_context.push()
        outputs = engine.infer(input_data)
        self.cuda_context.pop()

        # Postprocess
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

    def get_models(self) -> List[ModelInfo]:
        """Get loaded models."""
        return list(self.model_info.values())

    def close(self) -> None:
        """Close all engines and CUDA context."""
        for model_id in list(self.engines.keys()):
            self.unload_model(model_id)
        if self.cuda_context:
            self.cuda_context.pop()

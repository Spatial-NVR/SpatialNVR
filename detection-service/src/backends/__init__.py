# Detection Backends
from .base import Detector, DetectionResult, BoundingBox
from .onnx_runtime import ONNXDetector

__all__ = [
    'Detector',
    'DetectionResult',
    'BoundingBox',
    'ONNXDetector',
]

# Optional imports
try:
    from .nvidia import NVIDIADetector
    __all__.append('NVIDIADetector')
except ImportError:
    pass

try:
    from .openvino import OpenVINODetector
    __all__.append('OpenVINODetector')
except ImportError:
    pass

try:
    from .coral import CoralDetector
    __all__.append('CoralDetector')
except ImportError:
    pass

try:
    from .coreml import CoreMLDetector
    __all__.append('CoreMLDetector')
except ImportError:
    pass

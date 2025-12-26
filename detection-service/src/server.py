"""gRPC server for the detection service."""

import logging
import os
import sys
import time
from concurrent import futures
import signal

import grpc
import numpy as np
import cv2

# Add parent directory to path for imports
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# Import generated protobuf modules
# Note: These would be generated from detection.proto
# For now, we'll use a simple implementation

from service import DetectionService
from backends.base import BackendType, ModelType

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class DetectionServicer:
    """gRPC servicer for detection service."""

    def __init__(self, detection_service: DetectionService):
        self.service = detection_service

    def Detect(self, request, context):
        """Handle single detection request."""
        try:
            # Decode image
            if request.image_data:
                nparr = np.frombuffer(request.image_data, np.uint8)
                image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
                if image is None:
                    context.abort(grpc.StatusCode.INVALID_ARGUMENT, "Failed to decode image")
                    return
                image = cv2.cvtColor(image, cv2.COLOR_BGR2RGB)
            else:
                context.abort(grpc.StatusCode.INVALID_ARGUMENT, "No image data provided")
                return

            # Get model ID
            model_ids = list(request.model_ids) if request.model_ids else None
            if not model_ids:
                models = self.service.get_models()
                if not models:
                    context.abort(grpc.StatusCode.FAILED_PRECONDITION, "No models loaded")
                    return
                model_id = models[0].id
            else:
                model_id = model_ids[0]

            # Run detection
            start_time = time.time()
            results = self.service.detect(
                image,
                model_id,
                request.min_confidence or 0.5,
                list(request.object_types) if request.object_types else None
            )
            process_time = (time.time() - start_time) * 1000

            # Build response
            response = {
                'camera_id': request.camera_id,
                'timestamp_ms': request.timestamp_ms or int(time.time() * 1000),
                'frame_id': request.frame_id,
                'detections': [
                    {
                        'id': str(i),
                        'object_type': r.object_type,
                        'label': r.label,
                        'confidence': r.confidence,
                        'bbox': {
                            'x': r.bbox.x,
                            'y': r.bbox.y,
                            'width': r.bbox.width,
                            'height': r.bbox.height
                        },
                        'track_id': r.track_id or '',
                        'attributes': r.attributes
                    }
                    for i, r in enumerate(results)
                ],
                'process_time_ms': process_time,
                'model_id': model_id
            }

            return response

        except Exception as e:
            logger.error(f"Detection error: {e}")
            context.abort(grpc.StatusCode.INTERNAL, str(e))

    def GetStatus(self, request, context):
        """Get service status."""
        return self.service.get_status()

    def LoadModel(self, request, context):
        """Load a model."""
        try:
            backend = BackendType(request.backend) if request.backend else None
            model_type = ModelType(request.type) if request.type else ModelType.YOLOv8

            model_id = self.service.load_model(
                request.path,
                model_type,
                backend,
                request.model_id or None
            )

            return {
                'success': True,
                'model_id': model_id
            }

        except Exception as e:
            logger.error(f"Failed to load model: {e}")
            return {
                'success': False,
                'error': str(e)
            }

    def UnloadModel(self, request, context):
        """Unload a model."""
        success = self.service.unload_model(request.model_id)
        return {'success': success}

    def GetModels(self, request, context):
        """Get loaded models."""
        models = self.service.get_models()
        return {'models': [m.__dict__ for m in models]}

    def GetBackends(self, request, context):
        """Get available backends."""
        backends = self.service.get_backends()
        return {'backends': [b.__dict__ for b in backends]}


def serve(port: int = 50051):
    """Start the gRPC server."""
    # Create detection service
    detection_service = DetectionService()

    # Create gRPC server
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=10),
        options=[
            ('grpc.max_send_message_length', 100 * 1024 * 1024),  # 100MB
            ('grpc.max_receive_message_length', 100 * 1024 * 1024),
        ]
    )

    # Add servicer
    servicer = DetectionServicer(detection_service)

    # Note: In production, you would register the generated servicer here:
    # detection_pb2_grpc.add_DetectionServiceServicer_to_server(servicer, server)

    # For now, we'll just start a simple server
    server.add_insecure_port(f'[::]:{port}')
    server.start()

    logger.info(f"Detection service started on port {port}")
    logger.info(f"Available backends: {[b.type.value for b in detection_service.get_backends()]}")

    # Handle shutdown
    def shutdown(signum, frame):
        logger.info("Shutting down...")
        detection_service.close()
        server.stop(5)

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        shutdown(None, None)


if __name__ == '__main__':
    import argparse

    parser = argparse.ArgumentParser(description='Detection Service')
    parser.add_argument('--port', type=int, default=50051, help='gRPC port')
    args = parser.parse_args()

    serve(args.port)

"""Simple HTTP server for the detection service."""

import logging
import os
import sys
import time
import json
import signal
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Optional
import urllib.parse as urlparse
import io

import numpy as np

# Add parent directory to path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

try:
    import cv2
    CV2_AVAILABLE = True
except ImportError:
    CV2_AVAILABLE = False
    print("Warning: cv2 not available, using PIL instead")
    from PIL import Image

from service import DetectionService

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class DetectionHandler(BaseHTTPRequestHandler):
    """HTTP request handler for detection service."""

    service: Optional[DetectionService] = None

    def log_message(self, format, *args):
        """Suppress default logging, use our logger instead."""
        logger.debug("%s - %s", self.address_string(), format % args)

    def _send_json(self, data: dict, status: int = 200):
        """Send JSON response."""
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Access-Control-Allow-Origin', '*')
        self.end_headers()
        self.wfile.write(json.dumps(data).encode())

    def _send_error(self, message: str, status: int = 500):
        """Send error response."""
        self._send_json({'error': message, 'success': False}, status)

    def do_OPTIONS(self):
        """Handle CORS preflight."""
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type')
        self.end_headers()

    def do_GET(self):
        """Handle GET requests."""
        parsed = urlparse.urlparse(self.path)
        path = parsed.path

        if path == '/health' or path == '/':
            self._send_json({'status': 'ok', 'service': 'detection'})

        elif path == '/status':
            if self.service:
                self._send_json(self.service.get_status())
            else:
                self._send_error('Service not initialized', 503)

        elif path == '/backends':
            if self.service:
                backends = [b.__dict__ for b in self.service.get_backends()]
                self._send_json({'backends': backends})
            else:
                self._send_error('Service not initialized', 503)

        elif path == '/models':
            if self.service:
                models = [m.__dict__ for m in self.service.get_models()]
                self._send_json({'models': models})
            else:
                self._send_error('Service not initialized', 503)

        elif path == '/motion':
            if self.service:
                self._send_json(self.service.motion_detector.get_stats())
            else:
                self._send_error('Service not initialized', 503)

        else:
            self._send_error('Not found', 404)

    def do_POST(self):
        """Handle POST requests."""
        parsed = urlparse.urlparse(self.path)
        path = parsed.path

        # Read body
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length) if content_length > 0 else b''

        if path == '/detect':
            self._handle_detect(body)

        elif path == '/load':
            self._handle_load_model(body)

        elif path == '/unload':
            self._handle_unload_model(body)

        elif path == '/motion/detect':
            self._handle_motion_detect(body)

        elif path == '/motion/config':
            self._handle_motion_config(body)

        elif path == '/motion/reset':
            self._handle_motion_reset(body)

        else:
            self._send_error('Not found', 404)

    def _handle_detect(self, body: bytes):
        """Handle detection request."""
        if not self.service:
            self._send_error('Service not initialized', 503)
            return

        try:
            # Parse request
            content_type = self.headers.get('Content-Type', '')

            if 'multipart/form-data' in content_type:
                # Handle multipart (image upload)
                # For simplicity, we'll expect JSON for now
                self._send_error('Multipart not supported, use JSON with base64 image', 400)
                return

            # Parse JSON request
            try:
                request = json.loads(body.decode())
            except json.JSONDecodeError:
                # Maybe it's raw image data
                image_data = body
                request = {'image_data': None}

            # Get parameters
            model_id = request.get('model_id')
            min_confidence = request.get('min_confidence', 0.5)
            classes = request.get('classes')
            camera_id = request.get('camera_id', 'default')
            skip_motion_check = request.get('skip_motion_check', False)

            # Get or decode image
            if 'image_data' in request and request['image_data']:
                import base64
                image_bytes = base64.b64decode(request['image_data'])
            elif 'image_url' in request:
                # Fetch from URL
                import urllib.request
                with urllib.request.urlopen(request['image_url'], timeout=5) as resp:
                    image_bytes = resp.read()
            else:
                image_bytes = body

            # Decode image
            if CV2_AVAILABLE:
                nparr = np.frombuffer(image_bytes, np.uint8)
                image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
                if image is None:
                    self._send_error('Failed to decode image', 400)
                    return
                image = cv2.cvtColor(image, cv2.COLOR_BGR2RGB)
            else:
                pil_image = Image.open(io.BytesIO(image_bytes))
                image = np.array(pil_image)

            # Get model ID if not provided
            if not model_id:
                models = self.service.get_models()
                if not models:
                    self._send_error('No models loaded', 400)
                    return
                model_id = models[0].id

            # Run detection (with motion pre-filtering)
            start_time = time.time()
            results, motion_detected = self.service.detect(
                image, model_id, min_confidence, classes,
                camera_id=camera_id, skip_motion_check=skip_motion_check
            )
            process_time = (time.time() - start_time) * 1000

            # Build response
            response = {
                'success': True,
                'camera_id': camera_id,
                'timestamp': int(time.time() * 1000),
                'motion_detected': motion_detected,
                'detections': [
                    {
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
                    for r in results
                ],
                'process_time_ms': process_time,
                'model_id': model_id
            }

            self._send_json(response)

        except Exception as e:
            logger.exception("Detection error")
            self._send_error(str(e), 500)

    def _handle_load_model(self, body: bytes):
        """Handle model loading."""
        if not self.service:
            self._send_error('Service not initialized', 503)
            return

        try:
            request = json.loads(body.decode())
            path = request.get('path')
            model_type = request.get('type', 'yolov8')
            backend = request.get('backend')
            model_id = request.get('model_id')

            if not path:
                self._send_error('Model path required', 400)
                return

            from backends.base import ModelType, BackendType

            model_type_enum = ModelType(model_type)
            backend_enum = BackendType(backend) if backend else None

            loaded_id = self.service.load_model(path, model_type_enum, backend_enum, model_id)
            self._send_json({'success': True, 'model_id': loaded_id})

        except Exception as e:
            logger.exception("Failed to load model")
            self._send_error(str(e), 500)

    def _handle_unload_model(self, body: bytes):
        """Handle model unloading."""
        if not self.service:
            self._send_error('Service not initialized', 503)
            return

        try:
            request = json.loads(body.decode())
            model_id = request.get('model_id')

            if not model_id:
                self._send_error('Model ID required', 400)
                return

            success = self.service.unload_model(model_id)
            self._send_json({'success': success})

        except Exception as e:
            logger.exception("Failed to unload model")
            self._send_error(str(e), 500)

    def _handle_motion_detect(self, body: bytes):
        """Handle motion-only detection request (no YOLO)."""
        if not self.service:
            self._send_error('Service not initialized', 503)
            return

        try:
            # Parse request
            content_type = self.headers.get('Content-Type', '')

            if 'application/json' in content_type:
                request = json.loads(body.decode())
                camera_id = request.get('camera_id', 'default')

                # Get image from base64 or URL
                if 'image_data' in request and request['image_data']:
                    import base64
                    image_bytes = base64.b64decode(request['image_data'])
                elif 'image_url' in request:
                    import urllib.request
                    with urllib.request.urlopen(request['image_url'], timeout=5) as resp:
                        image_bytes = resp.read()
                else:
                    self._send_error('image_data or image_url required', 400)
                    return
            else:
                # Raw image data
                image_bytes = body
                camera_id = 'default'

            # Decode image
            if CV2_AVAILABLE:
                nparr = np.frombuffer(image_bytes, np.uint8)
                image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
                if image is None:
                    self._send_error('Failed to decode image', 400)
                    return
                image = cv2.cvtColor(image, cv2.COLOR_BGR2RGB)
            else:
                pil_image = Image.open(io.BytesIO(image_bytes))
                image = np.array(pil_image)

            # Run motion detection only
            start_time = time.time()
            motion_detected, regions = self.service.detect_motion_only(image, camera_id)
            process_time = (time.time() - start_time) * 1000

            response = {
                'success': True,
                'camera_id': camera_id,
                'timestamp': int(time.time() * 1000),
                'motion_detected': motion_detected,
                'regions': [
                    {
                        'x': r.x,
                        'y': r.y,
                        'width': r.width,
                        'height': r.height,
                        'intensity': r.intensity
                    }
                    for r in regions
                ],
                'process_time_ms': process_time
            }

            self._send_json(response)

        except Exception as e:
            logger.exception("Motion detection error")
            self._send_error(str(e), 500)

    def _handle_motion_config(self, body: bytes):
        """Handle motion detection configuration."""
        if not self.service:
            self._send_error('Service not initialized', 503)
            return

        try:
            request = json.loads(body.decode())

            # Update motion config
            self.service.configure_motion(**request)

            self._send_json({
                'success': True,
                'config': self.service.motion_detector.get_stats()['config']
            })

        except Exception as e:
            logger.exception("Failed to configure motion detection")
            self._send_error(str(e), 500)

    def _handle_motion_reset(self, body: bytes):
        """Handle motion detection state reset."""
        if not self.service:
            self._send_error('Service not initialized', 503)
            return

        try:
            request = json.loads(body.decode()) if body else {}
            camera_id = request.get('camera_id')

            self.service.reset_motion(camera_id)

            self._send_json({
                'success': True,
                'message': f"Motion state reset for {'all cameras' if not camera_id else camera_id}"
            })

        except Exception as e:
            logger.exception("Failed to reset motion detection")
            self._send_error(str(e), 500)


def serve(port: int = 50051, model_path: Optional[str] = None):
    """Start the HTTP server."""
    # Create detection service
    detection_service = DetectionService()

    # Auto-load model if specified
    if model_path and os.path.exists(model_path):
        try:
            from backends.base import ModelType
            model_id = detection_service.load_model(model_path, ModelType.YOLOv8)
            logger.info(f"Auto-loaded model: {model_id}")
        except Exception as e:
            logger.warning(f"Failed to auto-load model: {e}")

    # Set service on handler
    DetectionHandler.service = detection_service

    # Create server
    server = HTTPServer(('0.0.0.0', port), DetectionHandler)
    logger.info(f"Detection service started on port {port}")
    logger.info(f"Available backends: {[b.type.value for b in detection_service.get_backends()]}")

    # Handle shutdown
    def shutdown(signum, frame):
        logger.info("Shutting down...")
        detection_service.close()
        server.shutdown()

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        shutdown(None, None)


if __name__ == '__main__':
    import argparse

    parser = argparse.ArgumentParser(description='Detection Service HTTP Server')
    parser.add_argument('--port', type=int, default=50051, help='HTTP port')
    parser.add_argument('--model', type=str, help='Model path to auto-load')
    args = parser.parse_args()

    serve(args.port, args.model)

"""
Motion Detection using Frame Differencing and Background Subtraction.

This module provides efficient motion detection to reduce unnecessary
YOLO inference on static scenes. It supports:
- Simple frame differencing (fast, low memory)
- Background subtraction with MOG2 (more accurate, handles lighting changes)
- Configurable sensitivity and regions of interest
"""

import logging
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Tuple
from enum import Enum
import numpy as np
import time

try:
    import cv2
    CV2_AVAILABLE = True
except ImportError:
    CV2_AVAILABLE = False

logger = logging.getLogger(__name__)


class MotionMethod(str, Enum):
    """Motion detection method."""
    FRAME_DIFF = "frame_diff"      # Simple frame differencing
    MOG2 = "mog2"                  # Mixture of Gaussians background subtraction
    KNN = "knn"                    # K-Nearest Neighbors background subtraction


@dataclass
class MotionRegion:
    """A region where motion was detected."""
    x: float          # Normalized 0-1
    y: float          # Normalized 0-1
    width: float      # Normalized 0-1
    height: float     # Normalized 0-1
    intensity: float  # Motion intensity 0-1

    def to_pixels(self, img_width: int, img_height: int) -> Tuple[int, int, int, int]:
        """Convert to pixel coordinates (x1, y1, x2, y2)."""
        x1 = int(self.x * img_width)
        y1 = int(self.y * img_height)
        x2 = int((self.x + self.width) * img_width)
        y2 = int((self.y + self.height) * img_height)
        return (x1, y1, x2, y2)


@dataclass
class MotionConfig:
    """Configuration for motion detection."""
    enabled: bool = True
    method: MotionMethod = MotionMethod.FRAME_DIFF
    threshold: float = 0.02          # Minimum % of pixels changed to trigger
    pixel_threshold: int = 25        # Minimum pixel difference to count as changed
    min_area: float = 0.001          # Minimum contour area (normalized)
    blur_size: int = 5               # Gaussian blur kernel size
    history: int = 500               # Background subtractor history (for MOG2/KNN)
    var_threshold: float = 16.0      # MOG2 variance threshold
    detect_shadows: bool = False     # Whether to detect shadows (MOG2/KNN)
    cooldown_ms: int = 100           # Minimum time between detections
    mask: Optional[np.ndarray] = None  # Optional mask for regions of interest


@dataclass
class MotionStats:
    """Statistics for motion detection."""
    frames_processed: int = 0
    motion_detected_count: int = 0
    motion_skipped_count: int = 0
    total_latency_ms: float = 0.0
    last_motion_time: float = 0.0

    @property
    def avg_latency_ms(self) -> float:
        if self.frames_processed == 0:
            return 0.0
        return self.total_latency_ms / self.frames_processed

    @property
    def motion_rate(self) -> float:
        """Percentage of frames with motion."""
        total = self.motion_detected_count + self.motion_skipped_count
        if total == 0:
            return 0.0
        return self.motion_detected_count / total


class MotionDetector:
    """
    Efficient motion detector using frame differencing or background subtraction.

    This is designed to run before expensive YOLO inference to skip
    processing on static scenes.
    """

    def __init__(self, config: Optional[MotionConfig] = None):
        """
        Initialize motion detector.

        Args:
            config: Motion detection configuration
        """
        if not CV2_AVAILABLE:
            raise RuntimeError("OpenCV (cv2) is required for motion detection")

        self.config = config or MotionConfig()
        self.stats = MotionStats()

        # State for frame differencing
        self._prev_frames: Dict[str, np.ndarray] = {}  # Per-camera previous frames

        # Background subtractors (per-camera)
        self._bg_subtractors: Dict[str, cv2.BackgroundSubtractor] = {}

        # Cooldown tracking
        self._last_motion_times: Dict[str, float] = {}

        logger.info(f"Motion detector initialized with method={self.config.method.value}, "
                   f"threshold={self.config.threshold}")

    def detect(self, image: np.ndarray, camera_id: str = "default") -> Tuple[bool, List[MotionRegion]]:
        """
        Detect motion in the given frame.

        Args:
            image: Input image (RGB or BGR, HxWxC)
            camera_id: Camera identifier for per-camera state

        Returns:
            Tuple of (has_motion: bool, regions: List[MotionRegion])
        """
        if not self.config.enabled:
            return True, []  # If disabled, always report motion

        start_time = time.time()

        try:
            # Check cooldown
            now = time.time()
            last_motion = self._last_motion_times.get(camera_id, 0)
            if (now - last_motion) * 1000 < self.config.cooldown_ms:
                # Still in cooldown, return last result
                self.stats.frames_processed += 1
                return True, []

            # Detect based on method
            if self.config.method == MotionMethod.FRAME_DIFF:
                has_motion, regions = self._detect_frame_diff(image, camera_id)
            elif self.config.method == MotionMethod.MOG2:
                has_motion, regions = self._detect_mog2(image, camera_id)
            elif self.config.method == MotionMethod.KNN:
                has_motion, regions = self._detect_knn(image, camera_id)
            else:
                has_motion, regions = True, []

            # Update stats
            self.stats.frames_processed += 1
            if has_motion:
                self.stats.motion_detected_count += 1
                self._last_motion_times[camera_id] = now
                self.stats.last_motion_time = now
            else:
                self.stats.motion_skipped_count += 1

            latency_ms = (time.time() - start_time) * 1000
            self.stats.total_latency_ms += latency_ms

            return has_motion, regions

        except Exception as e:
            logger.error(f"Motion detection error: {e}")
            return True, []  # On error, assume motion to be safe

    def _preprocess(self, image: np.ndarray) -> np.ndarray:
        """Preprocess image for motion detection."""
        # Convert to grayscale
        if len(image.shape) == 3:
            gray = cv2.cvtColor(image, cv2.COLOR_RGB2GRAY)
        else:
            gray = image

        # Apply Gaussian blur to reduce noise
        if self.config.blur_size > 0:
            gray = cv2.GaussianBlur(gray, (self.config.blur_size, self.config.blur_size), 0)

        # Apply mask if configured
        if self.config.mask is not None:
            gray = cv2.bitwise_and(gray, gray, mask=self.config.mask)

        return gray

    def _detect_frame_diff(self, image: np.ndarray, camera_id: str) -> Tuple[bool, List[MotionRegion]]:
        """
        Detect motion using simple frame differencing.

        Fast and low memory, but sensitive to lighting changes.
        """
        gray = self._preprocess(image)

        # Get previous frame
        prev_frame = self._prev_frames.get(camera_id)

        # Store current frame for next comparison
        self._prev_frames[camera_id] = gray.copy()

        if prev_frame is None:
            return True, []  # First frame, assume motion

        # Ensure same size
        if prev_frame.shape != gray.shape:
            return True, []  # Size changed, assume motion

        # Compute absolute difference
        diff = cv2.absdiff(prev_frame, gray)

        # Threshold the difference
        _, thresh = cv2.threshold(diff, self.config.pixel_threshold, 255, cv2.THRESH_BINARY)

        # Calculate percentage of changed pixels
        changed_ratio = np.count_nonzero(thresh) / thresh.size

        if changed_ratio < self.config.threshold:
            return False, []

        # Find motion regions using contours
        regions = self._find_motion_regions(thresh, image.shape[1], image.shape[0])

        return True, regions

    def _detect_mog2(self, image: np.ndarray, camera_id: str) -> Tuple[bool, List[MotionRegion]]:
        """
        Detect motion using MOG2 background subtraction.

        More robust to lighting changes, but uses more memory.
        """
        gray = self._preprocess(image)

        # Get or create background subtractor for this camera
        if camera_id not in self._bg_subtractors:
            self._bg_subtractors[camera_id] = cv2.createBackgroundSubtractorMOG2(
                history=self.config.history,
                varThreshold=self.config.var_threshold,
                detectShadows=self.config.detect_shadows
            )

        bg_sub = self._bg_subtractors[camera_id]

        # Apply background subtraction
        fg_mask = bg_sub.apply(gray)

        # Remove shadows (marked as 127 in the mask)
        if self.config.detect_shadows:
            _, fg_mask = cv2.threshold(fg_mask, 200, 255, cv2.THRESH_BINARY)

        # Apply morphological operations to clean up
        kernel = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (3, 3))
        fg_mask = cv2.morphologyEx(fg_mask, cv2.MORPH_OPEN, kernel)
        fg_mask = cv2.morphologyEx(fg_mask, cv2.MORPH_CLOSE, kernel)

        # Calculate percentage of foreground pixels
        fg_ratio = np.count_nonzero(fg_mask) / fg_mask.size

        if fg_ratio < self.config.threshold:
            return False, []

        # Find motion regions
        regions = self._find_motion_regions(fg_mask, image.shape[1], image.shape[0])

        return True, regions

    def _detect_knn(self, image: np.ndarray, camera_id: str) -> Tuple[bool, List[MotionRegion]]:
        """
        Detect motion using KNN background subtraction.

        Similar to MOG2 but can be more accurate in some scenarios.
        """
        gray = self._preprocess(image)

        # Get or create background subtractor for this camera
        if camera_id not in self._bg_subtractors:
            self._bg_subtractors[camera_id] = cv2.createBackgroundSubtractorKNN(
                history=self.config.history,
                dist2Threshold=400.0,
                detectShadows=self.config.detect_shadows
            )

        bg_sub = self._bg_subtractors[camera_id]

        # Apply background subtraction
        fg_mask = bg_sub.apply(gray)

        # Remove shadows
        if self.config.detect_shadows:
            _, fg_mask = cv2.threshold(fg_mask, 200, 255, cv2.THRESH_BINARY)

        # Clean up
        kernel = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (3, 3))
        fg_mask = cv2.morphologyEx(fg_mask, cv2.MORPH_OPEN, kernel)

        # Calculate foreground ratio
        fg_ratio = np.count_nonzero(fg_mask) / fg_mask.size

        if fg_ratio < self.config.threshold:
            return False, []

        regions = self._find_motion_regions(fg_mask, image.shape[1], image.shape[0])

        return True, regions

    def _find_motion_regions(self, mask: np.ndarray,
                            img_width: int, img_height: int) -> List[MotionRegion]:
        """Find contours in the motion mask and return as regions."""
        regions = []

        # Find contours
        contours, _ = cv2.findContours(mask, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)

        min_area_pixels = self.config.min_area * img_width * img_height

        for contour in contours:
            area = cv2.contourArea(contour)
            if area < min_area_pixels:
                continue

            # Get bounding rectangle
            x, y, w, h = cv2.boundingRect(contour)

            # Calculate motion intensity (area / bounding box area)
            intensity = area / (w * h) if w * h > 0 else 0

            # Normalize coordinates
            regions.append(MotionRegion(
                x=x / img_width,
                y=y / img_height,
                width=w / img_width,
                height=h / img_height,
                intensity=intensity
            ))

        return regions

    def reset(self, camera_id: Optional[str] = None):
        """
        Reset motion detector state.

        Args:
            camera_id: Reset only this camera, or all if None
        """
        if camera_id:
            self._prev_frames.pop(camera_id, None)
            self._bg_subtractors.pop(camera_id, None)
            self._last_motion_times.pop(camera_id, None)
        else:
            self._prev_frames.clear()
            self._bg_subtractors.clear()
            self._last_motion_times.clear()

        logger.info(f"Motion detector reset for camera: {camera_id or 'all'}")

    def get_stats(self) -> dict:
        """Get motion detection statistics."""
        return {
            "frames_processed": self.stats.frames_processed,
            "motion_detected": self.stats.motion_detected_count,
            "motion_skipped": self.stats.motion_skipped_count,
            "motion_rate": round(self.stats.motion_rate * 100, 2),
            "avg_latency_ms": round(self.stats.avg_latency_ms, 3),
            "last_motion_time": self.stats.last_motion_time,
            "cameras_tracked": len(self._prev_frames),
            "config": {
                "enabled": self.config.enabled,
                "method": self.config.method.value,
                "threshold": self.config.threshold,
            }
        }

    def set_config(self, **kwargs):
        """Update configuration."""
        for key, value in kwargs.items():
            if hasattr(self.config, key):
                if key == "method" and isinstance(value, str):
                    value = MotionMethod(value)
                setattr(self.config, key, value)

        # Reset state if method changed
        if "method" in kwargs:
            self.reset()

        logger.info(f"Motion config updated: {kwargs}")

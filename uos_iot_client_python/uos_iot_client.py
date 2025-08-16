#!/usr/bin/env python3
"""
UrbanOS IoT MQTT Client
- Publishes geodata messages to a private topic: client/{client_id}/session/{session_id}/
- Sends ONLY: lat, lon, elevation, speed, activity, timestamp (UTC ISO8601)
- Queues locally on disconnect and drains when reconnected (self-healing)
- Safe defaults: QoS 1 (at-least-once), no retain, no PII
"""

import os
import json
import time
import uuid
import queue
import signal
import logging
from datetime import datetime, timezone
from typing import Optional, Dict, Any

import paho.mqtt.client as mqtt

# ──────────────────────────────────────────────────────────────────────────────
# Configuration (env overrides)
# ──────────────────────────────────────────────────────────────────────────────
MQTT_BROKER = os.getenv("MQTT_BROKER", "localhost")
MQTT_PORT = int(os.getenv("MQTT_PORT", "1883"))
MQTT_USERNAME = os.getenv("MQTT_USERNAME")      # optional
MQTT_PASSWORD = os.getenv("MQTT_PASSWORD")      # optional
MQTT_KEEPALIVE = int(os.getenv("MQTT_KEEPALIVE", "60"))
MQTT_QOS = int(os.getenv("MQTT_QOS", "1"))      # QoS 1 pairs with broker persistence

CLIENT_ID = os.getenv("UOS_CLIENT_ID", f"cli-{uuid.uuid4().hex[:12]}")
SESSION_TTL_HOURS = int(os.getenv("UOS_SESSION_TTL_HOURS", "26"))

# Topic template; deployer can switch formats but must keep this shape
TOPIC_TEMPLATE = os.getenv(
    "UOS_TOPIC_TEMPLATE",
    "client/{client_id}/session/{session_id}/"
).rstrip("/") + "/"

# Send interval seconds (device sampling)
PUBLISH_INTERVAL = float(os.getenv("UOS_PUBLISH_INTERVAL", "1.0"))

# Local queue caps (protect memory in prolonged outages)
MAX_QUEUE = int(os.getenv("UOS_MAX_QUEUE", "10000"))

logging.basicConfig(
    level=getattr(logging, os.getenv("LOG_LEVEL", "INFO")),
    format="%(asctime)s | %(levelname)s | %(message)s"
)

# ──────────────────────────────────────────────────────────────────────────────
# Session management
# ──────────────────────────────────────────────────────────────────────────────
def now_utc() -> datetime:
    return datetime.now(timezone.utc)

class Session:
    """
    One 26h logical session per UrbanOS spec.
    Devices DO NOT need to compute start/end; UrbanOS will window by timestamp.
    We still maintain a stable session_id to avoid per-message UUID churn.
    """
    def __init__(self, client_id: str):
        self.client_id = client_id
        # Stable session_id per 26h window; deployer can rotate on their schedule
        self.session_id = os.getenv("UOS_SESSION_ID") or self._new_session_id()
        self.start_time = now_utc()

    def _new_session_id(self) -> str:
        # Using a compact, opaque string (not PII). Could also be numeric.
        return str(uuid.uuid4())

    def maybe_rotate(self):
        # Optional client-side rotation. UrbanOS works even if you never rotate.
        if (now_utc() - self.start_time).total_seconds() >= SESSION_TTL_HOURS * 3600:
            logging.info("🔄 Rotating session_id after TTL")
            self.session_id = self._new_session_id()
            self.start_time = now_utc()

# ──────────────────────────────────────────────────────────────────────────────
# Local queue for offline persistence
# ──────────────────────────────────────────────────────────────────────────────
class OfflineQueue:
    def __init__(self, maxsize: int = MAX_QUEUE):
        self.q = queue.Queue(maxsize=maxsize)

    def put(self, item: Dict[str, Any]):
        try:
            self.q.put_nowait(item)
            return True
        except queue.Full:
            # Drop oldest; keep stream fresh
            try:
                _ = self.q.get_nowait()
                self.q.put_nowait(item)
                logging.warning("🧹 Queue full → dropped oldest to admit new item")
                return True
            except Exception:
                logging.error("❌ Queue full and could not drop oldest")
                return False

    def drain_to(self, fn_publish):
        sent = 0
        while not self.q.empty():
            try:
                fn_publish(self.q.get_nowait())
                sent += 1
            except Exception as e:
                logging.error(f"❌ Failed while draining queue: {e}")
                # Put it back and stop draining to avoid spin
                self.q.task_done()
                break
            self.q.task_done()
        if sent:
            logging.info(f"✅ Drained {sent} queued messages")

# ──────────────────────────────────────────────────────────────────────────────
# MQTT client
# ──────────────────────────────────────────────────────────────────────────────
class UOSPublisher:
    def __init__(self):
        self.session = Session(CLIENT_ID)
        self.offline = OfflineQueue()
        self.client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=f"uos-{CLIENT_ID}")
        self._configure()

    def _configure(self):
        self.client.enable_logger()
        if MQTT_USERNAME:
            self.client.username_pw_set(MQTT_USERNAME, MQTT_PASSWORD)

        # Robust reconnect posture
        self.client.reconnect_delay_set(min_delay=1, max_delay=60)
        self.client.max_inflight_messages_set(100)

        self.client.on_connect = self.on_connect
        self.client.on_disconnect = self.on_disconnect

        self.client.connect(MQTT_BROKER, MQTT_PORT, MQTT_KEEPALIVE)
        self.client.loop_start()

    # Callbacks
    def on_connect(self, client, userdata, flags, rc, properties=None):
        if rc == 0:
            logging.info("✅ Connected to MQTT broker")
            # Drain queued messages once we’re online
            self.offline.drain_to(self._publish_raw)
        else:
            logging.error(f"❌ MQTT connect failed (rc={rc})")

    def on_disconnect(self, client, userdata, rc):
        # rc 0 is clean; others mean unexpected disconnect
        if rc != 0:
            logging.warning(f"⚠ Disconnected from MQTT (rc={rc})")

    # Public API: publish one point
    def publish_point(self, lat: float, lon: float,
                      elevation: Optional[float] = None,
                      speed: Optional[float] = None,
                      activity: Optional[str] = None,
                      timestamp: Optional[datetime] = None):
        """
        Publish one geodata point. timestamp defaults to now (UTC).
        """
        self.session.maybe_rotate()
        ts = timestamp or now_utc()

        payload = {
            "lat": float(lat),
            "lon": float(lon),
            "elevation": None if elevation is None else float(elevation),
            "speed": None if speed is None else float(speed),
            "activity": activity or None,
            "timestamp": ts.isoformat(),
        }

        topic = TOPIC_TEMPLATE.format(
            client_id=self.session.client_id,
            session_id=self.session.session_id
        )

        msg = {"topic": topic, "payload": payload}

        # Try to send immediately; if not connected, queue
        try:
            self._publish_raw(msg)
        except Exception as e:
            logging.warning(f"📥 Offline queueing ({e}); will resend on reconnect")
            self.offline.put(msg)

    # Low-level send; raises if publish failed
    def _publish_raw(self, msg: Dict[str, Any]):
        info = self.client.publish(
            msg["topic"],
            json.dumps(msg["payload"], separators=(",", ":")),
            qos=MQTT_QOS,
            retain=False
        )
        # Block briefly until sent to local socket; non-blocking would also work
        info.wait_for_publish()
        if info.rc != mqtt.MQTT_ERR_SUCCESS:
            raise RuntimeError(f"publish rc={info.rc}")
        logging.debug(f"📤 {msg['topic']} {msg['payload']}")

    def close(self):
        self.client.loop_stop()
        self.client.disconnect()

# ──────────────────────────────────────────────────────────────────────────────
# Example data source (replace with real GNSS/IMU/SDK)
# ──────────────────────────────────────────────────────────────────────────────
def fake_sensor_stream():
    """
    Replace this generator with your device’s real location source.
    Yields dicts with lat/lon/elevation/speed/activity.
    """
    lat, lon = 59.3293, 18.0686  # Stockholm 🙂
    speed = 1.2
    elev = 10.0
    activity = "walking"
    while True:
        yield {
            "lat": lat,
            "lon": lon,
            "elevation": elev,
            "speed": speed,
            "activity": activity
        }
        # Drift a tiny bit
        lat += 0.00005
        lon += 0.00007
        time.sleep(PUBLISH_INTERVAL)

# ──────────────────────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────────────────────
def main():
    pub = UOSPublisher()

    # Graceful shutdown
    stop = False
    def _sig(_a, _b):
        nonlocal stop
        stop = True
    signal.signal(signal.SIGINT, _sig)
    signal.signal(signal.SIGTERM, _sig)

    try:
        for reading in fake_sensor_stream():
            if stop:
                break
            pub.publish_point(
                lat=reading["lat"],
                lon=reading["lon"],
                elevation=reading.get("elevation"),
                speed=reading.get("speed"),
                activity=reading.get("activity"),
                # tim

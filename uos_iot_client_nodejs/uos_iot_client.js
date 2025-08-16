#!/usr/bin/env node
/**
 * UrbanOS IoT MQTT Client (Node.js)
 * - Publishes ONLY: lat, lon, elevation, speed, activity, timestamp
 * - Topic: client/{client_id}/session/{session_id}/
 * - QoS 1, no retain, offline queue, optional 26h session rotation
 */

const mqtt = require("mqtt");
const { randomUUID } = require("crypto");

// ──────────────────────────────────────────────────────────────────────────────
// Config via env
// ──────────────────────────────────────────────────────────────────────────────
const MQTT_BROKER = process.env.MQTT_BROKER || "mqtt://localhost:1883";
const MQTT_USERNAME = process.env.MQTT_USERNAME || undefined;
const MQTT_PASSWORD = process.env.MQTT_PASSWORD || undefined;
const MQTT_QOS = parseInt(process.env.MQTT_QOS || "1", 10);
const MQTT_KEEPALIVE = parseInt(process.env.MQTT_KEEPALIVE || "60", 10);

const CLIENT_ID = process.env.UOS_CLIENT_ID || `cli-${randomUUID().slice(0, 12)}`;
const SESSION_TTL_HOURS = parseInt(process.env.UOS_SESSION_TTL_HOURS || "26", 10);
const TOPIC_TEMPLATE = (process.env.UOS_TOPIC_TEMPLATE || "client/{client_id}/session/{session_id}/").replace(/\/+$/, "") + "/";

const PUBLISH_INTERVAL = parseFloat(process.env.UOS_PUBLISH_INTERVAL || "1.0"); // seconds
const MAX_QUEUE = parseInt(process.env.UOS_MAX_QUEUE || "10000", 10);

// ──────────────────────────────────────────────────────────────────────────────
// Session (stable id per ~26h window)
// ──────────────────────────────────────────────────────────────────────────────
function nowUtc() { return new Date(); }

class Session {
  constructor(clientId) {
    this.clientId = clientId;
    this.sessionId = process.env.UOS_SESSION_ID || randomUUID();
    this.startedAt = nowUtc();
  }
  maybeRotate() {
    if (!SESSION_TTL_HOURS) return;
    const ageMs = nowUtc() - this.startedAt;
    if (ageMs >= SESSION_TTL_HOURS * 3600 * 1000) {
      console.log("🔄 Rotating session_id after TTL");
      this.sessionId = randomUUID();
      this.startedAt = nowUtc();
    }
  }
}

// ──────────────────────────────────────────────────────────────────────────────
// Offline queue
// ──────────────────────────────────────────────────────────────────────────────
class OfflineQueue {
  constructor(limit) {
    this.limit = limit;
    this.buf = [];
  }
  enqueue(msg) {
    if (this.buf.length >= this.limit) {
      // drop oldest
      this.buf.shift();
      console.warn("🧹 Queue full → dropped oldest");
    }
    this.buf.push(msg);
  }
  drain(publishFn) {
    let sent = 0;
    while (this.buf.length) {
      const msg = this.buf.shift();
      publishFn(msg);
      sent++;
    }
    if (sent) console.log(`✅ Drained ${sent} queued messages`);
  }
}

// ──────────────────────────────────────────────────────────────────────────────
// Publisher
// ──────────────────────────────────────────────────────────────────────────────
class UOSPublisher {
  constructor() {
    this.session = new Session(CLIENT_ID);
    this.queue = new OfflineQueue(MAX_QUEUE);

    const options = {
      keepalive: MQTT_KEEPALIVE,
      username: MQTT_USERNAME,
      password: MQTT_PASSWORD,
      reconnectPeriod: 1000,      // ms
      resubscribe: false,
      clean: true,
      clientId: `uos-${CLIENT_ID}-${randomUUID().slice(0, 6)}`,
    };

    this.client = mqtt.connect(MQTT_BROKER, options);
    this.client.on("connect", () => {
      console.log("✅ Connected to MQTT broker");
      this.queue.drain((m) => this._publishRaw(m));
    });
    this.client.on("reconnect", () => console.warn("↻ Reconnecting…"));
    this.client.on("offline", () => console.warn("⚠ MQTT offline"));
    this.client.on("error", (err) => console.error("❌ MQTT error:", err.message));
    this.client.on("close", () => console.warn("⚠ MQTT connection closed"));
  }

  _topic() {
    return TOPIC_TEMPLATE
      .replace("{client_id}", this.session.clientId)
      .replace("{session_id}", this.session.sessionId);
  }

  publishPoint({ lat, lon, elevation = null, speed = null, activity = null, timestamp = null }) {
    this.session.maybeRotate();

    const ts = (timestamp instanceof Date ? timestamp : nowUtc()).toISOString();
    const payload = {
      lat: Number(lat),
      lon: Number(lon),
      elevation: elevation == null ? null : Number(elevation),
      speed: speed == null ? null : Number(speed),
      activity: activity || null,
      timestamp: ts,
    };

    const msg = { topic: this._topic(), payload };
    if (this.client.connected) {
      this._publishRaw(msg);
    } else {
      this.queue.enqueue(msg);
    }
  }

  _publishRaw({ topic, payload }) {
    const data = JSON.stringify(payload);
    this.client.publish(topic, data, { qos: MQTT_QOS, retain: false }, (err) => {
      if (err) {
        console.warn("📥 Offline queueing due to publish error:", err.message);
        this.queue.enqueue({ topic, payload });
      }
    });
  }

  close() {
    try { this.client.end(true); } catch {}
  }
}

// ──────────────────────────────────────────────────────────────────────────────
// Example sensor (replace with your GNSS/IMU feed)
// ──────────────────────────────────────────────────────────────────────────────
async function* fakeSensorStream() {
  let lat = 59.3293, lon = 18.0686, elev = 10.0, speed = 1.2, activity = "walking";
  while (true) {
    yield { lat, lon, elevation: elev, speed, activity };
    lat += 0.00005; lon += 0.00007;
    await new Promise(r => setTimeout(r, PUBLISH_INTERVAL * 1000));
  }
}

// ──────────────────────────────────────────────────────────────────────────────
async function main() {
  const pub = new UOSPublisher();

  const stop = (() => {
    let s = false;
    const handler = () => { s = true; };
    process.on("SIGINT", handler);
    process.on("SIGTERM", handler);
    return () => s;
  })();

  try {
    for await (const reading of fakeSensorStream()) {
      if (stop()) break;
      pub.publishPoint(reading); // replace with real device data
    }
  } finally {
    pub.close();
  }
}

if (require.main === module) main();

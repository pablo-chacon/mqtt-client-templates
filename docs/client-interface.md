# UrbanOS Client Interface (stable contract)

## MQTT → ingest
Topic: `client/{client_id}/session/{session_id}/` (trailing slash)  
QoS: **1**, retain: **false**, rate: up to client.  
Payload (JSON):
{
  "lat": <float>, "lon": <float>,
  "elevation": <float|null>, "speed": <float|null>,
  "activity": <string|null>, "timestamp": "<UTC ISO8601>"
}

Notes:
- TLS 8883 recommended; mutual TLS optional.
- Offline: clients SHOULD queue and drain on reconnect.

## HTTP → get my live route
Base: `http://<uos_api_host>:8181`  
Endpoints:
- `GET /api/view_routes_live/{client_id}` — latest chosen route for that client.
- (Optional) `GET /api/view_routes_live` — derive `client_id` from auth.

Response (example):
{
  "client_id":"usr-12345",
  "stop_id":"direct",
  "origin_lat":59.3015,"origin_lon":17.9901,
  "destination_lat":59.28708,"destination_lon":17.98336,
  "path_geojson": { "type":"LineString", "coordinates":[... ] },
  "segment_type":"fallback",
  "created_at":"2025-08-23T18:54:30Z"
}

Backed by DB view: **view_routes_live** (one row per client, latest chosen). :contentReference[oaicite:0]{index=0}  
API port mapping: **8181:8181** in docker-compose. :contentReference[oaicite:1]{index=1}

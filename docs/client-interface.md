
---

# UrbanOS Client Interface

**Version** 1.0
This document defines the stable interface between end devices and UrbanOS.
Clients publish geodata over MQTT, UrbanOS produces routing outputs, deployers decide how to expose and filter those outputs.

## Scope

* The MQTT publish contract for device data.
* The reference API outputs that return routing results for all clients.
* Guidance for deployers on filtering, shaping, and security.

UrbanOS is sovereign, self healing, modular, and client scoped. Data ownership stays with the deployer at all times.

---

## MQTT publish contract

### Topic

```
client/{client_id}/session/{session_id}/
```

* `client_id` is an opaque identifier, assigned by the deployer platform.
* `session_id` is a deployer chosen identifier, a simple string or integer is fine.
* The trailing slash is intentional.

### Payload

The client publishes one compact JSON object per message. Timestamps are UTC ISO8601.

```json
{
  "lat": 59.3293,
  "lon": 18.0686,
  "elevation": 10.0,
  "speed": 1.2,
  "activity": "walking",
  "timestamp": "2025-08-23T17:00:00Z"
}
```

**Field notes**

| field     | type           | required | description                                      |
| --------- | -------------- | :------: | ------------------------------------------------ |
| lat       | float          |    yes   | latitude in WGS84                                |
| lon       | float          |    yes   | longitude in WGS84                               |
| elevation | float or null  |    no    | meters above sea level                           |
| speed     | float or null  |    no    | meters per second or a consistent unit of choice |
| activity  | string or null |    no    | walking, cycling, driving, or similar labels     |
| timestamp | string         |    yes   | UTC ISO8601, example `2025-08-23T17:00:00Z`      |

**Quality of service**

* QoS is 1.
* Retain is false.
* Clients should queue offline and drain on reconnect.

**Sampling guidance**

* Sampling cadence is up to the deployer, one to ten seconds is typical, faster sampling creates higher throughput.
* UrbanOS handles late arriving data and persists by design.

**Authentication and transport**

* Use per device credentials.
* Restrict publish permissions to `client/{client_id}/#`.
* Prefer TLS on port 8883, mutual TLS is optional.
* For Android, a Termux quick start is available in this repository.

---

## Reference API outputs

UrbanOS exposes read only endpoints that return routing data for all clients. The deployer filters in their own backend and applies policy. The default base is:

```
http://<uos_api_host>:8181
```

Available endpoints:

* `GET /api/view_routes_unified_latest`, returns the latest record per client and per destination across optimized routes and reroutes.
* `GET /api/view_routes_live`, returns one chosen route per client, the most recent decision.
* `GET /api/view_routes_unified`, returns a unified snapshot of optimized routes and reroutes, latest per client and destination.

**Example**

```bash
curl http://<uos_api_host>:8181/api/view_routes_live
```

This returns an array for all clients. The deployer filters by `client_id` or any other attribute in their own backend.

**Thin proxy example**

```python
# Example only, a deployer owned API that filters by client_id
from flask import Flask, request, jsonify
import requests

app = Flask(__name__)
UOS_API = "http://uos-api:8181"

@app.get("/myapp/live_route")
def my_live_route():
    client_id = request.args.get("client_id")
    data = requests.get(f"{UOS_API}/api/view_routes_live", timeout=5).json()
    rows = [r for r in data if not client_id or r["client_id"] == client_id]
    return jsonify(rows)
```

**GeoJSON shaping example**

If your backend connects directly to the UrbanOS database, select from the shipping views and convert the path to GeoJSON.

```sql
SELECT
  client_id,
  stop_id,
  origin_lat, origin_lon,
  destination_lat, destination_lon,
  ST_AsGeoJSON(path)::json AS path_geojson,
  segment_type,
  created_at
FROM view_routes_live;
```

---

## Data shapes in the reference views

These views are created by UrbanOS and are intended for downstream use. Geometry is stored as `LineString` in SRID 4326. Coordinates are WGS84. GeoJSON uses `[lon, lat]`.

### `view_routes_live`, one chosen route per client

| column                             | type                       | note                          |
| ---------------------------------- | -------------------------- | ----------------------------- |
| source                             | text                       | optimized, reroute            |
| client\_id                         | text                       | client identifier             |
| stop\_id                           | text nullable              | direct when null              |
| origin\_lat, origin\_lon           | float                      | origin in WGS84               |
| destination\_lat, destination\_lon | float                      | destination in WGS84          |
| path                               | geometry(LineString, 4326) | route geometry                |
| segment\_type                      | text                       | direct, multimodal, fallback  |
| is\_chosen                         | boolean                    | chosen flag                   |
| created\_at                        | timestamp                  | creation time                 |
| reason                             | text nullable              | reason for a reroute when set |
| previous\_stop\_id                 | text nullable              | previous stop id              |
| previous\_segment\_type            | text nullable              | previous segment type         |
| rn                                 | integer                    | helper field                  |

**GeoJSON select**

```sql
SELECT
  client_id,
  stop_id,
  origin_lat, origin_lon,
  destination_lat, destination_lon,
  ST_AsGeoJSON(path)::json AS path_geojson,
  segment_type,
  created_at
FROM view_routes_live;
```

### `view_routes_unified_latest`, latest per client and destination

| column                             | type                       | note                          |
| ---------------------------------- | -------------------------- | ----------------------------- |
| source                             | text                       | optimized, reroute            |
| client\_id                         | text                       | client identifier             |
| stop\_id                           | text nullable              | direct when null              |
| origin\_lat, origin\_lon           | float                      | origin in WGS84               |
| destination\_lat, destination\_lon | float                      | destination in WGS84          |
| path                               | geometry(LineString, 4326) | route geometry                |
| segment\_type                      | text                       | direct, multimodal, fallback  |
| is\_chosen                         | boolean                    | chosen flag                   |
| created\_at                        | timestamp                  | creation time                 |
| reason                             | text nullable              | reason for a reroute when set |
| previous\_stop\_id                 | text nullable              | previous stop id              |
| previous\_segment\_type            | text nullable              | previous segment type         |
| rn                                 | integer                    | helper field                  |

**GeoJSON select**

```sql
SELECT
  client_id,
  stop_id,
  origin_lat, origin_lon,
  destination_lat, destination_lon,
  ST_AsGeoJSON(path)::json AS path_geojson,
  segment_type,
  created_at
FROM view_routes_unified_latest;
```

### `view_routes_unified`, latest per client and destination, essential surface

| column                             | type                       | note                         |
| ---------------------------------- | -------------------------- | ---------------------------- |
| client\_id                         | text                       | client identifier            |
| stop\_id                           | text nullable              | direct when null             |
| destination\_lat, destination\_lon | float                      | destination in WGS84         |
| path                               | geometry(LineString, 4326) | route geometry               |
| segment\_type                      | text                       | direct, multimodal, fallback |
| created\_at                        | timestamp                  | creation time                |

**GeoJSON select**

```sql
SELECT
  client_id,
  stop_id,
  destination_lat, destination_lon,
  ST_AsGeoJSON(path)::json AS path_geojson,
  segment_type,
  created_at
FROM view_routes_unified;
```

### Latest live position per client

`view_geodata_latest_point` returns one point per client as a Point in SRID 4326, useful for live markers on a map.

```sql
SELECT
  client_id,
  ST_AsGeoJSON(geom)::json AS point_geojson,
  speed,
  activity,
  timestamp
FROM view_geodata_latest_point;
```

---

## Geometry and performance notes

All route geometries are `LineString` in SRID 4326.
GeoJSON uses `[lon, lat]`, which matches Leaflet and Mapbox.
You can limit decimals with `ST_AsGeoJSON(path, 6)` to reduce payload size.
The underlying tables are append only with spatial and pragmatic indexes.
Indexes cover geometry, client lookup, and time based retrieval.

---

## Versioning

This client interface is stable. Any breaking change will bump the version and will be documented. Additions that are backward compatible will not bump the version.

---

## Security

Use per device credentials on the broker.
Scope publish permissions to `client/{client_id}/#`.
Prefer TLS on 8883. Pin a private CA when possible.
Place the UrbanOS API behind your gateway. Enforce authentication and rate limiting. Derive client identity in your backend. Return only what is allowed for that caller.

---

## Termux quick start

A minimal Termux guide is available in this repository. It shows how to install Python and mosquitto clients, set environment variables, and publish a test message.

---

## Disclaimer

This document describes a technical interface. It does not prescribe how deployers must expose data. Deployers own their policies and filters. Use the software at your own risk.

---

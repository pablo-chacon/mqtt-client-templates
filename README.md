
---

# mqtt-client-templates

UrbanOS MQTT client templates in Go, JavaScript, and Python.
Use these templates to publish geodata to UrbanOS, then let your own backend decide what to expose and how to filter.

[**UrbanOS PoC**](https://github.com/pablo-chacon/UrbanOS-POC)

## Why this exists

UrbanOS is sovereign, self healing, modular, and client scoped. It produces high quality routing outputs. The deployer owns how that data is exposed to apps and dashboards, the deployer decides what to publish and how to filter.

## Client interface

**MQTT topic**

```
client/{client_id}/session/{session_id}/
```

**Payload**

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

QoS is 1, retain is false, timestamps are UTC ISO8601, TLS on 8883 is recommended, mutual TLS is optional.

## Reference API outputs

UrbanOS ships API endpoints that return unified routing data for all clients. This is intentional. The deployer filters in their own backend.

Base URL, `http://<uos_api_host>:8181`.

* `GET /api/view_routes_unified_latest`, the latest record per client and per destination across optimized routes and reroutes.
* `GET /api/view_routes_live`, one chosen route per client, the most recent decision.
* `GET /api/view_routes_unified`, a unified snapshot of optimized routes and reroutes, latest per client and destination.

These endpoints reflect database views that UrbanOS maintains. They are stable entry points for downstream systems. You can filter by `client_id` and shape to GeoJSON in your own backend.

## How deployers filter and shape data

UrbanOS does not impose per client filtering at the API layer. Deployer backends apply policy and filtering. This keeps control with the owner of the deployment.

### Option A, thin proxy that filters by client id

Example in Flask. Fetch once, filter before responding.

```python
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

### Option B, query the database view directly and shape to GeoJSON

If your backend connects to the UrbanOS database, select from the shipping views and convert the geometry to GeoJSON for map front ends.

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

### Security and policy

Place the UrbanOS API behind your gateway, enforce authentication, derive client identity in your backend, return only what is allowed for that caller, keep UrbanOS focused on intelligence and data production.

## Termux quick start

Install Termux from F-Droid. In Termux, run:

```bash
pkg update -y && pkg upgrade -y
pkg install -y python openssl mosquitto-clients
python -m pip install --upgrade pip
pip install paho-mqtt
```

Environment and a quick publish test:

```bash
cat > ~/.uos.env <<'EOF'
export MQTT_BROKER=<broker_host_or_ip>
export MQTT_PORT=1883
export UOS_CLIENT_ID=usr-12345
# TLS optional
# export MQTT_TLS=1
# export MQTT_PORT=8883
# export MQTT_CAFILE=/sdcard/Download/ca.crt
EOF
echo '. ~/.uos.env' >> ~/.bashrc
source ~/.uos.env

mosquitto_pub -h $MQTT_BROKER -p $MQTT_PORT \
  -t client/$UOS_CLIENT_ID/session/test/ -q 1 \
  -m '{"lat":59.3293,"lon":18.0686,"timestamp":"2025-08-23T17:00:00Z"}'
```

## Templates

### Python

Folder `uos_iot_client_python`.
Install `paho-mqtt`. Run `uos_iot_client.py`. Set environment as above. TLS can be enabled with `MQTT_TLS=1` and `MQTT_CAFILE`.

### Node.js

Folder `uos_iot_client_nodejs`.

```bash
cd uos_iot_client_nodejs
npm i
node uos_iot_client.js
```

Environment variables mirror the Python template. The code uses the `mqtt` package and the same topic and payload.

### Go

Folder `uos_iot_client_go`.

```bash
cd uos_iot_client_go
go mod tidy
go run .
```

The client uses `eclipse/paho.mqtt.golang`. Environment variables are read at start. TLS can be set by pointing to a CA file.

## DB field notes for reference views

### `view_routes_live`, one chosen route per client, latest decision across optimized routes and reroutes

| column                             | type                       | note                            |
| ---------------------------------- | -------------------------- | ------------------------------- |
| source                             | text                       | optimized, reroute              |
| client\_id                         | text                       | id of the client                |
| stop\_id                           | text nullable              | direct when null                |
| origin\_lat, origin\_lon           | float                      | origin in WGS84                 |
| destination\_lat, destination\_lon | float                      | destination in WGS84            |
| path                               | geometry(LineString, 4326) | route geometry                  |
| segment\_type                      | text                       | direct, multimodal, fallback    |
| is\_chosen                         | boolean                    | chosen flag                     |
| created\_at                        | timestamp                  | creation time                   |
| reason                             | text nullable              | reason for a reroute when set   |
| previous\_stop\_id                 | text nullable              | previous stop id                |
| previous\_segment\_type            | text nullable              | previous type                   |
| rn                                 | integer                    | window row number, helper field |

**Map friendly select**

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

### `view_routes_unified_latest`, latest record per client, stop id, segment type, and destination, unified across optimized routes and reroutes

| column                             | type                       | note                            |
| ---------------------------------- | -------------------------- | ------------------------------- |
| source                             | text                       | optimized, reroute              |
| client\_id                         | text                       | id of the client                |
| stop\_id                           | text nullable              | direct when null                |
| origin\_lat, origin\_lon           | float                      | origin in WGS84                 |
| destination\_lat, destination\_lon | float                      | destination in WGS84            |
| path                               | geometry(LineString, 4326) | route geometry                  |
| segment\_type                      | text                       | direct, multimodal, fallback    |
| is\_chosen                         | boolean                    | chosen flag                     |
| created\_at                        | timestamp                  | creation time                   |
| reason                             | text nullable              | reason for a reroute when set   |
| previous\_stop\_id                 | text nullable              | previous stop id                |
| previous\_segment\_type            | text nullable              | previous type                   |
| rn                                 | integer                    | window row number, helper field |

**Map friendly select**

```sql
SELECT
  client_id,
  stop_id,
  origin_lat, origin_lon,
  destination_lat, destination_lon,
  ST_AsGeoJSON(path)::json AS path_geoJSON,
  segment_type,
  created_at
FROM view_routes_unified_latest;
```

### `view_routes_unified`, latest per client and destination, unified across optimized routes and reroutes

| column                             | type                       | note                         |
| ---------------------------------- | -------------------------- | ---------------------------- |
| client\_id                         | text                       | id of the client             |
| stop\_id                           | text nullable              | direct when null             |
| destination\_lat, destination\_lon | float                      | destination in WGS84         |
| path                               | geometry(LineString, 4326) | route geometry               |
| segment\_type                      | text                       | direct, multimodal, fallback |
| created\_at                        | timestamp                  | creation time                |

**Map friendly select**

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

### Helpful companion view, latest live position per client

`view_geodata_latest_point` returns one point per client with a `geom` Point in SRID 4326. This is useful for placing a live marker on a map.

| column      | type                  | note                |
| ----------- | --------------------- | ------------------- |
| client\_id  | text                  | id of the client    |
| session\_id | integer               | current session id  |
| lat, lon    | float                 | WGS84               |
| geom        | geometry(Point, 4326) | live point geometry |
| speed       | float                 | optional            |
| activity    | text                  | optional            |
| timestamp   | timestamp             | source timestamp    |
| updated\_at | timestamp             | ingest update time  |

**Map friendly select**

```sql
SELECT
  client_id,
  ST_AsGeoJSON(geom)::json AS point_geojson,
  speed,
  activity,
  timestamp
FROM view_geodata_latest_point;
```

### Geometry and projection notes

All route geometries are stored as `GEOMETRY(LineString, 4326)`. Coordinates are WGS84 in longitudinal and latitudinal order. GeoJSON produced by `ST_AsGeoJSON` uses `[lon, lat]`, which is what Leaflet and Mapbox expect. You can limit decimals with `ST_AsGeoJSON(path, 6)` to reduce payload size while keeping map quality.

### Performance notes

The views sit on top of append only tables with spatial and pragmatic indexes. This covers client lookup, destination filtering, and geometry operations. Useful indexes include geometry indexes on `path`, B tree indexes on `client_id`, and compound indexes on `client_id` with `created_at`. The schema favors appends and reads, cleanup and rollups can be done by background jobs.

## Security notes

Use per device credentials on the broker.
Scope publish permissions to `client/{client_id}/#`.
Prefer TLS on 8883. Pin a private CA when possible.
If you expose the UrbanOS API on the internet, add authentication and rate limiting. Derive client identity in your backend rather than trusting input.

## Repository layout

```
.
├── docs/
│   ├── client-interface.md
│   ├── CONTRIBUTING.md
│   ├── termux.md
│   └── uos_api/openapi.yaml
├── uos_iot_client_go/
├── uos_iot_client_nodejs/
└── uos_iot_client_python/
```

## Community TODO

1. Android wrapper that starts the MQTT client and shows a button to fetch the latest route from the deployer backend.
2. Web map that calls the deployer backend and draws `path_geojson`.
3. iOS mini client with background tasks for steady publishing.
4. Broker examples with mutual TLS and per client ACLs.
5. Postman collection that targets the three reference endpoints and shows filtered examples.

## Disclaimer

This repository provides client templates and reference outputs. It does not prescribe how to expose data. Deployers decide what to publish and how to filter. Use it at your own risk.

## Links

UrbanOS PoC, [https://github.com/pablo-chacon/UrbanOS-POC](https://github.com/pablo-chacon/UrbanOS-POC)

Sovereign, Self-Healing AI, [https://github.com/pablo-chacon/Sovereign-Self-Healing-AI](https://github.com/pablo-chacon/Sovereign-Self-Healing-AI)

---



---

# mqtt-client-templates

UrbanOS MQTT client templates in Go, JavaScript, and Python.
Use these templates to publish geodata to UrbanOS, then fetch the current route from `uos_api`.

**UrbanOS repo:** [https://github.com/pablo-chacon/UrbanOS-POC](https://github.com/pablo-chacon/UrbanOS-POC)

## What this repo gives you

1. A stable client contract for MQTT and HTTP.
2. Ready to run templates for Go, Node.js, and Python.
3. A Termux quick start so any Android phone can be a client.
4. Minimal API docs to request the live route.
5. A short TODO list for community apps.

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

QoS is 1, retain is false, timestamps are UTC ISO8601.
TLS on 8883 is recommended, mutual TLS is optional.

**HTTP route fetch**

* `GET /api/view_routes_live/{client_id}` returns the latest chosen route for that client.
* Optional, `GET /api/view_routes_live` can derive `client_id` from auth if your deployment adds JWT.

Response shape, simplified for front ends:

```json
{
  "client_id": "usr-12345",
  "stop_id": "direct",
  "origin_lat": 59.3015,
  "origin_lon": 17.9901,
  "destination_lat": 59.28708,
  "destination_lon": 17.98336,
  "path_geojson": { "type": "LineString", "coordinates": [[18.0,59.30],[17.98,59.29]] },
  "segment_type": "fallback",
  "created_at": "2025-08-23T18:54:30Z"
}
```

An OpenAPI stub lives in `docs/uos_api/openapi.yaml`.
The full UrbanOS API and database flow are in the UrbanOS repo.

## Quick start on Android with Termux

1. Install Termux from F-Droid.
2. In Termux, run:

```bash
pkg update -y && pkg upgrade -y
pkg install -y python openssl mosquitto-clients
python -m pip install --upgrade pip
pip install paho-mqtt
```

3. Create env and load it:

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
```

4. Sanity test publish:

```bash
mosquitto_pub -h $MQTT_BROKER -p $MQTT_PORT \
  -t client/$UOS_CLIENT_ID/session/test/ -q 1 \
  -m '{"lat":59.3293,"lon":18.0686,"timestamp":"2025-08-23T17:00:00Z"}'
```

5. Fetch live route:

```bash
curl http://<uos_api_host>:8181/api/view_routes_live/$UOS_CLIENT_ID
```

A fuller Termux guide is in `docs/termux.md`.

## Templates

### Python

Folder `uos_iot_client_python`.
Install `paho-mqtt`, run `uos_iot_client.py`, set env as in the Termux section.
TLS can be enabled with `MQTT_TLS=1` and `MQTT_CAFILE=/path/to/ca.crt`.

### Node.js

Folder `uos_iot_client_nodejs`.

```bash
cd uos_iot_client_nodejs
npm i
node uos_iot_client.js
```

Env variables mirror the Python template, the code uses the `mqtt` package and the same topic and payload.

### Go

Folder `uos_iot_client_go`.

```bash
cd uos_iot_client_go
go mod tidy
go run .
```

The client uses `eclipse/paho.mqtt.golang`, env variables are read at start, TLS can be set by pointing to a CA file.

## Security notes

Use per device credentials on the broker.
Scope publish permissions to `client/{client_id}/#`.
Prefer TLS on 8883, pin a private CA when possible.
If you expose `GET /api/view_routes_live`, add authentication, then derive `client_id` from the token so users do not pass IDs in URLs.

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

1. Android wrapper that starts the MQTT client and shows a “Get my route” button.
2. Web map that calls the API and draws `path_geojson`.
3. iOS mini client with Background Tasks for steady publishing.
4. Broker examples with mTLS and per client ACLs.
5. Postman collection and a small demo dataset.

## Disclaimer

This software is provided “as is” and “as available”, without warranties of any kind, use it at your own risk, the authors are not liable for any claim or damage.
UrbanOS is a sovereign, self healing AI software, it is designed to be modular and to run on anything from a Raspberry Pi to the cloud, data ownership remains with the deployer at all times.

## Links

UrbanOS PoC, [https://github.com/pablo-chacon/UrbanOS-POC](https://github.com/pablo-chacon/UrbanOS-POC)

Sovereign, Self-Healing AI, [https://github.com/pablo-chacon/Sovereign-Self-Healing-AI](https://github.com/pablo-chacon/Sovereign-Self-Healing-AI)

---


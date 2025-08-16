# Broker
export MQTT_BROKER="mqtt://mqtt-broker:1883"        # Node
# Go uses paho url scheme:
# export MQTT_BROKER="tcp://mqtt-broker:1883"
# Optional auth
# export MQTT_USERNAME=...
# export MQTT_PASSWORD=...

# UrbanOS topic + identity
export UOS_CLIENT_ID="veh-abc123"                   # opaque ID your platform controls
# Optional fixed session (otherwise auto-rotates on TTL)
# export UOS_SESSION_ID="some-stable-uuid"
export UOS_SESSION_TTL_HOURS=26
export UOS_TOPIC_TEMPLATE="client/{client_id}/session/{session_id}/"

# Device sampling / durability
export UOS_PUBLISH_INTERVAL=1.0
export UOS_MAX_QUEUE=10000

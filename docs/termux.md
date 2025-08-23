# Termux quick start
pkg update -y && pkg install -y python openssl mosquitto-clients
pip install --upgrade pip paho-mqtt

# env
cat > ~/.uos.env <<'EOF'
export MQTT_BROKER=<host>; export MQTT_PORT=1883
export UOS_CLIENT_ID=usr-12345
# TLS (optional):
# export MQTT_TLS=1; export MQTT_PORT=8883
# export MQTT_CAFILE=/sdcard/Download/ca.crt
EOF
echo '. ~/.uos.env' >> ~/.bashrc && source ~/.uos.env

# sanity test
mosquitto_pub -h $MQTT_BROKER -p $MQTT_PORT \
  -t client/$UOS_CLIENT_ID/session/test/ -q 1 \
  -m '{"lat":59.3293,"lon":18.0686,"timestamp":"2025-08-23T17:00:00Z"}'

# fetch my route
curl http://<uos_api_host>:8181/api/view_routes_live/$UOS_CLIENT_ID

// UrbanOS IoT MQTT Client (Go)
// - Publishes ONLY: lat, lon, elevation, speed, activity, timestamp
// - Topic: client/{client_id}/session/{session_id}/
// - QoS 1, no retain, offline queue, optional 26h session rotation

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	BrokerURL      string
	Username       string
	Password       string
	QoS            byte
	KeepAlive      int
	ClientID       string
	SessionTTL     time.Duration
	TopicTemplate  string
	PubInterval    time.Duration
	MaxQueue       int
	FixedSessionID string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustAtoi(env string, def int) int {
	if v := os.Getenv(env); v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return def
}

func mustAtof(env string, def float64) float64 {
	if v := os.Getenv(env); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return def
}

func randID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ──────────────────────────────────────────────────────────────────────────────
// Session
// ──────────────────────────────────────────────────────────────────────────────
type Session struct {
	ClientID  string
	SessionID string
	StartedAt time.Time
	TTL       time.Duration
}

func NewSession(clientID string, ttl time.Duration, fixed string) *Session {
	sid := fixed
	if sid == "" {
		sid = fmt.Sprintf("%s-%s", "sess", randID(8))
	}
	return &Session{
		ClientID:  clientID,
		SessionID: sid,
		StartedAt: time.Now().UTC(),
		TTL:       ttl,
	}
}

func (s *Session) MaybeRotate() {
	if s.TTL <= 0 {
		return
	}
	if time.Since(s.StartedAt) >= s.TTL {
		log.Println("🔄 Rotating session_id after TTL")
		s.SessionID = fmt.Sprintf("%s-%s", "sess", randID(8))
		s.StartedAt = time.Now().UTC()
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Offline queue (bounded)
// ──────────────────────────────────────────────────────────────────────────────
type Msg struct {
	Topic   string
	Payload []byte
}

type OfflineQueue struct {
	buf []Msg
	max int
}

func NewQueue(max int) *OfflineQueue { return &OfflineQueue{max: max} }

func (q *OfflineQueue) Enqueue(m Msg) {
	if len(q.buf) >= q.max {
		// Drop oldest
		q.buf = q.buf[1:]
		log.Println("🧹 Queue full → dropped oldest")
	}
	q.buf = append(q.buf, m)
}

func (q *OfflineQueue) Drain(publish func(Msg)) {
	count := 0
	for len(q.buf) > 0 {
		m := q.buf[0]
		q.buf = q.buf[1:]
		publish(m)
		count++
	}
	if count > 0 {
		log.Printf("✅ Drained %d queued messages\n", count)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Publisher
// ──────────────────────────────────────────────────────────────────────────────
type Publisher struct {
	cfg    Config
	sess   *Session
	queue  *OfflineQueue
	client mqtt.Client
}

func NewPublisher(cfg Config) *Publisher {
	sess := NewSession(cfg.ClientID, cfg.SessionTTL, cfg.FixedSessionID)
	queue := NewQueue(cfg.MaxQueue)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.BrokerURL)
	opts.SetClientID("uos-" + cfg.ClientID + "-" + randID(4))
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(1 * time.Second)
	opts.SetKeepAlive(time.Duration(cfg.KeepAlive) * time.Second)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("✅ Connected to MQTT broker")
		queue.Drain(func(m Msg) {
			t := c.Publish(m.Topic, cfg.QoS, false, m.Payload)
			t.Wait()
			if t.Error() != nil {
				log.Println("❌ publish (drain) error:", t.Error())
			}
		})
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Println("⚠ Connection lost:", err)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal("❌ MQTT connect failed:", token.Error())
	}

	return &Publisher{cfg: cfg, sess: sess, queue: queue, client: client}
}

func (p *Publisher) topic() string {
	return strings.TrimRight(
		strings.Replace(
			strings.Replace(p.cfg.TopicTemplate, "{client_id}", p.sess.ClientID, 1),
			"{session_id}", p.sess.SessionID, 1,
		, "/", -1), "/") + "/"
}

type Payload struct {
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Elevation *float64 `json:"elevation"`
	Speed     *float64 `json:"speed"`
	Activity  *string  `json:"activity"`
	Timestamp string   `json:"timestamp"`
}

func fptr(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) {
		return nil
	}
	return v
}
func sptr(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func (pbl *Publisher) PublishPoint(lat, lon float64, elevation *float64, speed *float64, activity *string, ts *time.Time) {
	pbl.sess.MaybeRotate()
	t := time.Now().UTC()
	if ts != nil {
		t = ts.UTC()
	}
	pl := Payload{
		Lat:       lat,
		Lon:       lon,
		Elevation: fptr(elevation),
		Speed:     fptr(speed),
		Activity:  sptr(activity),
		Timestamp: t.Format(time.RFC3339Nano),
	}
	data, _ := json.Marshal(pl)
	msg := Msg{Topic: pbl.topic(), Payload: data}

	if pbl.client.IsConnectionOpen() {
		token := pbl.client.Publish(msg.Topic, pbl.cfg.QoS, false, msg.Payload)
		token.Wait()
		if err := token.Error(); err != nil {
			log.Println("📥 queueing due to publish error:", err)
			pbl.queue.Enqueue(msg)
		}
	} else {
		pbl.queue.Enqueue(msg)
	}
}

func (p *Publisher) Close() {
	p.client.Disconnect(250)
}

// ──────────────────────────────────────────────────────────────────────────────
// Example sensor (replace with real device feed)
// ──────────────────────────────────────────────────────────────────────────────
func fakeSensor(ctx context.Context, interval time.Duration, out chan<- struct {
	lat, lon float64
	elev, spd *float64
	act      *string
}) {
	defer close(out)
	lat := 59.3293
	lon := 18.0686
	elev := 10.0
	spd := 1.2
	act := "walking"

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			out <- struct {
				lat, lon float64
				elev, spd *float64
				act      *string
			}{
				lat: lat, lon: lon, elev: &elev, spd: &spd, act: &act,
			}
			lat += 0.00005
			lon += 0.00007
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
func main() {
	cfg := Config{
		BrokerURL:     getenv("MQTT_BROKER", "tcp://localhost:1883"),
		Username:      getenv("MQTT_USERNAME", ""),
		Password:      getenv("MQTT_PASSWORD", ""),
		QoS:           byte(mustAtoi("MQTT_QOS", 1)),
		KeepAlive:     mustAtoi("MQTT_KEEPALIVE", 60),
		ClientID:      getenv("UOS_CLIENT_ID", "cli-"+randID(6)),
		SessionTTL:    time.Duration(mustAtoi("UOS_SESSION_TTL_HOURS", 26)) * time.Hour,
		TopicTemplate: strings.TrimRight(getenv("UOS_TOPIC_TEMPLATE", "client/{client_id}/session/{session_id}/"), "/") + "/",
		PubInterval:   time.Duration(mustAtof("UOS_PUBLISH_INTERVAL", 1.0) * float64(time.Second)),
		MaxQueue:      mustAtoi("UOS_MAX_QUEUE", 10000),
		FixedSessionID: getenv("UOS_SESSION_ID", ""),
	}

	pub := NewPublisher(cfg)
	defer pub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	// demo data source
	out := make(chan struct {
		lat, lon float64
		elev, spd *float64
		act      *string
	}, 1)
	go fakeSensor(ctx, cfg.PubInterval, out)

	for {
		select {
		case <-ctx.Done():
			return
		case r, ok := <-out:
			if !ok {
				return
			}
			pub.PublishPoint(r.lat, r.lon, r.elev, r.spd, r.act, nil)
		}
	}
}

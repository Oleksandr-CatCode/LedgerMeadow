package changes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	initialListenerBackoff = time.Second
	maximumListenerBackoff = 30 * time.Second
	maximumPayloadBytes    = 1024
)

type Listener struct {
	config *pgx.ConnConfig
	hub    *Hub
	logger *slog.Logger
}

func NewListener(databaseURL string, hub *Hub, logger *slog.Logger) (*Listener, error) {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	return &Listener{config: config, hub: hub, logger: logger}, nil
}

func (l *Listener) Run(ctx context.Context) {
	backoff := initialListenerBackoff
	for ctx.Err() == nil {
		connection, err := pgx.ConnectConfig(ctx, l.config.Copy())
		if err != nil {
			l.hub.DisconnectAll()
			l.logger.Warn("change listener connection failed", "operation", "change_listener", "error_code", "DATABASE_CONNECT_FAILED")
			if !waitForRetry(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		if _, err := connection.Exec(ctx, "LISTEN "+channelName); err != nil {
			closeConnection(connection)
			l.hub.DisconnectAll()
			l.logger.Warn("change listener setup failed", "operation", "change_listener", "error_code", "LISTEN_FAILED")
			if !waitForRetry(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		l.hub.DisconnectAll()
		backoff = initialListenerBackoff
		for ctx.Err() == nil {
			notification, err := connection.WaitForNotification(ctx)
			if err != nil {
				break
			}
			event, err := decodeEvent(notification.Payload)
			if err != nil {
				l.logger.Warn("invalid change notification ignored", "operation", "change_listener", "error_code", "INVALID_CHANGE_EVENT")
				continue
			}
			l.hub.Publish(event)
		}
		closeConnection(connection)
		l.hub.DisconnectAll()
		if ctx.Err() == nil {
			l.logger.Warn("change listener disconnected", "operation", "change_listener", "error_code", "LISTENER_DISCONNECTED")
			if !waitForRetry(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
		}
	}
}

func decodeEvent(payload string) (Event, error) {
	if len(payload) > maximumPayloadBytes {
		return Event{}, errInvalidEvent
	}
	decoder := json.NewDecoder(bytes.NewBufferString(payload))
	decoder.DisallowUnknownFields()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		return Event{}, errInvalidEvent
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Event{}, errInvalidEvent
	}
	if err := validateEvent(event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > maximumListenerBackoff {
		return maximumListenerBackoff
	}
	return next
}

func closeConnection(connection *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = connection.Close(ctx)
}

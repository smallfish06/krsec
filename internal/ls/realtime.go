package ls

import (
	"context"
	"fmt"
	"strings"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/smallfish06/krsec/pkg/broker"
)

const (
	realtimeRegister   = "3"
	realtimeUnregister = "4"
)

// RealtimeMessage is one decoded LS WebSocket message.
type RealtimeMessage struct {
	Header map[string]any `json:"header,omitempty"`
	Body   map[string]any `json:"body,omitempty"`
}

// RealtimeConn is an active LS realtime WebSocket connection.
type RealtimeConn struct {
	conn  *websocket.Conn
	token string
}

// ConnectRealtime opens a WebSocket connection to LS realtime.
func (c *Client) ConnectRealtime(ctx context.Context) (*RealtimeConn, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}
	_, wsURL := c.urls()
	if strings.TrimSpace(wsURL) == "" {
		return nil, fmt.Errorf("%w: LS websocket URL is empty", broker.ErrInvalidCredentials)
	}
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("connect LS websocket: %w", err)
	}
	token, _ := c.getToken()
	return &RealtimeConn{conn: conn, token: token}, nil
}

// Subscribe registers one realtime TR/key pair.
func (r *RealtimeConn) Subscribe(ctx context.Context, trCD, trKey string) error {
	return r.send(ctx, realtimeRegister, trCD, trKey)
}

// SubscribeMany registers multiple realtime TR/key pairs in order.
func (r *RealtimeConn) SubscribeMany(ctx context.Context, subs []RealtimeSubscription) error {
	for _, sub := range subs {
		if err := r.Subscribe(ctx, sub.TRCode, sub.TRKey); err != nil {
			return err
		}
	}
	return nil
}

// Unsubscribe removes one realtime TR/key pair.
func (r *RealtimeConn) Unsubscribe(ctx context.Context, trCD, trKey string) error {
	return r.send(ctx, realtimeUnregister, trCD, trKey)
}

// Read receives and decodes one realtime message.
func (r *RealtimeConn) Read(ctx context.Context) (*RealtimeMessage, error) {
	var msg RealtimeMessage
	if err := wsjson.Read(ctx, r.conn, &msg); err != nil {
		return nil, fmt.Errorf("read LS websocket: %w", err)
	}
	return &msg, nil
}

// Close closes the realtime WebSocket connection.
func (r *RealtimeConn) Close() error {
	return r.conn.Close(websocket.StatusNormalClosure, "")
}

func (r *RealtimeConn) send(ctx context.Context, trType, trCD, trKey string) error {
	trCD = strings.TrimSpace(trCD)
	if trCD == "" {
		return fmt.Errorf("%w: realtime tr_cd is required", broker.ErrUpstreamBadRequest)
	}
	key := trKey
	if strings.TrimSpace(key) == "" {
		key = ""
	}
	payload := map[string]any{
		"header": map[string]any{
			"token":   r.token,
			"tr_type": trType,
		},
		"body": map[string]any{
			"tr_cd":  trCD,
			"tr_key": key,
		},
	}
	if err := wsjson.Write(ctx, r.conn, payload); err != nil {
		return fmt.Errorf("send LS websocket subscription %s/%s: %w", trCD, trKey, err)
	}
	return nil
}

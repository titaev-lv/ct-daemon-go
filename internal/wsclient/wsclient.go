package wsclient

import (
	"context"
	"time"

	"daemon-go/pkg/log"

	"github.com/gorilla/websocket"
)

type WSClient struct {
	url    string
	logger *log.Logger
	conn   *websocket.Conn
}

// New создает новый клиент
func New(url string) (*WSClient, error) {
	return &WSClient{
		url:    url,
		logger: log.New("wsclient"),
	}, nil
}

// Run запускает клиент с автопереподключением
func (c *WSClient) Run(ctx context.Context) error {
	for {
		c.logger.Info("connecting to %s...", c.url)
		conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
		if err != nil {
			c.logger.Error("dial error: %v, retrying in 5s", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}
		c.conn = conn
		c.logger.Info("connected")

		readDone := make(chan struct{})
		go c.readLoop(ctx, readDone)

		select {
		case <-ctx.Done():
			c.conn.Close()
			return nil
		case <-readDone:
			c.conn.Close()
			c.logger.Warn("connection closed, reconnecting...")
			time.Sleep(3 * time.Second)
		}
	}
}

func (c *WSClient) readLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			c.logger.Error("read error: %v", err)
			return
		}
		c.logger.Debug("recv: %s", string(msg))
	}
}

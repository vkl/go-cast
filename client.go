package cast

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/net/context"

	"github.com/vkl/go-cast/controllers"
	"github.com/vkl/go-cast/events"
	"github.com/vkl/go-cast/log"
	castnet "github.com/vkl/go-cast/net"
)

type Client struct {
	name       string
	info       map[string]string
	host       net.IP
	port       int
	conn       *castnet.Connection
	ctx        context.Context
	cancel     context.CancelFunc
	heartbeat  *controllers.HeartbeatController
	connection *controllers.ConnectionController
	receiver   *controllers.ReceiverController
	media      *controllers.MediaController
	url        *controllers.URLController

	Events chan events.Event
}

const DefaultSender = "sender-0"
const DefaultReceiver = "receiver-0"
const TransportSender = "Tr@n$p0rt-0"
const TransportReceiver = "Tr@n$p0rt-0"

func NewClient(host net.IP, port int) *Client {
	return &Client{
		host:   host,
		port:   port,
		ctx:    context.Background(),
		Events: make(chan events.Event, 16),
	}
}

func (c *Client) IP() net.IP {
	return c.host
}

func (c *Client) Port() int {
	return c.port
}

func (c *Client) SetName(name string) {
	c.name = name
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) SetInfo(info map[string]string) {
	c.info = info
}

func (c *Client) Uuid() string {
	return c.info["id"]
}

func (c *Client) Device() string {
	return c.info["md"]
}

func (c *Client) Status() string {
	return c.info["rs"]
}

func (c *Client) String() string {
	return fmt.Sprintf("%s - %s:%d", c.name, c.host, c.port)
}

func (c *Client) IsConnected() bool {
	if c.conn == nil {
		return false
	}
	connState := c.conn.GetTlsConnectionState()
	if connState == nil {
		return false
	}
	return connState.HandshakeComplete
}

func (c *Client) Connect(ctx context.Context) error {
	c.conn = castnet.NewConnection()
	err := c.conn.Connect(ctx, c.host, c.port)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(c.ctx)
	c.cancel = cancel

	// start connection
	c.connection = controllers.NewConnectionController(c.conn, c.Events, DefaultSender, DefaultReceiver)
	if err := c.connection.Start(ctx); err != nil {
		return err
	}

	// start heartbeat
	c.heartbeat = controllers.NewHeartbeatController(c.conn, c.Events, TransportSender, TransportReceiver)
	if err := c.heartbeat.Start(ctx); err != nil {
		return err
	}

	// start receiver
	c.receiver = controllers.NewReceiverController(c.conn, c.Events, DefaultSender, DefaultReceiver)
	if err := c.receiver.Start(ctx); err != nil {
		return err
	}

	c.Events <- events.Connected{}

	return nil
}

func (c *Client) NewChannel(sourceId, destinationId, namespace string) *castnet.Channel {
	return c.conn.NewChannel(sourceId, destinationId, namespace)
}

func (c *Client) Close() error {
	var err error
	c.cancel()
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
	}
	c.media = nil
	return err
}

func (c *Client) Receiver() *controllers.ReceiverController {
	return c.receiver
}

func (c *Client) launchApp(ctx context.Context, appId string) (string, error) {
	// get transport id
	status, err := c.receiver.GetStatus(ctx)
	if err != nil {
		return "", err
	}
	app := status.GetSessionByAppId(appId)
	if app == nil {
		// needs launching
		status, err = c.receiver.LaunchApp(ctx, appId)
		if err != nil {
			return "", err
		}
		app = status.GetSessionByAppId(appId)
	}

	if app == nil {
		return "", errors.New("Failed to get transport")
	}
	return *app.TransportId, nil
}

func (c *Client) launchMediaApp(ctx context.Context) (string, error) {
	return c.launchApp(ctx, AppMedia)
}

func (c *Client) launchURLApp(ctx context.Context) (string, error) {
	return c.launchApp(ctx, AppURL)
}

func (c *Client) IsPlaying(ctx context.Context) bool {
	status, err := c.receiver.GetStatus(ctx)
	if err != nil {
		log.Println(err)
		return false
	}
	app := status.GetSessionByAppId(AppMedia)
	if app == nil {
		return false
	}
	if *app.StatusText == "Ready To Cast" {
		return false
	}
	return true
}

func (c *Client) Media(ctx context.Context) (*controllers.MediaController, error) {
	if c.media == nil {
		transportId, err := c.launchMediaApp(ctx)
		if err != nil {
			return nil, err
		}
		conn := controllers.NewConnectionController(c.conn, c.Events, DefaultSender, transportId)
		if err := conn.Start(ctx); err != nil {
			return nil, err
		}
		c.media = controllers.NewMediaController(c.conn, c.Events, DefaultSender, transportId)
		if err := c.media.Start(ctx); err != nil {
			return nil, err
		}
	}
	return c.media, nil
}

func (c *Client) URL(ctx context.Context) (*controllers.URLController, error) {
	if c.url == nil {
		transportId, err := c.launchURLApp(ctx)
		if err != nil {
			return nil, err
		}
		conn := controllers.NewConnectionController(c.conn, c.Events, DefaultSender, transportId)
		if err := conn.Start(ctx); err != nil {
			return nil, err
		}
		c.url = controllers.NewURLController(c.conn, c.Events, DefaultSender, transportId)
	}
	return c.url, nil
}

package cast

import (
	"fmt"
	"log"
	"net"

	"golang.org/x/net/context"

	"github.com/vkl/go-cast/controllers"
	"github.com/vkl/go-cast/events"
	_ "github.com/vkl/go-cast/logger"
	castnet "github.com/vkl/go-cast/net"
)

type DisplayStatus struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	MediaStatus string  `json:"media_status"`
	MediaData   string  `json:"media_data"`
	Volume      float64 `json:"volume"`
}

type Client struct {
	name          string
	info          map[string]string
	host          net.IP
	port          int
	conn          *castnet.Connection
	ctx           context.Context
	cancel        context.CancelFunc
	heartbeat     *controllers.HeartbeatController
	connection    *controllers.ConnectionController
	receiver      *controllers.ReceiverController
	media         *controllers.MediaController
	youtubemdx    *controllers.YouTubeMdxController
	url           *controllers.URLController
	displayStatus DisplayStatus
	isconnected   bool

	Events chan events.Event
}

const DefaultSender = "sender-0"
const DefaultReceiver = "receiver-0"
const TransportSender = "Tr@n$p0rt-0"
const TransportReceiver = "Tr@n$p0rt-0"

func NewClient(host net.IP, port int) *Client {
	return &Client{
		host:          host,
		port:          port,
		ctx:           context.Background(),
		Events:        make(chan events.Event, 16),
		displayStatus: DisplayStatus{},
		isconnected:   false,
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

func (c *Client) DisplayStatus() DisplayStatus {
	return c.displayStatus
}

func (c *Client) String() string {
	return fmt.Sprintf("%s - %s:%d", c.name, c.host, c.port)
}

func (c *Client) IsConnected() bool {
	return c.isconnected
}

func (c *Client) Connect(ctx context.Context) error {

	log.Println("Connect client " + c.name)

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.conn = castnet.NewConnection()
	err := c.conn.Connect(ctx, c.host, c.port)
	if err != nil {
		return err
	}

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

	c.isconnected = true

	c.Events <- events.Connected{}

	// start listening gorouting
	go c.Listen(ctx)

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
	c.receiver = nil
	c.youtubemdx = nil
	c.url = nil
	c.isconnected = false
	return err
}

func (c *Client) Receiver() *controllers.ReceiverController {
	return c.receiver
}

func (c *Client) Media(ctx context.Context, appId string) (*controllers.MediaController, error) {
	if c.media != nil {
		return c.media, nil
	}

	status, err := c.receiver.GetStatus(ctx)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for _, app := range status.Applications {
		if *app.AppID == appId {
			media := controllers.NewMediaController(
				c.conn,
				c.Events,
				DefaultSender,
				*app.TransportId,
			)
			log.Println("Media", *app.TransportId)
			c.media = media
			goto CONN
		}
	}

	status, err = c.receiver.LaunchApp(ctx, appId)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for _, app := range status.Applications {
		if *app.AppID == appId {
			media := controllers.NewMediaController(
				c.conn,
				c.Events,
				DefaultSender,
				*app.TransportId,
			)
			log.Println("Media", *app.TransportId)
			c.media = media
			goto CONN
		}
	}
CONN:
	if appId == AppYouTubeMusic || appId == AppYouTube {
		c.youtubemdx = controllers.NewAppTubeController(
			c.conn,
			c.Events,
			DefaultSender,
			c.media.DestinationID)
	}
	c.connection = controllers.NewConnectionController(
		c.conn,
		c.Events,
		DefaultSender,
		c.media.DestinationID)
	if err := c.connection.Start(ctx); err != nil {
		return nil, err
	}

	return c.media, nil
}

func (c *Client) YouTubeMdx() *controllers.YouTubeMdxController {
	return c.youtubemdx
}

func (c *Client) Listen(ctx context.Context) {
	c.displayStatus.Name = c.name
	for {
		select {
		case <-ctx.Done():
			log.Println("stop listening")
			return
		default:
			event := <-c.Events
			if value, ok := event.(events.StatusUpdated); ok {
				log.Println("status", value)
				c.displayStatus.Volume = value.Level
			}
			if value, ok := event.(events.MediaStatusUpdated); ok {
				log.Println("media status", value)
				c.displayStatus.MediaStatus = value.PlayerState
				if value.MetaData != nil {
					c.displayStatus.MediaData = *value.MetaData
				}
			}
			if value, ok := event.(events.AppStarted); ok {
				log.Println("app started", value)
				c.displayStatus.Status = value.DisplayName
			}
			if value, ok := event.(events.AppStopped); ok {
				log.Println("app stopped", value)
				c.media = nil
			}
			if value, ok := event.(events.Disconnected); ok {
				log.Println("disconnected", value)
			}
			if value, ok := event.(events.Connected); ok {
				log.Println("connected", value)
			}
			if value, ok := event.(events.ChannelClosed); ok {
				log.Println("channel closed", value)
			}
		}
	}
}

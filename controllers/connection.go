package controllers

import (
	"log"

	"github.com/vkl/go-cast/api"
	"github.com/vkl/go-cast/events"
	"github.com/vkl/go-cast/net"
	"golang.org/x/net/context"
)

type ConnectionController struct {
	channel  *net.Channel
	eventsCh chan events.Event
}

var connect = net.PayloadHeaders{Type: "CONNECT"}
var close = net.PayloadHeaders{Type: "CLOSE"}

func NewConnectionController(conn *net.Connection, eventsCh chan events.Event, sourceId, destinationId string) *ConnectionController {
	controller := &ConnectionController{
		channel:  conn.NewChannel(sourceId, destinationId, "urn:x-cast:com.google.cast.tp.connection"),
		eventsCh: eventsCh,
	}

	controller.channel.OnMessage("CLOSE", controller.onClose)

	return controller
}

func (c *ConnectionController) sendEvent(event events.Event) {
	select {
	case c.eventsCh <- event:
	default:
		log.Printf("Dropped event: %#v", event)
	}
}

func (c *ConnectionController) Start(ctx context.Context) error {
	return c.channel.Send(connect)
}

func (c *ConnectionController) Close() error {
	return c.channel.Send(close)
}

func (c *ConnectionController) onClose(message *api.CastMessage) {
	event := events.ChannelClosed{}
	c.sendEvent(event)
}

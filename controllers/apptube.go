package controllers

import (
	"encoding/json"
	"fmt"
	"log"

	"golang.org/x/net/context"

	"github.com/vkl/go-cast/api"
	"github.com/vkl/go-cast/events"
	_ "github.com/vkl/go-cast/logger"
	"github.com/vkl/go-cast/net"
)

type YouTubeMdxController struct {
	channel          *net.Channel
	eventsCh         chan events.Event
	mdxSessionStatus *MdxSessionStatus
}

type MdxSessionStatus struct {
	net.PayloadHeaders
	Data struct {
		ScreenId string `json:"screenId"`
		DeviceId string `json:"deviceId"`
	} `json:"data"`
}

var screenId = net.PayloadHeaders{Type: "getMdxSessionStatus"}

func NewAppTubeController(conn *net.Connection, eventsCh chan events.Event, sourceId, destinationId string) *YouTubeMdxController {
	controller := &YouTubeMdxController{
		channel:  conn.NewChannel(sourceId, destinationId, "urn:x-cast:com.google.youtube.mdx"),
		eventsCh: eventsCh,
	}

	controller.channel.OnMessage("mdxSessionStatus", controller.onSessionStatus)

	return controller
}

func (y *YouTubeMdxController) RequestMdxSessionStatus(ctx context.Context) error {
	err := y.channel.Send(screenId)
	if err != nil {
		return fmt.Errorf("failed to get mdx session status: %s", err)
	}
	return nil
}

func (y *YouTubeMdxController) onSessionStatus(message *api.CastMessage) {
	response := &MdxSessionStatus{}
	err := json.Unmarshal([]byte(*message.PayloadUtf8), response)
	if err != nil {
		log.Printf("Failed to unmarshal status message:%s - %s", err, *message.PayloadUtf8)
		return
	}
	y.mdxSessionStatus = response
}

func (y *YouTubeMdxController) MdxSesionStatus() *MdxSessionStatus {
	return y.mdxSessionStatus
}

package controllers

import (
	"encoding/json"
	"fmt"

	"golang.org/x/net/context"

	"github.com/vkl/go-cast/api"
	"github.com/vkl/go-cast/events"
	"github.com/vkl/go-cast/log"
	"github.com/vkl/go-cast/net"
)

type StatusResponse struct {
	net.PayloadHeaders
	Status *ReceiverStatus `json:"status,omitempty"`
}

type ReceiverStatus struct {
	net.PayloadHeaders
	Applications []*ApplicationSession `json:"applications"`
	Volume       *Volume               `json:"volume,omitempty"`
}

type LaunchRequest struct {
	net.PayloadHeaders
	AppId string `json:"appId"`
}

func (s *ReceiverStatus) GetSessionByNamespace(namespace string) *ApplicationSession {
	for _, app := range s.Applications {
		for _, ns := range app.Namespaces {
			if ns.Name == namespace {
				return app
			}
		}
	}
	return nil
}

func (s *ReceiverStatus) GetSessionByAppId(appId string) *ApplicationSession {
	for _, app := range s.Applications {
		if *app.AppID == appId {
			return app
		}
	}
	return nil
}

type ApplicationSession struct {
	AppID       *string      `json:"appId,omitempty"`
	DisplayName *string      `json:"displayName,omitempty"`
	Namespaces  []*Namespace `json:"namespaces"`
	SessionID   *string      `json:"sessionId,omitempty"`
	StatusText  *string      `json:"statusText,omitempty"`
	TransportId *string      `json:"transportId,omitempty"`
}

type Namespace struct {
	Name string `json:"name"`
}

type Volume struct {
	Level *float64 `json:"level,omitempty"`
	Muted *bool    `json:"muted,omitempty"`
}

type ReceiverController struct {
	channel  *net.Channel
	eventsCh chan events.Event
	status   *ReceiverStatus
}

var getStatus = net.PayloadHeaders{Type: "GET_STATUS"}
var commandLaunch = net.PayloadHeaders{Type: "LAUNCH"}
var commandStop = net.PayloadHeaders{Type: "STOP"}

func NewReceiverController(conn *net.Connection, eventsCh chan events.Event, sourceId, destinationId string) *ReceiverController {
	controller := &ReceiverController{
		channel:  conn.NewChannel(sourceId, destinationId, "urn:x-cast:com.google.cast.receiver"),
		eventsCh: eventsCh,
	}

	controller.channel.OnMessage("RECEIVER_STATUS", controller.onStatus)

	return controller
}

func (r *ReceiverController) sendEvent(event events.Event) {
	select {
	case r.eventsCh <- event:
	default:
		log.Debugf("Dropped event: %#v", event)
	}
}

func (r *ReceiverController) onStatus(message *api.CastMessage) {
	response := &StatusResponse{}
	err := json.Unmarshal([]byte(*message.PayloadUtf8), response)
	if err != nil {
		log.Errorf("Failed to unmarshal status message:%s - %s", err, *message.PayloadUtf8)
		return
	}

	previous := map[string]*ApplicationSession{}
	if r.status != nil {
		for _, app := range r.status.Applications {
			previous[*app.AppID] = app
		}
	}

	r.status = response.Status
	vol := response.Status.Volume
	displayName := ""
	for _, app := range response.Status.Applications {
		displayName += *app.DisplayName
	}
	r.sendEvent(events.StatusUpdated{
		Level:       *vol.Level,
		Muted:       *vol.Muted,
		DisplayName: displayName,
	})

	for _, app := range response.Status.Applications {
		if _, ok := previous[*app.AppID]; ok {
			// Already running
			delete(previous, *app.AppID)
			continue
		}
		event := events.AppStarted{
			AppID:       *app.AppID,
			DisplayName: *app.DisplayName,
			StatusText:  *app.StatusText,
		}
		r.sendEvent(event)
	}

	// Stopped apps
	for _, app := range previous {
		event := events.AppStopped{
			AppID:       *app.AppID,
			DisplayName: *app.DisplayName,
			StatusText:  *app.StatusText,
		}
		r.sendEvent(event)
	}
}

func (r *ReceiverController) Start(ctx context.Context) error {
	// noop
	return nil
}

func (r *ReceiverController) GetStatus(ctx context.Context) (*ReceiverStatus, error) {
	message, err := r.channel.Request(ctx, &getStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to get receiver status: %s", err)
	}

	response := &StatusResponse{}
	err = json.Unmarshal([]byte(*message.PayloadUtf8), response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal status message: %s - %s", err, *message.PayloadUtf8)
	}

	return response.Status, nil
}

func (r *ReceiverController) SetVolume(ctx context.Context, volume *Volume) (*api.CastMessage, error) {
	return r.channel.Request(ctx, &ReceiverStatus{
		PayloadHeaders: net.PayloadHeaders{Type: "SET_VOLUME"},
		Volume:         volume,
	})
}

func (r *ReceiverController) GetVolume(ctx context.Context) (*Volume, error) {
	status, err := r.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	return status.Volume, err
}

func (r *ReceiverController) LaunchApp(ctx context.Context, appId string) (*ReceiverStatus, error) {
	message, err := r.channel.Request(ctx, &LaunchRequest{
		PayloadHeaders: commandLaunch,
		AppId:          appId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed sending request: %s", err)
	}

	response := &StatusResponse{}
	err = json.Unmarshal([]byte(*message.PayloadUtf8), response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal status message: %s - %s", err, *message.PayloadUtf8)
	}

	return response.Status, nil
}

func (r *ReceiverController) QuitApp(ctx context.Context) (*api.CastMessage, error) {
	return r.channel.Request(ctx, &commandStop)
}

func (r *ReceiverController) IsPlaying(ctx context.Context) bool {
	status, err := r.GetStatus(ctx)
	if err != nil {
		log.Debugln(err)
		return false
	}
	if len(status.Applications) == 0 {
		return false
	}
	for _, app := range status.Applications {
		log.Debugln("status", *app.StatusText)
		if *app.StatusText == "Ready To Cast" {
			return false
		}
	}
	return true
}

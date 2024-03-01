package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"golang.org/x/net/context"

	"github.com/vkl/go-cast/api"
	"github.com/vkl/go-cast/events"
	_ "github.com/vkl/go-cast/logger"
	"github.com/vkl/go-cast/net"
)

type MetadataType byte

const (
	GENERIC MetadataType = iota
	MOVIE
	TV_SHOW
	MUSIC_TRACK
	PHOTO
	AUDIOBOOK_CHAPTER
)

type MediaImage struct {
	Url    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type MediaMetadata struct {
	MetadataType MetadataType `json:"metadataType"`
	Artist       string       `json:"artist"`
	Title        string       `json:"title"`
	PosterUrl    string       `json:"posterUrl"`
	Images       []MediaImage
}

type LoadMediaCommand struct {
	net.PayloadHeaders
	Media       MediaItem   `json:"media"`
	CurrentTime int         `json:"currentTime"`
	Autoplay    bool        `json:"autoplay"`
	CustomData  interface{} `json:"customData"`
}

type MediaItemQueue struct {
	Media       MediaItem `json:"media"`
	Autoplay    bool      `json:"autoplay"`
	StartTime   int       `json:"startTime"`
	PreloadTime int       `json:"preloadTime"`
}

type QueueMediaCommand struct {
	net.PayloadHeaders
	Items          []MediaItemQueue `json:"items"`
	MediaSessionID int              `json:"mediaSessionId"`
	CurrentTime    int              `json:"currentTime"`
	Autoplay       bool             `json:"autoplay"`
	CustomData     interface{}      `json:"customData"`
}

type MediaItem struct {
	ContentId   string        `json:"contentId"`
	StreamType  string        `json:"streamType"`
	ContentType string        `json:"contentType"`
	MetaData    MediaMetadata `json:"metadata"`
}

type MediaStatusMedia struct {
	ContentId   string        `json:"contentId"`
	StreamType  string        `json:"streamType"`
	ContentType string        `json:"contentType"`
	Duration    float64       `json:"duration"`
	MetaData    MediaMetadata `json:"metadata"`
}

const NamespaceMedia = "urn:x-cast:com.google.cast.media"

var getMediaStatus = net.PayloadHeaders{Type: "GET_STATUS"}

var commandMediaPlay = net.PayloadHeaders{Type: "PLAY"}
var commandMediaPause = net.PayloadHeaders{Type: "PAUSE"}
var commandMediaStop = net.PayloadHeaders{Type: "STOP"}
var commandMediaLoad = net.PayloadHeaders{Type: "LOAD"}
var commandMediaQueueInsert = net.PayloadHeaders{Type: "QUEUE_INSERT"}
var commandMediaQueueNext = net.PayloadHeaders{Type: "QUEUE_NEXT"}
var commandMediaQueuePrev = net.PayloadHeaders{Type: "QUEUE_PREV"}

type MediaCommand struct {
	net.PayloadHeaders
	MediaSessionID int `json:"mediaSessionId"`
}

type MediaController struct {
	channel        *net.Channel
	eventsCh       chan events.Event
	DestinationID  string
	MediaSessionID int
}

func NewMediaController(
	conn *net.Connection,
	eventsCh chan events.Event,
	sourceId,
	destinationID string,
) *MediaController {
	controller := &MediaController{
		channel:       conn.NewChannel(sourceId, destinationID, NamespaceMedia),
		eventsCh:      eventsCh,
		DestinationID: destinationID,
	}

	controller.channel.OnMessage("MEDIA_STATUS", controller.onStatus)

	return controller
}

func (c *MediaController) SetDestinationID(id string) {
	c.channel.DestinationId = id
	c.DestinationID = id
}

func (c *MediaController) sendEvent(event events.Event) {
	select {
	case c.eventsCh <- event:
	default:
		log.Printf("Dropped event: %#v", event)
	}
}

func (c *MediaController) onStatus(message *api.CastMessage) {
	response, err := c.parseStatus(message)
	if err != nil {
		log.Printf("Error parsing status: %s", err)
	}

	for _, status := range response.Status {
		event := events.MediaStatusUpdated{
			PlayerState: (*status).PlayerState,
			CurrentTime: (*status).CurrentTime,
		}
		if status.Media != nil {
			event.MetaData = new(string)
			*(event.MetaData) = fmt.Sprintf(
				"%s : %s", status.Media.MetaData.Artist, status.Media.MetaData.Title)
		}
		c.sendEvent(event)
	}
}

func (c *MediaController) parseStatus(message *api.CastMessage) (*MediaStatusResponse, error) {
	response := &MediaStatusResponse{}

	err := json.Unmarshal([]byte(*message.PayloadUtf8), response)

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal status message:%s - %s", err, *message.PayloadUtf8)
	}

	for _, status := range response.Status {
		c.MediaSessionID = status.MediaSessionID
	}

	return response, nil
}

type MediaStatusResponse struct {
	net.PayloadHeaders
	Status []*MediaStatus `json:"status,omitempty"`
}

type MediaStatus struct {
	net.PayloadHeaders
	MediaSessionID         int                    `json:"mediaSessionId"`
	PlaybackRate           float64                `json:"playbackRate"`
	PlayerState            string                 `json:"playerState"`
	CurrentTime            float64                `json:"currentTime"`
	SupportedMediaCommands int                    `json:"supportedMediaCommands"`
	Volume                 *Volume                `json:"volume,omitempty"`
	Media                  *MediaStatusMedia      `json:"media"`
	CustomData             map[string]interface{} `json:"customData"`
	RepeatMode             string                 `json:"repeatMode"`
	IdleReason             string                 `json:"idleReason"`
}

func (c *MediaController) Start(ctx context.Context) error {
	_, err := c.GetStatus(ctx)
	return err
}

func (c *MediaController) GetStatus(ctx context.Context) (*MediaStatusResponse, error) {
	message, err := c.channel.Request(ctx, &getMediaStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to get receiver status: %s", err)
	}

	return c.parseStatus(message)
}

func (c *MediaController) Play(ctx context.Context) (*api.CastMessage, error) {
	message, err := c.channel.Request(ctx, &MediaCommand{commandMediaPlay, c.MediaSessionID})
	if err != nil {
		return nil, fmt.Errorf("failed to send play command: %s", err)
	}
	return message, nil
}

func (c *MediaController) QueueNext(ctx context.Context) (*api.CastMessage, error) {
	message, err := c.channel.Request(ctx, &MediaCommand{commandMediaQueueNext, c.MediaSessionID})
	if err != nil {
		return nil, fmt.Errorf("failed to send queue next command: %s", err)
	}
	return message, nil
}

func (c *MediaController) QueuePrev(ctx context.Context) (*api.CastMessage, error) {
	message, err := c.channel.Request(ctx, &MediaCommand{commandMediaQueuePrev, c.MediaSessionID})
	if err != nil {
		return nil, fmt.Errorf("failed to send queue prev command: %s", err)
	}
	return message, nil
}

func (c *MediaController) Pause(ctx context.Context) (*api.CastMessage, error) {
	message, err := c.channel.Request(ctx, &MediaCommand{commandMediaPause, c.MediaSessionID})
	if err != nil {
		return nil, fmt.Errorf("failed to send pause command: %s", err)
	}
	return message, nil
}

func (c *MediaController) Stop(ctx context.Context) (*api.CastMessage, error) {
	if c.MediaSessionID == 0 {
		// no current session to stop
		return nil, nil
	}
	message, err := c.channel.Request(ctx, &MediaCommand{commandMediaStop, c.MediaSessionID})
	if err != nil {
		return nil, fmt.Errorf("failed to send stop command: %s", err)
	}
	return message, nil
}

func (c *MediaController) LoadMedia(
	ctx context.Context,
	media MediaItem,
	currentTime int,
	autoplay bool,
	customData interface{}) (*api.CastMessage, error) {

	command := &LoadMediaCommand{
		PayloadHeaders: commandMediaLoad,
		Media:          media,
		CurrentTime:    currentTime,
		Autoplay:       autoplay,
		CustomData:     customData,
	}
	message, err := c.channel.Request(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("failed to send load command: %s", err)
	}
	response := &net.PayloadHeaders{}
	err = json.Unmarshal([]byte(*message.PayloadUtf8), response)
	if err != nil {
		return nil, err
	}
	if response.Type == "LOAD_FAILED" {
		return nil, errors.New("load media failed")
	}

	return message, nil
}

func (c *MediaController) QueueInsert(
	ctx context.Context,
	mediaItems []MediaItemQueue,
	currentTime int,
	autoplay bool,
	customData interface{}) (*api.CastMessage, error) {
	if c.MediaSessionID == 0 {
		// no current session to stop
		return nil, nil
	}
	command := &QueueMediaCommand{
		PayloadHeaders: commandMediaQueueInsert,
		Items:          mediaItems,
		MediaSessionID: c.MediaSessionID,
		CurrentTime:    currentTime,
		Autoplay:       autoplay,
		CustomData:     customData,
	}
	message, err := c.channel.Request(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("failed to send queue insert command: %s", err)
	}
	response := &net.PayloadHeaders{}
	err = json.Unmarshal([]byte(*message.PayloadUtf8), response)
	if err != nil {
		return nil, err
	}
	if response.Type == "LOAD_FAILED" {
		return nil, errors.New("queue insert media failed")
	}

	return message, nil
}

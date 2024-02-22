package events

type MediaStatusUpdated struct {
	PlayerState string
	CurrentTime float64
	MetaData    *string
}

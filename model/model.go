package model

type NorthvoltIdentity interface {
	IsNorthvoltIdentity()
	GetID() string
}

var _ NorthvoltIdentity = &EdgeGateway{}

type EdgeGateway struct {
	ID string
}

func (e *EdgeGateway) IsNorthvoltIdentity() {}

func (e *EdgeGateway) GetID() string {
	return e.ID
}

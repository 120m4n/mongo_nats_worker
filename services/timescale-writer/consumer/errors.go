package consumer

import "errors"

var (
	ErrEmptyUniqueID       = errors.New("unique_id vac\u00edo")
	ErrEmptyUserID         = errors.New("user_id vac\u00edo")
	ErrEmptyFleet          = errors.New("fleet vac\u00edo")
	ErrEmptyLocationType   = errors.New("location.type vac\u00edo")
	ErrInvalidCoordinates  = errors.New("location.coordinates debe tener longitud 2")
	ErrInvalidLongitude    = errors.New("longitude fuera de rango [-180, 180]")
	ErrInvalidLatitude     = errors.New("latitude fuera de rango [-90, 90]")
)

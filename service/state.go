package service

import "github.com/calebstewart/go-embermug"

type State struct {
	Connected bool
	State     embermug.State
	Target    embermug.Temperature
	Current   embermug.Temperature
	Battery   embermug.BatteryState
	HasLiquid bool
}

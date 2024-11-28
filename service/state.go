package service

import (
	"fmt"
	"log/slog"

	"github.com/calebstewart/go-embermug"
)

type State struct {
	Connected bool
	State     embermug.State
	Target    embermug.Temperature
	Current   embermug.Temperature
	Battery   embermug.BatteryState
	HasLiquid bool
}

func (s *State) Update(mug *embermug.Mug) {
	s.Connected = true

	if state, err := mug.GetState(); err != nil {
		slog.Error("Could not update liquid state", "Error", err)
	} else {
		s.State = state
	}

	if current, err := mug.GetCurrentTemperature(); err != nil {
		slog.Error("Could not update current temperature", "Error", err)
	} else {
		s.Current = current
	}

	if target, err := mug.GetTargetTemperature(); err != nil {
		slog.Error("Could not update target temperature", "Error", err)
	} else {
		s.Target = target
	}

	if hasLiquid, err := mug.HasLiquid(); err != nil {
		slog.Error("Could not update liquid level", "Error", err)
	} else {
		s.HasLiquid = hasLiquid
	}

	if battery, err := mug.GetBatteryState(); err != nil {
		slog.Error("Could not update battery state", "Error", err)
	} else {
		s.Battery = battery
	}
}

func (s *State) HandleEvent(mug *embermug.Mug, event embermug.Event) (changed bool, err error) {
	// Handle update events
	switch event {
	case embermug.EventRefreshState:
		if state, err := mug.GetState(); err != nil {
			return false, fmt.Errorf("Could not update liquid state: %w", err)
		} else if state == s.State {
			slog.Debug("No change in reported liquid state")
			return false, nil
		} else {
			slog.Debug("Updated Mug State", "State", state)
			s.State = state
		}
	case embermug.EventRefreshTemperature:
		if current, err := mug.GetCurrentTemperature(); err != nil {
			return false, fmt.Errorf("Could not update current temperature: %w", err)
		} else if current == s.Current {
			slog.Debug("No change in reported temperature")
			return false, nil
		} else {
			slog.Debug("Updated Mug Temperature", "TempF", current.Fahrenheit())
			s.Current = current
		}
	case embermug.EventRefreshTarget:
		if target, err := mug.GetTargetTemperature(); err != nil {
			return false, fmt.Errorf("Could not update target temperature: %w", err)
		} else if target == s.Target {
			slog.Debug("No change in reported target temperature")
			return false, nil
		} else {
			slog.Debug("Updated Mug Target Temperature", "TempF", target.Fahrenheit())
			s.Target = target
		}
	case embermug.EventRefreshLevel:
		if hasLiquid, err := mug.HasLiquid(); err != nil {
			return false, fmt.Errorf("Could not update liquid level: %w", err)
		} else if hasLiquid == s.HasLiquid {
			slog.Debug("No change in reported liquid level")
			return false, nil
		} else {
			slog.Debug("Updated Mug Liquid Level", "HasLiquid", hasLiquid)
			s.HasLiquid = hasLiquid
		}
	case embermug.EventRefreshBattery:
		if battery, err := mug.GetBatteryState(); err != nil {
			return false, fmt.Errorf("Could not update battery state: %w", err)
		} else if old := s.Battery; battery.Charging == old.Charging && battery.Charge == old.Charge && battery.Temperature == old.Temperature {
			slog.Debug("No change in reported battery state")
			return false, nil
		} else {
			slog.Debug(
				"Update Battery State",
				"Charging", battery.Charging,
				"Level", battery.Charge,
				"TempF", battery.Temperature.Fahrenheit(),
			)
			s.Battery = battery
		}
	case embermug.EventCharging:
		slog.Debug("Mug Charging")
		s.Battery.Charging = true
	case embermug.EventNotCharging:
		slog.Debug("Mug Not Charging")
		s.Battery.Charging = false
	}

	return true, nil
}

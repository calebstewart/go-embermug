package cmd

import (
	"errors"
)

// ServiceConfig holds the configuration specific to the embermug service
type ServiceConfig struct {
	DeviceAddress       string `toml:"device-address" mapstructure:"device-address"`
	EnableNotifications bool   `toml:"enable-notifications" mapstructure:"enable-notifications"`
}

// PercentageSource defines the value to place in the 'percentage' field of
// the waybar block.
type PercentageSource string

var (
	ErrInvalidPercentageSource = errors.New("invalid percentage source: expected 'battery' or 'level'")
)

const (
	PercentageBattery PercentageSource = "battery" // Set percentage to the battery percentage level
	PercentageLevel   PercentageSource = "level"   // Set percentage to the liquid level (0 - empty, 1 - full)
)

// validate the contents of the percentage source. This is invoked on both unmarshaling
// and marshaling.
func (p PercentageSource) validate() error {
	switch p {
	case PercentageLevel:
	case PercentageBattery:
	default:
		return ErrInvalidPercentageSource
	}
	return nil
}

// UnmarshalText implements [encoding.TextUnmarshaler] for [PercentageSource]
func (p *PercentageSource) UnmarshalText(data []byte) error {
	*p = PercentageSource(data)
	return p.validate()
}

// MarshalText implements [encoding.TextMarshaler] for [PercentageSource]
func (p PercentageSource) MarshalText() ([]byte, error) {
	return []byte(p), p.validate()
}

// WaybarBlockConfig defines the custom block output sent to Waybar. The string values
// in this structure are 'text/template' template strings. The object in the template
// is an instance of [service.State].
type WaybarBlockConfig struct {
	ToolTip    string           `toml:"tooltip" mapstructure:"tooltip"`       // Golang Template String for Tooltip
	Text       string           `toml:"text" mapstructure:"text"`             // Golang Template String for Main Text
	Alt        string           `toml:"alt" mapstructure:"alt"`               // Golang Template String for Alt Text
	Class      string           `toml:"class" mapstructure:"class"`           // Golang Tempalte String for CSS CLass
	Percentage PercentageSource `toml:"percentage" mapstructure:"percentage"` // Either PercentageBattery or PercentageLevel
}

type WaybarConfig struct {
	ByState      map[string]WaybarBlockConfig `toml:"state" mapstructure:"state"`               // Block config for each mug state
	Disconnected *WaybarBlockConfig           `toml:"disconnected" mapstructure:"disconnected"` // Block config when disconnected
	Default      *WaybarBlockConfig           `toml:"default" mapstructure:"default"`           // Default block config
}

type Config struct {
	// LogLevel   slog.Level    `toml:"log-level" mapstructure:"log-level"`
	SocketPath string        `toml:"socket-path" mapstructure:"socket-path"`
	Service    ServiceConfig `toml:"service" mapstructure:"service"`
	Waybar     WaybarConfig  `toml:"waybar" mapstructure:"waybar"`
}

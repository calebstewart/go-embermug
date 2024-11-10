package embermug

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"time"

	"tinygo.org/x/bluetooth"
)

var (
	ServiceUUID                          = uuidMustParse("fc543622-236c-4c94-8fa9-944a3e5353fa")
	MugNameCharacteristicUUID            = uuidMustParse("fc540001-236c-4c94-8fa9-944a3e5353fa")
	CurrentTemperatureCharacteristicUUID = uuidMustParse("fc540002-236c-4c94-8fa9-944a3e5353fa")
	TargetTemperatureCharacteristicUUID  = uuidMustParse("fc540003-236c-4c94-8fa9-944a3e5353fa")
	TemperatureUnitCharacteristicUUID    = uuidMustParse("fc540004-236c-4c94-8fa9-944a3e5353fa")
	LiquidLevelCharacteristicUUID        = uuidMustParse("fc540005-236c-4c94-8fa9-944a3e5353fa")
	DateTimeCharacteristicUUID           = uuidMustParse("fc540006-236c-4c94-8fa9-944a3e5353fa")
	BatteryStateCharacteristicUUID       = uuidMustParse("fc540007-236c-4c94-8fa9-944a3e5353fa")
	LiquidStateCharacteristicUUID        = uuidMustParse("fc540008-236c-4c94-8fa9-944a3e5353fa")
	VersionInfoCharacteristicUUID        = uuidMustParse("fc54000C-236c-4c94-8fa9-944a3e5353fa")
	EventsCharacteristicUUID             = uuidMustParse("fc540012-236c-4c94-8fa9-944a3e5353fa")
	MugColorCharacteristicUUID           = uuidMustParse("fc540014-236c-4c94-8fa9-944a3e5353fa")
)

var (
	ErrNotImplemented            = errors.New("not implemented")
	ErrUnsupportedDevice         = errors.New("the connected device does not advertise the ember service")
	ErrUnsupportedCharacteristic = errors.New("device characteristic is not present")
	ErrMalformedData             = errors.New("device returned malformed or unknown data")
	ErrUnknownTemperatureUnit    = errors.New("unknown or invalid temperature unit")
	ErrNameTooLong               = errors.New("mug name must be 14 bytes or fewer")
)

// Color is  the color representation for the Ember Mug LED.
type Color struct {
	Red   uint8
	Green uint8
	Blue  uint8
	Alpha uint8
}

func (c *Color) Read(ch *bluetooth.DeviceCharacteristic) error {
	var data = make([]byte, 4)
	if _, err := ch.Read(data); err != nil {
		return err
	} else {
		return c.UnmarshalBinary(data)
	}
}

func (c *Color) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("%w: mug color: %v (expected 4 bytes)", ErrMalformedData, data)
	}

	c.Red = data[0]
	c.Blue = data[1]
	c.Green = data[2]
	c.Alpha = data[3]
	return nil
}

func (c Color) MarshalBinary() ([]byte, error) {
	return []byte{c.Red, c.Green, c.Blue, c.Alpha}, nil
}

type TemperatureUnit int

const (
	UnitCelsius    TemperatureUnit = 0
	UnitFahrenheit TemperatureUnit = 1
)

func (u *TemperatureUnit) Read(ch *bluetooth.DeviceCharacteristic) error {
	var data = make([]byte, 1)
	if _, err := ch.Read(data); err != nil {
		return err
	} else {
		return u.UnmarshalBinary(data)
	}
}

func (u *TemperatureUnit) UnmarshalBinary(data []byte) error {
	switch data[0] {
	case byte(UnitCelsius):
		*u = UnitCelsius
	case byte(UnitFahrenheit):
		*u = UnitFahrenheit
	default:
		return fmt.Errorf("%w: %v", ErrUnknownTemperatureUnit, data[0])
	}

	return nil
}

// Temperature is the raw temperature value returned from the mug.
// Internally, it is always represented in Celsius, and is
// multiplied by 100. The value must be divided by 100 to get
// a Celsius value, and then converted to Fahrenheit if necessary.
type Temperature float64

func Celsius(v float64) Temperature {
	return Temperature(v * 100)
}

func Fahrenheit(v float64) Temperature {
	return Temperature((((v - 32) * 5) / 9) * 100)
}

func (t Temperature) Fahrenheit() float64 {
	return 32 + (float64(t)*9.0)/500
}

func (t Temperature) Celsius() float64 {
	return (float64(t) / 100)
}

func (t *Temperature) Read(ch *bluetooth.DeviceCharacteristic) error {
	var data = make([]byte, 2)
	if _, err := ch.Read(data); err != nil {
		return err
	} else {
		return t.UnmarshalBinary(data)
	}
}

func (t *Temperature) UnmarshalBinary(data []byte) error {
	if len(data) != 2 {
		return fmt.Errorf("%w: temperature: %v (expected 2 bytes)", ErrMalformedData, data)
	}

	*t = Temperature(binary.LittleEndian.Uint16(data))
	return nil
}

func (t Temperature) MarshalBinary() ([]byte, error) {
	var result = make([]byte, 2)
	binary.LittleEndian.PutUint16(result, uint16(t))
	return result, nil
}

// BatteryState holds the  decoded battery information
type BatteryState struct {
	Charge      int         // Percent charged (0-100)
	Charging    bool        // Whether we are currently charging
	Temperature Temperature // Battery Temperature
	Voltage     int         // Likely battery voltage, but this is legacy and unused normally
}

func (b *BatteryState) Read(ch *bluetooth.DeviceCharacteristic) error {
	var data = make([]byte, 5)
	if _, err := ch.Read(data); err != nil {
		return err
	} else {
		return b.UnmarshalBinary(data)
	}
}

func (b *BatteryState) UnmarshalBinary(data []byte) error {
	if len(data) != 5 {
		return fmt.Errorf("%w: battery: %v (expected 5 bytes)", ErrMalformedData, data)
	}

	b.Charge = int(data[0])
	b.Charging = data[1] == 1
	b.Voltage = int(data[4])

	if err := b.Temperature.UnmarshalBinary(data[2:4]); err != nil {
		return fmt.Errorf("battery: temperature: %w", err)
	}

	return nil
}

// State represents the current action being taken by the
// mug. In other words, the state of the liquid in the mug.
type State int

const (
	StateEmpty   State = 1
	StateFilling State = 2
	StateUnknown State = 3
	StateCooling State = 4
	StateHeating State = 5
	StateStable  State = 6
)

var (
	stateNameMap = map[State]string{
		StateEmpty:   "empty",
		StateFilling: "filling",
		StateUnknown: "unknown",
		StateCooling: "cooling",
		StateHeating: "heating",
		StateStable:  "stable",
	}
)

func (s State) String() string {
	if v, ok := stateNameMap[s]; ok {
		return v
	} else {
		return "invalid"
	}
}

func (s *State) Read(ch *bluetooth.DeviceCharacteristic) error {
	var data = make([]byte, 1)
	if _, err := ch.Read(data); err != nil {
		return err
	} else {
		return s.UnmarshalBinary(data)
	}
}

func (s *State) UnmarshalBinary(data []byte) error {
	*s = State(data[0])
	return nil
}

// Event holds the possible notification events from the mug
type Event int

const (
	EventRefreshBattery     Event = 1 // Refresh the battery level
	EventCharging           Event = 2 // Mug is charging
	EventNotCharging        Event = 3 // Mug is no longer charging
	EventRefreshTarget      Event = 4 // Refresh target temperature
	EventRefreshTemperature Event = 5 // Refresh current temperature
	EventNotImplemented     Event = 6 // Unimplemented, but documented
	EventRefreshLevel       Event = 7 // Refresh liquid level
	EventRefreshState       Event = 8 // Refresh liquid state
)

var (
	eventNameMap = map[Event]string{
		EventRefreshBattery:     "RefreshBattery",
		EventCharging:           "Charging",
		EventNotCharging:        "NotCharging",
		EventRefreshTarget:      "RefreshTarget",
		EventRefreshTemperature: "RefreshTemperature",
		EventNotImplemented:     "NotImplemented",
		EventRefreshLevel:       "RefreshLevel",
		EventRefreshState:       "RefreshState",
	}
)

func (e Event) String() string {
	if v, ok := eventNameMap[e]; ok {
		return v
	} else {
		return "invalid"
	}
}

func (e *Event) UnmarshalBinary(data []byte) error {
	*e = Event(data[0])
	return nil
}

// VersionInfo holds version numbers for the mug firmware and hardware
type VersionInfo struct {
	Firmware   uint16 // Firmware version
	Hardware   uint16 // Hardware version
	BootLoader uint16 // Bootloader version (optional, defaults to zero)
}

func (v *VersionInfo) Read(ch *bluetooth.DeviceCharacteristic) error {
	var data = make([]byte, 6)
	if _, err := ch.Read(data); err != nil {
		return err
	} else {
		return v.UnmarshalBinary(data)
	}
}

func (v *VersionInfo) UnmarshalBinary(data []byte) error {
	reader := bytes.NewReader(data)

	if err := binary.Read(reader, binary.LittleEndian, &v.Firmware); err != nil {
		return err
	} else if err := binary.Read(reader, binary.LittleEndian, &v.Hardware); err != nil {
		return err
	} else if len(data) <= 4 {
		v.BootLoader = 0
		return nil
	} else if err := binary.Read(reader, binary.LittleEndian, &v.BootLoader); err != nil {
		return err
	} else {
		return nil
	}
}

// Mug represents a connected Ember Mug device
type Mug struct {
	batteryState *bluetooth.DeviceCharacteristic
	currentTemp  *bluetooth.DeviceCharacteristic
	liquidLevel  *bluetooth.DeviceCharacteristic
	liquidState  *bluetooth.DeviceCharacteristic
	mugColor     *bluetooth.DeviceCharacteristic
	mugName      *bluetooth.DeviceCharacteristic
	versionInfo  *bluetooth.DeviceCharacteristic
	events       *bluetooth.DeviceCharacteristic
	targetTemp   *bluetooth.DeviceCharacteristic
	tempUnit     *bluetooth.DeviceCharacteristic
	dateTime     *bluetooth.DeviceCharacteristic

	Device *bluetooth.Device
}

type MugFilter func(device bluetooth.ScanResult) bool

// Scan returns an iterator which will only return devices advertising the
// Ember Mug service UUID.
func Scan(adapter *bluetooth.Adapter) iter.Seq2[bluetooth.ScanResult, error] {
	return func(yield func(r bluetooth.ScanResult, err error) bool) {
		var done = false

		err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			if done || !result.AdvertisementPayload.HasServiceUUID(ServiceUUID) {
				return
			}

			if !yield(result, nil) {
				err := adapter.StopScan()
				if err != nil {
					yield(result, err)
				}

				done = true
			}
		})

		if err != nil {
			yield(bluetooth.ScanResult{}, err)
		}
	}
}

// New creates a new mug controller from a connected bluetooth device.
// The device must implement the [ServiceUUID] service, and expose
// the appropriate characteristics. While all characteristics are
// expected, the only requirement is that the service is exposed.
func New(device *bluetooth.Device) (*Mug, error) {
	m := &Mug{
		Device: device,
	}

	if services, err := device.DiscoverServices([]bluetooth.UUID{
		ServiceUUID,
	}); err != nil {
		return nil, err
	} else if len(services) == 0 {
		return nil, ErrUnsupportedDevice
	} else if characteristics, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{
		BatteryStateCharacteristicUUID,
		CurrentTemperatureCharacteristicUUID,
		LiquidLevelCharacteristicUUID,
		LiquidStateCharacteristicUUID,
		MugColorCharacteristicUUID,
		MugNameCharacteristicUUID,
		VersionInfoCharacteristicUUID,
		EventsCharacteristicUUID,
		TargetTemperatureCharacteristicUUID,
		TemperatureUnitCharacteristicUUID,
		DateTimeCharacteristicUUID,
	}); err != nil {
		return nil, err
	} else {
		for _, ch := range characteristics {
			switch ch.UUID() {
			case BatteryStateCharacteristicUUID:
				m.batteryState = &ch
			case CurrentTemperatureCharacteristicUUID:
				m.currentTemp = &ch
			case LiquidLevelCharacteristicUUID:
				m.liquidLevel = &ch
			case LiquidStateCharacteristicUUID:
				m.liquidState = &ch
			case MugColorCharacteristicUUID:
				m.mugColor = &ch
			case MugNameCharacteristicUUID:
				m.mugName = &ch
			case VersionInfoCharacteristicUUID:
				m.versionInfo = &ch
			case EventsCharacteristicUUID:
				m.events = &ch
			case TargetTemperatureCharacteristicUUID:
				m.targetTemp = &ch
			case TemperatureUnitCharacteristicUUID:
				m.tempUnit = &ch
			case DateTimeCharacteristicUUID:
				m.dateTime = &ch
			}
		}
	}

	return m, nil
}

func (m *Mug) Close() error {
	return m.Device.Disconnect()
}

func (m *Mug) ReadVersionInfo() (v VersionInfo, err error) {
	if m.versionInfo == nil {
		return v, ErrUnsupportedCharacteristic
	}

	err = v.Read(m.versionInfo)
	return v, err
}

func (m *Mug) GetColor() (c Color, err error) {
	if m.mugColor == nil {
		return c, ErrUnsupportedCharacteristic
	}

	err = c.Read(m.mugColor)
	return c, err
}

func (m *Mug) SetColor(c Color) error {
	if m.mugColor == nil {
		return ErrUnsupportedCharacteristic
	}

	if data, err := c.MarshalBinary(); err != nil {
		return err
	} else if _, err := m.mugColor.WriteWithoutResponse(data); err != nil {
		return err
	}

	return nil
}

func (m *Mug) GetTargetTemperature() (t Temperature, err error) {
	if m.targetTemp == nil {
		return t, ErrUnsupportedCharacteristic
	}

	err = t.Read(m.targetTemp)
	return t, err
}

func (m *Mug) SetTargetTemperature(t Temperature) error {
	if m.targetTemp == nil {
		return ErrUnsupportedCharacteristic
	}

	if data, err := t.MarshalBinary(); err != nil {
		return err
	} else if _, err := m.targetTemp.WriteWithoutResponse(data); err != nil {
		return err
	} else {
		return nil
	}
}

func (m *Mug) GetCurrentTemperature() (t Temperature, err error) {
	if m.currentTemp == nil {
		return t, ErrUnsupportedCharacteristic
	}

	err = t.Read(m.currentTemp)
	return t, err
}

func (m *Mug) GetTemperatureUnit() (u TemperatureUnit, err error) {
	if m.tempUnit == nil {
		return u, ErrUnsupportedCharacteristic
	}

	err = u.Read(m.tempUnit)
	return u, err
}

func (m *Mug) GetBatteryState() (b BatteryState, err error) {
	if m.batteryState == nil {
		return b, ErrUnsupportedCharacteristic
	}

	err = b.Read(m.batteryState)
	return b, err
}

func (m *Mug) HasLiquid() (bool, error) {
	if m.liquidLevel == nil {
		return false, ErrUnsupportedCharacteristic
	}

	var data = make([]byte, 1)
	if _, err := m.liquidLevel.Read(data); err != nil {
		return false, err
	} else {
		return data[0] > 0, nil
	}
}

func (m *Mug) GetState() (s State, err error) {
	if m.liquidState == nil {
		return s, ErrUnsupportedCharacteristic
	}

	err = s.Read(m.liquidState)
	return s, err
}

func (m *Mug) GetName() (string, error) {
	if m.mugName == nil {
		return "", ErrUnsupportedCharacteristic
	}

	var data = make([]byte, 14)
	if length, err := m.mugName.Read(data); err != nil {
		return "", err
	} else {
		return string(data[:length]), nil
	}
}

func (m *Mug) SetName(name string) error {
	if len(name) > 14 {
		return ErrNameTooLong
	}

	if m.mugName == nil {
		return ErrUnsupportedCharacteristic
	}

	_, err := m.mugName.WriteWithoutResponse([]byte(name))
	return err
}

func (m *Mug) SetTime(t time.Time) error {
	var (
		timestamp      = uint32(t.Unix())
		_, tzOffsetSec = t.Zone()
		data           = append(
			binary.LittleEndian.AppendUint32([]byte{}, timestamp),
			uint8((time.Second*time.Duration(tzOffsetSec))/time.Hour),
		)
	)

	if m.dateTime == nil {
		return ErrUnsupportedCharacteristic
	}

	_, err := m.dateTime.WriteWithoutResponse(data)
	return err
}

func (m *Mug) StartEventNotifications(handler func(Event)) error {
	return m.events.EnableNotifications(func(data []byte) {
		handler(Event(data[0]))
	})
}

func (m *Mug) StopEventNotifications() error {
	return m.events.EnableNotifications(nil)
}

func (m *Mug) Events(ctx context.Context) (iter.Seq[Event], error) {
	if m.events == nil {
		return nil, ErrUnsupportedCharacteristic
	}

	return func(yield func(Event) bool) {
		// Deregister the callback before leaving
		defer m.events.EnableNotifications(nil)

		// Create a channel for events
		events := make(chan Event)
		defer close(events)

		// Create a way to cancel the context internally
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		m.events.EnableNotifications(func(data []byte) {
			events <- Event(data[0])
		})

		for {
			select {
			case event := <-events:
				if !yield(event) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

func uuidMustParse(v string) bluetooth.UUID {
	if u, err := bluetooth.ParseUUID(v); err != nil {
		panic(fmt.Sprintf("invalid uuid: %s: %v", v, err))
	} else {
		return u
	}
}

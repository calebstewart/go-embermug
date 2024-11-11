package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"syscall"

	"github.com/calebstewart/go-embermug"
	"github.com/google/uuid"
	"tinygo.org/x/bluetooth"
)

// Service encapsulates the centralized interaction with an [embermug.Mug]
// across multiple potential clients. The service maintains a [net.Listener]
// where clients can connect, and receive status updates in JSON format.
// Additionally, clients can send [Message] objects (in JSON format) to the
// service to make changes to mug or manually refresh the state.
type Service struct {
	bluetoothAdapter *bluetooth.Adapter // Adapter used to connect to the device
	deviceAddress    bluetooth.Address  // Address of the target ember mug device
	state            State              // The current state of the mug as known by our service
	clientLock       sync.Locker        // Lock for modifying or interacting with clients
	clients          map[string]*Client // Mapping of unique client IDs to client objects
	mugLock          sync.Locker        // Lock for the mug client
	mug              *embermug.Mug      // Mug client created from a bluetooth device
}

// New returns a new (non-running) service object. The service will manage
// an ember mug device at the given bluetooth address using the given bluetooth
// adapter.
func New(adapter *bluetooth.Adapter, device bluetooth.Address) *Service {
	return &Service{
		bluetoothAdapter: adapter,
		deviceAddress:    device,
		state:            State{},
		clientLock:       &sync.Mutex{},
		clients:          make(map[string]*Client),
		mugLock:          &sync.Mutex{},
		mug:              nil,
	}
}

// Run executes the service main loop. The service will run indefinitely or
// until the context is closed. It will accept clients from the listener,
// and write mug state updates to the listener in JSON format. Data sent
// to the service from the client must be newline-delimeted JSON. Each
// object must be a [Message] object with some command for the service.
func (s *Service) Run(ctx context.Context, socket net.Listener) error {
	defer s.disconnect()

	var group sync.WaitGroup
	defer group.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Ensure the socket is closed if the context is closed
	group.Add(1)
	go func() {
		defer group.Done()
		<-ctx.Done()
		slog.Info("Received shutdown request")
		socket.Close()
	}()

	// Attempt to connect multiple times because the Ember Mug is dumb as hell
	for i := 0; i < 10; i++ {
		if device, err := s.connect(); err == nil {
			s.handleConnectionEvent(*device, true)
			break
		}
	}

	s.bluetoothAdapter.SetConnectHandler(s.handleConnectionEvent)

	for {
		// Accept a client connection
		conn, err := socket.Accept()
		if err != nil {
			return err
		}

		// Handle the client in the background
		group.Add(1)
		go func() {
			defer group.Done()
			s.handleClient(ctx, conn)
		}()
	}
}

// lockMug locks the mug lock and returns the current mug client
func (s *Service) lockMug() *embermug.Mug {
	s.mugLock.Lock()
	return s.mug
}

func (s *Service) handleConnectionEvent(device bluetooth.Device, connected bool) {
	slog.Debug("Received bluetooth connection event", "Addr", device.Address, "Connected", connected, "TargetAddr", s.deviceAddress)

	if device.Address != s.deviceAddress {
		return
	}

	s.mugLock.Lock()
	defer s.mugLock.Unlock()

	if !connected {
		s.mug = nil
		s.state.Connected = false
	} else if mug, err := embermug.New(&device); err != nil {
		slog.Error("Could not create embermug client for connected device", "Error", err)
		return
	} else if err := mug.StartEventNotifications(s.handleEvent); err != nil {
		slog.Error("Could not start event notifications for device", "Error", err)
		mug.Close()
		return
	} else {
		s.mug = mug
		s.state.Connected = true

		if state, err := mug.GetState(); err != nil {
			slog.Error("Could not update liquid state", "Error", err)
		} else {
			s.state.State = state
		}

		if current, err := mug.GetCurrentTemperature(); err != nil {
			slog.Error("Could not update current temperature", "Error", err)
		} else {
			s.state.Current = current
		}

		if target, err := mug.GetTargetTemperature(); err != nil {
			slog.Error("Could not update target temperature", "Error", err)
		} else {
			s.state.Target = target
		}

		if hasLiquid, err := mug.HasLiquid(); err != nil {
			slog.Error("Could not update liquid level", "Error", err)
		} else {
			s.state.HasLiquid = hasLiquid
		}

		if battery, err := mug.GetBatteryState(); err != nil {
			slog.Error("Could not update battery state", "Error", err)
		} else {
			s.state.Battery = battery
		}

		slog.Debug(
			"Connected to mug",
			"State", s.state.State,
			"CurrentTempF", s.state.Current.Fahrenheit(),
			"TargetTempF", s.state.Target.Fahrenheit(),
			"HasLiquid", s.state.HasLiquid,
			"BatteryLevel", s.state.Battery.Charge,
			"Charging", s.state.Battery.Charging,
		)
	}

	// Send updated state to all clients
	s.dispatchState(s.state)
}

// connect connects to the target mug, saves the client to the
// service object, and returns the client. This method takes
// the mug lock. The caller is responsible for releasing the
// lock.
func (s *Service) connect() (device *bluetooth.Device, lastErr error) {
	for try := 0; try < 10; try++ {
		if d, err := s.bluetoothAdapter.Connect(s.deviceAddress, bluetooth.ConnectionParams{}); err != nil {
			slog.Debug("Failed to connect to device", "Error", err, "Try", try)
			lastErr = err
		} else {
			return &d, nil
		}
	}

	return nil, lastErr
}

// disconnect disables event notifications, and then disconnects from the
// device. You should hold the mug lock before invoking this method.
func (s *Service) disconnect() {
	s.mugLock.Lock()
	defer s.mugLock.Unlock()
	s.disconnectLocked()
}

func (s *Service) disconnectLocked() {
	if s.mug != nil {
		s.mug.StopEventNotifications()
		s.mug.Close()
	}
}

// handleEvent is invoked when the state of the ember mug changes
// in some way. This is a callback for the event characteristic
// in the mug itself, and is invoked asynchronously by the
// [bluetooth.Adapter] when we are connected to the mug.
func (s *Service) handleEvent(event embermug.Event) {
	s.mugLock.Lock()
	defer s.mugLock.Unlock()

	// Get a reference to the connected mug
	var mug = s.mug
	if mug == nil {
		slog.Debug("Mug disconnected before handling event", "Event", event)
		return
	}

	slog.Debug("Received Mug Event", "Event", event)

	var changed = true

	// Handle update events
	switch event {
	case embermug.EventRefreshState:
		if state, err := mug.GetState(); err != nil {
			slog.Error("Could not update liquid state", "Error", err)
			changed = true
		} else if state == s.state.State {
			slog.Debug("No change in reported liquid state")
			changed = false
		} else {
			slog.Debug("Updated Mug State", "State", state)
			s.state.State = state
		}
	case embermug.EventRefreshTemperature:
		if current, err := mug.GetCurrentTemperature(); err != nil {
			slog.Error("Could not update current temperature", "Error", err)
		} else if current == s.state.Current {
			slog.Debug("No change in reported temperature")
			changed = false
		} else {
			slog.Debug("Updated Mug Temperature", "TempF", current.Fahrenheit())
			s.state.Current = current
		}
	case embermug.EventRefreshTarget:
		if target, err := mug.GetTargetTemperature(); err != nil {
			slog.Error("Could not update target temperature", "Error", err)
		} else if target == s.state.Target {
			slog.Debug("No change in reported target temperature")
			changed = false
		} else {
			slog.Debug("Updated Mug Target Temperature", "TempF", target.Fahrenheit())
			s.state.Target = target
		}
	case embermug.EventRefreshLevel:
		if hasLiquid, err := mug.HasLiquid(); err != nil {
			slog.Error("Could not update liquid level", "Error", err)
		} else if hasLiquid == s.state.HasLiquid {
			slog.Debug("No change in reported liquid level")
			changed = false
		} else {
			slog.Debug("Updated Mug Liquid Level", "HasLiquid", hasLiquid)
			s.state.HasLiquid = hasLiquid
		}
	case embermug.EventRefreshBattery:
		if battery, err := mug.GetBatteryState(); err != nil {
			slog.Error("Could not update battery state", "Error", err)
		} else if old := s.state.Battery; battery.Charging == old.Charging && battery.Charge == old.Charge && battery.Temperature == old.Temperature {
			slog.Debug("No change in reported battery state")
			changed = false
		} else {
			slog.Debug(
				"Update Battery State",
				"Charging", battery.Charging,
				"Level", battery.Charge,
				"TempF", battery.Temperature.Fahrenheit(),
			)
			s.state.Battery = battery
		}
	case embermug.EventCharging:
		slog.Debug("Mug Charging")
		s.state.Battery.Charging = true
	case embermug.EventNotCharging:
		slog.Debug("Mug Not Charging")
		s.state.Battery.Charging = false
	}

	if changed {
		s.dispatchState(s.state)
	}
}

// dispatchState sends the given state object to all registered clients.
// This method is also responsible for cleaning up clients which have
// been canceled.
func (s *Service) dispatchState(state State) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	for key, client := range s.clients {
		select {
		case <-client.Context.Done():
			close(client.Channel)
			delete(s.clients, key)
		case client.Channel <- state:
		}
	}
}

// RegisterClient creates a new state channel, and registers it with the
// service. The returned client object can be used to receive state
// objects whenever the target ember mug changes state. When the client
// is no longer needed, the [Client.Cancel] function can be called to
// deregister the client. [Client.Context] will be a child of the
// given context, and will be closed either when the parent closes
// or when the client is canceled.
func (s *Service) RegisterClient(ctx context.Context) *Client {
	ctx, cancel := context.WithCancel(ctx)

	var (
		key    = uuid.New().String()
		client = Client{
			Channel: make(chan State),
			Context: ctx,
			Cancel:  cancel,
			ID:      key,
		}
	)

	// Register the client
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	s.clients[key] = &client

	return &client
}

// handleClient is invoked for each client connection. This method is
// expected to run in it's own goroutine, and handles both the read
// and write ends of the client connection itself. It will register
// itself as a client using [registerClient], and then process
// state changes, and client messages appropriately.
func (s *Service) handleClient(ctx context.Context, conn net.Conn) {
	var (
		group       = sync.WaitGroup{}
		client      = s.RegisterClient(ctx)
		messageChan = make(chan Message)
		encoder     = json.NewEncoder(conn)
		logger      = slog.With(slog.String("ClientID", client.ID))
	)

	logger.Debug("Client Connected")

	// Execute the input handler
	group.Add(1)
	go func() {
		defer group.Done()
		s.parseAndDeliverClientMessages(client, conn, messageChan)
	}()

	// Wait for the background tasks to finish
	defer group.Wait()

	// This is a weird way to drain a channel. The problem is that
	// we don't want to hold up the event routine since we stopped
	// processing events, but the only way for the client to be
	// fully removed is for the events to be processed... So, this
	// is an unmanaged routine which will exit once the channel is
	// closed. The client is already cancelled, so this is just until
	// the next mug even comes through.
	defer func() {
		go func() {
			for range client.Channel {
			}
		}()
	}()

	// Deregister the client, which stops state updates
	defer client.Cancel()

	// Close the client connection
	defer conn.Close()

	defer logger.Debug("Client disconnecting")

	if err := s.sendStateToClient(encoder, s.state); errors.Is(err, syscall.EPIPE) {
		return
	} else if err != nil {
		logger.Error("Failed to write initial state to client", "Error", err)
		return
	}

	for {
		select {
		case <-client.Context.Done():
			logger.Debug("Client received shutdown request")
			return
		case msg, ok := <-messageChan:
			if !ok {
				logger.Debug("Disconnecting client due to invalid messages")
				return
			}

			if msg.Reconnect {
				// Any received message means "connect to the mug"
				logger.Debug("Client received mug connection request")
				if _, err := s.connect(); errors.Is(err, syscall.EPIPE) {
					return
				} else if err != nil {
					logger.Error("Could not connect to device", "Error", err)
					return
				}
			}
		case state := <-client.Channel:
			if err := s.sendStateToClient(encoder, state); errors.Is(err, syscall.EPIPE) {
				return
			} else if err != nil {
				logger.Error("Could not write state to client", "Error", err)
				return
			}
		}
	}
}

// sendStateToClient serializes the given state as a JSON object, and writes it to the
// client encoder.
func (s *Service) sendStateToClient(encoder *json.Encoder, state State) error {
	return encoder.Encode(state)
}

// parseAndDeliverClientMessages reads messages from the given client connection, parses them as
// [Message] objects, and then delivers them to [messageChan]. The function will continue until
// an EOF or read error is encountered. This could be due to the client being closed or due to
// an invalid message being sent by the client. In either case, the function will return. This
// function is normally only executed in a background routine from [Service.handleClient].
func (s *Service) parseAndDeliverClientMessages(client *Client, conn io.Reader, messageChan chan Message) {
	var (
		decoder *json.Decoder = json.NewDecoder(conn)
		message Message
	)

	for decoder.More() {
		if err := decoder.Decode(&message); errors.Is(err, syscall.EPIPE) {
			return
		} else if err != nil {
			slog.Error("Failed to decode client message", "Error", err, "ClientID", client.ID)
			client.Cancel()
			return
		} else {
			slog.Debug("Received message from client", "ClientID", client.ID)
			messageChan <- message
		}
	}
}

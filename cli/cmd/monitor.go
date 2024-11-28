package cmd

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"

	"github.com/calebstewart/go-embermug"
	"github.com/calebstewart/go-embermug/service"
	"github.com/spf13/cobra"
	"tinygo.org/x/bluetooth"
)

var monitorCommand = cobra.Command{
	Use:   "monitor device-address",
	Short: "Monitor an embermug device",
	Long: `Monitor an Ember Mug device

This command is a greedy version of the service. Instead of providing a socket
where multiple clients can connect and listen to asynchronous events, this
command will simply connect to the mug, and stream events back to standard
output until the device is disconnected.
`,
	Args: cobra.ExactArgs(1),
	Run:  commandExitWrapper(monitor),
}

func init() {
	rootCmd.AddCommand(&monitorCommand)

	// flags := monitorCommand.Flags()
}

func monitor(cmd *cobra.Command, args []string) error {
	var ctx, cancel = signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
	defer cancel()

	// Grab the default adapter
	var (
		adapter *bluetooth.Adapter = bluetooth.DefaultAdapter
		state   service.State
		encoder *json.Encoder = json.NewEncoder(os.Stdout)
	)

	// Parse the device address
	addr, err := ParseAddress(args[0])
	if err != nil {
		slog.Error("Invalid device address", "Address", args[0], "Error", err)
	}

	// Enable the bluetooth adapter
	slog.Info("Enabling Default Bluetooth Adapter")
	if err := adapter.Enable(); err != nil {
		slog.Error("Could not enable bluetooth adapter", "Error", err)
		return err
	}

	// Attempt to the connect to the device
	slog.Info("Connecting to Ember Mug device", "MaxAttempts", 10)
	var device bluetooth.Device
	for i := 0; i < 10; i++ {
		device, err = adapter.Connect(addr, bluetooth.ConnectionParams{})
		if err != nil {
			slog.Warn(
				"Connection attempt failed",
				slog.Int("Attempt", i+1),
				slog.Int("MaxAttempt", 10),
				slog.String("Error", err.Error()),
			)
		} else {
			break
		}
	}

	// All connection attempts failed
	if err != nil {
		slog.Error("Connection to device failed", slog.String("Address", args[0]))
		return err
	}

	// Create a client for the mug
	mug, err := embermug.New(&device)
	if err != nil {
		slog.Error("Failed to initialize client", "Error", err)
		return err
	}

	// Perform an initial query of device state
	slog.Info("Querying initial mug state")
	state.Update(mug)
	if err := encoder.Encode(&state); err != nil {
		slog.Error("Failed to write mug state", "Error", err)
		return err
	}

	// Handle event notifications and print state when changed
	slog.Info("Registering mug event handler")
	mug.StartEventNotifications(func(event embermug.Event) {
		if changed, err := state.HandleEvent(mug, event); err != nil {
			slog.Error("Failed to handle event", "Event", event.String(), "Error", err)
		} else if changed {
			if err := encoder.Encode(&state); err != nil {
				slog.Error("Failed to write mug state", "Error", err)
			}
		}
	})

	// Wait for the context to close
	<-ctx.Done()

	return nil
}

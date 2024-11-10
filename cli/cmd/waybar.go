package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/calebstewart/go-embermug"
	"github.com/calebstewart/go-embermug/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var waybarCommand = cobra.Command{
	Use:   "waybar",
	Short: "Ember Mug Waybar Custom Block Client",
	Long: `Ember Mug Waybar Custom Block Client

This client will connect to the unix socket at the given path, and
write a waybar custom block in JSON format to stdout with ember
mug state whenever it changes. Sending SIGUSR1 will cause the
client to request a reconnect from the embermug service.

The socket must be a socket opened by the embermug monitor service
exposed by this same binary. If unspecified, the socket path is
assumed to be '/run/embermug.sock' which is the default SystemD
Socket Activation path for the ember mug service.
`,
	Args: cobra.ExactArgs(0),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Add command flags here
		return viper.BindPFlags(cmd.Flags())
	},
	Run: commandExitWrapper(waybarEntrypoint),
}

func waybarEntrypoint(cmd *cobra.Command, args []string) error {
	var (
		socketPath    = viper.GetString("socket")
		stateChannel  = make(chan service.State)
		signalChannel = make(chan os.Signal, 4)
		ctx, cancel   = signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
	)
	defer cancel()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		slog.Error("Could not connect to socket", "Path", socketPath, "Error", err)
		return err
	}
	defer conn.Close()

	// Create an encoder to send messages to the server
	encoder := json.NewEncoder(conn)

	// Read incoming state changes in the background
	go handleIncomingStates(ctx, conn, stateChannel, cancel)

	// Notify the channel when we get SIGUSR1 or SIGUSR2. This is for reconnect requests.
	signal.Notify(signalChannel, syscall.SIGUSR1, syscall.SIGUSR2)

mainLoop:
	for {
		select {
		case <-ctx.Done():
			slog.Info("Client shutdown requested")
			break mainLoop
		case <-signalChannel:
			slog.Debug("Sending reconnect request to server")
			if err := encoder.Encode(service.Message{
				Reconnect: true,
			}); errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
				slog.Info("Server disconnected")
				break mainLoop
			} else if err != nil {
				slog.Error("Could not write message to server", "Error", err)
				break mainLoop
			}
		case state := <-stateChannel:
			slog.Debug("Received updated state from server")
			if err := writeWaybarBlock(state); err != nil {
				slog.Error("Could not write waybar block", "Error", err)
			}
		}
	}

	return nil
}

func writeWaybarBlock(state service.State) error {
	var (
		encoder  *json.Encoder = json.NewEncoder(os.Stdout)
		charging string        = "charging"
		text     string        = ""
	)

	if !state.Connected {
		return encoder.Encode(map[string]interface{}{
			"text": "Disconnected",
			"tooltip": strings.Join([]string{
				"Battery:      UNK",
				"Target Temp:  UNK",
				"Current Temp: UNK",
			}, "\n"),
		})
	} else {
		if state.State == embermug.StateHeating || state.State == embermug.StateCooling {
			text = fmt.Sprintf("%v (%vF/%vF)", state.State.String(), int(state.Current.Fahrenheit()), int(state.Target.Fahrenheit()))
		} else if state.State == embermug.StateStable {
			text = fmt.Sprintf("%v (%vF)", state.State.String(), int(state.Current.Fahrenheit()))
		} else {
			text = state.State.String()
		}

		if !state.Battery.Charging {
			charging = "discharging"
		}

		return encoder.Encode(map[string]interface{}{
			"text": text,
			"tooltip": strings.Join([]string{
				fmt.Sprintf("Battery:      %v%% (%s)", state.Battery.Charge, charging),
				fmt.Sprintf("Target Temp:  %.2fF", state.Target.Fahrenheit()),
				fmt.Sprintf("Current Temp: %.2fF", state.Current.Fahrenheit()),
			}, "\n"),
		})
	}
}

func handleIncomingStates(ctx context.Context, conn net.Conn, stateChannel chan service.State, cancel func()) {
	defer cancel()
	defer close(stateChannel)

	for decoder := json.NewDecoder(conn); decoder.More(); {
		var state service.State

		if err := decoder.Decode(&state); errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
			return
		} else if err != nil {
			slog.Error("Could not decode state update", "Error", err)
			return
		} else {
			select {
			case <-ctx.Done():
				return
			case stateChannel <- state:
			}
		}
	}
}

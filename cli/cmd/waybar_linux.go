package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"

	"github.com/calebstewart/go-embermug"
	"github.com/calebstewart/go-embermug/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type WaybarBlock struct {
	Tooltip    *template.Template
	Text       *template.Template
	Alt        *template.Template
	Class      *template.Template
	Percentage PercentageSource
}

func NewWaybarBlock(cfg *WaybarBlockConfig) (*WaybarBlock, error) {
	var (
		block = WaybarBlock{
			Percentage: cfg.Percentage,
		}
		funcs = template.FuncMap{
			"toFahrenheit": func(t embermug.Temperature) int {
				return int(t.Fahrenheit())
			},
			"toCelsius": func(t embermug.Temperature) int {
				return int(t.Celsius())
			},
		}
	)

	if tooltip, err := template.New("tooltip").Funcs(funcs).Parse(cfg.ToolTip); err != nil {
		return nil, fmt.Errorf("tooltip: %w", err)
	} else {
		block.Tooltip = tooltip
	}

	if text, err := template.New("text").Funcs(funcs).Parse(cfg.Text); err != nil {
		return nil, fmt.Errorf("text: %w", err)
	} else {
		block.Text = text
	}

	if alt, err := template.New("alt").Funcs(funcs).Parse(cfg.Alt); err != nil {
		return nil, fmt.Errorf("alt: %w", err)
	} else {
		block.Alt = alt
	}

	if class, err := template.New("class").Funcs(funcs).Parse(cfg.Class); err != nil {
		return nil, fmt.Errorf("class: %w", err)
	} else {
		block.Class = class
	}

	return &block, nil
}

func (b *WaybarBlock) Render(state service.State) (map[string]interface{}, error) {
	var (
		result = make(map[string]interface{})
		buffer = &strings.Builder{}
	)

	buffer.Reset()
	if err := b.Tooltip.Execute(buffer, state); err != nil {
		return nil, fmt.Errorf("tooltip: %w", err)
	} else if value := buffer.String(); value != "" {
		result["tooltip"] = value
	}

	buffer.Reset()
	if err := b.Text.Execute(buffer, state); err != nil {
		return nil, fmt.Errorf("text: %w", err)
	} else if value := buffer.String(); value != "" {
		result["text"] = value
	}

	buffer.Reset()
	if err := b.Alt.Execute(buffer, state); err != nil {
		return nil, fmt.Errorf("alt: %w", err)
	} else if value := buffer.String(); value != "" {
		result["alt"] = value
	}

	buffer.Reset()
	if err := b.Class.Execute(buffer, state); err != nil {
		return nil, fmt.Errorf("class: %w", err)
	} else if value := buffer.String(); value != "" {
		result["class"] = value
	}

	switch b.Percentage {
	case PercentageBattery:
		result["percentage"] = state.Battery.Charge
	case PercentageLevel:
		if state.HasLiquid {
			result["percentage"] = 1
		} else {
			result["percentage"] = 0
		}
	}

	return result, nil
}

type WaybarEncoder struct {
	BlockByState      map[embermug.State]*WaybarBlock
	DefaultBlock      *WaybarBlock
	DisconnectedBlock *WaybarBlock
	Encoder           *json.Encoder
}

func NewWaybarEncoder(cfg *WaybarConfig, stream io.Writer) (*WaybarEncoder, error) {
	var (
		encoder = &WaybarEncoder{
			Encoder:      json.NewEncoder(stream),
			BlockByState: make(map[embermug.State]*WaybarBlock),
		}
	)

	if cfg.Disconnected == nil {
		if block, err := NewWaybarBlock(&WaybarBlockConfig{
			Text: "Disconnected",
		}); err != nil {
			return nil, fmt.Errorf("could not compile default disconnected block: %w", err)
		} else {
			encoder.DisconnectedBlock = block
		}
	} else if block, err := NewWaybarBlock(cfg.Disconnected); err != nil {
		return nil, fmt.Errorf("could not compile disconnected block: %w", err)
	} else {
		encoder.DisconnectedBlock = block
	}

	if cfg.Default == nil {
		if block, err := NewWaybarBlock(&WaybarBlockConfig{
			Text: "{{ .State }}",
			ToolTip: strings.Join([]string{
				"Battery: {{ .Battery.Charge }}% ({{if .Battery.Charging}}charging{{else}}discharging{{end}})",
			}, "\n"),
		}); err != nil {
			return nil, fmt.Errorf("could not compile default default block: %w", err)
		} else {
			encoder.DefaultBlock = block
		}
	} else if block, err := NewWaybarBlock(cfg.Default); err != nil {
		return nil, fmt.Errorf("could not compile default block: %w", err)
	} else {
		encoder.DefaultBlock = block
	}

	if cfg.ByState != nil {
		for stateName, blockConfig := range cfg.ByState {
			if state, ok := embermug.ParseState(stateName); !ok {
				return nil, fmt.Errorf("invalid state: %v", stateName)
			} else if block, err := NewWaybarBlock(&blockConfig); err != nil {
				return nil, fmt.Errorf("could not compile block: %v: %w", stateName, err)
			} else {
				encoder.BlockByState[state] = block
			}
		}
	} else {
		if block, err := NewWaybarBlock(&WaybarBlockConfig{
			Text: "{{ .State }} ({{ toFahrenheit .Current }}F/{{ toFahrenheit .Target }}F)",
			ToolTip: strings.Join([]string{
				"Battery: {{ .Battery.Charge }}% ({{if .Battery.Charging}}charging{{else}}discharging{{end}})",
			}, "\n"),
		}); err != nil {
			return nil, fmt.Errorf("could not compile default heating/cooling block: %w", err)
		} else {
			encoder.BlockByState[embermug.StateHeating] = block
			encoder.BlockByState[embermug.StateCooling] = block
		}

		if block, err := NewWaybarBlock(&WaybarBlockConfig{
			Text: "{{ .State }} ({{ toFahrenheit .Current }}F)",
			ToolTip: strings.Join([]string{
				"Battery: {{ .Battery.Charge }}% ({{if .Battery.Charging}}charging{{else}}discharging{{end}})",
			}, "\n"),
		}); err != nil {
			return nil, fmt.Errorf("could not compile default stable block: %w", err)
		} else {
			encoder.BlockByState[embermug.StateStable] = block
		}
	}

	return encoder, nil
}

func (e *WaybarEncoder) Encode(s service.State) error {
	var block *WaybarBlock

	if !s.Connected {
		block = e.DisconnectedBlock
	} else if b, ok := e.BlockByState[s.State]; ok {
		block = b
	} else {
		block = e.DefaultBlock
	}

	if data, err := block.Render(s); err != nil {
		return err
	} else {
		return e.Encoder.Encode(data)
	}
}

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

func init() {
	rootCmd.AddCommand(&waybarCommand)
}

func waybarEntrypoint(cmd *cobra.Command, args []string) error {
	var (
		cfg           Config
		waybar        *WaybarEncoder
		stateChannel  = make(chan service.State)
		signalChannel = make(chan os.Signal, 4)
		ctx, cancel   = signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
	)
	defer cancel()

	if err := viper.Unmarshal(&cfg); err != nil {
		slog.Error("Invalid configuration", "Error", err)
		return err
	}

	waybar, err := NewWaybarEncoder(&cfg.Waybar, os.Stdout)
	if err != nil {
		slog.Error("Could not compile waybar block definitions", "Error", err)
		return err
	}

	conn, err := net.Dial("unix", cfg.SocketPath)
	if err != nil {
		slog.Error("Could not connect to socket", "Path", cfg.SocketPath, "Error", err)
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
			if err := waybar.Encode(state); err != nil {
				slog.Error("Could not write waybar block", "Error", err)
			}
		}
	}

	return nil
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

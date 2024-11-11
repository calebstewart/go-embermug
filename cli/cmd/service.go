package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/calebstewart/go-embermug"
	"github.com/calebstewart/go-embermug/service"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/esiqveland/notify"
	"github.com/godbus/dbus/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"tinygo.org/x/bluetooth"
)

var serviceCommand = cobra.Command{
	Use:   "service device-address",
	Short: "Ember Mug device monitoring service",
	Long: `Ember Mug device monitoring service.

The service will listen on a socket passed via SystemD socket activation.
Clients to the socket will be sent state change updates about the ember
mug at the given address, and can send messages to reconnect or update
settings such as the set point temperature or device color.
`,
	Args: cobra.MaximumNArgs(1),
	Run:  commandExitWrapper(serviceEntrypoint),
}

func init() {
	flags := serviceCommand.Flags()
	flags.Bool("enable-notifications", false, "Send a desktop notification when the target temperature is reached")
	viper.BindPFlag("service.enable-notifications", flags.Lookup("enable-notifications"))
}

func serviceEntrypoint(cmd *cobra.Command, args []string) error {
	var (
		cfg         Config
		ctx, cancel = signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
		svc         *service.Service
		listener    net.Listener
	)
	defer cancel()

	if len(args) > 0 {
		viper.Set("service.device-address", args[0])
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		slog.Error("Invalid configuration", "Error", err)
		return err
	}

	if mac, err := bluetooth.ParseMAC(cfg.Service.DeviceAddress); err != nil {
		slog.Error("Invalid device address", "Address", cfg.Service.DeviceAddress, "Error", err)
		return err
	} else {
		svc = service.New(
			bluetooth.DefaultAdapter,
			bluetooth.Address{
				MACAddress: bluetooth.MACAddress{
					MAC: mac,
				},
			},
		)
	}

	if listeners, err := activation.Listeners(); err != nil {
		slog.Error("Could not find systemd activation listeners", "Error", err)
		return err
	} else if len(listeners) > 0 {
		listener = listeners[0]
		for _, l := range listeners[1:] {
			l.Close()
		}
		slog.Info("Received SystemD Activation Listener", "Addr", listener.Addr())
	} else {
		slog.Warn("No systemd sockets found")
		slog.Warn("Listening on default socket path", "Path", cfg.SocketPath)

		if l, err := net.Listen("unix", cfg.SocketPath); err != nil {
			slog.Error("Could not open unix socket", "Path", cfg.SocketPath, "Error", err)
			return err
		} else {
			listener = l
		}
	}
	defer listener.Close()

	slog.Info("Enabling Default Bluetooth Adapter")
	if err := bluetooth.DefaultAdapter.Enable(); err != nil {
		slog.Error("Could not enable bluetooth adapter", "Error", err)
		return err
	}

	if cfg.Service.EnableNotifications {
		// Start a client which will notify the desktop when the temp is reached
		go notifierClient(svc.RegisterClient(ctx))
	}

	slog.Info("Starting Ember Mug Monitor")
	if err := svc.Run(ctx, listener); err != nil {
		slog.Error("Service failed", "Error", err)
		return err
	}

	return nil
}

func notifierClient(client *service.Client) {
	var (
		lastState embermug.State
		logger    = slog.With("ClientID", client.ID)
	)

	conn, err := dbus.SessionBus()
	if err != nil {
		logger.Error("Could not open private bus. Notifications Disabled.", "Error", err)
		return
	}
	defer conn.Close()

	logger.Info("Notification Client Started")

	for {
		select {
		case <-client.Context.Done():
			return
		case state := <-client.Channel:
			if lastState != state.State && state.State == embermug.StateStable {
				logger.Debug("Sending desktop notification for stable temperature")
				_, err := notify.SendNotification(conn, notify.Notification{
					AppName:       "Ember Mug",
					Summary:       "Ember Mug Optimal Temperature Reached!",
					Body:          fmt.Sprintf("Your Ember Mug has reached its target optimal temperature of %v!", int(state.Target.Fahrenheit())),
					ExpireTimeout: time.Second * 5,
				})
				if err != nil {
					logger.Error("Could not deliver notification", "Error", err)
				}
			}
			lastState = state.State
		}
	}
}

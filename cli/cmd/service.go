package cmd

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"

	"github.com/calebstewart/go-embermug/service"
	"github.com/coreos/go-systemd/v22/activation"
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
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Add command flags here
		return viper.BindPFlags(cmd.Flags())
	},
	Run: commandExitWrapper(serviceEntrypoint),
}

func serviceEntrypoint(cmd *cobra.Command, args []string) error {
	var (
		ctx, cancel = signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
		svc         *service.Service
		listener    net.Listener
	)
	defer cancel()

	if mac, err := bluetooth.ParseMAC(args[0]); err != nil {
		slog.Error("Invalid device address", "Error", err)
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
	} else {
		slog.Warn("No systemd sockets found")
		slog.Warn("Listening on default socket path", "Path", viper.GetString("socket"))

		if l, err := net.Listen("unix", viper.GetString("socket")); err != nil {
			slog.Error("Could not open unix socket", "Path", viper.GetString("socket"), "Error", err)
			return err
		} else {
			listener = l
		}
	}
	defer listener.Close()

	if err := bluetooth.DefaultAdapter.Enable(); err != nil {
		slog.Error("Could not enable bluetooth adapter", "Error", err)
		return err
	}

	if err := svc.Run(ctx, listener); err != nil {
		slog.Error("Service failed", "Error", err)
		return err
	}

	return nil
}

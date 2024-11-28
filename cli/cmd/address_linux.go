package cmd

import "tinygo.org/x/bluetooth"

func ParseAddress(text string) (addr bluetooth.Address, err error) {
	if mac, err := bluetooth.ParseMAC(text); err != nil {
		return addr, err
	} else {
		return bluetooth.Address{
			MACAddress: bluetooth.MACAddress{
				MAC: mac,
			},
		}, nil
	}
}

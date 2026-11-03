// claudetemp prints the temperature of the first SHT41 or SHTC3 sensor it
// finds on the I2C adapter (an MCP2221, or else an FT232H), as a number with
// one decimal, and exits.
//
// The progress-pie Claude Code plugin runs it every 30 seconds to show the
// temperature in the status line. With no adapter or no sensor it prints
// nothing to standard output, says why on standard error, and exits 1. If the
// adapter is there but can't be opened (another program, such as claudematrix,
// has it), it prints "busy" and exits 3, so the plugin can keep the last
// reading.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"rafaelmartins.com/p/usbhid"

	mcp2221 "github.com/ApolloX939/mcp2221"
	"github.com/ApolloX939/mcp2221/ft232h"
	"github.com/ApolloX939/mcp2221/sht41"
	"github.com/ApolloX939/mcp2221/shtc3"
)

func main() {
	unit := flag.String("unit", "F", "temperature unit: F (Fahrenheit) or C (Celsius)")
	flag.Parse()
	u := strings.ToUpper(strings.TrimSpace(*unit))
	if u != "F" && u != "C" {
		fmt.Fprintf(os.Stderr, "claudetemp: unit must be F or C, not %q\n", *unit)
		os.Exit(2)
	}
	celsius, err := read()
	if errors.Is(err, errBusy) {
		fmt.Fprintln(os.Stderr, "claudetemp:", err)
		fmt.Println("busy")
		os.Exit(3)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudetemp:", err)
		os.Exit(1)
	}
	fmt.Println(format(celsius, u))
}

// format renders a Celsius temperature in unit "F" or "C" as %0.1f.
func format(celsius float64, unit string) string {
	if unit == "F" {
		celsius = celsius*9/5 + 32
	}
	return fmt.Sprintf("%0.1f", celsius)
}

type bus interface {
	sht41.Bus
	shtc3.Bus
	Close() error
}

// errBusy marks an adapter that is present but can't be opened.
var errBusy = errors.New("adapter busy")

func openBus() (bus, error) {
	m, err := mcp2221.Open()
	if err == nil {
		return m, nil
	}
	if !errors.Is(err, usbhid.ErrNoDeviceFound) {
		return nil, fmt.Errorf("%w: %v", errBusy, err)
	}
	f, ferr := ft232h.Open()
	if ferr == nil {
		return f, nil
	}
	if errors.Is(ferr, ft232h.ErrNoDevice) {
		return nil, errors.New("no MCP2221 or FT232H adapter found")
	}
	return nil, fmt.Errorf("%w: %v", errBusy, ferr)
}

// read measures once, trying the SHT41 and then the SHTC3.
func read() (float64, error) {
	b, err := openBus()
	if err != nil {
		return 0, err
	}
	defer b.Close()
	if s, err := sht41.New(b); err == nil {
		r, err := s.Read()
		return r.Celsius, err
	}
	if s, err := shtc3.New(b); err == nil {
		r, err := s.Read()
		return r.Celsius, err
	}
	return 0, errors.New("no SHT41 or SHTC3 temperature sensor found")
}

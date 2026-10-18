package mcp2221_test

import (
	"fmt"
	"log"

	mcp2221 "github.com/ApolloX939/mcp2221"
)

func Example() {
	dev, err := mcp2221.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer dev.Close()

	// List the devices on the I2C bus.
	addrs, err := dev.Scan()
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range addrs {
		fmt.Printf("found device at 0x%02x\n", a)
	}

	// Turn on the oscillator of an HT16K33 LED driver at 0x70.
	if err := dev.I2CWrite(0x70, []byte{0x21}); err != nil {
		log.Fatal(err)
	}

	// Read two bytes from register 0x00 of the same device.
	buf := make([]byte, 2)
	if err := dev.I2CTx(0x70, []byte{0x00}, buf); err != nil {
		log.Fatal(err)
	}
}

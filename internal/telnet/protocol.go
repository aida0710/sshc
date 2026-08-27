package telnet

import "fmt"

const (
	commandEOF  byte = 236
	commandSE   byte = 240
	commandSB   byte = 250
	commandWILL byte = 251
	commandWONT byte = 252
	commandDO   byte = 253
	commandDONT byte = 254
	commandIAC  byte = 255

	optionBinary       byte = 0
	optionEcho         byte = 1
	optionSuppressGA   byte = 3
	optionTerminalType byte = 24
	optionNAWS         byte = 31

	terminalTypeIS   byte = 0
	terminalTypeSEND byte = 1
)

func (c *Conn) handleNegotiation(command, option byte) error {
	switch command {
	case commandWILL:
		if !supportsRemote(option) {
			if c.markRemoteRejected(option) {
				return c.sendControl(commandIAC, commandDONT, option)
			}
			return nil
		}
		if c.enableRemote(option) {
			return c.sendControl(commandIAC, commandDO, option)
		}
	case commandWONT:
		c.disableRemote(option)
	case commandDO:
		if !supportsLocal(option) {
			if c.markLocalRejected(option) {
				return c.sendControl(commandIAC, commandWONT, option)
			}
			return nil
		}
		if c.enableLocal(option) {
			if err := c.sendControl(commandIAC, commandWILL, option); err != nil {
				return err
			}
			if option == optionNAWS {
				width, height := c.windowSize()
				return c.sendWindowSize(width, height)
			}
		}
	case commandDONT:
		if c.disableLocal(option) {
			return c.sendControl(commandIAC, commandWONT, option)
		}
	}
	return nil
}

func supportsRemote(option byte) bool {
	return option == optionBinary || option == optionEcho || option == optionSuppressGA
}

func supportsLocal(option byte) bool {
	return option == optionBinary || option == optionSuppressGA || option == optionTerminalType || option == optionNAWS
}

func (c *Conn) enableRemote(option byte) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.remoteEnabled[option] {
		return false
	}
	c.remoteEnabled[option] = true
	c.remoteRejected[option] = false
	return true
}

func (c *Conn) disableRemote(option byte) {
	c.stateMu.Lock()
	c.remoteEnabled[option] = false
	c.remoteRejected[option] = false
	c.stateMu.Unlock()
}

func (c *Conn) markRemoteRejected(option byte) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.remoteRejected[option] {
		return false
	}
	c.remoteRejected[option] = true
	return true
}

func (c *Conn) enableLocal(option byte) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.localEnabled[option] {
		return false
	}
	c.localEnabled[option] = true
	c.localRejected[option] = false
	return true
}

func (c *Conn) disableLocal(option byte) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	wasEnabled := c.localEnabled[option]
	c.localEnabled[option] = false
	c.localRejected[option] = false
	return wasEnabled
}

func (c *Conn) markLocalRejected(option byte) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.localRejected[option] {
		return false
	}
	c.localRejected[option] = true
	return true
}

func (c *Conn) windowSize() (uint16, uint16) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.windowWidth, c.windowHeight
}

func (c *Conn) readSubnegotiation() error {
	option, err := c.reader.ReadByte()
	if err != nil {
		return err
	}
	payload := make([]byte, 0, min(c.maxSubnegotiation, 256))
	wireBytes := 0
	for {
		value, readErr := c.reader.ReadByte()
		if readErr != nil {
			return readErr
		}
		wireBytes++
		if wireBytes > c.maxSubnegotiation {
			return ErrSubnegotiationTooLarge
		}
		if value != commandIAC {
			payload = append(payload, value)
			continue
		}

		next, nextErr := c.reader.ReadByte()
		if nextErr != nil {
			return nextErr
		}
		wireBytes++
		if wireBytes > c.maxSubnegotiation {
			return ErrSubnegotiationTooLarge
		}
		switch next {
		case commandIAC:
			payload = append(payload, commandIAC)
		case commandSE:
			return c.handleSubnegotiation(option, payload)
		default:
			return fmt.Errorf("%w: command %d inside subnegotiation", ErrMalformedNegotiation, next)
		}
	}
}

func (c *Conn) handleSubnegotiation(option byte, payload []byte) error {
	if option != optionTerminalType || len(payload) != 1 || payload[0] != terminalTypeSEND {
		return nil
	}
	c.stateMu.Lock()
	enabled := c.localEnabled[optionTerminalType]
	terminalType := c.terminalType
	c.stateMu.Unlock()
	if !enabled {
		return nil
	}
	response := make([]byte, 0, len(terminalType)+6)
	response = append(response, commandIAC, commandSB, optionTerminalType, terminalTypeIS)
	response = append(response, terminalType...)
	response = append(response, commandIAC, commandSE)
	return c.sendControl(response...)
}

func (c *Conn) sendWindowSize(width, height uint16) error {
	payload := []byte{
		commandIAC, commandSB, optionNAWS,
		byte(width >> 8), byte(width), byte(height >> 8), byte(height),
		commandIAC, commandSE,
	}
	// NAWS values are data inside subnegotiation, so a dimension byte equal to
	// IAC must itself be doubled.
	escaped := make([]byte, 0, len(payload)+4)
	escaped = append(escaped, payload[:3]...)
	for _, value := range payload[3:7] {
		escaped = append(escaped, value)
		if value == commandIAC {
			escaped = append(escaped, commandIAC)
		}
	}
	escaped = append(escaped, commandIAC, commandSE)
	return c.sendControl(escaped...)
}

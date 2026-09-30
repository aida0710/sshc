package telnet

import (
	"errors"
	"fmt"
)

// inboundState is the position inside the peer's byte stream where the last
// Read stopped. The automation runner reads with a deadline, and the deadline
// can expire between any two bytes of an IAC command. Keeping the position on
// Conn lets the next Read finish that command instead of returning its
// remaining bytes as application data.
type inboundState uint8

const (
	inboundData inboundState = iota
	// inboundCommand follows IAC; the command byte is next.
	inboundCommand
	// inboundNegotiationOption follows IAC WILL, WONT, DO or DONT.
	inboundNegotiationOption
	// inboundSubnegotiationOption follows IAC SB.
	inboundSubnegotiationOption
	// inboundSubnegotiationPayload is inside IAC SB <option> ... IAC SE.
	inboundSubnegotiationPayload
	// inboundSubnegotiationIAC follows an IAC inside the payload.
	inboundSubnegotiationIAC
)

// inboundDecoder is the partially read IAC command carried between Reads.
// Only Read touches it, while holding readMu.
type inboundDecoder struct {
	state                   inboundState
	negotiationCommand      byte
	subnegotiationOption    byte
	subnegotiationPayload   []byte
	subnegotiationWireBytes int
}

// breaksFraming reports a peer error after which no later byte can be trusted
// to start a command or application data, so the connection must be closed.
func breaksFraming(err error) bool {
	return errors.Is(err, ErrMalformedNegotiation) || errors.Is(err, ErrSubnegotiationTooLarge)
}

// decodeInboundByte advances the decoder by one byte from the peer and appends
// the byte to destination when it is application data.
func (c *Conn) decodeInboundByte(value byte, destination []byte, written *int) error {
	decoder := &c.inbound
	switch decoder.state {
	case inboundData:
		if value == commandIAC {
			decoder.state = inboundCommand
			return nil
		}
		c.appendApplicationByte(destination, written, value)
	case inboundCommand:
		return c.decodeCommand(value, destination, written)
	case inboundNegotiationOption:
		decoder.state = inboundData
		return c.handleNegotiation(decoder.negotiationCommand, value)
	case inboundSubnegotiationOption:
		decoder.subnegotiationOption = value
		decoder.subnegotiationPayload = decoder.subnegotiationPayload[:0]
		decoder.subnegotiationWireBytes = 0
		decoder.state = inboundSubnegotiationPayload
	case inboundSubnegotiationPayload:
		if err := decoder.countSubnegotiationByte(c.maxSubnegotiation); err != nil {
			return err
		}
		if value == commandIAC {
			decoder.state = inboundSubnegotiationIAC
			return nil
		}
		decoder.subnegotiationPayload = append(decoder.subnegotiationPayload, value)
	case inboundSubnegotiationIAC:
		if err := decoder.countSubnegotiationByte(c.maxSubnegotiation); err != nil {
			return err
		}
		switch value {
		case commandIAC:
			decoder.subnegotiationPayload = append(decoder.subnegotiationPayload, commandIAC)
			decoder.state = inboundSubnegotiationPayload
		case commandSE:
			decoder.state = inboundData
			return c.handleSubnegotiation(decoder.subnegotiationOption, decoder.subnegotiationPayload)
		default:
			return fmt.Errorf("%w: command %d inside subnegotiation", ErrMalformedNegotiation, value)
		}
	}
	return nil
}

func (c *Conn) decodeCommand(command byte, destination []byte, written *int) error {
	decoder := &c.inbound
	decoder.state = inboundData
	switch command {
	case commandIAC:
		c.appendApplicationByte(destination, written, commandIAC)
	case commandWILL, commandWONT, commandDO, commandDONT:
		decoder.negotiationCommand = command
		decoder.state = inboundNegotiationOption
	case commandSB:
		decoder.state = inboundSubnegotiationOption
	default:
		if command < commandEOF {
			return ErrMalformedNegotiation
		}
		// EOF through GA are commands without option payloads. They do not
		// belong in application output.
	}
	return nil
}

// countSubnegotiationByte bounds the peer-controlled payload, counting every
// byte after the option including the escaping IAC and the closing IAC SE.
func (decoder *inboundDecoder) countSubnegotiationByte(limit int) error {
	decoder.subnegotiationWireBytes++
	if decoder.subnegotiationWireBytes > limit {
		return ErrSubnegotiationTooLarge
	}
	return nil
}

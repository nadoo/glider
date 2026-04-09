package session

import "encoding/binary"

// Command types for the anytls session protocol.
const (
	cmdWaste               byte = 0  // padding, discarded
	cmdSYN                 byte = 1  // stream open
	cmdPSH                 byte = 2  // data push
	cmdFIN                 byte = 3  // stream close (EOF)
	cmdSettings            byte = 4  // client -> server settings
	cmdAlert               byte = 5  // alert message
	cmdUpdatePaddingScheme byte = 6  // update padding scheme
	cmdSYNACK              byte = 7  // server -> client stream opened (v2)
	cmdHeartRequest        byte = 8  // keepalive request (v2)
	cmdHeartResponse       byte = 9  // keepalive response (v2)
	cmdServerSettings      byte = 10 // server -> client settings (v2)
)

const headerSize = 1 + 4 + 2 // cmd(1) + streamID(4) + length(2)

type frame struct {
	cmd  byte
	sid  uint32
	data []byte
}

func newFrame(cmd byte, sid uint32) frame {
	return frame{cmd: cmd, sid: sid}
}

type rawHeader [headerSize]byte

func (h rawHeader) Cmd() byte        { return h[0] }
func (h rawHeader) StreamID() uint32 { return binary.BigEndian.Uint32(h[1:]) }
func (h rawHeader) Length() uint16   { return binary.BigEndian.Uint16(h[5:]) }

package console

import (
	"context"
	"encoding/binary"
	"net"
)

var (
	DefaultMaxMessageSize = int(1 << 20)
)

func byteArrayToUInt32(bytes []byte) (result int64, bytesRead int) {
	return binary.Varint(bytes)
}

func intToByteArray(value int64, bufferSize int) []byte {
	toWriteLen := make([]byte, bufferSize)
	binary.PutVarint(toWriteLen, value)
	return toWriteLen
}

type ListenCb func(context.Context, *net.TCPListener) error
type RecvCb func(context.Context, *net.TCPConn, int, []byte) error
type ConnectCb func(context.Context, *net.TCPConn) error
type CloseCb func(context.Context, *net.TCPConn) error
type CmdCb func(context.Context, *net.TCPConn, string) error
type RetCb func(string) string

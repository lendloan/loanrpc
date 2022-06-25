package console

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"go-micro.dev/v4/logger"
)

type ConsoleListener struct {
	socket          *net.TCPListener
	address         string
	maxMessageSize  int
	listenCb        ListenCb
	connectCb       ConnectCb
	cmdCb           CmdCb
	closeCb         CloseCb
	shutdownChannel chan struct{}
	shutdownGroup   *sync.WaitGroup

	rTiemout int
	wTimeout int
}

type ConsoleListenerConfig struct {
	MaxMessageSize int
	EnableLogging  bool
	Address        string
	ListenCb       ListenCb
	ConnectCb      ConnectCb
	CmdCb          CmdCb
	CloseCb        CloseCb
	RTimeout       int
	WTimeout       int
}

func ListenConsole(cfg ConsoleListenerConfig) (*ConsoleListener, error) {
	maxMessageSize := DefaultMaxMessageSize
	// 0 is the default, and the message must be atleast 1 byte large
	if cfg.MaxMessageSize != 0 {
		maxMessageSize = cfg.MaxMessageSize
	}
	btl := &ConsoleListener{
		maxMessageSize:  maxMessageSize,
		listenCb:        cfg.ListenCb,
		connectCb:       cfg.ConnectCb,
		cmdCb:           cfg.CmdCb,
		closeCb:         cfg.CloseCb,
		shutdownChannel: make(chan struct{}),
		address:         cfg.Address,
		shutdownGroup:   &sync.WaitGroup{},
		rTiemout:        cfg.RTimeout,
		wTimeout:        cfg.WTimeout,
	}

	if err := btl.openSocket(); err != nil {
		return nil, err
	}

	return btl, nil
}

func (btl *ConsoleListener) blockListen() error {
	for {
		conn, err := btl.socket.AcceptTCP()
		if err != nil {
			logger.Error("[blockListen] Error attempting to accept connection: ", err)
			select {
			case <-btl.shutdownChannel:
				return nil
			default:
			}
		} else {
			if nil != btl.connectCb {
				btl.connectCb(context.TODO(), conn)
			}
			go handleConsoleConn(conn, btl.maxMessageSize, btl.cmdCb, btl.closeCb, btl.shutdownGroup, uint64(btl.rTiemout))
		}
	}
}

func (btl *ConsoleListener) openSocket() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", btl.address)
	if err != nil {
		return err
	}
	receiveSocket, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}
	btl.socket = receiveSocket
	if nil != btl.listenCb {
		btl.listenCb(context.TODO(), receiveSocket)
	}
	return err
}

func (btl *ConsoleListener) StartListening() error {
	return btl.blockListen()
}

func (btl *ConsoleListener) Close() {
	close(btl.shutdownChannel)
	btl.shutdownGroup.Wait()
}

func (btl *ConsoleListener) StartListeningAsync() error {
	var err error
	go func() {
		err = btl.blockListen()
	}()
	return err
}

func handleConsoleConn(conn *net.TCPConn, maxMessageSize int, rcb CmdCb, ccb CloseCb, sdGroup *sync.WaitGroup, r_timeout uint64) {

	// sdGroup.Add(1)
	// defer sdGroup.Done()
	defer func() {
		if err := recover(); nil != err {
			logger.Error("handleListenedConn ", err)
		}

		if nil != conn {
			logger.Infof("Address %s: Client closed connection", conn.RemoteAddr())
			if nil != ccb {
				ccb(context.TODO(), conn)
			}
			conn.Close()
		}

		return
	}()

	recvCh := make(chan int)
	// 读数据协程
	go func() {
		for {
			dataBuffer := make([]byte, maxMessageSize)
			_, dataReadError := readFromConsole(conn, dataBuffer[0:maxMessageSize])
			if dataReadError != nil {
				if dataReadError != io.EOF {
					// log the error from the call to read
					logger.Infof("Address %s: Failure to read from connection. Underlying error: %s", conn.RemoteAddr(), dataReadError)
				} else {
					// The client wrote the header but closed the connection
					logger.Infof("Address %s: Client closed connection during data read. Underlying error: %s", conn.RemoteAddr(), dataReadError)
				}

				return
			}

			// 如果读取消息没有错误，就调用回调函数
			err := rcb(context.TODO(), conn, string(dataBuffer))
			if err != nil {
				logger.Infof("Error in Callback: %s", err)
			}

			recvCh <- 0
		}
	}()

	if 0 == r_timeout {
		r_timeout = 60
	}

	isStop := false
	rstamp := time.Now().Unix()
	timer := time.NewTicker(time.Duration(5 * uint64(time.Second)))
	// 主逻辑协程
	for {
		select {
		case <-recvCh:
			rstamp = time.Now().Unix()

		case <-timer.C:
			// 超时断开连接
			if int64(r_timeout) < int64(time.Now().Unix()-rstamp) {
				isStop = true
				WriteToConsole(conn, []byte("Timeout Closed!\n"))

				return
			}

		}

		if isStop {
			break
		}
	}

	return
}

// Handles reading from a given connection.
func readFromConsole(reader *net.TCPConn, buffer []byte) (int, error) {
	// This fills the buffer
	bytesLen, err := reader.Read(buffer)
	// Output the content of the bytes to the queue
	if bytesLen == 0 {
		if err != nil && err == io.EOF {
			// "End of individual transmission"
			// We're just done reading from that conn
			return bytesLen, err
		}
	}

	if err != nil {
		//"Underlying network failure?"
		// Not sure what this error would be, but it could exist and i've seen it handled
		// as a general case in other networking code. Following in the footsteps of (greatness|madness)
		return bytesLen, err
	}
	// Read some bytes, return the length
	return bytesLen, nil
}

func WriteToConsole(conn *net.TCPConn, packet []byte) (n int, err error) {
	if 0 == len(packet) {
		return
	}

	conn.Write(packet)
	return
}

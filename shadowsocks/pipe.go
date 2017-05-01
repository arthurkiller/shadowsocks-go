package shadowsocks

import (
	"net"
	"syscall"
	"time"
)

// PipeThenClose copies data from src to dst, closes dst when done.
func PipeThenClose(src, dst net.Conn, timeout int) {
	defer dst.Close()
	buf := leakyBuf.Get()
	defer leakyBuf.Put(buf)
	for {
		src.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
		n, err := src.Read(buf)
		// read may return EOF with n > 0
		// should always process n > 0 bytes before handling error
		if n > 0 {
			// Note: avoid overwrite err returned by Read.
			if _, err := dst.Write(buf[0:n]); err != nil {
				Debug.Println("write:", err)
				break
			}
		}
		if err != nil {
			// Always "use of closed network connection", but no easy way to
			// identify this specific error. So just leave the error along for now.
			// More info here: https://code.google.com/p/go/issues/detail?id=4373
			/*
				if bool(Debug) && err != io.EOF {
					Debug.Println("read:", err)
				}
			*/
			if err == errBufferTooSmall {
				// unlikely
				Debug.Println("read:", err)
			} else if err == ErrPacketOtaFailed {
				Debug.Println("read:", err)
			}
			break
		}
	}
}

func UDPClientReceiveThenClose(write net.PacketConn, writeAddr net.Addr, readClose net.PacketConn) {
	buf := make([]byte, 4096)
	defer readClose.Close()
	for {
		readClose.SetDeadline(time.Now().Add(udpTimeout))
		n, _, err := readClose.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(*net.OpError); ok {
				if ne.Err == syscall.EMFILE || ne.Err == syscall.ENFILE {
					// log too many open file error
					// EMFILE is process reaches open file limits, ENFILE is system limit
					Debug.Println("[udp]read error:", err)
				}
			}
			Debug.Printf("[udp]closed pipe %s<-%s\n", writeAddr, readClose.LocalAddr())
			return
		}
		write.WriteTo(buf[:n], writeAddr)
	}
}

func udpReceiveThenClose(write net.PacketConn, writeAddr net.Addr, readClose net.PacketConn) {
	buf := leakyBuf.Get()
	defer leakyBuf.Put(buf)
	defer readClose.Close()
	for {
		readClose.SetDeadline(time.Now().Add(udpTimeout))
		n, raddr, err := readClose.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(*net.OpError); ok {
				if ne.Err == syscall.EMFILE || ne.Err == syscall.ENFILE {
					// log too many open file error
					// EMFILE is process reaches open file limits, ENFILE is system limit
					Debug.Println("[udp]read error:", err)
				}
			}
			Debug.Printf("[udp]closed pipe %s<-%s\n", writeAddr, readClose.LocalAddr())
			return
		}
		// need improvement here
		if req, ok := reqList.Get(raddr.String()); ok {
			write.WriteTo(append(req, buf[:n]...), writeAddr)
		} else {
			header := parseHeaderFromAddr(raddr)
			write.WriteTo(append(header, buf[:n]...), writeAddr)
		}
	}
}

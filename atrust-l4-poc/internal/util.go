package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

func CopyWithDeadline(dst net.Conn, src net.Conn, stopCh chan struct{}) {
	buf := make([]byte, 8192)
	for {
		select {
		case <-stopCh:
			return
		default:
		}
		src.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := src.Read(buf)
		if n > 0 {
			_, werr := dst.Write(buf[:n])
			if werr != nil {
				return
			}
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			// timeout or other error -> stop
			return
		}
	}
}

func ToJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func FromJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func NewStreamID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

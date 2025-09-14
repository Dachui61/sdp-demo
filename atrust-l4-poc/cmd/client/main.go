package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/armon/go-socks5"
	pb "github.com/atrust/poctunnel/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Sock5Handler implements socks5.Handler interface
type Sock5Handler struct {
	gatewayClient pb.TunnelServiceClient
	stream        pb.TunnelService_TunnelClient
	streamMutex   sync.Mutex
	connMap       map[string]net.Conn
	connMutex     sync.Mutex
}

// NewStreamID generates a new stream ID
func NewStreamID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Dial implements socks5.Dial interface
func (h *Sock5Handler) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	log.Printf("[client] SOCKS5 dial request: %s %s", network, addr)

	// Create a virtual connection that will forward data through gRPC
	streamID := NewStreamID()

	// Create a pipe to simulate a connection
	clientConn, serverConn := net.Pipe()

	// Store the server side of the pipe
	h.connMutex.Lock()
	h.connMap[streamID] = serverConn
	h.connMutex.Unlock()

	// Send OPEN_STREAM frame to gateway
	openStream := struct {
		StreamID string `json:"stream_id"`
		Target   string `json:"target"`
	}{
		StreamID: streamID,
		Target:   addr,
	}
	
	payload, err := json.Marshal(openStream)
	if err != nil {
		log.Printf("[client] failed to marshal open stream: %v", err)
		return nil, err
	}
	
	h.streamMutex.Lock()
	err = h.stream.Send(&pb.Frame{
		Type:     pb.FrameType_OPEN_STREAM,
		StreamId: streamID,
		Payload:  payload,
	})
	h.streamMutex.Unlock()
	
	if err != nil {
		log.Printf("[client] failed to send OPEN_STREAM: %v", err)
		return nil, err
	}
	
	// Start goroutine to forward data from the pipe to gRPC
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := serverConn.Read(buf)
			if n > 0 {
				// Send data to gateway
				h.streamMutex.Lock()
				sendErr := h.stream.Send(&pb.Frame{
					Type:     pb.FrameType_DATA,
					StreamId: streamID,
					Payload:  buf[:n],
				})
				h.streamMutex.Unlock()
				
				if sendErr != nil {
					log.Printf("[client] failed to send data: %v", sendErr)
					break
				}
			}
			
			if err != nil {
				if err != io.EOF {
					log.Printf("[client] read from pipe error: %v", err)
				}
				// Send CLOSE_STREAM to gateway
				h.streamMutex.Lock()
				_ = h.stream.Send(&pb.Frame{
					Type:     pb.FrameType_CLOSE_STREAM,
					StreamId: streamID,
				})
				h.streamMutex.Unlock()
				
				// Remove connection from map
				h.connMutex.Lock()
				delete(h.connMap, streamID)
				h.connMutex.Unlock()
				break
			}
		}
	}()
	
	// Wrap the connection to handle LocalAddr properly
	return NewWrappedConn(clientConn), nil
}

func runClient() error {
	// Connect to gateway
	conn, err := grpc.Dial("127.0.0.1:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to gateway: %v", err)
	}
	defer conn.Close()
	
	// Create gateway client
	gatewayClient := pb.NewTunnelServiceClient(conn)
	
	// Create bidirectional stream
	stream, err := gatewayClient.Tunnel(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create tunnel stream: %v", err)
	}
	
	// Create SOCKS5 handler
	handler := &Sock5Handler{
		gatewayClient: gatewayClient,
		stream:        stream,
		connMap:       make(map[string]net.Conn),
	}
	
	// Start receiving frames from gateway
	go func() {
		for {
			frame, err := stream.Recv()
			if err != nil {
				log.Printf("[client] failed to receive frame: %v", err)
				return
			}
			
			switch frame.Type {
			case pb.FrameType_DATA:
				// Forward data to the corresponding connection
				handler.connMutex.Lock()
				conn, ok := handler.connMap[frame.StreamId]
				handler.connMutex.Unlock()
				
				if ok && len(frame.Payload) > 0 {
					_, writeErr := conn.Write(frame.Payload)
					if writeErr != nil {
						log.Printf("[client] failed to write to connection: %v", writeErr)
					}
				}
				
			case pb.FrameType_CLOSE_STREAM:
				// Close the corresponding connection
				handler.connMutex.Lock()
				if conn, ok := handler.connMap[frame.StreamId]; ok {
					conn.Close()
					delete(handler.connMap, frame.StreamId)
				}
				handler.connMutex.Unlock()
				
			case pb.FrameType_HEARTBEAT:
				// Respond to heartbeat
				handler.streamMutex.Lock()
				_ = stream.Send(&pb.Frame{Type: pb.FrameType_HEARTBEAT})
				handler.streamMutex.Unlock()
			}
		}
	}()
	
	// Start SOCKS5 server
	conf := &socks5.Config{
		Dial: handler.Dial,
	}
	
	server, err := socks5.New(conf)
	if err != nil {
		return err
	}
	
	// Use a different port to avoid conflicts
	socks5Addr := "127.0.0.1:1081"
	log.Printf("[client] Starting SOCKS5 server on %s", socks5Addr)
	if err := server.ListenAndServe("tcp", socks5Addr); err != nil {
		return err
	}
	
	return nil
}

func main() {
	log.Println("[client] Starting client. SOCKS5 at 127.0.0.1:1081")
	if err := runClient(); err != nil {
		log.Fatal(err)
	}
}

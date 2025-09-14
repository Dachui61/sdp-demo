package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	pb "github.com/atrust/poctunnel/proto"
	"google.golang.org/grpc"
)

type gatewayServer struct {
	pb.UnimplementedTunnelServiceServer
	// per-gRPC-stream we maintain its own streams map (handled in handler)
}

// OpenStream used for unmarshalling payload
type OpenStream struct {
	StreamID string `json:"stream_id"`
	Target   string `json:"target"`
}

func (s *gatewayServer) Tunnel(stream pb.TunnelService_TunnelServer) error {
	// For each connected client (one gRPC stream), we manage their child streams
	ctx := stream.Context()
	log.Println("[gateway] new gRPC tunnel stream established")

	// Map stream_id -> net.Conn
	var mu sync.Mutex
	conns := make(map[string]net.Conn)
	recvCh := make(chan *pb.Frame, 64)
	errCh := make(chan error, 1)

	// goroutine: read incoming frames from client and push to recvCh
	go func() {
		for {
			in, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}
			recvCh <- in
		}
	}()

	// goroutine: heartbeat (optional)
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = stream.Send(&pb.Frame{Type: pb.FrameType_HEARTBEAT})
			}
		}
	}()

	// handler loop
	for {
		select {
		case <-ctx.Done():
			// close all child conns
			mu.Lock()
			for id, c := range conns {
				_ = c.Close()
				delete(conns, id)
			}
			mu.Unlock()
			return ctx.Err()
		case err := <-errCh:
			// client closed or error
			if err == io.EOF {
				log.Println("[gateway] client closed stream (EOF)")
				return nil
			}
			log.Printf("[gateway] recv error: %v\n", err)
			return err
		case frame := <-recvCh:
			switch frame.Type {
			case pb.FrameType_OPEN_STREAM:
				// parse open stream payload (JSON)
				var os OpenStream
				if len(frame.Payload) > 0 {
					if jerr := json.Unmarshal(frame.Payload, &os); jerr != nil {
						log.Println("[gateway] open stream payload unmarshal error:", jerr)
						continue
					}
				} else {
					log.Println("[gateway] open stream payload empty")
					continue
				}
				target := os.Target
				id := os.StreamID
				log.Printf("[gateway] OPEN stream %s -> %s\n", id, target)
				// dial target
				conn, err := net.Dial("tcp", target)
				if err != nil {
					log.Printf("[gateway] dial %s fail: %v\n", target, err)
					// send CLOSE_STREAM to inform client
					_ = stream.Send(&pb.Frame{Type: pb.FrameType_CLOSE_STREAM, StreamId: id})
					continue
				}
				// store conn
				mu.Lock()
				conns[id] = conn
				mu.Unlock()

				// start goroutine reading from conn -> send DATA frames back to client
				go func(id string, c net.Conn) {
					buf := make([]byte, 8192)
					for {
						n, err := c.Read(buf)
						if n > 0 {
							chunk := make([]byte, n)
							copy(chunk, buf[:n])
							sendErr := stream.Send(&pb.Frame{
								Type:     pb.FrameType_DATA,
								StreamId: id,
								Payload:  chunk,
							})
							if sendErr != nil {
								log.Println("[gateway] send to client error:", sendErr)
								return
							}
						}
						if err != nil {
							if err != io.EOF {
								log.Println("[gateway] read from target err:", err)
							}
							// notify client stream closed
							_ = stream.Send(&pb.Frame{Type: pb.FrameType_CLOSE_STREAM, StreamId: id})
							// close local conn and cleanup
							_ = c.Close()
							mu.Lock()
							delete(conns, id)
							mu.Unlock()
							return
						}
					}
				}(id, conn)

			case pb.FrameType_DATA:
				// forward payload to corresponding local conn
				id := frame.StreamId
				mu.Lock()
				c, ok := conns[id]
				mu.Unlock()
				if !ok {
					log.Printf("[gateway] DATA: unknown stream %s\n", id)
					// maybe send CLOSE_STREAM back
					_ = stream.Send(&pb.Frame{Type: pb.FrameType_CLOSE_STREAM, StreamId: id})
					continue
				}
				if len(frame.Payload) > 0 {
					_, werr := c.Write(frame.Payload)
					if werr != nil {
						log.Printf("[gateway] write to target %s error: %v\n", id, werr)
						_ = stream.Send(&pb.Frame{Type: pb.FrameType_CLOSE_STREAM, StreamId: id})
						_ = c.Close()
						mu.Lock()
						delete(conns, id)
						mu.Unlock()
					}
				}
			case pb.FrameType_CLOSE_STREAM:
				id := frame.StreamId
				mu.Lock()
				if c, ok := conns[id]; ok {
					_ = c.Close()
					delete(conns, id)
				}
				mu.Unlock()
			case pb.FrameType_HEARTBEAT:
				// ignore
			default:
				log.Printf("[gateway] unknown frame type: %v\n", frame.Type)
			}
		}
	}
}

func startGRPCServer(addr string) error {
	srv := grpc.NewServer()
	pb.RegisterTunnelServiceServer(srv, &gatewayServer{})
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("[gateway] gRPC listening %s\n", addr)
	return srv.Serve(lis)
}

func startDummyBackend() {
	// Simple HTTP backend (for testing)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		remote := r.RemoteAddr
		fmt.Fprintf(w, "Hello from backend! remote=%s\n", remote)
	})
	go func() {
		addr := ":9000"
		log.Printf("[gateway] starting dummy backend on %s (for test)\n", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("backend error: %v", err)
		}
	}()
}

func main() {
	// start a test backend on gateway machine at :9000
	startDummyBackend()

	go func() {
		if err := startGRPCServer(":50051"); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	// block
	select {}
}

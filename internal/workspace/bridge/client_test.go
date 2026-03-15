package bridge

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

const testBufSize = 1 << 20

type rawReadTestServer struct {
	pb.UnimplementedContainerServiceServer
	files map[string][]byte
}

func (s *rawReadTestServer) ReadRaw(req *pb.ReadRawRequest, stream pb.ContainerService_ReadRawServer) error {
	data, ok := s.files[req.GetPath()]
	if !ok {
		return status.Errorf(codes.NotFound, "open: open %s: no such file or directory", req.GetPath())
	}
	if len(data) == 0 {
		return nil
	}
	if err := stream.Send(&pb.DataChunk{Data: data[:1]}); err != nil {
		return err
	}
	if len(data) > 1 {
		if err := stream.Send(&pb.DataChunk{Data: data[1:]}); err != nil {
			return err
		}
	}
	return nil
}

func newTestReadRawClient(t *testing.T, files map[string][]byte) *Client {
	t.Helper()

	lis := bufconn.Listen(testBufSize)
	srv := grpc.NewServer()
	pb.RegisterContainerServiceServer(srv, &rawReadTestServer{files: files})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		<-done
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return NewClientFromConn(conn)
}

func TestClientReadRawMissingFileReturnsNotFoundImmediately(t *testing.T) {
	t.Parallel()

	client := newTestReadRawClient(t, map[string][]byte{})
	_, err := client.ReadRaw(context.Background(), "/data/media/missing.jpg")
	if err == nil {
		t.Fatal("expected read raw to fail for missing file")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestClientReadRawPreservesFirstChunk(t *testing.T) {
	t.Parallel()

	client := newTestReadRawClient(t, map[string][]byte{
		"/data/media/existing.jpg": []byte("hello"),
	})
	reader, err := client.ReadRaw(context.Background(), "/data/media/existing.jpg")
	if err != nil {
		t.Fatalf("ReadRaw returned error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read raw reader failed: %v", err)
	}
	if got := string(data); got != "hello" {
		t.Fatalf("expected full payload, got %q", got)
	}
}

func TestClientReadRawSupportsEmptyFile(t *testing.T) {
	t.Parallel()

	client := newTestReadRawClient(t, map[string][]byte{
		"/data/media/empty.txt": {},
	})
	reader, err := client.ReadRaw(context.Background(), "/data/media/empty.txt")
	if err != nil {
		t.Fatalf("ReadRaw returned error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read raw empty reader failed: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty payload, got %q", string(data))
	}
}

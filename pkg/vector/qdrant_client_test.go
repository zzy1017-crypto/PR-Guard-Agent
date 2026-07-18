package vector

import (
	"context"
	"net"
	"testing"
	"time"

	qdrantapi "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"

	"pr-guard-agent/internal/config"
)

type deleteCaptureServer struct {
	qdrantapi.UnimplementedPointsServer
	requests chan *qdrantapi.DeletePoints
}

func (s *deleteCaptureServer) Delete(_ context.Context, request *qdrantapi.DeletePoints) (*qdrantapi.PointsOperationResponse, error) {
	s.requests <- request
	return &qdrantapi.PointsOperationResponse{Result: &qdrantapi.UpdateResult{}}, nil
}

func TestChunkFilterRequiresProjectID(t *testing.T) {
	_, err := chunkFilter(SearchFilter{CodeVersionHash: "version-a"})
	if err == nil || err.Error() != "project_id is required" {
		t.Fatalf("chunkFilter() error = %v, want project_id validation", err)
	}
}

func TestChunkFilterRequiresCodeVersionHash(t *testing.T) {
	_, err := chunkFilter(SearchFilter{ProjectID: 7, CodeVersionHash: "  "})
	if err == nil || err.Error() != "code_version_hash is required" {
		t.Fatalf("chunkFilter() error = %v, want code_version_hash validation", err)
	}
}

func TestChunkFilterMatchesProjectAndCodeVersion(t *testing.T) {
	filter, err := chunkFilter(SearchFilter{ProjectID: 7, CodeVersionHash: " version-a "})
	if err != nil {
		t.Fatalf("chunkFilter() error = %v", err)
	}
	if len(filter.Must) != 2 {
		t.Fatalf("Must condition count = %d, want 2", len(filter.Must))
	}

	projectCondition := filter.Must[0].GetField()
	if projectCondition == nil || projectCondition.GetKey() != payloadProjectID ||
		projectCondition.GetMatch().GetInteger() != 7 {
		t.Fatalf("unexpected project condition: %#v", projectCondition)
	}

	versionCondition := filter.Must[1].GetField()
	if versionCondition == nil || versionCondition.GetKey() != payloadCodeVersionHash ||
		versionCondition.GetMatch().GetKeyword() != "version-a" {
		t.Fatalf("unexpected code version condition: %#v", versionCondition)
	}
}

func TestDeleteChunksSendsProjectAndVersionFilter(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for fake qdrant server: %v", err)
	}
	grpcServer := grpc.NewServer()
	capture := &deleteCaptureServer{requests: make(chan *qdrantapi.DeletePoints, 1)}
	qdrantapi.RegisterPointsServer(grpcServer, capture)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	port := listener.Addr().(*net.TCPAddr).Port
	client, err := NewClient(config.QdrantConfig{
		Host:           "127.0.0.1",
		Port:           port,
		CollectionName: "test-code-chunks",
		VectorSize:     3,
		TimeoutSeconds: 2,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	err = client.DeleteChunks(context.Background(), SearchFilter{
		ProjectID:       7,
		CodeVersionHash: "version-a",
	})
	if err != nil {
		t.Fatalf("DeleteChunks() error = %v", err)
	}

	select {
	case request := <-capture.requests:
		if request.GetCollectionName() != "test-code-chunks" || !request.GetWait() || request.GetTimeout() != 2 {
			t.Fatalf("unexpected delete options: %#v", request)
		}
		filter := request.GetPoints().GetFilter()
		if filter == nil || len(filter.Must) != 2 {
			t.Fatalf("unexpected delete filter: %#v", filter)
		}
		projectCondition := filter.Must[0].GetField()
		versionCondition := filter.Must[1].GetField()
		if projectCondition.GetKey() != payloadProjectID || projectCondition.GetMatch().GetInteger() != 7 ||
			versionCondition.GetKey() != payloadCodeVersionHash || versionCondition.GetMatch().GetKeyword() != "version-a" {
			t.Fatalf("unexpected delete conditions: %#v", filter.Must)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fake qdrant server did not receive delete request")
	}
}

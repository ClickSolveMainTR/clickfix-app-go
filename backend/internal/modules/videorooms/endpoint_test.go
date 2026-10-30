package videorooms

import (
	"context"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
)

// TestPublicEndpoint: an organization's own S3 endpoint must be public HTTPS
// (CF-153 added CGNAT / cloud metadata, NAT64 and other special ranges).
func TestPublicEndpoint(t *testing.T) {
	for _, raw := range []string{
		"http://s3.example.com", "https://localhost:9000", "https://seaweedfs:8333", "https://10.0.0.5",
		"https://169.254.169.254", "https://100.100.100.200", "https://[64:ff9b::a9fe:a9fe]", "https://[fd00:ec2::254]",
		"https://minio.internal", "https://nas.local", "https://user:pw@s3.example.com",
	} {
		if publicEndpoint(raw) == nil {
			t.Errorf("publicEndpoint(%q) accepted", raw)
		}
	}
	for _, raw := range []string{"https://s3.eu-central-1.amazonaws.com", "https://fra1.digitaloceanspaces.com"} {
		if err := publicEndpoint(raw); err != nil {
			t.Errorf("publicEndpoint(%q) = %v", raw, err)
		}
	}
}

// TestUntrustedS3DriverRefusesInternalEndpoint: the driver used for
// organization buckets refuses internal endpoints on its own, not only
// through the settings validation.
func TestUntrustedS3DriverRefusesInternalEndpoint(t *testing.T) {
	_, err := storage.NewS3Untrusted(context.Background(), config.ObjectStoreConfig{Endpoint: "https://169.254.169.254", Bucket: "b"})
	if err == nil {
		t.Fatal("driver accepted a link-local endpoint")
	}
}

package financialengine

import "testing"

func TestNewClientRejectsNonLoopbackAddresses(t *testing.T) {
	addresses := []string{
		"0.0.0.0:50051",
		"[::]:50051",
		"192.0.2.1:50051",
		"10.0.0.1:50051",
		"[2001:db8::1]:50051",
		"localhost:50051",
		"engine.invalid:50051",
		"dns:///127.0.0.1:50051",
		"[::ffff:127.0.0.1]:50051",
		"[::1%lo]:50051",
		"127.0.0.1:0",
		"127.0.0.1:65536",
		"127.0.0.1:not-a-port",
		"https://example.invalid/private-path?token=test",
	}
	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			client, err := NewClient(address)
			if client != nil {
				client.Close()
				t.Fatal("invalid address created a client")
			}
			if err == nil || err.Error() != "financial engine address must be a literal loopback IP with a nonzero port" {
				t.Fatalf("expected generic address rejection, got %v", err)
			}
		})
	}
}

func TestNewClientRequiresAddress(t *testing.T) {
	client, err := NewClient("")
	if client != nil || err == nil || err.Error() != "financial engine address is required" {
		t.Fatalf("expected missing address rejection, got %v", err)
	}
}

func TestNewClientAcceptsLiteralLoopbackAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1:50051", "127.0.0.2:50051", "[::1]:50051"} {
		t.Run(address, func(t *testing.T) {
			client, err := NewClient(address)
			if err != nil {
				t.Fatalf("create loopback client: %v", err)
			}
			if err := client.Close(); err != nil {
				t.Fatalf("close loopback client: %v", err)
			}
		})
	}
}

package discovery

import (
	"context"
	"fmt"
	"log"

	"github.com/grandcat/zeroconf"
)

// MDNSBeacon registers and announces media services via Multicast DNS (Bonjour/mDNS) on port 5353.
// This enables Smart TVs (Android TV, Apple TV, webOS, Roku) to autodiscover services.
type MDNSBeacon struct {
	instanceName string
	serviceType  string
	port         int
	txtRecords   []string
}

// NewMDNSBeacon creates an mDNS responder for a specific service type.
func NewMDNSBeacon(instanceName, serviceType string, port int, txtRecords []string) *MDNSBeacon {
	return &MDNSBeacon{
		instanceName: instanceName,
		serviceType:  serviceType,
		port:         port,
		txtRecords:   txtRecords,
	}
}

// Name returns the protocol identifier.
func (b *MDNSBeacon) Name() string {
	return fmt.Sprintf("mDNS (%s)", b.serviceType)
}

// Start registers the service with mDNS and runs until ctx is canceled.
func (b *MDNSBeacon) Start(ctx context.Context) error {
	server, err := zeroconf.Register(
		b.instanceName,
		b.serviceType,
		"local.",
		b.port,
		b.txtRecords,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service %s on port %d: %w", b.serviceType, b.port, err)
	}
	defer server.Shutdown()

	log.Printf("[mDNS Beacon] Broadcasting %s (%s) on port %d", b.instanceName, b.serviceType, b.port)

	<-ctx.Done()
	log.Printf("[mDNS Beacon] Stopping broadcast for %s", b.serviceType)
	return nil
}

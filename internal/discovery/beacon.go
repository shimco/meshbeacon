package discovery

import "context"

// Beacon defines a local network autodiscovery responder.
// Each discovery protocol (Jellyfin UDP, Plex GDM, mDNS) implements this interface.
type Beacon interface {
	// Name returns a human-readable identifier for the beacon.
	Name() string
	// Start begins listening and responding to discovery queries until ctx is canceled.
	Start(ctx context.Context) error
}

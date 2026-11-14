package agentcli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riba2534/herdrx/internal/tunnel"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

type connectRequest struct {
	Relay *tunnel.RelayBootstrap `json:"relay,omitempty"`
}

func relayProbePublic(cfg Config) string {
	if cfg.RelayProbePrivate.IsZero() {
		return ""
	}
	return cfg.RelayProbePrivate.Public().String()
}

func probeConfiguredRelay(ctx context.Context, cfg Config) []tunnel.DERPProbe {
	if cfg.RelayProbePrivate.IsZero() {
		return tunnel.ProbeDERPRegion(ctx, regionFromConfig(cfg))
	}
	return tunnel.ProbeDERPRegionWithKey(ctx, regionFromConfig(cfg), cfg.RelayProbePrivate)
}

// selectWorkbenchRelay chooses once, before publishing the full Tailcat
// address. The controller consequently rendezvous at the same relay. An
// explicit workbench request never falls back to public relays.
func selectWorkbenchRelay(ctx context.Context, cfg Config, bootstrap *tunnel.RelayBootstrap, nodes []key.NodePublic) (*tailcfg.DERPRegion, string, error) {
	if bootstrap == nil {
		region := regionFromConfig(cfg)
		if region == nil {
			// A nil region lets Tailcat pick a public DERP node.
			return nil, "", errors.New("no relay is pinned for this host; copy the connection command from your workbench's add-host page")
		}
		return region, "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	region, err := tunnel.RegisterWorkbenchRelay(ctx, *bootstrap, nodes, key.NewNode())
	if err != nil {
		return nil, "", fmt.Errorf("workbench relay unavailable; no public relay was used and the existing connection was kept. Check the workbench HTTPS reverse proxy, then copy a new command from the website: %w", err)
	}
	return region, "workbench", nil
}

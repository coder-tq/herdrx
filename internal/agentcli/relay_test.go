package agentcli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/herdrx/internal/secure"
	"github.com/riba2534/herdrx/internal/tunnel"
	"github.com/tailscale/tailcat"
	"tailscale.com/tailcfg"
	"tailscale.com/types/logger"
)

func TestWorkbenchSelectionFailsWithoutPublicFallback(t *testing.T) {
	public := &tailcfg.DERPRegion{RegionID: 1, RegionCode: "public", Nodes: []*tailcfg.DERPNode{{HostName: "8.8.8.8"}}}
	cfg := Config{Node: *tailcat.NewPrivateKey()}
	cfg.Node.Public.Region = []*tailcfg.DERPRegion{public}
	bootstrap := &tunnel.RelayBootstrap{Workbench: "https://127.0.0.1", Token: strings.Repeat("x", 43)}
	region, status, err := selectWorkbenchRelay(context.Background(), cfg, bootstrap, nil)
	if err == nil || region != nil || status != "" || !strings.Contains(err.Error(), "no public relay was used") {
		t.Fatalf("explicit workbench fell back: region=%v status=%q err=%v", region, status, err)
	}
	if cfg.Node.Public.Region[0] != public {
		t.Fatal("selection mutated persisted region")
	}
	region, status, err = selectWorkbenchRelay(context.Background(), cfg, nil, nil)
	if err != nil || region != public || status != "" {
		t.Fatal("plain connect unexpectedly changed the configured region")
	}
	cfg.Node.Public.Region = nil
	if region, _, err = selectWorkbenchRelay(context.Background(), cfg, nil, nil); err == nil || region != nil {
		t.Fatal("unpinned host fell through to public relay selection")
	}
}

func TestConnectWithFailedWorkbenchKeepsEnrollment(t *testing.T) {
	dm := tunnel.RunTestDERPAndSTUN(t, logger.Discard, "127.0.0.1")
	dir := shortTempDir(t)
	node := tailcat.NewPrivateKey()
	node.Public.Region = []*tailcfg.DERPRegion{dm.Regions[1]}
	hostPrivate, _, err := secure.GenerateSSHKey("relay-host")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := SafeSaveConfig(path, Config{Version: 1, Node: *node, SSHHostPrivate: string(hostPrivate), SetupID: "agent-relay"}); err != nil {
		t.Fatal(err)
	}
	store, err := NewStateStore(path)
	if err != nil {
		t.Fatal(err)
	}
	env := Environment{ConfigDir: dir, RuntimeDir: dir, HomeDir: dir}
	server := NewIPCServer(env, store)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	sock := filepath.Join(dir, "control.sock")
	bootstrap := &tunnel.RelayBootstrap{Workbench: "https://127.0.0.1", Token: strings.Repeat("x", 43)}

	noEnrollment := func(msg string) {
		t.Helper()
		server.mu.Lock()
		started := server.ephemServer != nil
		server.mu.Unlock()
		if saved, _ := store.Snapshot(); started || saved.Enrollment.EnrollmentID != "" || saved.Enrollment.ConnectionString != "" {
			t.Fatal(msg)
		}
	}
	var res ConnectResult
	if err := ClientCallIPC(sock, "POST", "/connect", connectRequest{Relay: bootstrap}, &res); err == nil || res.ConnectionString != "" {
		t.Fatalf("failed workbench produced a connection string: %+v", res)
	}
	noEnrollment("failed workbench started or persisted an enrollment")

	pinned := node.Public.Region
	if _, err := store.Update(func(c *Config) error { c.Node.Public.Region = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ClientCallIPC(sock, "POST", "/connect", nil, &res); err == nil || res.ConnectionString != "" || !strings.Contains(err.Error(), "workbench") {
		t.Fatalf("unpinned host produced a connection string: %v", err)
	}
	noEnrollment("unpinned host started or persisted an enrollment")
	if _, err := store.Update(func(c *Config) error { c.Node.Public.Region = pinned; return nil }); err != nil {
		t.Fatal(err)
	}

	// Without a bootstrap the configured relay remains the pairing path.
	if err := ClientCallIPC(sock, "POST", "/connect", nil, &res); err != nil || !strings.HasPrefix(res.ConnectionString, "herdrx://v1/") {
		t.Fatalf("plain connect failed: %v", err)
	}
	server.mu.Lock()
	ephem := server.ephemServer
	server.mu.Unlock()
	before, _ := store.Snapshot()
	var renewed ConnectResult
	if err := ClientCallIPC(sock, "POST", "/connect?renew=true", connectRequest{Relay: bootstrap}, &renewed); err == nil || renewed.ConnectionString != "" {
		t.Fatal("failed workbench renewed the enrollment")
	}
	after, _ := store.Snapshot()
	server.mu.Lock()
	kept := server.ephemServer == ephem && server.activeConnStr == res.ConnectionString
	server.mu.Unlock()
	if !kept || after.Enrollment.EnrollmentID != before.Enrollment.EnrollmentID || after.Binding.Epoch != before.Binding.Epoch {
		t.Fatal("failed workbench replaced the existing enrollment")
	}
}

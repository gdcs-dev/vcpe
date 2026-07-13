// Package types aggregates the built-in service types and registers them with
// the global type registry. The control plane calls Register once at startup.
package types

import (
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/bng"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/eventsink"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/gateway"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/genericcontainer"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/oktopus"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/webpa"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/xb10"
)

var registered bool

// Register installs every built-in service type. It is idempotent so tests and
// the daemon can call it freely.
func Register() {
	if registered {
		return
	}
	bng.Register()
	eventsink.Register()
	gateway.Register()
	oktopus.Register()
	webpa.Register()
	xb10.Register()
	genericcontainer.Register()
	registered = true
}

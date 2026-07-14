## 1. Interface Discovery

- [x] 1.1 Add `InterfaceInfo` struct to `internal/hostnet/adapter.go`: `Name`, `LinkType`, `OperState`, `Addresses []string`
- [x] 1.2 Add `ListInterfaces(ctx context.Context) ([]InterfaceInfo, error)` to `hostnet.Adapter` using `ip -j link` via `runner()` — parse JSON output, filter out loopback (`lo`), podman/cni virtual interfaces, and macvlan sub-interfaces (`link_type == "macvlan"`)
- [x] 1.3 Add `ListInterfaces` unit test covering: normal interfaces included, loopback excluded, podman bridges excluded

## 2. Prompt Helper

- [x] 2.1 Create `internal/app/wizard/prompt.go`: `Prompt(w io.Writer, r io.Reader, label, defaultVal string) string` — writes `label [defaultVal]: ` to `w`; returns `defaultVal` immediately when `r` is not a TTY or when the input line is empty; strips trailing newline from non-empty input
- [x] 2.2 Create `internal/app/wizard/prompt_test.go` covering: empty input returns default, non-empty input returns trimmed value, non-TTY reader returns default

## 3. Network Phase

- [x] 3.1 Create `internal/app/wizard/network.go`: `AskNetworks(ctx, r, w, existing []manifest.Network, ha *hostnet.Adapter) []manifest.Network`
- [x] 3.2 For each network: prompt role, driver (default `bridge`), bridge override (optional)
- [x] 3.3 When driver is `macvlan` or `ipvlan`: call `ha.ListInterfaces(ctx)`, display numbered menu with name/type/state/addresses; fall back to free-text on error; store selection in `driverOptions["parent"]`
- [x] 3.4 For bridge networks: prompt `nat` (bool), `firewall` (bool); skip both for non-bridge drivers
- [x] 3.5 Prompt optional `ipv4` block: cidr, gateway, pool start/end (all with sensible defaults or empty)
- [x] 3.6 Return collected `[]manifest.Network` and a `map[string]networkEntry` lookup for the service phase

## 4. Service Phase

- [x] 4.1 Create `internal/app/wizard/service.go`: `AskServices(ctx, r, w, existing []manifest.Service, nets map[string]networkEntry, types []string) []manifest.Service`
- [x] 4.2 For each service: prompt name, type (menu from registered types), image repo/tag/pullPolicy/buildContext, dependsOn (multi-select from already-defined service names)
- [x] 4.3 Prompt interface attachments: for each interface, select role (from known network roles), optional static ipv4, defaultRoute bool
- [x] 4.4 Dispatch to type-specific config asker:
  - `case "bng"`: `askBNGConfig(r, w, nets, interfaces)` — walk access[] per attached network role; pre-fill dhcp4.subnet/ranges/options.routers from network entry; prompt leaseSeconds [3600]
  - `case "gateway"`: `askGatewayConfig(r, w, nets, interfaces)` — pre-fill lan.ipv4 cidr from lan-p1 network, dhcpStart/End from pool
  - `case "generic-container"`: prompt command (csv), env (key=value pairs), ports, volumes
  - default: skip config (emit empty config node)

## 5. Output Phase

- [x] 5.1 Create `internal/app/wizard/output.go`: `AskOutput(r, w, defaultPath string) string` — prompt output path; serialize `manifest.Document` to YAML using `gopkg.in/yaml.v3` encoder with 2-space indent; write to path; print confirmation
- [x] 5.2 Add `Run(ctx, opts WizardOpts) error` to `internal/app/wizard/wizard.go`: orchestrates phases 1-4, handles `--manifest` load for update mode, calls AskOutput

## 6. Command Wiring

- [x] 6.1 Add `case "build"` to `runManifest()` in `internal/app/commands.go` dispatching to `wizard.Run`
- [x] 6.2 Add `--output` flag parsing to `parseArgs()` in `internal/app/cli.go` for the `manifest build` subcommand; store in `opts.OutputPath`
- [x] 6.3 Update `commandHelp["manifest"]` in `internal/app/help.go`: update synopsis to "Manage and discover manifest files"; add examples for `vcpe manifest build` and `vcpe manifest build --manifest existing.yaml`

## 7. Spec Sync

- [x] 7.1 Apply ADDED `vcpe manifest build wizard` requirement to `openspec/specs/manifest-discovery/spec.md`
- [x] 7.2 Apply MODIFIED `Declarative local control-plane commands` (adds `manifest build` scenario) to `openspec/specs/local-control-plane-cli/spec.md`

## 8. Verification

- [x] 8.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 8.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 8.3 Run `controlplane/bin/vcpe manifest build --help` — output documents `--manifest` and `--output` flags
- [x] 8.4 Run `controlplane/bin/vcpe manifest --help` — lists `list` and `build` subcommands
- [x] 8.5 Pipe stdin: `echo "" | controlplane/bin/vcpe manifest build` — completes without hanging, writes manifest with all defaults to stdout or default path

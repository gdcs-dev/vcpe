## 1. Go — ServiceType interface (BREAKING refactors)

- [x] 1.1 Add `Description() string` and `DefaultImage() string` to `ServiceType` interface in `controlplane/internal/typeregistry/registry.go`
- [x] 1.2 Implement `Description()` and `DefaultImage()` in `controlplane/internal/types/bng/bng.go`
- [x] 1.3 Implement `Description()` and `DefaultImage()` in `controlplane/internal/types/gateway/gateway.go`
- [x] 1.4 Implement `Description()` and `DefaultImage()` in `controlplane/internal/types/webpa/webpa.go`
- [x] 1.5 Implement `Description()` and `DefaultImage()` in `controlplane/internal/types/eventsink/eventsink.go`
- [x] 1.6 Implement `Description()` and `DefaultImage()` in `controlplane/internal/types/genericcontainer/genericcontainer.go`
- [x] 1.7 Implement `Description()` and `DefaultImage()` in `controlplane/internal/types/oktopus/oktopus.go`
- [x] 1.8 Implement `Description()` and `DefaultImage()` in `controlplane/internal/types/xb10/xb10.go`
- [x] 1.9 Fix `bng.ExpectedRoles()` from `nil` to `[{wan, required}, {cm, optional}, {mgmt, optional}]` in `bng.go` (BREAKING: BNG services without a `wan` interface will fail preflight)
- [x] 1.10 Fix `gateway.ExpectedRoles()` from `[{lan}, {erouter}]` to `[{wan, required}, {cm, optional}, {lan-p1, optional}]` in `gateway.go` (BREAKING: gateway services without a `wan` interface will fail preflight)
- [x] 1.11 Update `controlplane/internal/typeregistry/registry_test.go` to verify `Description()` and `DefaultImage()` are non-panicking for all registered types

## 2. Go — vcpe service types command

- [x] 2.1 Add `"service"` to `topLevelCommands` map in `controlplane/internal/app/cli.go`
- [x] 2.2 Add `case "service":` dispatch to `executeLocal` switch in `controlplane/internal/app/local.go`
- [x] 2.3 Implement `runService(opts Options)` subcommand dispatcher (routes `types`) in `controlplane/internal/app/commands.go`
- [x] 2.4 Implement `runServiceTypes(opts Options)` — human-readable table output (NAME, DESCRIPTION, PULL_POLICY columns)
- [x] 2.5 Implement `runServiceTypes` JSON branch — emit `{"types": [...]}` with `name`, `description`, `defaultPullPolicy`, `defaultImage`, `expectedRoles` per type
- [x] 2.6 Add help text for `service` command group and `service types` subcommand to `controlplane/internal/app/help.go`
- [x] 2.7 Write unit tests for `runServiceTypes`: verify JSON structure, verify table output, verify unknown subcommand error

## 3. Extension scaffold

- [x] 3.1 Create `extensions/vcpe-visual-editor/` directory with `package.json`
- [x] 3.2 Write `extensions/vcpe-visual-editor/tsconfig.json` for extension host compilation
- [x] 3.3 Write `extensions/vcpe-visual-editor/esbuild.js` extension host bundle script
- [x] 3.4 Write `extensions/vcpe-visual-editor/webview/vite.config.ts` (React, TypeScript, output to `webview/dist/`)
- [x] 3.5 Write `extensions/vcpe-visual-editor/webview/tsconfig.json`
- [x] 3.6 Add `build-extension` target to root `Makefile` (`npm install && npm run build` in extension dir, then `vsce package`)
- [x] 3.7 Add `install-extension` target to root `Makefile` (`make build-extension` then `code --install-extension`)
- [x] 3.8 Add `extensions/vcpe-visual-editor/webview/dist/` to `.gitignore`

## 4. Extension host core

- [x] 4.1 Write `src/extension.ts`
- [x] 4.2 Write `src/VcpeBinaryClient.ts`
- [x] 4.3 Write `src/ManifestScanner.ts`
- [x] 4.4 Write `src/LayoutStore.ts`
- [x] 4.5 Write `src/VcpeEditorProvider.ts`
- [x] 4.6 Implement webview HTML template in `VcpeEditorProvider` with strict CSP

## 5. YAML round-trip

- [x] 5.1 Write `webview/src/yaml/parse.ts`
- [x] 5.2 Write `webview/src/yaml/serialize.ts` — easy mutations
- [x] 5.3 Write `webview/src/yaml/serialize.ts` — medium mutations
- [x] 5.4 Write `webview/src/yaml/serialize.ts` — hard mutations (cross-reference cleanup)
- [x] 5.5 Write `webview/src/yaml/parse.test.ts`
- [x] 5.6 Write `webview/src/yaml/serialize.test.ts`

## 6. Canvas foundation

- [x] 6.1 Install npm dependencies: `@xyflow/react`, `yaml`, `@dagrejs/dagre`, `zustand`, `lucide-react`
- [x] 6.2 Write `webview/src/store/manifestStore.ts`
- [x] 6.3 Write `webview/src/App.tsx`

## 7. Canvas node and edge types

- [x] 7.1 Write `webview/src/nodes/NetworkBusNode.tsx`
- [x] 7.2 Write `webview/src/nodes/ServiceNode.tsx`
- [x] 7.3 Write `webview/src/nodes/PhysicalNicNode.tsx`
- [x] 7.4 Write `webview/src/edges/InterfaceEdge.tsx`
- [x] 7.5 Write `webview/src/edges/DependsOnEdge.tsx`
- [x] 7.6 Write `webview/src/utils/roleColor.ts`

## 8. Auto-layout

- [x] 8.1 Write `webview/src/layout/autoLayout.ts`
- [x] 8.2 Integrate `autoLayout` in `App.tsx`

## 9. Type palette

- [x] 9.1 Write `webview/src/panels/TypePalette.tsx`
- [x] 9.2 Implement drag-from-palette → canvas drop handler
- [x] 9.3 Render error card in palette when binary unavailable

## 10. Property panel

- [x] 10.1 Write `webview/src/panels/PropertyPanel.tsx`
- [x] 10.2 Write `NetworkForm` (role read-only with tooltip)
- [x] 10.3 Write `ServiceForm` (raw YAML textarea for config)
- [x] 10.4 Write `InterfaceForm` fields (inline in PropertyPanel)
- [x] 10.5 Write `DeploymentSettingsDrawer`

## 11. Manifest dropdown and welcome screen

- [x] 11.1 Write `webview/src/panels/ManifestDropdown.tsx`
- [x] 11.2 Implement New Manifest flow
- [x] 11.3 Write `WelcomeScreen` component

## 12. DependsOn toggle

- [x] 12.1 Add "⇢ Dependencies" toggle button to canvas toolbar in `App.tsx`

## 13. Verification

- [x] 13.1 Run `cd controlplane && go build ./...` — confirm no compile errors after ServiceType interface additions
- [x] 13.2 Run `cd controlplane && go test ./...` — confirm all Go tests pass, including new `runServiceTypes` and updated typeregistry tests
- [x] 13.3 Run `npm test` in `extensions/vcpe-visual-editor/` — confirm all Vitest YAML round-trip tests pass
- [x] 13.4 Run `npm run build` in `extensions/vcpe-visual-editor/` — confirm clean TypeScript/Vite build
- [x] 13.5 Run `make install-extension` and open `manifests/example.yaml` via "Open With → vCPE Visual Manifest Editor" — verify full topology renders with correct NetworkBusNodes, ServiceNodes, and InterfaceEdges
- [x] 13.6 Manual smoke: drag `bng` from type palette onto canvas, wire it to the `wan` network bus — verify YAML `services` and `interfaces` sections updated correctly
- [x] 13.7 Manual smoke: delete `gateway` service node — verify `bng` dependsOn cleanup and all gateway interface cross-references are removed from YAML in one edit
- [x] 13.8 Manual smoke: open `manifests/example-macvlan.yaml` — verify `PhysicalNicNode` appears for the macvlan parent NIC and the `wan` network bus is anchored to it
- [x] 13.9 Manual smoke: toggle "⇢ Dependencies" toolbar button — verify DependsOnEdge arrows appear and disappear

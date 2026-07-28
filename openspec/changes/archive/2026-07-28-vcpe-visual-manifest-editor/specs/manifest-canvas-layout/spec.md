## ADDED Requirements

### Requirement: Canvas layout sidecar file
The visual editor SHALL persist canvas node positions to a sidecar file named `<manifest-basename>.vcpe-layout.json` alongside each manifest file (e.g., `manifests/example.vcpe-layout.json`). The sidecar SHALL be a valid JSON file with schema `{ "version": 1, "nodes": { "<kind>:<identifier>": { "x": <number>, "y": <number> } } }`. Node identifiers SHALL use the format `network:<role>`, `service:<name>`, or `nic:<parent>`. The sidecar SHALL be committed to version control as shared team context. The webview `dist/` directory SHALL be gitignored; the sidecar SHALL not be.

#### Scenario: Sidecar created on first open
- **WHEN** the visual editor opens a manifest with no existing `.vcpe-layout.json` sidecar
- **THEN** an initial layout is computed and a sidecar file is written alongside the manifest

#### Scenario: Sidecar loaded on subsequent opens
- **WHEN** the visual editor opens a manifest with an existing `.vcpe-layout.json` sidecar
- **THEN** node positions are restored from the sidecar file

#### Scenario: Sidecar updated on canvas position change
- **WHEN** the user drags a node to a new position on the canvas
- **THEN** the sidecar file is updated with the new x/y coordinates for that node's ID

### Requirement: Unknown sidecar entries are silently ignored
If the sidecar contains node IDs that do not correspond to any element in the current manifest (e.g., a service was deleted since the sidecar was last written), those entries SHALL be silently ignored. The stale entries SHALL be pruned from the sidecar the next time a position change triggers a sidecar write.

#### Scenario: Deleted service node ID in sidecar is ignored
- **WHEN** the sidecar references `service:old-svc` but the manifest no longer has a service named `old-svc`
- **THEN** the canvas opens without error and `old-svc` is not rendered; the stale entry is removed from the sidecar on the next write

### Requirement: Dagre-seeded initial layout
When no sidecar exists, the visual editor SHALL compute initial canvas positions using the `@dagrejs/dagre` library. Network buses SHALL be assigned fixed vertical positions sorted by total interface connection count (most-connected network at top). Service nodes SHALL be laid out by dagre using `dependsOn` relationships as DAG edges. The resulting positions SHALL be immediately persisted to a new sidecar file. Dagre SHALL NOT run again for that manifest as long as the sidecar exists.

#### Scenario: Initial layout seeds sidecar via dagre
- **WHEN** the visual editor opens a manifest for the first time (no sidecar present)
- **THEN** dagre computes non-overlapping initial positions for all service nodes, a sidecar is created, and subsequent opens restore those positions from the sidecar

#### Scenario: Dagre does not re-run when sidecar exists
- **WHEN** the visual editor opens a manifest that has an existing sidecar
- **THEN** dagre is not invoked; positions come entirely from the sidecar

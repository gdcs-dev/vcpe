## 1. ServiceNode — Bridge Header

- [x] 1.1 Remove the `<Handle>` from bridge group header rows in `ServiceNode.tsx`
- [x] 1.2 Verify bridge header still renders `▣` icon, bridge name, IP label, and purple styling

## 2. ServiceNode — Bridge Member Rows

- [x] 2.1 Add `position: 'relative'` to bridge member row `div` elements
- [x] 2.2 Add right-side `<Handle type="source" position={Position.Right} id={\`iface-${m.role}\`}>` with `10×10` role color styling
- [x] 2.3 Add left-side `<Handle type="source" position={Position.Left} id={\`iface-${m.role}-left\`}>` with matching `10×10` role color styling

## 3. ServiceNode — Non-Bridged Interface Rows

- [x] 3.1 Add left-side `<Handle type="source" position={Position.Left} id={\`iface-${n.role}-left\`}>` to each non-bridged interface row with `10×10` role color styling

## 4. App.tsx — Handle Resolution Cleanup

- [x] 4.1 Delete the `svcBridgeForRole` map and its population loop
- [x] 4.2 Delete the `resolveHandle` helper function
- [x] 4.3 Replace both `resolveHandle(a, role)` and `resolveHandle(b, role)` calls in edge-building with `\`iface-${role}\`` directly

## 5. Verification

- [ ] 5.1 Open `manifests/example.yaml` in the visual editor; confirm edges from `client` land on the `lan-p2` member row inside `brlan1`, not on the `brlan1` header
- [ ] 5.2 Confirm `brlan0` and `brlan1` header rows show no drag handle and cannot be used to initiate a connection
- [ ] 5.3 Confirm all interface rows (bridged and non-bridged) show handles on both left and right sides
- [ ] 5.4 Confirm bridge member handles are `10×10` and role-colored, matching non-bridged interface handles
- [x] 5.5 Run `npm run build` (or `vite build`) in `extensions/vcpe-visual-editor` and confirm no TypeScript errors

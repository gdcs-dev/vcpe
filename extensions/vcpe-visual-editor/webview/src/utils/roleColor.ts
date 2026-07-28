// 10-color accessible palette for network role → edge/node color assignment.
// Colors are chosen for contrast on both light and dark VS Code themes.
const PALETTE = [
  '#4E9AF1', // blue
  '#E67E22', // orange
  '#9B59B6', // purple
  '#27AE60', // green
  '#16A085', // teal
  '#E74C3C', // red
  '#F39C12', // amber
  '#2980B9', // dark blue
  '#8E44AD', // dark purple
  '#1ABC9C', // turquoise
] as const;

/** Hashes a network role name to a stable color from the palette. */
export function roleColor(role: string): string {
  let hash = 0;
  for (let i = 0; i < role.length; i++) {
    hash = (hash * 31 + role.charCodeAt(i)) >>> 0;
  }
  return PALETTE[hash % PALETTE.length];
}

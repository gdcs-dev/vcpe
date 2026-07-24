// VS Code's acquireVsCodeApi() MUST be called exactly once per webview session.
// Calling it a second time throws "An instance of the VS Code API has already
// been acquired". This module holds the singleton so every component imports
// the same instance instead of calling acquireVsCodeApi() directly.

declare function acquireVsCodeApi(): { postMessage: (msg: unknown) => void };

export const vscodeApi: { postMessage: (msg: unknown) => void } =
  typeof acquireVsCodeApi !== 'undefined'
    ? acquireVsCodeApi()
    : { postMessage: () => {} };

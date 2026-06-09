import "@testing-library/jest-dom/vitest";

// jsdom does not implement ResizeObserver. Provide a no-op mock so components
// that observe element size (e.g. TrendChart) do not invoke their resize
// callback with malformed entries — which otherwise throws an unhandled error
// in the test environment. A no-op observer simply never delivers a resize
// entry; the suite does not assert on measured widths, so this is sufficient.
class ResizeObserverMock {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}
globalThis.ResizeObserver = ResizeObserverMock as unknown as typeof ResizeObserver;

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

// jsdom's localStorage is shadowed by Node's own experimental implementation,
// which reports itself unavailable unless the process was started with
// --localstorage-file. The result is that `window.localStorage` is undefined
// under test while it exists in every real browser — so components that use it
// would be tested against the wrong world.
//
// An in-memory Storage restores the browser's behaviour. Code under test is
// still expected to cope with storage being absent, because it genuinely can
// be (private browsing, a locked-down profile); that path is asserted
// explicitly rather than left to the environment.
class MemoryStorage implements Storage {
  private entries = new Map<string, string>();

  get length(): number {
    return this.entries.size;
  }
  clear(): void {
    this.entries.clear();
  }
  getItem(key: string): string | null {
    return this.entries.has(key) ? (this.entries.get(key) as string) : null;
  }
  key(index: number): string | null {
    return [...this.entries.keys()][index] ?? null;
  }
  removeItem(key: string): void {
    this.entries.delete(key);
  }
  setItem(key: string, value: string): void {
    this.entries.set(key, String(value));
  }
}

if (typeof window !== "undefined" && !window.localStorage) {
  Object.defineProperty(window, "localStorage", {
    value: new MemoryStorage(),
    configurable: true,
  });
}

import React from "react";
import { describe, it, vi, expect, afterEach } from "vitest";
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
} from "@testing-library/react";
import { useDebounced } from "./lib";
import { Subscriptions } from "./pages/Subscriptions";
import type { Console } from "./console";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
describe("subscriptions page", () => {
  it("shows provider usage, the download node and skipped items", () => {
    const ctx = {
      cfg: {
        version: 3,
        nodes: [{ id: "relay", name: "Relay", disabled: false, outbound: {} }],
        groups: [],
        entrances: [],
        subscriptions: [
          {
            id: "s",
            name: "Provider",
            url: "<redacted>",
            enabled: true,
            interval_seconds: 3600,
            via: "relay",
          },
        ],
      },
      busy: false,
      subStatus: [
        {
          id: "s",
          pending: false,
          pending_count: 0,
          removed: 0,
          count: 12,
          skipped: 2,
          last_check: "2026-01-01T00:00:00Z",
          next_check: "2026-01-01T01:00:00Z",
          usage: {
            upload: 1024 ** 3,
            download: 3 * 1024 ** 3,
            total: 100 * 1024 ** 3,
            expire: 1893456000,
          },
          issues: [
            { item: 3, name: "WG", error: "unsupported scheme wireguard" },
          ],
        },
      ],
      action: vi.fn(),
      setModal: vi.fn(),
      load: vi.fn(),
      tell: vi.fn(),
      remove: vi.fn(),
    } as unknown as Console;
    render(<Subscriptions ctx={ctx} />);
    expect(screen.getByText(/已用 4.0 GiB \/ 100.0 GiB/)).toBeTruthy();
    expect(screen.getByText(/经由 Relay 下载/)).toBeTruthy();
    expect(screen.getByText(/已跳过 2 个不支持的条目/)).toBeTruthy();
    expect(screen.getByText(/unsupported scheme wireguard/)).toBeTruthy();
    expect(screen.queryByText("接受更新")).toBeNull();
  });
  it("debounces values", () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(({ v }) => useDebounced(v, 250), {
      initialProps: { v: "a" },
    });
    rerender({ v: "ab" });
    rerender({ v: "abc" });
    expect(result.current).toBe("a");
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(result.current).toBe("abc");
  });
});

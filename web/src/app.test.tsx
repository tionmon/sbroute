import React from "react";
import { describe, it, vi, expect, afterEach } from "vitest";
import {
  cleanup,
  render,
  screen,
  within,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "./main";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
describe("management workflow", () => {
  it("imports a node, creates an ordered group, and creates a proxy entrance", async () => {
    let config: any = {
      version: 1,
      nodes: [],
      groups: [],
      entrances: [],
      subscriptions: [],
      resources: {
        preset: "balanced",
        memory_mib: 512,
        max_sessions: 8192,
        max_udp: 2048,
        max_dials: 64,
        probe_concurrency: 4,
        probe_interval_seconds: 120,
        stats_interval_seconds: 60,
        udp_timeout_seconds: 60,
      },
      health: {
        urls: ["https://example.com/"],
        udp_dns: "1.1.1.1:53",
        timeout_seconds: 5,
        failure_threshold: 3,
        recovery_threshold: 2,
      },
    };
    const node = {
      id: "tokyo",
      name: "Tokyo",
      disabled: false,
      outbound: {
        type: "socks",
        server: "127.0.0.1",
        server_port: 1080,
        _redacted: true,
      },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string, init?: RequestInit) => {
        const method = init?.method || "GET";
        const body = init?.body ? JSON.parse(init.body as string) : null;
        let value: any = {};
        if (input === "/api/v1/config" && method === "GET") value = config;
        else if (input === "/api/v1/config" && method === "PUT") {
          config = { ...body, version: config.version + 1 };
          value = config;
        } else if (input === "/api/v1/imports/preview")
          value = {
            preview_id: "preview",
            nodes: [node],
            duplicates: 0,
            issues: [],
          };
        else if (input === "/api/v1/imports/apply") {
          config = { ...config, version: config.version + 1, nodes: [node] };
          value = config;
        } else if (input.startsWith("/api/v1/stats"))
          value = {
            version: "0.1.0",
            core_version: "1.14.4",
            active: 0,
            udp: 0,
            pending: 0,
            upload: 0,
            download: 0,
            rejected: 0,
            rss_bytes: 20000000,
            heap_bytes: 10000000,
            uptime_seconds: 10,
            nodes: [],
            entrances: [],
            draining_versions: 0,
          };
        else if (
          [
            "/api/v1/events",
            "/api/v1/revisions",
            "/api/v1/presets",
            "/api/v1/subscriptions/status",
          ].includes(input)
        )
          value = [];
        return new Response(JSON.stringify(value), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    const u = userEvent.setup();
    render(<App />);
    await screen.findByRole("heading", { name: "运行概览" });
    await u.click(screen.getByRole("button", { name: /出口节点/ }));
    await u.click(screen.getAllByRole("button", { name: "导入节点" })[0]);
    let dialog = screen.getByRole("dialog");
    await u.type(
      within(dialog).getByPlaceholderText(/vless/),
      "socks5://127.0.0.1:1080#Tokyo",
    );
    await u.click(within(dialog).getByRole("button", { name: "解析预览" }));
    await within(dialog).findByText("Tokyo");
    await u.click(within(dialog).getByRole("button", { name: "确认导入" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await u.click(screen.getByRole("button", { name: "节点分组" }));
    await u.click(screen.getAllByRole("button", { name: "新建分组" })[0]);
    dialog = screen.getByRole("dialog");
    await u.type(within(dialog).getByLabelText("分组名称"), "Japan");
    await u.click(within(dialog).getByRole("checkbox", { name: "Tokyo" }));
    await u.click(within(dialog).getByRole("button", { name: "保存并应用" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(config.groups[0].members).toEqual(["tokyo"]);
    await u.click(screen.getByRole("button", { name: "分发入口" }));
    await u.click(screen.getAllByRole("button", { name: "新建入口" })[0]);
    dialog = screen.getByRole("dialog");
    await u.type(within(dialog).getByLabelText("入口名称"), "Japan proxy");
    await u.selectOptions(
      within(dialog).getByLabelText("节点组"),
      config.groups[0].id,
    );
    await u.click(within(dialog).getByRole("button", { name: "保存并应用" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(config.entrances[0]).toMatchObject({
      name: "Japan proxy",
      kind: "mixed",
      port: 10001,
      strategy: "round_robin",
      enabled: true,
    });
    await screen.findByText("Japan proxy");
  });
  it("shows the login form when there is no session", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: "authentication required" }), {
            status: 401,
          }),
      ),
    );
    render(<App />);
    await screen.findByRole("heading", { name: "登录控制台" });
    expect(screen.getByLabelText("管理员账号")).toBeTruthy();
    expect(screen.getByLabelText("密码")).toBeTruthy();
  });
});

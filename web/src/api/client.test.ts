import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "./client";

function mockFetch(status: number, body: unknown, headers: Record<string, string> = {}) {
  const text = typeof body === "string" ? body : JSON.stringify(body);
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: "",
    text: () => Promise.resolve(text),
    headers: new Headers(headers),
  } as Response);
}

afterEach(() => {
  vi.restoreAllMocks();
  document.cookie = "cw_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT";
});

describe("api", () => {
  it("returns parsed JSON on success", async () => {
    vi.stubGlobal("fetch", mockFetch(200, { hello: "world" }));
    await expect(api("/x")).resolves.toEqual({ hello: "world" });
  });

  it("returns undefined on 204", async () => {
    vi.stubGlobal("fetch", mockFetch(204, ""));
    await expect(api("/x", { method: "POST" })).resolves.toBeUndefined();
  });

  it("throws ApiError with server message", async () => {
    vi.stubGlobal("fetch", mockFetch(403, { error: "bad or missing CSRF token" }));
    await expect(api("/x", { method: "POST" })).rejects.toMatchObject({
      name: "ApiError",
      status: 403,
      message: "bad or missing CSRF token",
    });
    expect(new ApiError(1, "m")).toBeInstanceOf(Error);
  });

  it("sends the CSRF header from the cookie on mutations", async () => {
    document.cookie = "cw_csrf=tok123";
    const f = mockFetch(204, "");
    vi.stubGlobal("fetch", f);
    await api("/x", { method: "POST", body: { a: 1 } });
    const init = f.mock.calls[0][1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-CSRF-Token"]).toBe("tok123");
    expect((init.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
  });

  it("omits the CSRF header on GET", async () => {
    document.cookie = "cw_csrf=tok123";
    const f = mockFetch(200, {});
    vi.stubGlobal("fetch", f);
    await api("/x");
    const init = f.mock.calls[0][1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-CSRF-Token"]).toBeUndefined();
  });
});

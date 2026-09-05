import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ConfView } from "../api/client";

const getConfig = vi.fn();
const putConfig = vi.fn();
vi.mock("../api/client", () => ({
  getConfig: () => getConfig(),
  putConfig: (...a: unknown[]) => putConfig(...a),
}));

import ConfEditor from "./ConfEditor";

const view: ConfView = {
  which: "clamd",
  path: "/etc/clamav/clamd.conf",
  unit: "clamav-daemon",
  entries: [
    { name: "MaxThreads", kind: "int", repeatable: false, help: "", values: ["12"] },
    { name: "LogVerbose", kind: "bool", repeatable: false, help: "", values: ["no"] },
    { name: "OnAccessIncludePath", kind: "path", repeatable: true, help: "", values: ["/home"] },
  ],
};

beforeEach(() => {
  getConfig.mockReset();
  putConfig.mockReset();
});

describe("ConfEditor", () => {
  it("sends only changed keys", async () => {
    getConfig.mockResolvedValue(view);
    putConfig.mockResolvedValue({ config: view, restarted: true });
    render(<ConfEditor which="clamd" />);

    await waitFor(() => expect(screen.getByDisplayValue("12")).toBeInTheDocument());

    const maxThreads = screen.getByDisplayValue("12");
    await userEvent.clear(maxThreads);
    await userEvent.type(maxThreads, "4");

    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(putConfig).toHaveBeenCalledWith("clamd", { MaxThreads: ["4"] }, true),
    );
  });

  it("disables save until something changes", async () => {
    getConfig.mockResolvedValue(view);
    render(<ConfEditor which="clamd" />);
    await waitFor(() => expect(screen.getByDisplayValue("12")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});

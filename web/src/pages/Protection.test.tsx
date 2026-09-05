import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { OnAccessStatus } from "../api/client";

const getOnAccess = vi.fn();
const putOnAccess = vi.fn();
vi.mock("../api/client", () => ({
  getOnAccess: () => getOnAccess(),
  putOnAccess: (c: unknown) => putOnAccess(c),
  serviceAction: vi.fn(),
}));
vi.mock("../lib/useEvents", () => ({ useEvents: () => {} }));

import Protection from "./Protection";

const disabled: OnAccessStatus = {
  supported: true,
  daemon_active: true,
  clamonacc: {
    unit: "clamav-clamonacc", load: "loaded", active: "inactive", sub: "dead",
    enabled: "disabled", installed: true, since_unix: 0,
  },
  enabled: false,
  watch_paths: ["/srv"],
  exclude_paths: [],
  exclude_unames: ["clamav"],
  prevention: false,
};

beforeEach(() => {
  getOnAccess.mockReset();
  putOnAccess.mockReset();
});

describe("Protection page", () => {
  it("enables on-access with the current watch paths", async () => {
    getOnAccess.mockResolvedValue(disabled);
    putOnAccess.mockResolvedValue({ ...disabled, enabled: true });
    render(<Protection />);

    await waitFor(() => expect(screen.getByRole("button", { name: "Enable" })).toBeEnabled());
    await userEvent.click(screen.getByRole("button", { name: "Enable" }));
    await waitFor(() =>
      expect(putOnAccess).toHaveBeenCalledWith(expect.objectContaining({ enabled: true, paths: ["/srv"] })),
    );
  });

  it("prompts to install when clamonacc is absent", async () => {
    getOnAccess.mockResolvedValue({ ...disabled, supported: false });
    render(<Protection />);
    await waitFor(() => expect(screen.getByText(/on-access scanner is not installed/)).toBeInTheDocument());
  });
});

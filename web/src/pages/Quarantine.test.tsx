import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { QuarantineItem } from "../api/client";

const getQuarantine = vi.fn();
const restoreQuarantine = vi.fn();
const deleteQuarantine = vi.fn();
vi.mock("../api/client", () => ({
  getQuarantine: () => getQuarantine(),
  restoreQuarantine: (id: number) => restoreQuarantine(id),
  deleteQuarantine: (id: number) => deleteQuarantine(id),
}));
vi.mock("../lib/useEvents", () => ({ useEvents: () => {} }));

import Quarantine from "./Quarantine";

const held: QuarantineItem = {
  id: 3,
  store_name: "abc",
  orig_path: "/srv/data/eicar.com",
  signature: "Win.Test.EICAR_HDB-1",
  sha256: "deadbeef",
  status: "held",
  orig_mode: 420,
  created_at: "2024-09-20 08:00:00",
  updated_at: "2024-09-20 08:00:00",
};

beforeEach(() => {
  getQuarantine.mockReset();
  restoreQuarantine.mockReset();
  deleteQuarantine.mockReset();
});

describe("Quarantine page", () => {
  it("lists held items and restores one", async () => {
    getQuarantine.mockResolvedValue({ items: [held] });
    restoreQuarantine.mockResolvedValue({ ...held, status: "restored" });
    render(<Quarantine />);

    await waitFor(() => expect(screen.getByText("/srv/data/eicar.com")).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: "Restore" }));
    expect(restoreQuarantine).toHaveBeenCalledWith(3);
  });

  it("confirms before deleting", async () => {
    getQuarantine.mockResolvedValue({ items: [held] });
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<Quarantine />);

    await waitFor(() => expect(screen.getByText("/srv/data/eicar.com")).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(confirmSpy).toHaveBeenCalled();
    expect(deleteQuarantine).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("shows an empty state", async () => {
    getQuarantine.mockResolvedValue({ items: [] });
    render(<Quarantine />);
    await waitFor(() => expect(screen.getByText("Nothing quarantined.")).toBeInTheDocument());
  });
});

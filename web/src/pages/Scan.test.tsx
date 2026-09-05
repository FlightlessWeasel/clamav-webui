import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

const createScan = vi.fn();
const browse = vi.fn();
vi.mock("../api/client", () => ({
  createScan: (...a: unknown[]) => createScan(...a),
  browse: (...a: unknown[]) => browse(...a),
  getJob: vi.fn().mockResolvedValue({ id: 1, status: "running", log: "" }),
  getScanFindings: vi.fn().mockResolvedValue({ findings: [] }),
  quarantineFile: vi.fn(),
}));
vi.mock("../lib/useEvents", () => ({ useEvents: () => {} }));

import Scan from "./Scan";

beforeEach(() => {
  createScan.mockReset();
  browse.mockReset();
  browse.mockResolvedValue({
    path: "/srv",
    parent: "/",
    root: "/",
    entries: [
      { name: "data", path: "/srv/data", is_dir: true },
      { name: "readme.txt", path: "/srv/readme.txt", is_dir: false },
    ],
  });
});

function renderScan() {
  return render(
    <MemoryRouter>
      <Scan />
    </MemoryRouter>,
  );
}

describe("Scan page", () => {
  it("disables launch until a path is chosen, then posts the scan", async () => {
    createScan.mockResolvedValue({ job_id: 5, scan_id: 9 });
    renderScan();

    await waitFor(() => expect(screen.getByText(/readme.txt/)).toBeInTheDocument());

    const launch = screen.getByRole("button", { name: /Scan .* path/ });
    expect(launch).toBeDisabled();

    await userEvent.click(screen.getByText("add folder")); // adds /srv/data
    expect(launch).toBeEnabled();

    await userEvent.click(launch);
    await waitFor(() => expect(createScan).toHaveBeenCalledWith(["/srv/data"], expect.objectContaining({ recursive: true })));
  });

  it("adds a manually typed path", async () => {
    renderScan();
    await waitFor(() => expect(screen.getByText(/readme.txt/)).toBeInTheDocument());

    await userEvent.type(screen.getByPlaceholderText("/path/to/scan"), "/var/www");
    await userEvent.click(screen.getByRole("button", { name: "Add path" }));

    expect(screen.getByText("/var/www")).toBeInTheDocument();
  });
});

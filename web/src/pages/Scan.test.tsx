import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

const createScan = vi.fn();
const browse = vi.fn();
const getJob = vi.fn();
vi.mock("../api/client", () => ({
  createScan: (...a: unknown[]) => createScan(...a),
  browse: (...a: unknown[]) => browse(...a),
  getJob: (...a: unknown[]) => getJob(...a),
  getScan: vi.fn().mockResolvedValue({
    id: 9,
    status: "done",
    paths: ["/srv/data"],
    scanned: 12,
    infected: 0,
  }),
  getScanFindings: vi.fn().mockResolvedValue({ findings: [] }),
  getImageScan: vi.fn().mockResolvedValue({ enabled: false, extensions: [".iso", ".udf", ".img"] }),
  quarantineFile: vi.fn(),
}));
vi.mock("../lib/useEvents", () => ({ useEvents: () => {} }));

import Scan from "./Scan";

beforeEach(() => {
  createScan.mockReset();
  browse.mockReset();
  getJob.mockReset();
  getJob.mockResolvedValue({ id: 1, status: "running", log: "" });
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

  it("shows a completion summary when the job is already finished on first fetch", async () => {
    createScan.mockResolvedValue({ job_id: 5, scan_id: 9 });
    getJob.mockResolvedValue({ id: 5, status: "done", log: "/srv/data: OK\n" });
    renderScan();

    await waitFor(() => expect(screen.getByText(/readme.txt/)).toBeInTheDocument());
    await userEvent.click(screen.getByText("add folder"));
    await userEvent.click(screen.getByRole("button", { name: /Scan .* path/ }));

    await waitFor(() => expect(screen.getByText("Completed")).toBeInTheDocument());
    await waitFor(() =>
      expect(screen.getByText("Scanned 12 files in under a second — no threats found.")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Scanning…")).not.toBeInTheDocument();
  });

  it("adds a manually typed path", async () => {
    renderScan();
    await waitFor(() => expect(screen.getByText(/readme.txt/)).toBeInTheDocument());

    await userEvent.type(screen.getByPlaceholderText("/path/to/scan"), "/var/www");
    await userEvent.click(screen.getByRole("button", { name: "Add path" }));

    expect(screen.getByText("/var/www")).toBeInTheDocument();
  });

  it("flags a disk-image target when mounting is disabled", async () => {
    renderScan();
    await waitFor(() => expect(screen.getByText(/readme.txt/)).toBeInTheDocument());

    await userEvent.type(screen.getByPlaceholderText("/path/to/scan"), "/games/x.iso");
    await userEvent.click(screen.getByRole("button", { name: "Add path" }));

    expect(screen.getByText(/a raw scan skips its contents/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /enable disk-image scanning/ })).toBeInTheDocument();
  });
});

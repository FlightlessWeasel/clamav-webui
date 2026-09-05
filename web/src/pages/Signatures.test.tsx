import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { Signatures as SignaturesData } from "../api/client";

const getSignatures = vi.fn();
vi.mock("../api/client", () => ({
  getSignatures: () => getSignatures(),
  signaturesUpdate: vi.fn(),
  serviceAction: vi.fn(),
}));
vi.mock("../lib/useEvents", () => ({ useEvents: () => {} }));

import Signatures from "./Signatures";

const base: SignaturesData = {
  db_dir: "/var/lib/clamav",
  databases: [
    { name: "main", file: "main.cvd", present: true, version: 62, sigs: 6500000, build_time: "x", mtime_unix: 1726800000 },
    { name: "daily", file: "daily.cld", present: true, version: 27000, sigs: 2000000, build_time: "x", mtime_unix: 1726900000 },
    { name: "bytecode", file: "", present: false, version: 0, sigs: 0, build_time: "", mtime_unix: 0 },
  ],
  total_sigs: 8500000,
  newest_unix: 1726900000,
  age_seconds: 3600,
  freshclam_service: {
    unit: "clamav-freshclam", load: "loaded", active: "active", sub: "running",
    enabled: "enabled", installed: true, since_unix: 0,
  },
  checks: 24,
  has_freshclam: true,
  has_sigtool: true,
};

beforeEach(() => getSignatures.mockReset());

function renderPage() {
  return render(
    <MemoryRouter>
      <Signatures />
    </MemoryRouter>,
  );
}

describe("Signatures", () => {
  it("renders a fresh banner and the DB table", async () => {
    getSignatures.mockResolvedValue(base);
    renderPage();
    await waitFor(() => expect(screen.getByText(/Signatures updated 1h ago/)).toBeInTheDocument());
    expect(screen.getByText("main")).toBeInTheDocument();
    expect(screen.getByText("daily")).toBeInTheDocument();
    expect(screen.getByText("absent")).toBeInTheDocument(); // bytecode
    expect(screen.getByText(/8,500,000 signatures total/)).toBeInTheDocument();
  });

  it("warns when signatures are stale", async () => {
    getSignatures.mockResolvedValue({ ...base, age_seconds: 10 * 86400 });
    renderPage();
    await waitFor(() => expect(screen.getByText(/10d old — update now/)).toBeInTheDocument());
  });

  it("warns when no databases exist", async () => {
    getSignatures.mockResolvedValue({ ...base, age_seconds: -1 });
    renderPage();
    await waitFor(() => expect(screen.getByText(/No signature databases found/)).toBeInTheDocument());
  });
});

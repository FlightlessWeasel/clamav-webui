import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { Dashboard as DashboardData } from "../api/client";

const getDashboard = vi.fn();
vi.mock("../api/client", () => ({ getDashboard: () => getDashboard() }));
vi.mock("../lib/useEvents", () => ({ useEvents: () => {} }));

import Dashboard from "./Dashboard";

const base: DashboardData = {
  install: {
    installed: true,
    engine_version: "1.0.3",
    db_version: "27000",
    db_date: "Fri Sep 20 2024",
    has_daemon: true,
    has_freshclam: true,
    has_clamonacc: false,
    apt_installed: "1.0.3+dfsg-1",
    apt_candidate: "1.0.3+dfsg-1",
    upgrade_available: false,
  },
  services: [
    { unit: "clamav-daemon", load: "loaded", active: "active", sub: "running", enabled: "enabled", installed: true, since_unix: 0 },
    { unit: "clamav-freshclam", load: "loaded", active: "inactive", sub: "dead", enabled: "disabled", installed: true, since_unix: 0 },
    { unit: "clamav-clamonacc", load: "not-found", active: "inactive", sub: "dead", enabled: "not-found", installed: false, since_unix: 0 },
  ],
  signatures: {
    db_dir: "/var/lib/clamav",
    databases: [],
    total_sigs: 8500000,
    newest_unix: 0,
    age_seconds: 7200,
    freshclam_service: {
      unit: "clamav-freshclam",
      load: "loaded",
      active: "inactive",
      sub: "dead",
      enabled: "disabled",
      installed: true,
      since_unix: 0,
    },
    checks: 24,
    has_freshclam: true,
    has_sigtool: true,
  },
  quarantine_held: 2,
  last_scan: null,
};

beforeEach(() => getDashboard.mockReset());

function renderDash() {
  return render(
    <MemoryRouter>
      <Dashboard />
    </MemoryRouter>,
  );
}

describe("Dashboard", () => {
  it("shows engine version and service cards", async () => {
    getDashboard.mockResolvedValue(base);
    renderDash();
    await waitFor(() => expect(screen.getByText(/1\.0\.3/)).toBeInTheDocument());
    expect(screen.getByText("Scanner daemon (clamd)")).toBeInTheDocument();
    expect(screen.getByText("On-access scanner (clamonacc)")).toBeInTheDocument();
    expect(screen.getByText("not installed")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("prompts to install when ClamAV is absent", async () => {
    getDashboard.mockResolvedValue({
      ...base,
      install: { ...base.install, installed: false },
    });
    renderDash();
    await waitFor(() => expect(screen.getByText(/ClamAV is not installed/)).toBeInTheDocument());
  });

  it("surfaces an available upgrade", async () => {
    getDashboard.mockResolvedValue({
      ...base,
      install: { ...base.install, upgrade_available: true, apt_candidate: "1.0.5+dfsg-1" },
    });
    renderDash();
    await waitFor(() => expect(screen.getByText(/Upgrade available/)).toBeInTheDocument());
  });
});

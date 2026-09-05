import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Schedule } from "../api/client";

const getSchedules = vi.fn();
const createSchedule = vi.fn();
const runSchedule = vi.fn();
const deleteSchedule = vi.fn();
const updateSchedule = vi.fn();
vi.mock("../api/client", () => ({
  getSchedules: () => getSchedules(),
  createSchedule: (b: unknown) => createSchedule(b),
  runSchedule: (id: number) => runSchedule(id),
  deleteSchedule: (id: number) => deleteSchedule(id),
  updateSchedule: (id: number, b: unknown) => updateSchedule(id, b),
}));
vi.mock("../lib/useEvents", () => ({ useEvents: () => {} }));

import Schedules from "./Schedules";

const one: Schedule = {
  id: 1,
  name: "Nightly srv",
  cron_expr: "0 3 * * *",
  paths: ["/srv"],
  options: { recursive: true },
  enabled: true,
  created_at: "2024-09-20 00:00:00",
  updated_at: "2024-09-20 00:00:00",
};

beforeEach(() => {
  getSchedules.mockReset();
  createSchedule.mockReset();
  runSchedule.mockReset();
});

describe("Schedules page", () => {
  it("renders a schedule with a human cron gloss and runs it", async () => {
    getSchedules.mockResolvedValue({ schedules: [one] });
    runSchedule.mockResolvedValue({ result: "started" });
    render(<Schedules />);

    await waitFor(() => expect(screen.getByText("Nightly srv")).toBeInTheDocument());
    expect(screen.getByText(/every day, at 03:00/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Run now" }));
    expect(runSchedule).toHaveBeenCalledWith(1);
  });

  it("creates a new schedule from the form", async () => {
    getSchedules.mockResolvedValue({ schedules: [] });
    createSchedule.mockResolvedValue(one);
    render(<Schedules />);

    await waitFor(() => expect(screen.getByText("No schedules.")).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: "New schedule" }));

    await userEvent.type(screen.getByLabelText("Name"), "Weekly");
    await userEvent.type(screen.getByLabelText("Paths (one per line)"), "/data");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(createSchedule).toHaveBeenCalledWith(
        expect.objectContaining({ name: "Weekly", paths: ["/data"], enabled: true }),
      ),
    );
  });
});

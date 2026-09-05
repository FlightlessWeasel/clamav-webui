import { describe, expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useAsync } from "./useAsync";

function Probe({ fn }: { fn: () => Promise<string> }) {
  const { data, error, loading, reload } = useAsync(fn, []);
  return (
    <div>
      <span data-testid="state">{loading ? "loading" : error ? `err:${error.message}` : `data:${data}`}</span>
      <button onClick={reload}>reload</button>
    </div>
  );
}

describe("useAsync", () => {
  it("resolves and exposes data", async () => {
    render(<Probe fn={() => Promise.resolve("ok")} />);
    await waitFor(() => expect(screen.getByTestId("state")).toHaveTextContent("data:ok"));
  });

  it("captures errors", async () => {
    render(<Probe fn={() => Promise.reject(new Error("boom"))} />);
    await waitFor(() => expect(screen.getByTestId("state")).toHaveTextContent("err:boom"));
  });

  it("re-runs on reload", async () => {
    let n = 0;
    render(<Probe fn={() => Promise.resolve(`call${++n}`)} />);
    await waitFor(() => expect(screen.getByTestId("state")).toHaveTextContent("data:call1"));
    await userEvent.click(screen.getByText("reload"));
    await waitFor(() => expect(screen.getByTestId("state")).toHaveTextContent("data:call2"));
  });
});

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Link, Route, Routes } from "react-router-dom";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import BudgetsPage from "@/pages/BudgetsPage";
import DashboardPage from "@/pages/DashboardPage";
import { MonthProvider, monthLabel, shiftMonth } from "./month";

const json = (status: number, body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status }));

type Handler = (url: string, init?: RequestInit) => Promise<Response>;
function mockApi(overrides: Handler = () => json(404, {})) {
  const fn = vi.fn<Handler>((url, init) => {
    if (url === "/api/categories")
      return json(200, [
        { id: 1, name: "Food", color: "#ff0000", is_default: true },
      ]);
    if (url.startsWith("/api/expenses")) return json(200, []);
    if (url.startsWith("/api/budgets") && !init?.method) return json(200, []);
    return overrides(url, init);
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}
const urls = (fn: ReturnType<typeof mockApi>) => fn.mock.calls.map(([u]) => u);

beforeAll(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"], now: new Date(2026, 9, 8, 12) });
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("month helpers", () => {
  it("shifts across year boundaries and clamps", () => {
    expect(shiftMonth("2026-01", -1)).toBe("2025-12");
    expect(shiftMonth("2026-12", 1)).toBe("2027-01");
    expect(shiftMonth("0001-01", -1)).toBe("0001-01");
    expect(monthLabel("2026-10")).toBe("October 2026");
  });
});

describe("month picker on pages", () => {
  it("dashboard refetches for the previous and next month and shows the empty state", async () => {
    const fn = mockApi();
    render(<DashboardPage />);
    expect(
      await screen.findByText("No expenses this month"),
    ).toBeInTheDocument();
    expect(screen.getByText("October 2026")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Previous month" }));
    expect(await screen.findByText("September 2026")).toBeInTheDocument();
    await screen.findByText("No expenses this month");
    expect(urls(fn)).toContain("/api/expenses?month=2026-09");
    expect(urls(fn)).toContain("/api/budgets?month=2026-09");
    fireEvent.click(screen.getByRole("button", { name: "Next month" }));
    fireEvent.click(screen.getByRole("button", { name: "Next month" }));
    expect(await screen.findByText("November 2026")).toBeInTheDocument();
    await waitFor(() =>
      expect(urls(fn)).toContain("/api/expenses?month=2026-11"),
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("dashboard ignores a stale response from an earlier month", async () => {
    let releaseOct!: (r: Response) => void;
    const fn = mockApi((url) => json(404, { url }));
    fn.mockImplementation((url) => {
      if (url === "/api/categories")
        return json(200, [{ id: 1, name: "Food", color: "#ff0000" }]);
      if (url === "/api/expenses?month=2026-10")
        return new Promise<Response>((r) => (releaseOct = r));
      if (url.startsWith("/api/expenses"))
        return json(200, [
          { id: 1, category_id: 1, amount: 777, date: "2026-09-02" },
        ]);
      return json(200, []);
    });
    render(<DashboardPage />);
    fireEvent.click(screen.getByRole("button", { name: "Previous month" }));
    await waitFor(() =>
      expect(screen.getByTestId("total-spent").textContent).toMatch(/777/),
    );
    releaseOct(
      new Response(
        JSON.stringify([
          { id: 2, category_id: 1, amount: 999, date: "2026-10-02" },
        ]),
      ),
    );
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.getByTestId("total-spent").textContent).toMatch(/777/);
  });

  it("budgets refetches for another month and saves to that month", async () => {
    const fn = mockApi(() => json(200, { category_id: 1, amount: 500 }));
    render(<BudgetsPage />);
    await screen.findByLabelText("Food budget in tenge");
    fireEvent.click(screen.getByRole("button", { name: "Next month" }));
    expect(await screen.findByText("November 2026")).toBeInTheDocument();
    fireEvent.change(await screen.findByLabelText("Food budget in tenge"), {
      target: { value: "500" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Food budget" }));
    await screen.findByText("Saved");
    expect(urls(fn)).toContain("/api/budgets?month=2026-11");
    const put = fn.mock.calls.find(([, i]) => i?.method === "PUT")!;
    expect(put[0]).toBe("/api/budgets?month=2026-11");
  });

  it("keeps the selected month when navigating between pages", async () => {
    mockApi();
    render(
      <MemoryRouter initialEntries={["/dashboard"]}>
        <MonthProvider>
          <Link to="/budgets">to budgets</Link>
          <Link to="/dashboard">to dashboard</Link>
          <Routes>
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/budgets" element={<BudgetsPage />} />
          </Routes>
        </MonthProvider>
      </MemoryRouter>,
    );
    await screen.findByText("No expenses this month");
    fireEvent.click(screen.getByRole("button", { name: "Previous month" }));
    await screen.findByText("September 2026");
    fireEvent.click(screen.getByText("to budgets"));
    expect(
      await screen.findByLabelText("Food budget in tenge"),
    ).toBeInTheDocument();
    expect(screen.getByText("September 2026")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Previous month" }));
    fireEvent.click(screen.getByText("to dashboard"));
    expect(await screen.findByText("August 2026")).toBeInTheDocument();
  });
});

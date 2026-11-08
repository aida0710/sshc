import { act, render, screen } from "@testing-library/react";
import { Suspense } from "react";
import { describe, expect, it, vi } from "vitest";
import { lazyWithPreload } from "./lazyWithPreload";

function Page() {
  return <p>page</p>;
}

describe("lazyWithPreload", () => {
  it("draws a preloaded page at once, without its fallback", async () => {
    const PreloadedPage = lazyWithPreload(() => Promise.resolve({ default: Page }));
    await PreloadedPage.preload();
    render(
      <Suspense fallback={<p>skeleton</p>}>
        <PreloadedPage />
      </Suspense>,
    );
    expect(screen.getByText("page")).toBeInTheDocument();
    expect(screen.queryByText("skeleton")).not.toBeInTheDocument();
  });

  it("draws the fallback until a page that was not preloaded arrives", async () => {
    const LatePage = lazyWithPreload(() => Promise.resolve({ default: Page }));
    render(
      <Suspense fallback={<p>skeleton</p>}>
        <LatePage />
      </Suspense>,
    );
    expect(screen.getByText("skeleton")).toBeInTheDocument();
    expect(await screen.findByText("page")).toBeInTheDocument();
  });

  it("fetches the code once however often it is preloaded", async () => {
    const load = vi.fn(() => Promise.resolve({ default: Page }));
    const OncePage = lazyWithPreload(load);
    await Promise.all([OncePage.preload(), OncePage.preload()]);
    await act(async () => {
      render(<Suspense fallback={null}><OncePage /></Suspense>);
    });
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("fetches again when the page is opened after a preload that failed", async () => {
    const load = vi.fn<() => Promise<{ default: typeof Page }>>()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue({ default: Page });
    const RetriedPage = lazyWithPreload(load);
    await expect(RetriedPage.preload()).rejects.toThrow("offline");
    render(<Suspense fallback={<p>skeleton</p>}><RetriedPage /></Suspense>);
    expect(await screen.findByText("page")).toBeInTheDocument();
    expect(load).toHaveBeenCalledTimes(2);
  });
});

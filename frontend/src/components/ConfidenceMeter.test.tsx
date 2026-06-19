import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { ConfidenceMeter } from "./ConfidenceMeter";

describe("ConfidenceMeter", () => {
  it("renders fill width and label from the confidence value", () => {
    const { container, getByText } = render(<ConfidenceMeter value={0.98} />);
    const fill = container.querySelector("[data-testid='meter-fill']") as HTMLElement;
    expect(fill.style.width).toBe("98%");
    expect(getByText("98%")).toBeTruthy();
  });

  it("clamps out-of-range values", () => {
    const { container } = render(<ConfidenceMeter value={1.5} />);
    const fill = container.querySelector("[data-testid='meter-fill']") as HTMLElement;
    expect(fill.style.width).toBe("100%");
  });
});

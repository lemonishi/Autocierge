import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useTheme, THEME_KEY } from "./theme";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("useTheme", () => {
  it("uses a stored preference when present", () => {
    localStorage.setItem(THEME_KEY, "dark");
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("toggle flips the theme, sets data-theme, and persists", () => {
    localStorage.setItem(THEME_KEY, "light");
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("light");

    act(() => result.current.toggle());

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(localStorage.getItem(THEME_KEY)).toBe("dark");
  });
});

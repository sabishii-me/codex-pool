import { afterEach, describe, expect, it, vi } from "vitest";
import { formatRelativeTimestamp } from "./features/admin";

describe("connection reset countdown", () => {
  afterEach(() => vi.useRealTimers());

  it("formats long durations as days, hours, minutes, and seconds", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-25T00:00:00Z"));

    expect(formatRelativeTimestamp("2026-07-30T10:48:03Z")).toBe("in 5d 10h 48m 3s");
  });

  it("keeps all relevant lower units and handles expired resets", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-25T00:00:00Z"));

    expect(formatRelativeTimestamp("2026-07-25T01:02:03Z")).toBe("in 1h 2m 3s");
    expect(formatRelativeTimestamp("2026-07-25T00:00:09Z")).toBe("in 9s");
    expect(formatRelativeTimestamp("2026-07-24T23:59:59Z")).toBe("now");
  });
});

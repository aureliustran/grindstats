import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { UnverifiedBanner } from "./UnverifiedBanner";

// Mock react-i18next so t(key) returns the key verbatim while keeping other exports
vi.mock("react-i18next", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-i18next")>();
  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string) => key,
      i18n: { resolvedLanguage: "en-US" },
    }),
  };
});

function WriteSectionHarness({ emailVerified }: { emailVerified: boolean }) {
  return (
    <div>
      <UnverifiedBanner emailVerified={emailVerified} />
      <button type="button" disabled={!emailVerified} data-testid="write-control">
        Create Workout
      </button>
    </div>
  );
}

describe("UnverifiedBanner", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  // --------------------------------------------------------------------------
  // Story Scenario: AUTH-001 TC-09
  // --------------------------------------------------------------------------
  it("unverified account is read-only on its own data", () => {
    // 1. Unverified session
    const { unmount } = render(<WriteSectionHarness emailVerified={false} />);

    // Renders the banner (asserted by translation key)
    expect(screen.getByText("auth.unverified.banner")).toBeInTheDocument();

    // The write control is disabled
    const writeControl = screen.getByTestId("write-control") as HTMLButtonElement;
    expect(writeControl.disabled).toBe(true);

    unmount();

    // 2. Verified session renders neither
    render(<WriteSectionHarness emailVerified={true} />);

    // Banner is not rendered
    expect(screen.queryByText("auth.unverified.banner")).toBeNull();

    // Write control is not disabled (active)
    const activeWriteControl = screen.getByTestId("write-control") as HTMLButtonElement;
    expect(activeWriteControl.disabled).toBe(false);
  });
});
